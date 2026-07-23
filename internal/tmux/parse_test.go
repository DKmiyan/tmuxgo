package tmux

import (
	"strings"
	"testing"
	"time"
)

func TestBuildTreeSortsAndNests(t *testing.T) {
	sessions := "$0\talpha\t1\t100\n$1\tbeta\t0\t200\n"
	windows := "@0\t$0\t1\teditor\t0\n@1\t$0\t0\tshell\t1\n@2\t$1\t0\tlogs\t1\n"
	panes := "%0\t@0\t1\t0\tvim\t/src\n%1\t@0\t0\t1\tbash\t/tmp\n%2\t@1\t0\t1\tbash\t/root\n%3\t@2\t0\t1\ttail\t/var\n"

	tree, err := buildTree(sessions, windows, panes)
	if err != nil {
		t.Fatalf("buildTree: %v", err)
	}
	if len(tree) != 2 {
		t.Fatalf("sessions = %d, want 2", len(tree))
	}
	// beta has the newer activity (200) so it must sort first.
	if tree[0].Name != "beta" || tree[1].Name != "alpha" {
		t.Fatalf("activity order = [%s %s], want [beta alpha]", tree[0].Name, tree[1].Name)
	}
	alpha := tree[1]
	if !alpha.Attached || tree[0].Attached {
		t.Fatalf("attached flags wrong: alpha=%v beta=%v", alpha.Attached, tree[0].Attached)
	}
	if got := alpha.Activity; !got.Equal(time.Unix(100, 0)) {
		t.Fatalf("alpha activity = %v", got)
	}
	// windows sorted by index, not by the order tmux printed them.
	if alpha.Windows[0].Name != "shell" || alpha.Windows[1].Name != "editor" {
		t.Fatalf("window order = [%s %s]", alpha.Windows[0].Name, alpha.Windows[1].Name)
	}
	// panes sorted by index too.
	editor := alpha.Windows[1]
	if editor.Panes[0].CurrentCommand != "bash" || editor.Panes[1].CurrentCommand != "vim" {
		t.Fatalf("pane order = [%s %s]", editor.Panes[0].CurrentCommand, editor.Panes[1].CurrentCommand)
	}
	if editor.Panes[0].CurrentPath != "/tmp" {
		t.Fatalf("pane path = %q", editor.Panes[0].CurrentPath)
	}
}

func TestBuildTreeEmpty(t *testing.T) {
	tree, err := buildTree("", "", "")
	if err != nil {
		t.Fatalf("buildTree: %v", err)
	}
	if len(tree) != 0 {
		t.Fatalf("sessions = %d, want 0", len(tree))
	}
}

func TestBuildTreeRejectsGarbage(t *testing.T) {
	if _, err := buildTree("not-a-record", "", ""); err == nil {
		t.Fatal("expected parse error")
	}
}

func TestTailLines(t *testing.T) {
	in := "a\nb\nc\nd\n"
	if got := tailLines(in, 2); got != "c\nd" {
		t.Fatalf("tailLines cap = %q, want %q", got, "c\\nd")
	}
	if got := tailLines(in, 0); got != "a\nb\nc\nd" {
		t.Fatalf("tailLines uncapped = %q", got)
	}
	if got := tailLines("x\n\n\n\n", 10); got != "x" {
		t.Fatalf("tailLines blank-trim = %q, want %q", got, "x")
	}
	if !strings.Contains(tailLines(in, 99), "a") {
		t.Fatal("cap larger than input must keep everything")
	}
}
