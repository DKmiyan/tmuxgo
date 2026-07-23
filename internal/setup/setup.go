// Package setup manages tmuxgo's block in the user's tmux.conf:
// popup binding, mouse, history, and OS-appropriate clipboard integration.
package setup

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

const (
	startMarker = "# >>> tmuxgo >>>"
	endMarker   = "# <<< tmuxgo <<<"
)

// Block renders the managed tmux.conf block for the given OS and the
// absolute path of the tmuxgo binary. The path must be absolute: the tmux
// server's PATH often lacks ~/.local/bin, which kills popup commands.
func Block(goos, binPath string) string {
	var b strings.Builder
	b.WriteString(startMarker + "\n")
	b.WriteString("# Managed by `tmuxgo setup`; edit outside this block.\n")
	b.WriteString("set -g mouse on\n")
	b.WriteString("set -g history-limit 5000\n")
	if goos == "darwin" {
		b.WriteString("bind-key -T copy-mode-vi MouseDragEnd1Pane send-keys -X copy-pipe-and-cancel \"pbcopy\"\n")
		b.WriteString("bind-key -T copy-mode MouseDragEnd1Pane send-keys -X copy-pipe-and-cancel \"pbcopy\"\n")
	} else {
		b.WriteString("set -s set-clipboard external\n")
		b.WriteString("bind-key -T copy-mode-vi MouseDragEnd1Pane send-keys -X copy-selection-and-cancel\n")
		b.WriteString("bind-key -T copy-mode MouseDragEnd1Pane send-keys -X copy-selection-and-cancel\n")
	}
	b.WriteString(fmt.Sprintf("bind g display-popup -E -w 90%% -h 85%% '%s --popup'\n", binPath))
	b.WriteString(endMarker + "\n")
	return b.String()
}

// ConfPath picks the tmux.conf to manage: an existing ~/.tmux.conf wins,
// then an existing ~/.config/tmux/tmux.conf, otherwise ~/.tmux.conf.
func ConfPath(home string) string {
	dot := filepath.Join(home, ".tmux.conf")
	if _, err := os.Stat(dot); err == nil {
		return dot
	}
	xdg := filepath.Join(home, ".config", "tmux", "tmux.conf")
	if _, err := os.Stat(xdg); err == nil {
		return xdg
	}
	return dot
}

// Apply writes block into the conf file at path, replacing any previous
// managed block and preserving all other content. It returns whether the
// file changed. A one-time backup (<path>.tmuxgo-bak) is made before the
// first modification.
func Apply(path, block string) (bool, error) {
	raw, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return false, err
	}
	old := string(raw)
	start := strings.Index(old, startMarker)
	end := strings.Index(old, endMarker)

	var next string
	switch {
	case start >= 0 && end >= start:
		end += len(endMarker)
		next = strings.TrimRight(old[:start], "\n") + "\n\n" + block + strings.TrimLeft(old[end:], "\n")
		next = strings.TrimRight(next, "\n") + "\n"
	default:
		next = old
		if next != "" && !strings.HasSuffix(next, "\n") {
			next += "\n"
		}
		if next != "" {
			next += "\n"
		}
		next += block
	}

	if next == old {
		return false, nil
	}
	if _, err := os.Stat(path); err == nil {
		bak := path + ".tmuxgo-bak"
		if _, err := os.Stat(bak); os.IsNotExist(err) {
			if err := os.WriteFile(bak, raw, 0o644); err != nil {
				return false, fmt.Errorf("backup %s: %w", bak, err)
			}
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return false, err
	}
	if err := os.WriteFile(path, []byte(next), 0o644); err != nil {
		return false, err
	}
	return true, nil
}

// Reload asks the running tmux server to re-read the conf. It is not an
// error when no server is running: the block applies on the next start.
func Reload(path string) error {
	cmd := exec.Command("tmux", "source-file", path)
	if out, err := cmd.CombinedOutput(); err != nil {
		if strings.Contains(string(out), "no server running") ||
			strings.Contains(string(out), "No such file or directory") {
			return nil
		}
		return fmt.Errorf("tmux source-file: %s", strings.TrimSpace(string(out)))
	}
	return nil
}

// Run performs the full setup: resolve binary path, render block, apply,
// reload. It returns the conf path and whether the file changed.
func Run(home string) (string, bool, error) {
	bin, err := os.Executable()
	if err != nil {
		return "", false, err
	}
	if bin, err = filepath.Abs(bin); err != nil {
		return "", false, err
	}
	conf := ConfPath(home)
	changed, err := Apply(conf, Block(runtime.GOOS, bin))
	if err != nil {
		return "", false, err
	}
	return conf, changed, Reload(conf)
}
