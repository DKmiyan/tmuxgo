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
	if m.mouseEnabled {
		v.MouseMode = tea.MouseModeCellMotion
	}
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
	case modeSettings:
		body = m.renderSettings(w, bodyH)
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
	case modeCreate:
		s = "↑/↓ choose · enter confirm · esc cancel"
	case modeMove:
		s = "↑/↓ choose · enter confirm · esc cancel"
		if m.move != nil && m.move.isTemplate {
			s = "↑/↓ choose · enter create · d delete · r rename · esc cancel"
		}
	case modeConfirm:
		s = "y confirm · n cancel"
	case modeHelp:
		s = "any key to close"
	case modeSettings:
		s = "↑/↓ move · enter change · esc save & close"
	default:
		s = m.defaultFooterHints()
		if m.filter != "" {
			s = fmt.Sprintf("filter: %q (esc clears) · %s", m.filter, s)
		}
	}
	return trunc(m.sty.meta().Render(s), w)
}

// defaultFooterHints builds the normal-mode hint line from the configured
// key bindings (first binding of each action).
func (m model) defaultFooterHints() string {
	actions := []struct{ name, label string }{
		{"new", "new"}, {"rename", "rename"}, {"move", "move"}, {"kill", "kill"},
		{"filter", "filter"}, {"preview", "preview"}, {"help", "help"},
		{"settings", "settings"}, {"quit", "quit"},
	}
	parts := make([]string, 0, len(actions))
	for _, a := range actions {
		k := "?"
		if keys := m.cfg.Keys[a.name]; len(keys) > 0 {
			k = keys[0]
		}
		parts = append(parts, k+" "+a.label)
	}
	return strings.Join(parts, " · ")
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
		drop := false
		if m.dragSource >= 0 && i == m.dragTarget {
			_, _, drop = m.dropTarget(m.dragSource, i)
		}
		lines = append(lines, m.renderRow(m.rows[i], listW, i == m.cursor, drop))
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

func (m model) renderRow(r row, w int, selected, dropHint bool) string {
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

	if selected || dropHint {
		gap := w - prefixW - lipgloss.Width(label) - lipgloss.Width(meta)
		if gap < 1 {
			gap = 1
		}
		if dropHint && !selected {
			return m.sty.dropTarget(w).Render(prefix + label + strings.Repeat(" ", gap) + meta)
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
	raw := []string{"keys", ""}
	for _, e := range helpEntries {
		raw = append(raw, fmt.Sprintf("  %-10s %s", strings.Join(m.cfg.Keys[e.name], "/"), e.desc))
	}
	raw = append(raw, "",
		"  mouse      click select, marker click expands,",
		"             double-click attaches, wheel scrolls,",
		"             drag window/pane to move it",
		"  esc        clear filter",
	)
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

// helpEntries lists bindable actions in help order.
var helpEntries = []struct{ name, desc string }{
	{"up", "move up"},
	{"down", "move down"},
	{"expand", "expand / enter first child"},
	{"collapse", "collapse / go to parent"},
	{"attach", "attach to selected session/window/pane"},
	{"new", "new session / window / split pane / from template"},
	{"rename", "rename session/window"},
	{"move", "move window/pane"},
	{"kill", "kill session/window/pane (confirms first)"},
	{"filter", "filter"},
	{"preview", "toggle pane preview (wide terminals)"},
	{"settings", "settings"},
	{"help", "this help"},
	{"quit", "quit"},
}

func (m model) renderSettings(w, bodyH int) string {
	values := []struct{ label, value string }{
		{"theme", m.cfg.Theme},
		{"preview on start", onOff(m.cfg.PreviewDefault)},
		{"mouse", onOff(m.cfg.Mouse)},
	}
	lines := []string{m.sty.title().Render("settings"), ""}
	for i, v := range values {
		line := fmt.Sprintf("%-18s %s", v.label, v.value)
		lines = append(lines, m.pickLine(line, i == m.settingsCursor))
	}
	lines = append(lines, "",
		m.sty.meta().Render("keys are configurable in:"),
		m.sty.meta().Render(trunc(m.cfgPath, w-12)))
	box := m.sty.box().Render(lipgloss.JoinVertical(lipgloss.Left, lines...))
	return lipgloss.Place(w, bodyH, lipgloss.Center, lipgloss.Center, box)
}

func onOff(b bool) string {
	if b {
		return "on"
	}
	return "off"
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
