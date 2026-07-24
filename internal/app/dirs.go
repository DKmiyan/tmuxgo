package app

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/DKmiyan/tmuxgo/internal/tmux"
)

// dirPickState holds the VSCode-style completion state for the new-session
// directory input: subdirectory matches for the currently typed path.
type dirPickState struct {
	matches []string
	cursor  int
}

// contextDir is the working directory offered as the default for a new
// session: the selected pane's cwd (or the active pane of the selected
// window/session), falling back to $HOME.
func (m model) contextDir() string {
	if r, ok := m.currentRow(); ok {
		switch r.kind {
		case rowPane:
			if r.pane.CurrentPath != "" {
				return r.pane.CurrentPath
			}
		case rowWindow:
			if d := paneDir(r.window); d != "" {
				return d
			}
		case rowSession:
			for i := range r.session.Windows {
				if r.session.Windows[i].Active {
					if d := paneDir(&r.session.Windows[i]); d != "" {
						return d
					}
				}
			}
		}
	}
	if home, err := os.UserHomeDir(); err == nil {
		return home
	}
	return "/"
}

func paneDir(w *tmux.Window) string {
	for _, p := range w.Panes {
		if p.Active && p.CurrentPath != "" {
			return p.CurrentPath
		}
	}
	if len(w.Panes) > 0 {
		return w.Panes[0].CurrentPath
	}
	return ""
}

// startDirStep begins a two-step create flow at the directory step: an
// input prefilled with the context cwd, with live subdirectory
// completion. After the directory is accepted, purpose/prompt drive the
// name step; targetID is the session for window creation.
func (m model) startDirStep(purpose inputPurpose, targetID, namePrompt string) (tea.Model, tea.Cmd) {
	m.pendingPurpose = purpose
	m.pendingPrompt = namePrompt
	m.inputTarget = targetID
	m.dirPick = &dirPickState{}
	m.input.Reset()
	m.input.SetValue(m.contextDir())
	m.input.CursorEnd()
	m.input.Prompt = "dir: "
	m.mode = modeDirPick
	m.refreshDirCompletions()
	return m, m.input.Focus()
}

func (m model) handleDirPickKey(k tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	st := m.dirPick
	if st == nil {
		m.mode = modeNormal
		return m, nil
	}
	switch k.String() {
	case "esc":
		m.mode = modeNormal
		m.dirPick = nil
		m.input.Blur()
		return m, nil
	case "tab", "right":
		m.applyDirCompletion()
		return m, nil
	case "up", "k":
		if st.cursor > 0 {
			st.cursor--
		}
		return m, nil
	case "down", "j":
		if st.cursor < len(st.matches)-1 {
			st.cursor++
		}
		return m, nil
	case "enter":
		dir := expandHome(m.input.Value())
		if info, err := os.Stat(dir); err != nil || !info.IsDir() {
			m.setStatus("not a directory: "+dir, true)
			return m, nil
		}
		// directory accepted: on to the name step
		m.pendingDir = dir
		m.dirPick = nil
		m.inputPurpose = m.pendingPurpose
		m.input.Reset()
		m.input.SetValue(sessionNameForDir(dir))
		m.input.CursorEnd()
		m.input.Prompt = m.pendingPrompt
		m.mode = modeInput
		return m, m.input.Focus()
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(k)
	m.refreshDirCompletions()
	return m, cmd
}

// refreshDirCompletions recomputes the subdirectory matches for the typed
// path and keeps the highlight in range.
func (m *model) refreshDirCompletions() {
	st := m.dirPick
	if st == nil {
		return
	}
	st.matches = dirCompletions(m.input.Value())
	if st.cursor >= len(st.matches) {
		st.cursor = len(st.matches) - 1
	}
	if st.cursor < 0 {
		st.cursor = 0
	}
}

// applyDirCompletion completes the highlighted subdirectory into the input
// (VSCode-style: Tab or Right descends into it).
func (m *model) applyDirCompletion() {
	st := m.dirPick
	if st == nil || len(st.matches) == 0 {
		return
	}
	dirPart, _ := splitPath(expandHome(m.input.Value()))
	m.input.SetValue(joinDir(dirPart, st.matches[st.cursor]) + "/")
	m.input.CursorEnd()
	st.cursor = 0
	m.refreshDirCompletions()
}

// dirCompletions lists subdirectories of the typed path's directory part
// whose names prefix-match the fragment after the last '/' (case
// insensitive, dot-dirs hidden unless the fragment starts with '.').
func dirCompletions(input string) []string {
	dirPart, frag := splitPath(expandHome(input))
	entries, err := os.ReadDir(dirPart)
	if err != nil {
		return nil
	}
	frag = strings.ToLower(frag)
	var matches []string
	for _, e := range entries {
		name := e.Name()
		if !e.IsDir() && !isDirSymlink(e) {
			continue
		}
		if strings.HasPrefix(name, ".") && !strings.HasPrefix(frag, ".") {
			continue
		}
		if frag == "" || strings.HasPrefix(strings.ToLower(name), frag) {
			matches = append(matches, name)
		}
	}
	sort.Strings(matches)
	if len(matches) > 50 {
		matches = matches[:50]
	}
	return matches
}

func isDirSymlink(e os.DirEntry) bool {
	if e.Type()&os.ModeSymlink == 0 {
		return false
	}
	info, err := e.Info()
	return err == nil && info.IsDir()
}

// splitPath splits a path at the last '/' into directory and fragment.
func splitPath(p string) (dir, frag string) {
	i := strings.LastIndex(p, "/")
	switch {
	case i < 0:
		return ".", p
	case i == 0:
		return "/", p[1:]
	default:
		return p[:i], p[i+1:]
	}

}

// joinDir joins a directory and a name.
func joinDir(dir, name string) string {
	if dir == "/" {
		return "/" + name
	}
	if dir == "." {
		return name
	}
	return dir + "/" + name
}

// expandHome expands a leading "~" to the user's home directory.
func expandHome(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return home + p[1:]
		}
	}
	return p
}

// sessionNameForDir derives a tmux-safe session name from a directory:
// its basename with '.', ':' and spaces replaced ('.' and ':' are illegal
// in tmux session names).
func sessionNameForDir(dir string) string {
	base := filepath.Base(dir)
	return strings.NewReplacer(".", "_", ":", "_", " ", "_").Replace(base)
}

// dirSessionMsg carries the result of creating a directory-anchored session.
type dirSessionMsg struct {
	id   string
	name string
	err  error
}
