package setup

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBlockDarwin(t *testing.T) {
	b := Block("darwin", "/usr/local/bin/tmuxgo")
	for _, want := range []string{
		startMarker, endMarker, "set -g mouse on", "set -g history-limit 5000",
		"copy-pipe-and-cancel \"pbcopy\"",
		"bind g display-popup -E -w 90% -h 85% '/usr/local/bin/tmuxgo --popup'",
	} {
		if !strings.Contains(b, want) {
			t.Fatalf("darwin block missing %q:\n%s", want, b)
		}
	}
	if strings.Contains(b, "set-clipboard") {
		t.Fatalf("darwin block must not use set-clipboard:\n%s", b)
	}
}

func TestBlockLinux(t *testing.T) {
	b := Block("linux", "/home/u/.local/bin/tmuxgo")
	for _, want := range []string{
		"set -s set-clipboard external",
		"copy-selection-and-cancel",
		"bind g display-popup -E -w 90% -h 85% '/home/u/.local/bin/tmuxgo --popup'",
	} {
		if !strings.Contains(b, want) {
			t.Fatalf("linux block missing %q:\n%s", want, b)
		}
	}
	if strings.Contains(b, "pbcopy") {
		t.Fatalf("linux block must not use pbcopy:\n%s", b)
	}
}

func TestApplyCreatesNewFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".tmux.conf")
	changed, err := Apply(path, Block("linux", "/x/tmuxgo"))
	if err != nil || !changed {
		t.Fatalf("Apply: changed=%v err=%v", changed, err)
	}
	raw, _ := os.ReadFile(path)
	if !strings.Contains(string(raw), startMarker) {
		t.Fatalf("file missing block:\n%s", raw)
	}
}

func TestApplyPreservesUserContentAndIsIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".tmux.conf")
	original := "# my own settings\nset -g status-left \"X\"\n"
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}

	changed, err := Apply(path, Block("linux", "/x/tmuxgo"))
	if err != nil || !changed {
		t.Fatalf("Apply: changed=%v err=%v", changed, err)
	}
	raw, _ := os.ReadFile(path)
	if !strings.HasPrefix(string(raw), original) {
		t.Fatalf("user content not preserved:\n%s", raw)
	}
	// one-time backup of the original file
	bak, err := os.ReadFile(path + ".tmuxgo-bak")
	if err != nil || string(bak) != original {
		t.Fatalf("backup = %q, %v", bak, err)
	}

	// second run with a new binary path: block replaced, not duplicated;
	// backup not overwritten
	changed, err = Apply(path, Block("linux", "/y/tmuxgo"))
	if err != nil || !changed {
		t.Fatalf("second Apply: changed=%v err=%v", changed, err)
	}
	raw, _ = os.ReadFile(path)
	if strings.Count(string(raw), startMarker) != 1 {
		t.Fatalf("block duplicated:\n%s", raw)
	}
	if !strings.Contains(string(raw), "/y/tmuxgo --popup") {
		t.Fatalf("block not updated:\n%s", raw)
	}
	if !strings.Contains(string(raw), "status-left") {
		t.Fatalf("user content lost on update:\n%s", raw)
	}
	bak, _ = os.ReadFile(path + ".tmuxgo-bak")
	if string(bak) != original {
		t.Fatalf("backup overwritten:\n%s", bak)
	}

	// third identical run: no change
	changed, err = Apply(path, Block("linux", "/y/tmuxgo"))
	if err != nil || changed {
		t.Fatalf("idempotent Apply: changed=%v err=%v", changed, err)
	}
}

func TestConfPath(t *testing.T) {
	home := t.TempDir()
	if got := ConfPath(home); got != filepath.Join(home, ".tmux.conf") {
		t.Fatalf("ConfPath (no files) = %s", got)
	}
	xdg := filepath.Join(home, ".config", "tmux", "tmux.conf")
	if err := os.MkdirAll(filepath.Dir(xdg), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(xdg, []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := ConfPath(home); got != xdg {
		t.Fatalf("ConfPath (xdg exists) = %s", got)
	}
	dot := filepath.Join(home, ".tmux.conf")
	if err := os.WriteFile(dot, []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := ConfPath(home); got != dot {
		t.Fatalf("ConfPath (dot wins) = %s", got)
	}
}
