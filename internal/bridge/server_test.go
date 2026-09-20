package bridge

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DKmiyan/tmuxgo/internal/i18n"
	"github.com/DKmiyan/tmuxgo/internal/tmux"
)

func fixture(t *testing.T) (*Server, *tmux.Tmux, string) {
	t.Helper()
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not installed")
	}
	home, err := os.MkdirTemp("/tmp", "tmuxgo-br-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(home) })
	t.Setenv("HOME", home)
	t.Setenv("TMUX_TMPDIR", home)
	t.Setenv("SHELL", "/bin/sh")
	t.Setenv("TMUX", "")
	t.Setenv("TMUX_PANE", "")
	b, err := tmux.NewBridge("tmuxgo-bridge-" + randomHex(8))
	if err != nil {
		t.Fatal(err)
	}
	s, err := New(b, i18n.EN)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		snapshot, _, err := s.observe(context.Background())
		if err == nil {
			for _, session := range snapshot.Sessions {
				_, _ = b.BridgeMutate(context.Background(), snapshot.Server, []string{"kill-session", "-t", session.ID}, "")
			}
		}
		s.Close()
	})
	return s, b, home
}
func call(t *testing.T, s *Server, command string, params map[string]any) Response {
	t.Helper()
	raw, _ := json.Marshal(map[string]any{"version": 0, "requestId": "req-" + randomHex(4), "command": command, "params": params})
	return s.Handle(context.Background(), raw)
}
func mutation(t *testing.T, s *Server, command string, params map[string]any) Response {
	t.Helper()
	snapshot, _, err := s.observe(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	params["expectedGeneration"] = snapshot.Server.Generation
	params["operationId"] = "op-" + randomHex(8)
	return call(t, s, command, params)
}
func result(t *testing.T, r Response) Result {
	t.Helper()
	if !r.OK {
		t.Fatalf("operation error: %+v", r.Error)
	}
	return r.Result.(Result)
}
func TestStrictRequests(t *testing.T) {
	valid := `{"version":0,"requestId":"r","command":"snapshot","params":{}}`
	if _, _, e := decode([]byte(valid)); e != nil {
		t.Fatal(e)
	}
	for _, raw := range []string{`{"requestId":"r","command":"snapshot","params":{}}`,
		`{"version":null,"requestId":"r","command":"snapshot","params":{}}`, `{"version":0,"version":0,"requestId":"r","command":"snapshot","params":{}}`, `{"version":0,"requestId":"r","command":"snapshot","params":null}`, strings.Replace(valid, `"params":{}`, `"params":{"argv":[]}`, 1), strings.Replace(valid, `"version":0`, `"version":1`, 1)} {
		if _, _, e := decode([]byte(raw)); e == nil {
			t.Fatalf("accepted invalid %s", raw)
		}
	}
}
func TestRealNativeMutationsMetadataAndDedup(t *testing.T) {
	s, _, home := fixture(t)
	first := result(t, mutation(t, s, "session.create", map[string]any{"name": "first", "cwd": home}))
	special := filepath.Join(home, "cwd'\"\\; $() `quoted` #{pid} 中文")
	if err := os.Mkdir(special, 0700); err != nil {
		t.Fatal(err)
	}
	canonical, err := filepath.EvalSymlinks(special)
	if err != nil {
		t.Fatal(err)
	}
	specialWindow := result(t, mutation(t, s, "window.create", map[string]any{"sessionId": first.SessionID, "name": "data-path", "cwd": special}))
	check, _, err := s.observe(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, w := range check.Sessions[0].Windows {
		if w.ID == specialWindow.WindowID && w.Panes[0].Cwd != canonical {
			t.Fatalf("cwd data changed: %q", w.Panes[0].Cwd)
		}
	}

	window := result(t, mutation(t, s, "window.create", map[string]any{"sessionId": first.SessionID, "name": "second window", "cwd": home}))
	snapshot, _, err := s.observe(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var paneID string
	for _, w := range snapshot.Sessions[0].Windows {
		if w.ID == window.WindowID {
			paneID = w.Panes[0].ID
		}
	}
	split := result(t, mutation(t, s, "pane.split", map[string]any{"sessionId": first.SessionID, "windowId": window.WindowID, "paneId": paneID, "direction": "horizontal", "cwd": home}))
	if split.PaneID == paneID {
		t.Fatal("split did not create a pane")
	}
	name := `literal" apostrophe' $TMUXGO_PROOF ; new-session -s injected \ $(touch ` + filepath.Join(home, "not-executed") + `) #{pid} 中文 ` + "`"
	t.Setenv("TMUXGO_PROOF", "expanded-value")
	result(t, mutation(t, s, "window.rename", map[string]any{"sessionId": first.SessionID, "windowId": window.WindowID, "name": name}))
	snapshot, _, err = s.observe(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Sessions) != 1 {
		t.Fatal("data became a command")
	}
	found := false
	for _, w := range snapshot.Sessions[0].Windows {
		if w.ID == window.WindowID {
			found = w.Name == name
			if len(w.Panes) != 2 || w.Layout == "" {
				t.Fatal("missing native pane layout")
			}
		}
	}
	if _, err := os.Stat(filepath.Join(home, "not-executed")); !os.IsNotExist(err) {
		t.Fatal("name executed a command")
	}
	if !found {
		t.Fatalf("literal name did not round trip: %#v want %q", snapshot.Sessions[0].Windows, name)
	}
	p := map[string]any{"name": "deduplicated", "cwd": home, "expectedGeneration": snapshot.Server.Generation, "operationId": "same-op"}
	one := result(t, call(t, s, "session.create", p))
	two := result(t, call(t, s, "session.create", p))
	if one.SessionID != two.SessionID {
		t.Fatal("repeated creation")
	}
	p["name"] = "changed"
	if r := call(t, s, "session.create", p); r.OK || r.Error.Code != "OPERATION_CONFLICT" {
		t.Fatal("operation conflict accepted")
	}
	if r := mutation(t, s, "window.rename", map[string]any{"sessionId": one.SessionID, "windowId": window.WindowID, "name": "wrong"}); r.OK || r.Error.Code != "OWNERSHIP_MISMATCH" {
		t.Fatal("cross-session window mutation")
	}
	if r := mutation(t, s, "session.kill", map[string]any{"sessionId": first.SessionID, "confirm": false}); r.OK || r.Error.Code != "CONFIRM_REQUIRED" {
		t.Fatal("kill without confirmation")
	}
	raw, _ := json.Marshal(snapshot)
	for _, forbidden := range []string{`"pid"`, `"argv"`, `"tty"`, `"content"`, `"socketPath"`} {
		if bytes.Contains(raw, []byte(forbidden)) {
			t.Fatal("private field in snapshot", forbidden)
		}
	}
	result(t, mutation(t, s, "window.kill", map[string]any{"sessionId": first.SessionID, "windowId": window.WindowID, "confirm": true}))
	afterWindow, _, err := s.observe(context.Background())
	if err != nil || len(afterWindow.Sessions) != 2 {
		t.Fatal("window kill changed unrelated sessions", err)
	}
	remaining := map[string]bool{}
	for _, session := range afterWindow.Sessions {
		for _, w := range session.Windows {
			if w.ID == window.WindowID {
				t.Fatal("killed window is still present")
			}
			remaining[w.ID] = true
			for _, pane := range w.Panes {
				if pane.ID == paneID || pane.ID == split.PaneID {
					t.Fatal("killed window pane survived")
				}
			}
		}
	}
	for _, w := range snapshot.Sessions[0].Windows {
		if w.ID != window.WindowID && !remaining[w.ID] {
			t.Fatal("window kill removed an unrelated window")
		}
	}
	result(t, mutation(t, s, "session.kill", map[string]any{"sessionId": first.SessionID, "confirm": true}))
	afterSession, _, err := s.observe(context.Background())
	if err != nil || len(afterSession.Sessions) != 1 || afterSession.Sessions[0].ID != one.SessionID {
		t.Fatal("session kill failed or changed the other session", err)
	}
	for _, w := range afterSession.Sessions[0].Windows {
		if !remaining[w.ID] {
			t.Fatal("unrelated session window identity changed")
		}
	}
}
func TestGenerationChangeRefusesReusedIDs(t *testing.T) {
	s, b, home := fixture(t)
	old := result(t, mutation(t, s, "session.create", map[string]any{"name": "old", "cwd": home}))
	before, _, _ := s.observe(context.Background())
	_, err := b.BridgeMutate(context.Background(), before.Server, []string{"kill-session", "-t", old.SessionID}, "")
	if err != nil {
		t.Fatal(err)
	}
	fresh := result(t, mutation(t, s, "session.create", map[string]any{"name": "new", "cwd": home}))
	if fresh.Generation == before.Server.Generation {
		t.Fatal("generation did not change")
	}
	r := call(t, s, "session.kill", map[string]any{"sessionId": fresh.SessionID, "confirm": true, "expectedGeneration": before.Server.Generation, "operationId": "stale-kill"})
	if r.OK || r.Error.Code != "SERVER_CHANGED" {
		t.Fatal("old generation mutated current server")
	}
	snapshot, _, _ := s.observe(context.Background())
	if len(snapshot.Sessions) != 1 {
		t.Fatal("new server was killed")
	}
}
func TestStdioSnapshotsAndEOFCleanup(t *testing.T) {
	_, b, _ := fixture(t)
	var output bytes.Buffer
	err := Run(context.Background(), strings.NewReader(`{"version":0,"requestId":"read","command":"snapshot","params":{}}`+"\n"), &output, b, i18n.EN, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(output.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("wire lines %d", len(lines))
	}
	for _, line := range lines {
		var value map[string]any
		if json.Unmarshal([]byte(line), &value) != nil || value["version"] != float64(0) {
			t.Fatal("non-versioned stdout")
		}
	}
}
func TestTicketCapabilityAndCleanup(t *testing.T) {
	s, _, home := fixture(t)
	created := result(t, mutation(t, s, "session.create", map[string]any{"name": "ticket", "cwd": home}))
	issued := result(t, mutation(t, s, "attach.issue", map[string]any{"sessionId": created.SessionID}))
	if !ticketPattern.MatchString(issued.Ticket) {
		t.Fatal("bad ticket")
	}
	err := Attach(context.Background(), issued.Ticket[:len(issued.Ticket)-1]+"x")
	if err == nil || err.Error() != "TICKET_INVALID" {
		t.Fatal("forged ticket accepted")
	}
	mode, err := os.Stat(s.tickets.dir)
	if err != nil || mode.Mode().Perm() != 0700 {
		t.Fatal("ticket directory not private")
	}
	file := filepath.Join(s.tickets.dir, strings.TrimPrefix(issued.AttachmentID, "attach:")+".pending")
	meta, err := os.Stat(file)
	if err != nil || meta.Mode().Perm() != 0600 {
		t.Fatal("ticket file not private")
	}
	s.Close()
	if _, err = os.Stat(file); !os.IsNotExist(err) {
		t.Fatal("bridge cleanup left ticket")
	}
	snapshot, _, err := s.observe(context.Background())
	if err != nil || len(snapshot.Sessions) != 1 {
		t.Fatal("cleanup killed remote task")
	}
}

func TestNativeNameFormatFieldsRemainLiteral(t *testing.T) {
	s, _, home := fixture(t)
	name := "literal-#{pid}-中文"
	created := result(t, mutation(t, s, "session.create", map[string]any{"name": name, "cwd": home}))
	inspect := func(command string, session, window string) {
		t.Helper()
		snapshot, _, err := s.observe(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		for _, v := range snapshot.Sessions {
			if v.ID != session {
				continue
			}
			got := v.Name
			if window != "" {
				for _, w := range v.Windows {
					if w.ID == window {
						got = w.Name
					}
				}
			}
			if got != name {
				t.Errorf("%s name=%q want=%q", command, got, name)
			}
		}
	}
	inspect("session.create", created.SessionID, "")
	result(t, mutation(t, s, "session.rename", map[string]any{"sessionId": created.SessionID, "name": name}))
	inspect("session.rename", created.SessionID, "")
	window := result(t, mutation(t, s, "window.create", map[string]any{"sessionId": created.SessionID, "name": name, "cwd": home}))
	inspect("window.create", created.SessionID, window.WindowID)
	result(t, mutation(t, s, "window.rename", map[string]any{"sessionId": created.SessionID, "windowId": window.WindowID, "name": name}))
	inspect("window.rename", created.SessionID, window.WindowID)
}

func TestFirstSessionCreationRetryUsesItsResultGeneration(t *testing.T) {
	s, _, home := fixture(t)
	before, _, err := s.observe(context.Background())
	if err != nil || before.Server.Running {
		t.Fatal("expected isolated absent server")
	}
	params := map[string]any{"name": "first", "cwd": home, "expectedGeneration": before.Server.Generation, "operationId": "first-operation"}
	first := result(t, call(t, s, "session.create", params))
	again := result(t, call(t, s, "session.create", params))
	if first.SessionID != again.SessionID || first.Generation != again.Generation {
		t.Fatal("first-session retry duplicated work")
	}
	snapshot, _, err := s.observe(context.Background())
	if err != nil || len(snapshot.Sessions) != 1 {
		t.Fatal("duplicated initial session")
	}
}

func TestLiteralDollarNames(t *testing.T) {
	s, _, home := fixture(t)
	session := result(t, mutation(t, s, "session.create", map[string]any{"name": "dollar-test", "cwd": home}))
	window := result(t, mutation(t, s, "window.create", map[string]any{"sessionId": session.SessionID, "name": "initial", "cwd": home}))
	for _, name := range []string{`$VALUE`, `\$VALUE`, `${VALUE}`, `\${VALUE}`, `$_VALUE`, `\$9`, `\$()`, `\\$VALUE`} {
		t.Run(name, func(t *testing.T) {
			result(t, mutation(t, s, "session.rename", map[string]any{"sessionId": session.SessionID, "name": name}))
			result(t, mutation(t, s, "window.rename", map[string]any{"sessionId": session.SessionID, "windowId": window.WindowID, "name": name}))
			snapshot, _, err := s.observe(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if len(snapshot.Sessions) != 1 || snapshot.Sessions[0].Name != name {
				t.Fatalf("session name did not round trip: %+v want %q", snapshot.Sessions, name)
			}
			found := false
			for _, w := range snapshot.Sessions[0].Windows {
				if w.ID == window.WindowID {
					found = w.Name == name
				}
			}
			if !found {
				t.Fatalf("window name did not round trip: %+v want %q", snapshot.Sessions[0].Windows, name)
			}
		})
	}
}
