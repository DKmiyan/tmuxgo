package app

import (
	"fmt"
	"os"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/DKmiyan/tmuxgo/internal/tmux"
)

// View implements tea.Model.
func (m model) View() tea.View {
	v := tea.NewView(m.render())
	v.AltScreen = true
	v.WindowTitle = "tmuxgo"
	return v
}

func (m model) render() string {
	w := m.width
	if w < 20 {
		w = 20
	}
	header := m.renderHeader(w)
	footer := m.renderFooter(w)
	middle := m.renderPromptOrStatus(w)
	bodyH := m.height - 3
	if bodyH < 1 {
		bodyH = 1
	}

	var body string
	switch m.mode {
	case modeHelp:
		body = m.renderHelp(w, bodyH)
	case modeCreate:
		body = m.renderCreate(w, bodyH)
	case modeConfirm:
		body = m.renderConfirm(w, bodyH)
	case modeMove:
		body = m.renderMove(w, bodyH)
	default:
		body = m.renderBody(w, bodyH)
	}
	return lipgloss.JoinVertical(lipgloss.Left, header, body, middle, footer)
}

// bodyHeight is the number of rows available for the tree area.
func (m model) bodyHeight() int {
	h := m.height - 3
	if h < 1 {
		return 1
	}
	return h
}

// --- header / footer / status ---

func (m model) renderHeader(w int) string {
	nWindows := 0
	for _, s := range m.tree {
		nWindows += len(s.Windows)
	}
	left := m.sty.title().Render("tmuxgo") +
		m.sty.meta().Render(fmt.Sprintf("  %s · %s", plural(len(m.tree), "session"), plural(nWindows, "window")))
	return trunc(left, w)
}

func (m model) renderFooter(w int) string {
	var s string
	switch m.mode {
	case modeFilter, modeInput:
		s = "enter apply · esc cancel"
	case modeCreate, modeMove:
		s = "↑/↓ choose · enter confirm · esc cancel"
	case modeConfirm:
		s = "y confirm · n cancel"
	case modeHelp:
		s = "any key to close"
	default:
		s = "n new · r rename · m move · d kill · / filter · p preview · ? help · q quit"
		if m.filter != "" {
			s = fmt.Sprintf("filter: %q (esc clears) · %s", m.filter, s)
		}
	}
	return trunc(m.sty.meta().Render(s), w)
}

func (m model) renderPromptOrStatus(w int) string {
	if m.mode == modeFilter || m.mode == modeInput {
		return trunc(m.input.View(), w)
	}
	if m.status == "" {
		return ""
	}
	if m.statusIsErr {
		return m.sty.dangerText().Render(trunc(m.status, w))
	}
	return m.sty.ok().Render(trunc(m.status, w))
}

// --- tree body ---

func (m model) renderBody(w, bodyH int) string {
	if !m.loaded {
		return lipgloss.Place(w, bodyH, lipgloss.Center, lipgloss.Center,
			m.sty.meta().Render("loading…"))
	}
	if len(m.rows) == 0 {
		hint := "No tmux sessions. Press n to create one."
		if m.filter != "" {
			hint = fmt.Sprintf("No matches for %q.", m.filter)
		}
		return lipgloss.Place(w, bodyH, lipgloss.Center, lipgloss.Center,
			m.sty.meta().Render(hint))
	}

	listW := w
	if m.showPreview() {
		listW = w * 3 / 5
	}

	end := m.offset + bodyH
	if end > len(m.rows) {
		end = len(m.rows)
	}
	lines := make([]string, 0, bodyH)
	for i := m.offset; i < end; i++ {
		lines = append(lines, m.renderRow(m.rows[i], listW, i == m.cursor))
	}
	for len(lines) < bodyH {
		lines = append(lines, "")
	}
	body := lipgloss.JoinVertical(lipgloss.Left, lines...)
	if m.showPreview() {
		body = lipgloss.JoinHorizontal(lipgloss.Top, body, m.renderPreview(w-listW, bodyH))
	}
	return body
}

func (m model) showPreview() bool {
	return m.previewOn && m.width >= previewMinWidth && len(m.rows) > 0
}

func (m model) renderRow(r row, w int, selected bool) string {
	indent := strings.Repeat("  ", r.depth)
	marker := "  "
	if r.kind != rowPane {
		if m.expanded[r.id] && m.filter == "" {
			marker = "▾ "
		} else {
			marker = "▸ "
		}
	}

	dot := "○"
	label := ""
	meta := ""
	switch r.kind {
	case rowSession:
		if r.session.Attached {
			dot = "●"
		}
		label = r.session.Name
		meta = fmt.Sprintf("%s · %s ago", plural(len(r.session.Windows), "window"), tmux.HumanAge(r.session.Activity))
	case rowWindow:
		if r.window.Active {
			dot = "●"
		}
		label = fmt.Sprintf("%d: %s", r.window.Index, r.window.Name)
		meta = plural(len(r.window.Panes), "pane")
	case rowPane:
		if r.pane.Active {
			dot = "●"
		}
		label = r.pane.CurrentCommand
		if label == "" {
			label = r.pane.ID
		}
		meta = shortPath(r.pane.CurrentPath)
	}

	prefix := indent + marker + dot + " "
	prefixW := lipgloss.Width(prefix)
	labelW := w - prefixW - lipgloss.Width(meta) - 2
	if labelW < 8 {
		// not enough room for metadata: drop it, give the space to the label
		meta = ""
		labelW = w - prefixW - 1
	}
	label = trunc(label, labelW)

	if selected {
		gap := w - prefixW - lipgloss.Width(label) - lipgloss.Width(meta)
		if gap < 1 {
			gap = 1
		}
		return m.sty.selected(w).Render(prefix + label + strings.Repeat(" ", gap) + meta)
	}

	dotStyled := m.sty.idleDot().Render(dot)
	switch r.kind {
	case rowSession:
		if r.session.Attached {
			dotStyled = m.sty.attachedDot().Render(dot)
		}
	case rowWindow, rowPane:
		if (r.kind == rowWindow && r.window.Active) || (r.kind == rowPane && r.pane.Active) {
			dotStyled = m.sty.activeDot().Render(dot)
		}
	}
	left := indent + marker + dotStyled + " " + label
	gap := w - lipgloss.Width(left) - lipgloss.Width(meta)
	if gap < 1 {
		gap = 1
	}
	return left + strings.Repeat(" ", gap) + m.sty.meta().Render(meta)
}

func shortPath(p string) string {
	home, err := os.UserHomeDir()
	if err == nil && home != "" && strings.HasPrefix(p, home) {
		return "~" + p[len(home):]
	}
	return p
}

// --- dialogs (replace the body area) ---

func (m model) renderHelp(w, bodyH int) string {
	raw := []string{
		"keys",
		"",
		"  ↑/k ↓/j   move",
		"  →/l       expand / enter first child",
		"  ←/h       collapse / go to parent",
		"  enter     attach to selected session/window/pane",
		"",
		"  n         new session / window / split pane",
		"  r         rename session/window",
		"  m         move window/pane",
		"  d         kill session/window/pane (confirms first)",
		"",
		"  /         filter",
		"  p         toggle pane preview (wide terminals)",
		"  ?         this help",
		"  q         quit",
	}
	lines := make([]string, 0, len(raw))
	for i, l := range raw {
		if i == 0 {
			lines = append(lines, m.sty.title().Render(l))
			continue
		}
		lines = append(lines, trunc(l, w-12))
	}
	box := m.sty.box().Render(lipgloss.JoinVertical(lipgloss.Left, lines...))
	return lipgloss.Place(w, bodyH, lipgloss.Center, lipgloss.Center, box)
}

func (m model) renderCreate(w, bodyH int) string {
	c := m.create
	if c == nil {
		return ""
	}
	lines := []string{m.sty.title().Render("create"), ""}
	for i, item := range c.items {
		lines = append(lines, m.pickLine(trunc(item.label, w-12), i == c.cursor))
	}
	box := m.sty.box().Render(lipgloss.JoinVertical(lipgloss.Left, lines...))
	return lipgloss.Place(w, bodyH, lipgloss.Center, lipgloss.Center, box)
}

func (m model) renderConfirm(w, bodyH int) string {
	c := m.confirm
	if c == nil {
		return ""
	}
	lines := make([]string, 0, len(c.lines)+2)
	for _, l := range c.lines {
		lines = append(lines, m.sty.dangerText().Render(trunc(l, w-10)))
	}
	lines = append(lines, "", m.sty.meta().Render("[y] confirm    [n/esc] cancel"))
	box := m.sty.dangerBox().Render(lipgloss.JoinVertical(lipgloss.Left, lines...))
	return lipgloss.Place(w, bodyH, lipgloss.Center, lipgloss.Center, box)
}

func (m model) renderMove(w, bodyH int) string {
	st := m.move
	if st == nil {
		return ""
	}
	lines := []string{m.sty.title().Render(trunc(st.title, w-12)), ""}
	for i, label := range st.labels {
		lines = append(lines, m.pickLine(trunc(label, w-12), i == st.cursor))
	}
	box := m.sty.box().Render(lipgloss.JoinVertical(lipgloss.Left, lines...))
	return lipgloss.Place(w, bodyH, lipgloss.Center, lipgloss.Center, box)
}

// pickLine renders one chooser/picker entry.
func (m model) pickLine(label string, selected bool) string {
	if selected {
		return lipgloss.NewStyle().Foreground(m.sty.accent).Bold(true).Render("▸ " + label)
	}
	return "  " + label
}
