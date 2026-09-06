package tmux

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// BridgeBackend is the metadata/control companion to the TUI Backend. It has
// no capture or send-keys port; every tmux invocation remains in this package.
type BridgeBackend interface {
	BridgeServer(context.Context) (BridgeServer, error)
	BridgeTree(context.Context) ([]Session, error)
	BridgeClients(context.Context) ([]BridgeClient, error)
	BridgeMutate(context.Context, BridgeServer, []string, string) (string, error)
	BridgeAttachCmd(BridgeServer, string) *exec.Cmd
}
type BridgeServer struct {
	Generation  string `json:"generation"`
	SocketName  string `json:"socketName"`
	Running     bool   `json:"running"`
	TmuxVersion string `json:"tmuxVersion"`
	PID         int    `json:"-"`
	Started     string `json:"-"`
	SocketPath  string `json:"-"`
}
type BridgeClient struct {
	TTY                         string
	PID                         int
	SessionID, WindowID, PaneID string
}

var bridgeSocket = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$`)
var nativePID = regexp.MustCompile(`^[0-9]+$`)
var ErrBridgeGeneration = errors.New("bridge-server-generation-changed")
var ErrBridgeMetadata = errors.New("bridge-metadata-unavailable")
var _ BridgeBackend = (*Tmux)(nil)

func NewBridge(socket string) (*Tmux, error) {
	if socket == "" {
		socket = "default"
	}
	if !bridgeSocket.MatchString(socket) {
		return nil, ErrBridgeMetadata
	}
	return NewWithSocket(socket), nil
}
func (t *Tmux) bridgeCommand(ctx context.Context, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, t.bin, append([]string{"-u", "-L", t.socket}, args...)...)
	for _, entry := range os.Environ() {
		if !strings.HasPrefix(entry, "TMUX=") && !strings.HasPrefix(entry, "TMUX_PANE=") && !strings.HasPrefix(entry, "LC_ALL=") {
			cmd.Env = append(cmd.Env, entry)
		}
	}
	cmd.Env = append(cmd.Env, "LC_ALL=C")
	return cmd
}

type boundedBuffer struct {
	bytes.Buffer
	limit int
}

func (b *boundedBuffer) Write(p []byte) (int, error) {
	if b.Len()+len(p) > b.limit {
		return 0, ErrBridgeMetadata
	}
	return b.Buffer.Write(p)
}
func (t *Tmux) bridgeOutput(ctx context.Context, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	cmd := t.bridgeCommand(ctx, args...)
	out := &boundedBuffer{limit: 2 << 20}
	stderr := &boundedBuffer{limit: 4096}
	cmd.Stdout = out
	cmd.Stderr = stderr
	if err := cmd.Run(); err != nil {
		return "", &Error{Args: args, Err: err, Stderr: stderr.String()}
	}
	return out.String(), nil
}
func (t *Tmux) BridgeServer(ctx context.Context) (BridgeServer, error) {
	value := BridgeServer{SocketName: t.socket}
	version, err := t.bridgeOutput(ctx, "-V")
	if err != nil {
		return value, ErrBridgeMetadata
	}
	value.TmuxVersion = strings.TrimSpace(version)
	raw, err := t.bridgeOutput(ctx, "display-message", "-p", "#{pid}\t#{start_time}\t#{socket_path}")
	if err != nil {
		if e, ok := err.(*Error); ok && (strings.Contains(e.Stderr, "no server running") || strings.Contains(e.Stderr, "No such file or directory")) {
			host, _ := os.Hostname()
			sum := sha256.Sum256([]byte("absent\x00" + host + "\x00" + t.socket + "\x00" + e.Stderr))
			value.Generation = "server:" + hex.EncodeToString(sum[:])
			return value, nil
		}
		return value, ErrBridgeMetadata
	}
	fields := strings.Split(strings.TrimSpace(raw), "\t")
	if len(fields) != 3 || !nativePID.MatchString(fields[0]) || !nativePID.MatchString(fields[1]) || !filepath.IsAbs(fields[2]) {
		return value, ErrBridgeMetadata
	}
	value.PID, _ = strconv.Atoi(fields[0])
	if value.PID < 1 {
		return value, ErrBridgeMetadata
	}
	value.Started = fields[1]
	value.SocketPath = fields[2]
	st, err := os.Lstat(fields[2])
	if err != nil || st.Mode()&os.ModeSocket == 0 || st.Mode()&os.ModeSymlink != 0 {
		return value, ErrBridgeMetadata
	}
	sys, ok := st.Sys().(*syscall.Stat_t)
	if !ok || int(sys.Uid) != os.Getuid() {
		return value, ErrBridgeMetadata
	}
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s\x00%d\x00%s\x00%s\x00%d\x00%d", t.socket, value.PID, value.Started, fields[2], sys.Dev, sys.Ino)))
	value.Generation = "server:" + hex.EncodeToString(sum[:])
	value.Running = true
	return value, nil
}
func (t *Tmux) BridgeTree(ctx context.Context) ([]Session, error) {
	s, e := t.bridgeOutput(ctx, "list-sessions", "-F", sessionFormat)
	if e != nil {
		return nil, e
	}
	w, e := t.bridgeOutput(ctx, "list-windows", "-a", "-F", windowFormat)
	if e != nil {
		return nil, e
	}
	p, e := t.bridgeOutput(ctx, "list-panes", "-a", "-F", paneFormat)
	if e != nil {
		return nil, e
	}
	tree, err := buildTree(s, w, p)
	if err != nil {
		return nil, err
	}
	// tmux sanitizes stored session/window names using vis escapes. Decode
	// these names only: pane_current_path is already the literal OS path.
	for si := range tree {
		if tree[si].Name, err = bridgeText(tree[si].Name); err != nil {
			return nil, err
		}
		for wi := range tree[si].Windows {
			window := &tree[si].Windows[wi]
			if window.Name, err = bridgeText(window.Name); err != nil {
				return nil, err
			}

		}
	}
	return tree, nil
}
func bridgeText(value string) (string, error) {
	decoded, err := strconv.Unquote("\"" + strings.ReplaceAll(value, "\"", "\\\"") + "\"")
	if err != nil {
		return "", ErrBridgeMetadata
	}
	return decoded, nil
}
func (t *Tmux) BridgeClients(ctx context.Context) ([]BridgeClient, error) {
	raw, err := t.bridgeOutput(ctx, "list-clients", "-F", "#{client_tty}\t#{client_pid}\t#{session_id}\t#{window_id}\t#{pane_id}")
	if err != nil {
		return nil, err
	}
	result := []BridgeClient{}
	for _, line := range splitLines(raw) {
		f := strings.Split(line, "\t")
		if len(f) != 5 || !filepath.IsAbs(f[0]) || !nativePID.MatchString(f[1]) {
			return nil, ErrBridgeMetadata
		}
		pid, _ := strconv.Atoi(f[1])
		result = append(result, BridgeClient{f[0], pid, f[2], f[3], f[4]})
	}
	return result, nil
}

// Native command-list quoting is used only inside if-shell -F. -F evaluates a
// tmux format; it never invokes a shell. No caller supplies a command string.
func quoteBridgeArgs(args []string) string {
	escaped := make([]string, len(args))
	for i, arg := range args {
		escaped[i] = "'" + strings.ReplaceAll(arg, "'", "'\\''") + "'"
	}
	return strings.Join(escaped, " ")
}
func bridgeGuard(server BridgeServer) string {
	return fmt.Sprintf("#{&&:#{==:#{pid},%d},#{==:#{start_time},%s}}", server.PID, server.Started)
}
func (t *Tmux) BridgeMutate(ctx context.Context, expected BridgeServer, args []string, extraGuard string) (string, error) {
	before, err := t.BridgeServer(ctx)
	if err != nil {
		return "", err
	}
	if before.Generation != expected.Generation {
		return "", ErrBridgeGeneration
	}
	if !before.Running {
		if len(args) == 0 || args[0] != "new-session" {
			return "", ErrBridgeGeneration
		}
		return t.bridgeOutput(ctx, args...)
	}
	condition := bridgeGuard(before)
	if extraGuard != "" {
		condition = "#{&&:" + condition + "," + extraGuard + "}"
	}
	out, err := t.bridgeOutput(ctx, "if-shell", "-F", condition, quoteBridgeArgs(args), "display-message -p TMUXGO_BRIDGE_GUARD_FAILED")
	if strings.Contains(out, "TMUXGO_BRIDGE_GUARD_FAILED") {
		return "", ErrBridgeGeneration
	}
	return out, err
}
func (t *Tmux) BridgeAttachCmd(server BridgeServer, target string) *exec.Cmd {
	cmd := t.bridgeCommand(context.Background(), "if-shell", "-F", bridgeGuard(server), quoteBridgeArgs([]string{"attach-session", "-t", target}), "display-message -p TMUXGO_BRIDGE_GUARD_FAILED")
	return cmd
}
