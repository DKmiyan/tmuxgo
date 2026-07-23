package tmux

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// SocketEnv names the environment variable that overrides the tmux socket
// for backends created with New. It is the injection point for tests and
// for running against a non-default server.
const SocketEnv = "TMUXGO_SOCKET"

// Tmux is a Backend that drives the tmux CLI with structured argument
// arrays. It never builds shell command strings.
type Tmux struct {
	bin    string
	socket string
}

var _ Backend = (*Tmux)(nil)

// New returns a Backend for the default tmux server, or the server named
// by TMUXGO_SOCKET when set.
func New() *Tmux {
	return NewWithSocket(os.Getenv(SocketEnv))
}

// NewWithSocket returns a Backend bound to an explicit tmux socket name
// (empty means the tmux default). Tests always pass a unique socket so the
// user's real server is never touched.
func NewWithSocket(socket string) *Tmux {
	return &Tmux{bin: "tmux", socket: socket}
}

// command builds an exec.Cmd with the socket flag prepended.
func (t *Tmux) command(args ...string) *exec.Cmd {
	full := make([]string, 0, len(args)+2)
	if t.socket != "" {
		full = append(full, "-L", t.socket)
	}
	return exec.Command(t.bin, append(full, args...)...)
}

// output runs a tmux command and returns stdout. The error wraps stderr.
func (t *Tmux) output(args ...string) (string, error) {
	cmd := t.command(args...)
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", &Error{Args: args, Err: err, Stderr: strings.TrimSpace(stderr.String())}
	}
	return stdout.String(), nil
}

// run is output for commands whose stdout is irrelevant.
func (t *Tmux) run(args ...string) error {
	_, err := t.output(args...)
	return err
}

// Error describes a failed tmux invocation.
type Error struct {
	Args   []string
	Err    error
	Stderr string
}

func (e *Error) Error() string {
	if e.Stderr != "" {
		return fmt.Sprintf("tmux %s: %s", strings.Join(e.Args, " "), e.Stderr)
	}
	return fmt.Sprintf("tmux %s: %v", strings.Join(e.Args, " "), e.Err)
}

func (e *Error) Unwrap() error { return e.Err }

// IsNoServer reports whether err means the tmux server is not running.
func IsNoServer(err error) bool {
	if e, ok := err.(*Error); ok {
		return strings.Contains(e.Stderr, "no server running") ||
			strings.Contains(e.Stderr, "error connecting to") ||
			strings.Contains(e.Stderr, "No such file or directory")
	}
	return false
}

// --- queries ---

// Tree implements Backend.Tree.
func (t *Tmux) Tree() ([]Session, error) {
	sessionsOut, err := t.output("list-sessions", "-F", sessionFormat)
	if err != nil {
		if IsNoServer(err) {
			return nil, nil
		}
		return nil, err
	}
	windowsOut, err := t.output("list-windows", "-a", "-F", windowFormat)
	if err != nil {
		return nil, err
	}
	panesOut, err := t.output("list-panes", "-a", "-F", paneFormat)
	if err != nil {
		return nil, err
	}
	return buildTree(sessionsOut, windowsOut, panesOut)
}

// CapturePane implements Backend.CapturePane.
func (t *Tmux) CapturePane(paneID string, lines int) (string, error) {
	out, err := t.output("capture-pane", "-p", "-t", paneID)
	if err != nil {
		return "", err
	}
	return tailLines(out, lines), nil
}

// --- mutations ---

// NewSession creates a detached session; name may be empty.
func (t *Tmux) NewSession(name string) error {
	args := []string{"new-session", "-d"}
	if name != "" {
		args = append(args, "-s", name)
	}
	return t.run(args...)
}

// NewWindow creates a window in the given session; name may be empty.
func (t *Tmux) NewWindow(sessionID, name string) error {
	args := []string{"new-window", "-t", sessionID}
	if name != "" {
		args = append(args, "-n", name)
	}
	return t.run(args...)
}

// SplitPane splits the window containing the given pane.
func (t *Tmux) SplitPane(paneID string) error {
	return t.run("split-window", "-t", paneID)
}

// RenameSession renames a session.
func (t *Tmux) RenameSession(id, name string) error {
	return t.run("rename-session", "-t", id, name)
}

// RenameWindow renames a window.
func (t *Tmux) RenameWindow(id, name string) error {
	return t.run("rename-window", "-t", id, name)
}

// MoveWindow moves a window to the end of another session.
func (t *Tmux) MoveWindow(windowID, sessionID string) error {
	return t.run("move-window", "-s", windowID, "-t", sessionID+":")
}

// MovePane moves a pane into another window.
func (t *Tmux) MovePane(paneID, windowID string) error {
	return t.run("move-pane", "-s", paneID, "-t", windowID)
}

// KillSession kills a session with everything in it.
func (t *Tmux) KillSession(id string) error {
	return t.run("kill-session", "-t", id)
}

// KillWindow kills a window. Killing a session's last window kills the session.
func (t *Tmux) KillWindow(id string) error {
	return t.run("kill-window", "-t", id)
}

// KillPane kills a pane. Killing a window's last pane kills the window.
func (t *Tmux) KillPane(id string) error {
	return t.run("kill-pane", "-t", id)
}

// tailLines trims trailing blank lines and keeps at most the last n lines
// (n <= 0 keeps everything).
func tailLines(s string, n int) string {
	s = strings.TrimRight(s, "\n")
	for strings.HasSuffix(s, "\n\n") {
		s = strings.TrimSuffix(s, "\n")
	}
	// drop trailing whitespace-only lines
	lines := strings.Split(s, "\n")
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	if n > 0 && len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}
