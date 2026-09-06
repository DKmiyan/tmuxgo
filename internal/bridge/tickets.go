package bridge

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"

	"github.com/DKmiyan/tmuxgo/internal/tmux"
)

const ticketTTL = 30 * time.Second

var ticketPattern = regexp.MustCompile(`^v0\.([a-f0-9]{32})\.([a-f0-9]{32})\.([a-f0-9]{64})$`)
var ttyPattern = regexp.MustCompile(`^/dev/(pts/[0-9]+|tty[^/]{1,64})$`)

func randomHex(n int) string {
	value := make([]byte, n)
	if _, err := rand.Read(value); err != nil {
		panic(err)
	}
	return hex.EncodeToString(value)
}

type ticketRecord struct {
	Version       int               `json:"version"`
	BridgeID      string            `json:"bridgeId"`
	AttachmentID  string            `json:"attachmentId"`
	SecretHash    string            `json:"secretHash"`
	ExpiresAt     time.Time         `json:"expiresAt"`
	Server        tmux.BridgeServer `json:"server"`
	SessionID     string            `json:"sessionId"`
	WindowID      string            `json:"windowId,omitempty"`
	BridgePID     int               `json:"bridgePid"`
	BridgeStarted string            `json:"bridgeStarted"`
	ClientPID     int               `json:"clientPid,omitempty"`
	TTY           string            `json:"tty,omitempty"`
}
type ticketStore struct {
	dir, id, started string
	records          map[string]ticketRecord
	attached         map[string]bool
}

func privateDirectory(dir string, create bool) error {
	if create {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return err
		}
	}
	st, err := os.Lstat(dir)
	if err != nil {
		return err
	}
	sys, ok := st.Sys().(*syscall.Stat_t)
	if !st.IsDir() || st.Mode()&os.ModeSymlink != 0 || st.Mode().Perm()&0077 != 0 || !ok || int(sys.Uid) != os.Getuid() {
		return errors.New("TICKET_INVALID")
	}
	return nil
}
func processStarted(pid int) string {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "ps", "-p", fmt.Sprint(pid), "-o", "lstart=")
	cmd.Env = []string{"PATH=/usr/bin:/bin", "LC_ALL=C"}
	raw, err := cmd.Output()
	if err != nil || len(raw) > 128 {
		return ""
	}
	return strings.TrimSpace(string(raw))
}
func newTicketStore() (*ticketStore, error) {
	base := filepath.Join(os.TempDir(), fmt.Sprintf("tmuxgo-bridge-%d", os.Getuid()))
	if err := privateDirectory(base, true); err != nil {
		return nil, err
	}
	id := randomHex(16)
	dir := filepath.Join(base, id)
	if err := os.Mkdir(dir, 0700); err != nil {
		return nil, err
	}
	started := processStarted(os.Getpid())
	if started == "" {
		_ = os.Remove(dir)
		return nil, errors.New("METADATA_UNAVAILABLE")
	}
	return &ticketStore{dir: dir, id: id, started: started, records: map[string]ticketRecord{}, attached: map[string]bool{}}, nil
}
func (s *ticketStore) close() { _ = os.RemoveAll(s.dir); clear(s.records); clear(s.attached) }
func (s *ticketStore) issue(server tmux.BridgeServer, session, window string) (Result, error) {
	s.expire()
	if len(s.records) >= 64 {
		return Result{}, errors.New("LIMIT_EXCEEDED")
	}
	id, secret := randomHex(16), randomHex(32)
	sum := sha256.Sum256([]byte(secret))
	expires := time.Now().Add(ticketTTL)
	r := ticketRecord{Version: Version, BridgeID: s.id, AttachmentID: "attach:" + id, SecretHash: hex.EncodeToString(sum[:]), ExpiresAt: expires, Server: server, SessionID: session, WindowID: window, BridgePID: os.Getpid(), BridgeStarted: s.started}
	bytes, _ := json.Marshal(r)
	if err := writePrivate(filepath.Join(s.dir, id+".pending"), bytes); err != nil {
		return Result{}, err
	}
	s.records[r.AttachmentID] = r
	return Result{Generation: server.Generation, SessionID: session, WindowID: window, AttachmentID: r.AttachmentID, Ticket: "v0." + s.id + "." + id + "." + secret, ExpiresAt: expires.UTC().Format(time.RFC3339Nano)}, nil
}
func writePrivate(file string, data []byte) error {
	f, err := os.OpenFile(file, os.O_WRONLY|os.O_CREATE|os.O_EXCL|syscall.O_NOFOLLOW, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(data)
	return err
}
func readPrivate(file string) (ticketRecord, error) {
	var r ticketRecord
	f, err := os.OpenFile(file, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return r, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return r, err
	}
	sys, ok := st.Sys().(*syscall.Stat_t)
	if !st.Mode().IsRegular() || st.Mode().Perm()&0077 != 0 || !ok || int(sys.Uid) != os.Getuid() || st.Size() > 8192 {
		return r, errors.New("TICKET_INVALID")
	}
	raw, err := io.ReadAll(io.LimitReader(f, 8193))
	if err != nil {
		return r, err
	}
	if strict(raw, &r) != nil {
		return r, errors.New("TICKET_INVALID")
	}
	return r, nil
}
func (s *ticketStore) expire() {
	for id, r := range s.records {
		file := filepath.Join(s.dir, strings.TrimPrefix(id, "attach:"))
		if time.Now().After(r.ExpiresAt) {
			_ = os.Remove(file + ".pending")
			if _, err := os.Lstat(file + ".claimed"); os.IsNotExist(err) {
				delete(s.records, id)
				delete(s.attached, id)
			}
		}
	}
}
func (s *ticketStore) states(server tmux.BridgeServer, clients []tmux.BridgeClient) []Attachment {
	s.expire()
	result := []Attachment{}
	for id, r := range s.records {
		state := Attachment{AttachmentID: id, State: "pending"}
		file := filepath.Join(s.dir, strings.TrimPrefix(id, "attach:")+".claimed")
		claimed, err := readPrivate(file)
		if err == nil && claimed.AttachmentID == id && claimed.SecretHash == r.SecretHash && claimed.Server.Generation == server.Generation {
			count := 0
			var found tmux.BridgeClient
			for _, c := range clients {
				if c.TTY == claimed.TTY && c.PID == claimed.ClientPID {
					count++
					found = c
				}
			}
			if count == 1 {
				state.State = "attached"
				sid, wid, pid := found.SessionID, found.WindowID, found.PaneID
				state.SessionID = &sid
				state.WindowID = &wid
				state.PaneID = &pid
				s.attached[id] = true
			}
		}
		if state.State != "attached" {
			if s.attached[id] || r.Server.Generation != server.Generation {
				state.State = "detached"
			} else if time.Now().After(r.ExpiresAt) {
				state.State = "expired"
			}
		}
		result = append(result, state)
	}
	return result
}
func (s *ticketStore) client(id string, server tmux.BridgeServer, clients []tmux.BridgeClient) (tmux.BridgeClient, error) {
	r, ok := s.records[id]
	if !ok || r.Server.Generation != server.Generation {
		return tmux.BridgeClient{}, errors.New("ATTACHMENT_NOT_CONNECTED")
	}
	claimed, err := readPrivate(filepath.Join(s.dir, strings.TrimPrefix(id, "attach:")+".claimed"))
	if err != nil || claimed.SecretHash != r.SecretHash {
		return tmux.BridgeClient{}, errors.New("ATTACHMENT_NOT_CONNECTED")
	}
	found := []tmux.BridgeClient{}
	for _, client := range clients {
		if client.TTY == claimed.TTY && client.PID == claimed.ClientPID {
			found = append(found, client)
		}
	}
	if len(found) != 1 {
		return tmux.BridgeClient{}, errors.New("ATTACHMENT_NOT_CONNECTED")
	}
	return found[0], nil
}

// Attach consumes a capability once and runs only a native tmux attach client.
// Losing its bridge or SSH terminal detaches this client, never its tasks.
func Attach(ctx context.Context, ticket string) error {
	parts := ticketPattern.FindStringSubmatch(ticket)
	if parts == nil {
		return errors.New("TICKET_INVALID")
	}
	base := filepath.Join(os.TempDir(), fmt.Sprintf("tmuxgo-bridge-%d", os.Getuid()))
	dir := filepath.Join(base, parts[1])
	if privateDirectory(base, false) != nil || privateDirectory(dir, false) != nil {
		return errors.New("TICKET_INVALID")
	}
	pending, claimed := filepath.Join(dir, parts[2]+".pending"), filepath.Join(dir, parts[2]+".claimed")
	r, err := readPrivate(pending)
	if err != nil {
		return errors.New("TICKET_INVALID")
	}
	sum := sha256.Sum256([]byte(parts[3]))
	if r.Version != Version || r.BridgeID != parts[1] || r.AttachmentID != "attach:"+parts[2] || subtle.ConstantTimeCompare([]byte(r.SecretHash), []byte(hex.EncodeToString(sum[:]))) != 1 {
		return errors.New("TICKET_INVALID")
	}
	if time.Now().After(r.ExpiresAt) {
		return errors.New("TICKET_EXPIRED")
	}
	if processStarted(r.BridgePID) != r.BridgeStarted {
		return errors.New("TICKET_INVALID")
	}
	// Atomic rename is the one-use claim. No arbitrary path comes from argv.
	if _, err := os.Lstat(claimed); err == nil {
		return errors.New("TICKET_INVALID")
	}
	if err := os.Rename(pending, claimed); err != nil {
		return errors.New("TICKET_INVALID")
	}
	defer os.Remove(claimed)
	ttyCmd := exec.CommandContext(ctx, "tty")
	ttyCmd.Stdin = os.Stdin
	raw, err := ttyCmd.Output()
	tty := strings.TrimSpace(string(raw))
	if err != nil || !ttyPattern.MatchString(tty) {
		return errors.New("TTY_REQUIRED")
	}
	backend, err := tmux.NewBridge(r.Server.SocketName)
	if err != nil {
		return errors.New("TICKET_INVALID")
	}
	server, err := backend.BridgeServer(ctx)
	if err != nil || server.Generation != r.Server.Generation {
		return errors.New("SERVER_CHANGED")
	}
	target := r.SessionID
	if r.WindowID != "" {
		target += ":" + r.WindowID
	}
	cmd := backend.BridgeAttachCmd(server, target)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return errors.New("ATTACH_FAILED")
	}
	r.ClientPID = cmd.Process.Pid
	r.TTY = tty
	data, _ := json.Marshal(r)
	updated := claimed + ".next"
	if err := writePrivate(updated, data); err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return errors.New("ATTACH_FAILED")
	}
	if err := os.Rename(updated, claimed); err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return errors.New("ATTACH_FAILED")
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case err := <-done:
			if err != nil {
				return errors.New("ATTACH_FAILED")
			}
			return nil
		case <-ctx.Done():
			_ = cmd.Process.Signal(syscall.SIGTERM)
			<-done
			return nil
		case <-ticker.C:
			if privateDirectory(dir, false) != nil || processStarted(r.BridgePID) != r.BridgeStarted {
				_ = cmd.Process.Signal(syscall.SIGTERM)
				<-done
				return nil
			}
		}
	}
}
