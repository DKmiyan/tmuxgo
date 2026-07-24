package tmux

// Integration tests for the real tmux backend. Every test runs against a
// unique isolated socket (tmux -L <name>); the user's real tmux server is
// never contacted. Tests skip when no tmux binary is installed.

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func newTestBackend(t *testing.T) *Tmux {
	t.Helper()
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux binary not found; skipping integration test")
	}
	socket := fmt.Sprintf("tmuxgo-test-%d-%s", os.Getpid(),
		strings.NewReplacer("/", "-", " ", "-").Replace(t.Name()))
	b := NewWithSocket(socket)
	t.Cleanup(func() { _ = b.run("kill-server") })
	return b
}

// mustSession starts the server with one named session and returns the tree.
func mustTree(t *testing.T, b *Tmux) []Session {
	t.Helper()
	tree, err := b.Tree()
	if err != nil {
		t.Fatalf("Tree: %v", err)
	}
	return tree
}

func findSession(t *testing.T, tree []Session, name string) *Session {
	t.Helper()
	for i := range tree {
		if tree[i].Name == name {
			return &tree[i]
		}
	}
	t.Fatalf("session %q not found in tree", name)
	return nil
}

func TestTreeNoServer(t *testing.T) {
	b := newTestBackend(t)
	tree, err := b.Tree()
	if err != nil {
		t.Fatalf("Tree with no server: %v", err)
	}
	if len(tree) != 0 {
		t.Fatalf("sessions = %d, want 0", len(tree))
	}
}

func TestNewSessionAndTree(t *testing.T) {
	b := newTestBackend(t)
	if err := b.NewSession("alpha"); err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	tree := mustTree(t, b)
	if len(tree) != 1 {
		t.Fatalf("sessions = %d, want 1", len(tree))
	}
	s := tree[0]
	if s.Name != "alpha" || !strings.HasPrefix(s.ID, "$") {
		t.Fatalf("session = %+v", s)
	}
	if len(s.Windows) != 1 || !strings.HasPrefix(s.Windows[0].ID, "@") {
		t.Fatalf("windows = %+v", s.Windows)
	}
	if len(s.Windows[0].Panes) != 1 || !strings.HasPrefix(s.Windows[0].Panes[0].ID, "%") {
		t.Fatalf("panes = %+v", s.Windows[0].Panes)
	}
	if s.Activity.IsZero() {
		t.Fatal("activity timestamp is zero")
	}
}

func TestRenameSessionAndWindow(t *testing.T) {
	b := newTestBackend(t)
	if err := b.NewSession("alpha"); err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	s := findSession(t, mustTree(t, b), "alpha")
	if err := b.RenameSession(s.ID, "renamed"); err != nil {
		t.Fatalf("RenameSession: %v", err)
	}
	if err := b.RenameWindow(s.Windows[0].ID, "editor"); err != nil {
		t.Fatalf("RenameWindow: %v", err)
	}
	s = findSession(t, mustTree(t, b), "renamed")
	if s.Windows[0].Name != "editor" {
		t.Fatalf("window name = %q, want editor", s.Windows[0].Name)
	}
}

func TestNewWindowAndSplitPane(t *testing.T) {
	b := newTestBackend(t)
	if err := b.NewSession("alpha"); err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	s := findSession(t, mustTree(t, b), "alpha")
	if err := b.NewWindow(s.ID, "", ""); err != nil {
		t.Fatalf("NewWindow: %v", err)
	}
	s = findSession(t, mustTree(t, b), "alpha")
	if len(s.Windows) != 2 {
		t.Fatalf("windows = %d, want 2", len(s.Windows))
	}
	if err := b.SplitPane(s.Windows[0].Panes[0].ID, ""); err != nil {
		t.Fatalf("SplitPane: %v", err)
	}
	s = findSession(t, mustTree(t, b), "alpha")
	if got := len(s.Windows[0].Panes); got != 2 {
		t.Fatalf("panes = %d, want 2", got)
	}
}

func TestMoveWindow(t *testing.T) {
	b := newTestBackend(t)
	if err := b.NewSession("alpha"); err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	if err := b.NewSession("beta"); err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	tree := mustTree(t, b)
	alpha := findSession(t, tree, "alpha")
	// give alpha a second window so moving one does not kill the session
	// (moving a session's last window kills the session).
	if err := b.NewWindow(alpha.ID, "", ""); err != nil {
		t.Fatalf("NewWindow: %v", err)
	}
	tree = mustTree(t, b)
	alpha = findSession(t, tree, "alpha")
	winID := alpha.Windows[0].ID
	beta := findSession(t, tree, "beta")
	if err := b.MoveWindow(winID, beta.ID); err != nil {
		t.Fatalf("MoveWindow: %v", err)
	}
	tree = mustTree(t, b)
	if got := len(findSession(t, tree, "alpha").Windows); got != 1 {
		t.Fatalf("alpha windows = %d, want 1", got)
	}
	beta = findSession(t, tree, "beta")
	if len(beta.Windows) != 2 {
		t.Fatalf("beta windows = %d, want 2", len(beta.Windows))
	}
	// the moved window keeps its tmux ID.
	found := false
	for _, w := range beta.Windows {
		if w.ID == winID {
			found = true
		}
	}
	if !found {
		t.Fatalf("moved window %s not found under beta", winID)
	}
}

func TestMovePane(t *testing.T) {
	b := newTestBackend(t)
	if err := b.NewSession("alpha"); err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	s := findSession(t, mustTree(t, b), "alpha")
	if err := b.NewWindow(s.ID, "", ""); err != nil {
		t.Fatalf("NewWindow: %v", err)
	}
	s = findSession(t, mustTree(t, b), "alpha")
	if err := b.SplitPane(s.Windows[0].Panes[0].ID, ""); err != nil {
		t.Fatalf("SplitPane: %v", err)
	}
	s = findSession(t, mustTree(t, b), "alpha")
	paneID := s.Windows[0].Panes[0].ID
	dstWin := s.Windows[1].ID
	if err := b.MovePane(paneID, dstWin); err != nil {
		t.Fatalf("MovePane: %v", err)
	}
	s = findSession(t, mustTree(t, b), "alpha")
	if got := len(s.Windows[0].Panes); got != 1 {
		t.Fatalf("source window panes = %d, want 1", got)
	}
	if got := len(s.Windows[1].Panes); got != 2 {
		t.Fatalf("dest window panes = %d, want 2", got)
	}
}

func TestKillCascade(t *testing.T) {
	b := newTestBackend(t)
	if err := b.NewSession("alpha"); err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	s := findSession(t, mustTree(t, b), "alpha")

	// killing the only pane kills the window, and with it the session,
	// which stops the server.
	if err := b.KillPane(s.Windows[0].Panes[0].ID); err != nil {
		t.Fatalf("KillPane: %v", err)
	}
	if tree := mustTree(t, b); len(tree) != 0 {
		t.Fatalf("sessions after last-pane kill = %d, want 0", len(tree))
	}

	// two sessions: killing a window leaves the other session alone.
	if err := b.NewSession("one"); err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	if err := b.NewSession("two"); err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	tree := mustTree(t, b)
	one := findSession(t, tree, "one")
	if err := b.KillWindow(one.Windows[0].ID); err != nil {
		t.Fatalf("KillWindow: %v", err)
	}
	tree = mustTree(t, b)
	if len(tree) != 1 || tree[0].Name != "two" {
		t.Fatalf("tree after KillWindow = %+v", tree)
	}
	if err := b.KillSession(tree[0].ID); err != nil {
		t.Fatalf("KillSession: %v", err)
	}
	if tree = mustTree(t, b); len(tree) != 0 {
		t.Fatalf("sessions after KillSession = %d, want 0", len(tree))
	}
}

func TestListSockets(t *testing.T) {
	b := newTestBackend(t)
	socketName := b.socket // unique per test, created below
	if err := b.NewSession("alpha"); err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	names, err := ListSockets()
	if err != nil {
		t.Fatalf("ListSockets: %v", err)
	}
	found := false
	for _, n := range names {
		if n == socketName {
			found = true
		}
		if n == "default" {
			t.Fatal("ListSockets must exclude the default socket")
		}
	}
	if !found {
		t.Fatalf("ListSockets = %v, missing %q", names, socketName)
	}
}

func TestCapturePane(t *testing.T) {
	b := newTestBackend(t)
	if err := b.NewSession("alpha"); err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	s := findSession(t, mustTree(t, b), "alpha")
	paneID := s.Windows[0].Panes[0].ID
	if err := b.run("send-keys", "-t", paneID, "echo hello-tmuxgo", "Enter"); err != nil {
		t.Fatalf("send-keys: %v", err)
	}
	// the shell needs a moment to print; poll briefly.
	deadline := time.Now().Add(3 * time.Second)
	for {
		out, err := b.CapturePane(paneID, 0)
		if err != nil {
			t.Fatalf("CapturePane: %v", err)
		}
		if strings.Contains(out, "hello-tmuxgo") {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("pane content never showed marker, got:\n%s", out)
		}
		time.Sleep(100 * time.Millisecond)
	}
}
