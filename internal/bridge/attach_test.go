package bridge

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// A real native PTY client is launched without a shell command string. Python
// only allocates/drains the PTY; it neither fakes tmux nor examines its content.
const attachPTY = `import os,pty,fcntl,termios,struct,subprocess,select,sys
master,slave=pty.openpty()
fcntl.ioctl(slave,termios.TIOCSWINSZ,struct.pack('HHHH',24,100,0,0))
def setup():
 os.setsid()
 fcntl.ioctl(slave,termios.TIOCSCTTY,0)
p=subprocess.Popen([sys.argv[1],'bridge-attach','--ticket',sys.argv[2]],stdin=slave,stdout=slave,stderr=slave,preexec_fn=setup,env={**os.environ,'TERM':'xterm-256color'})
os.close(slave)
try:
 while p.poll() is None:
  ready,_,_=select.select([master],[],[],0.1)
  if ready:
   try: os.read(master,65536)
   except OSError: break
 p.wait(timeout=5)
finally:
 if p.poll() is None: p.terminate();p.wait(timeout=5)
 os.close(master)
sys.exit(p.returncode)
`

func TestRealAttachTicketTracksNativeClientAndCleansOnlyItself(t *testing.T) {
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 PTY allocator unavailable")
	}
	binary := filepath.Join(t.TempDir(), "tmuxgo")
	build := exec.Command("go", "build", "-o", binary, "../..")
	build.Env = append(os.Environ(), "GOPROXY=off")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build attach helper: %v %s", err, out)
	}
	s, b, home := fixture(t)
	one := result(t, mutation(t, s, "session.create", map[string]any{"name": "one", "cwd": home}))
	two := result(t, mutation(t, s, "session.create", map[string]any{"name": "two", "cwd": home}))
	window := result(t, mutation(t, s, "window.create", map[string]any{"sessionId": one.SessionID, "name": "window two", "cwd": home}))
	issued := result(t, mutation(t, s, "attach.issue", map[string]any{"sessionId": one.SessionID, "windowId": window.WindowID}))
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	child := exec.CommandContext(ctx, python, "-u", "-c", attachPTY, binary, issued.Ticket)
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- child.Wait() }()
	t.Cleanup(func() { s.Close(); cancel() })
	waitState := func(session, window string) Attachment {
		t.Helper()
		deadline := time.Now().Add(7 * time.Second)
		for time.Now().Before(deadline) {
			snapshot, _, err := s.observe(context.Background())
			if err == nil {
				for _, a := range snapshot.Attachments {
					if a.AttachmentID == issued.AttachmentID && a.State == "attached" && a.SessionID != nil && *a.SessionID == session && (window == "" || a.WindowID != nil && *a.WindowID == window) {
						return a
					}
				}
			}
			time.Sleep(40 * time.Millisecond)
		}
		t.Fatal("native attach state did not converge")
		return Attachment{}
	}
	waitState(one.SessionID, window.WindowID)
	if err := Attach(context.Background(), issued.Ticket); err == nil || err.Error() != "TICKET_INVALID" {
		t.Fatalf("ticket reused: %v", err)
	}
	claimed, err := readPrivate(filepath.Join(s.tickets.dir, strings.TrimPrefix(issued.AttachmentID, "attach:")+".claimed"))
	if err != nil {
		t.Fatal(err)
	}
	snapshot, _, err := s.observe(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	// Simulate the user's native tmux/tmuxgo action outside the bridge request loop.
	if _, err = b.BridgeMutate(context.Background(), snapshot.Server, []string{"switch-client", "-c", claimed.TTY, "-t", two.SessionID}, ""); err != nil {
		t.Fatal(err)
	}
	waitState(two.SessionID, "")
	result(t, mutation(t, s, "window.select", map[string]any{"sessionId": one.SessionID, "windowId": window.WindowID, "attachmentId": issued.AttachmentID}))
	current := waitState(one.SessionID, window.WindowID)
	if current.PaneID == nil {
		t.Fatal("missing actual active pane")
	}
	split := result(t, mutation(t, s, "pane.split", map[string]any{"sessionId": one.SessionID, "windowId": window.WindowID, "paneId": *current.PaneID, "direction": "vertical", "cwd": home}))
	result(t, mutation(t, s, "pane.select", map[string]any{"sessionId": one.SessionID, "windowId": window.WindowID, "paneId": split.PaneID, "attachmentId": issued.AttachmentID}))
	selected := waitState(one.SessionID, window.WindowID)
	if selected.PaneID == nil || *selected.PaneID != split.PaneID {
		t.Fatal("actual client active pane not selected")
	}
	s.Close()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("attach client did not detach when bridge disappeared")
	}
	snapshot, _, err = s.observe(context.Background())
	if err != nil || len(snapshot.Sessions) != 2 {
		t.Fatal("disconnect killed remote sessions")
	}
}
