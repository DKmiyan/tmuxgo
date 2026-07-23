package template

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DKmiyan/tmuxgo/internal/tmux"
)

func testBackend(t *testing.T) *tmux.Tmux {
	t.Helper()
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux binary not found; skipping integration test")
	}
	socket := "tmuxgo-tpl-test-" + strings.NewReplacer("/", "-", " ", "-").Replace(t.Name())
	t.Cleanup(func() { _ = exec.Command("tmux", "-L", socket, "kill-server").Run() })
	return tmux.NewWithSocket(socket)
}

func findSession(t *testing.T, b *tmux.Tmux, name string) tmux.Session {
	t.Helper()
	tree, err := b.Tree()
	if err != nil {
		t.Fatalf("Tree: %v", err)
	}
	for _, s := range tree {
		if s.Name == name {
			return s
		}
	}
	t.Fatalf("session %q not found", name)
	return tmux.Session{}
}

func TestCaptureCreateRoundTrip(t *testing.T) {
	b := testBackend(t)

	// source session: two windows, first with two panes
	if err := b.NewSession("src"); err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	src := findSession(t, b, "src")
	if err := b.RenameWindow(src.Windows[0].ID, "editor"); err != nil {
		t.Fatalf("RenameWindow: %v", err)
	}
	if err := b.SplitPane(src.Windows[0].Panes[0].ID, ""); err != nil {
		t.Fatalf("SplitPane: %v", err)
	}
	if err := b.NewWindow(src.ID, "logs", ""); err != nil {
		t.Fatalf("NewWindow: %v", err)
	}

	src = findSession(t, b, "src")
	tpl := Capture("mytemplate", src)
	if len(tpl.Windows) != 2 || len(tpl.Windows[0].Panes) != 2 {
		t.Fatalf("captured template = %+v", tpl)
	}
	if tpl.Windows[0].Layout == "" {
		t.Fatal("captured window layout is empty")
	}

	newID, err := tpl.Create(b)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if newID == "" {
		t.Fatal("Create returned empty session ID")
	}
	dst := findSession(t, b, "mytemplate")
	if len(dst.Windows) != 2 {
		t.Fatalf("dst windows = %d, want 2", len(dst.Windows))
	}
	if dst.Windows[0].Name != "editor" || dst.Windows[1].Name != "logs" {
		t.Fatalf("dst window names = [%s %s]", dst.Windows[0].Name, dst.Windows[1].Name)
	}
	if len(dst.Windows[0].Panes) != 2 {
		t.Fatalf("dst window 0 panes = %d, want 2", len(dst.Windows[0].Panes))
	}
}

func TestStoreRoundTrip(t *testing.T) {
	s := NewStore(filepath.Join(t.TempDir(), "sub", "templates.json"))

	if ts, err := s.List(); err != nil || len(ts) != 0 {
		t.Fatalf("empty List = %v, %v", ts, err)
	}
	if _, err := s.Get("nope"); err == nil {
		t.Fatal("Get on missing template must fail")
	}
	tpl := Template{Name: "dev", Windows: []Window{{Name: "editor", Panes: []Pane{{Dir: "/x"}}}}}
	if err := s.Save(tpl); err != nil {
		t.Fatalf("Save: %v", err)
	}
	// upsert replaces, does not duplicate
	if err := s.Save(Template{Name: "dev", Windows: []Window{{Name: "renamed"}}}); err != nil {
		t.Fatalf("Save upsert: %v", err)
	}
	ts, err := s.List()
	if err != nil || len(ts) != 1 {
		t.Fatalf("List after upsert = %v, %v", ts, err)
	}
	got, err := s.Get("dev")
	if err != nil || got.Windows[0].Name != "renamed" {
		t.Fatalf("Get = %+v, %v", got, err)
	}
	if err := s.Delete("dev"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if err := s.Delete("dev"); err == nil {
		t.Fatal("Delete of missing template must fail")
	}
}

func TestCreateRejectsEmptyTemplate(t *testing.T) {
	b := testBackend(t)
	if _, err := (Template{Name: "empty"}).Create(b); err == nil {
		t.Fatal("Create of windowless template must fail")
	}
}
