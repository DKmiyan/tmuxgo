package app

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"
)

// dirPickState drives the "new session from directory" picker.
type dirPickState struct {
	all      []string
	filtered []string
	cursor   int
	offset   int
}

// loadDirs returns directory candidates: zoxide's frecency-ranked list
// when zoxide is installed, else the working directories of current panes
// plus $HOME. The dirList hook overrides both for tests.
func (m model) loadDirs() ([]string, error) {
	if m.dirList != nil {
		return m.dirList()
	}
	if out, err := exec.Command("zoxide", "query", "--list").Output(); err == nil {
		var dirs []string
		for _, l := range strings.Split(strings.TrimRight(string(out), "\n"), "\n") {
			if l != "" {
				dirs = append(dirs, l)
			}
		}
		if len(dirs) > 0 {
			return dirs, nil
		}
	}
	seen := map[string]bool{}
	var dirs []string
	for _, s := range m.tree {
		for _, w := range s.Windows {
			for _, p := range w.Panes {
				if p.CurrentPath != "" && !seen[p.CurrentPath] {
					seen[p.CurrentPath] = true
					dirs = append(dirs, p.CurrentPath)
				}
			}
		}
	}
	if home, err := os.UserHomeDir(); err == nil && !seen[home] {
		dirs = append(dirs, home)
	}
	return dirs, nil
}

// sessionNameForDir derives a tmux-safe session name from a directory:
// its basename with '.', ':' and spaces replaced ('.' and ':' are illegal
// in tmux session names).
func sessionNameForDir(dir string) string {
	base := filepath.Base(dir)
	return strings.NewReplacer(".", "_", ":", "_", " ", "_").Replace(base)
}

// startDirPick opens the directory picker with candidates loaded.
func (m model) startDirPick() (tea.Model, tea.Cmd) {
	dirs, err := m.loadDirs()
	if err != nil || len(dirs) == 0 {
		m.setStatus("no directories available", true)
		m.mode = modeNormal
		return m, nil
	}
	m.dirPick = &dirPickState{all: dirs, filtered: dirs}
	m.input.Reset()
	m.input.Prompt = "dir: "
	m.mode = modeDirPick
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
	case "up", "k":
		if st.cursor > 0 {
			st.cursor--
		}
		return m, nil
	case "down", "j":
		if st.cursor < len(st.filtered)-1 {
			st.cursor++
		}
		return m, nil
	case "enter":
		if len(st.filtered) == 0 {
			return m, nil
		}
		dir := st.filtered[st.cursor]
		m.mode = modeNormal
		m.dirPick = nil
		m.input.Blur()
		return m.openDirSession(dir)
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(k)
	f := strings.ToLower(m.input.Value())
	st.filtered = st.filtered[:0]
	for _, d := range st.all {
		if f == "" || strings.Contains(strings.ToLower(d), f) {
			st.filtered = append(st.filtered, d)
		}
	}
	st.cursor = 0
	st.offset = 0
	return m, cmd
}

// dirSessionMsg carries the result of creating a directory session.
type dirSessionMsg struct {
	id   string
	name string
	err  error
}

// openDirSession attaches to the session named after dir, creating it
// (anchored at dir) first when it does not exist.
func (m model) openDirSession(dir string) (tea.Model, tea.Cmd) {
	name := sessionNameForDir(dir)
	for _, s := range m.tree {
		if s.Name == name {
			return m, m.attach(s.ID)
		}
	}
	b := m.backend
	return m, func() tea.Msg {
		id, err := b.NewSessionID(name, dir)
		return dirSessionMsg{id: id, name: name, err: err}
	}
}
