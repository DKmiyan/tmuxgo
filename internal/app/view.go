package app

import (
	"fmt"
	"os"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/DKmiyan/tmuxgo/internal/i18n"
	"github.com/DKmiyan/tmuxgo/internal/tmux"
)

// View implements tea.Model.
func (m model) View() tea.View {
	v := tea.NewView(m.render())
	v.AltScreen = true
	v.WindowTitle = "tmuxgo"
	if m.mouseEnabled {
		v.MouseMode = tea.MouseModeAllMotion
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
	case modeDirPick:
		body = m.renderDirPick(w, bodyH)
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
		m.sty.meta().Render(fmt.Sprintf("  %s · %s", m.plural(len(m.tree), i18n.UnitSession), m.plural(nWindows, i18n.UnitWindow)))
	if m.socket != "" {
		left += m.sty.meta().Render("  ·  socket: " + m.socket)
	}
	return trunc(left, w)
}

func (m model) renderFooter(w int) string {
	var s string
	switch m.mode {
	case modeFilter, modeInput:
		s = m.tr(i18n.FooterApplyCancel)
	case modeCreate:
		s = m.tr(i18n.FooterChooseConfirm)
	case modeMove:
		s = m.tr(i18n.FooterChooseConfirm)
		if m.move != nil && m.move.isTemplate {
			s = m.tr(i18n.FooterTemplate)
		}
	case modeConfirm:
		s = m.tr(i18n.FooterConfirmYN)
	case modeDirPick:
		s = m.tr(i18n.FooterDirPick)
	case modeHelp:
		s = m.tr(i18n.FooterHelpClose)
	case modeSettings:
		s = m.tr(i18n.FooterSettings)
	default:
		return m.renderFooterHints(w)
	}
	return trunc(m.sty.meta().Render(s), w)
}

// footerSegment is one clickable normal-mode hint with its cell range
// [start, end) in the footer line.
type footerSegment struct {
	label      string // e.g. "n new"
	act        action
	start, end int
}

// footerSegments lays out the normal-mode hints left to right, dropping
// trailing segments that do not fit width w. An active filter prefix
// shifts the segments right. The layout is pure: render and mouse
// hit-testing use the same result.
func (m model) footerSegments(w int) []footerSegment {
	actions := []struct {
		name  string
		label i18n.ID
		act   action
	}{
		{"new", i18n.ActNew, actNew},
		{"rename", i18n.ActRename, actRename},
		{"move", i18n.ActMove, actMove},
		{"kill", i18n.ActKill, actKill},
		{"filter", i18n.ActFilter, actFilter},
		{"preview", i18n.ActPreview, actPreview},
		{"help", i18n.ActHelp, actHelp},
		{"settings", i18n.ActSettings, actSettings},
		{"quit", i18n.ActQuit, actQuit},
	}
	x := 0
	if m.filter != "" {
		x = lipgloss.Width(m.tr(i18n.FilterActive, m.filter, ""))
	}
	segs := make([]footerSegment, 0, len(actions))
	for i, a := range actions {
		k := "?"
		if keys := m.cfg.Keys[a.name]; len(keys) > 0 {
			k = keys[0]
		}
		label := k + " " + m.tr(a.label)
		segW := lipgloss.Width(label)
		need := segW
		if i > 0 {
			need += 3 // " · " separator
		}
		if x+need > w {
			break
		}
		if i > 0 {
			x += 3
		}
		segs = append(segs, footerSegment{label: label, act: a.act, start: x, end: x + segW})
		x += segW
	}
	return segs
}

// footerHit returns the index of the footer segment containing cell x, or
// -1 when x is on a separator, the filter prefix, or empty space.
func (m model) footerHit(x, w int) int {
	for i, s := range m.footerSegments(w) {
		if x >= s.start && x < s.end {
			return i
		}
	}
	return -1
}

// renderFooterHints renders the normal-mode hints with the hovered segment
// highlighted (mouse hover).
func (m model) renderFooterHints(w int) string {
	var b strings.Builder
	if m.filter != "" {
		b.WriteString(m.sty.meta().Render(m.tr(i18n.FilterActive, m.filter, "")))
	}
	for i, s := range m.footerSegments(w) {
		if i > 0 {
			b.WriteString(m.sty.meta().Render(" · "))
		}
		if i == m.hoverFooter {
			b.WriteString(lipgloss.NewStyle().Foreground(m.sty.accent).Bold(true).Render(s.label))
		} else {
			b.WriteString(m.sty.meta().Render(s.label))
		}
	}
	return trunc(b.String(), w)
}

func (m model) renderPromptOrStatus(w int) string {
	if m.mode == modeFilter || m.mode == modeInput || m.mode == modeDirPick {
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
			m.sty.meta().Render(m.tr(i18n.Loading)))
	}
	if len(m.rows) == 0 {
		hint := m.tr(i18n.NoSessions)
		if m.filter != "" {
			hint = m.tr(i18n.NoMatches, m.filter)
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
		meta = m.tr(i18n.SessionMeta, m.plural(len(r.session.Windows), i18n.UnitWindow), tmux.HumanAge(r.session.Activity))
	case rowWindow:
		if r.window.Active {
			dot = "●"
		}
		label = fmt.Sprintf("%d: %s", r.window.Index, r.window.Name)
		meta = m.plural(len(r.window.Panes), i18n.UnitPane)
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
	raw := []string{m.tr(i18n.HelpTitle), ""}
	for _, e := range helpEntries {
		raw = append(raw, fmt.Sprintf("  %-10s %s", strings.Join(m.cfg.Keys[e.name], "/"), m.tr(e.desc)))
	}
	raw = append(raw, "",
		fmt.Sprintf("  %-10s %s", "mouse", m.tr(i18n.HelpMouse1)),
		"             "+m.tr(i18n.HelpMouse2),
		"             "+m.tr(i18n.HelpMouse3),
		fmt.Sprintf("  %-10s %s", "esc", m.tr(i18n.HelpEscClear)),
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
var helpEntries = []struct {
	name string
	desc i18n.ID
}{
	{"up", i18n.HelpUp},
	{"down", i18n.HelpDown},
	{"expand", i18n.HelpExpand},
	{"collapse", i18n.HelpCollapse},
	{"attach", i18n.HelpAttach},
	{"new", i18n.HelpNew},
	{"rename", i18n.HelpRename},
	{"move", i18n.HelpMove},
	{"kill", i18n.HelpKill},
	{"filter", i18n.HelpFilter},
	{"preview", i18n.HelpPreview},
	{"settings", i18n.HelpSettings},
	{"socket", i18n.HelpSocket},
	{"help", i18n.HelpHelp},
	{"quit", i18n.HelpQuit},
}

func (m model) renderSettings(w, bodyH int) string {
	values := []struct{ label, value string }{
		{m.tr(i18n.SetTheme), m.cfg.Theme},
		{m.tr(i18n.SetLanguage), m.cfg.Language},
		{m.tr(i18n.SetPreviewStart), m.onOff(m.cfg.PreviewDefault)},
		{m.tr(i18n.SetMouse), m.onOff(m.cfg.Mouse)},
	}
	lines := []string{m.sty.title().Render(m.tr(i18n.SettingsTitle)), ""}
	for i, v := range values {
		lines = append(lines, m.pickLine(padCells(v.label, 18)+v.value, i == m.settingsCursor))
	}
	lines = append(lines, "",
		m.sty.meta().Render(m.tr(i18n.SettingsHint)),
		m.sty.meta().Render(m.tr(i18n.SettingsKeysIn)),
		m.sty.meta().Render(trunc(m.cfgPath, w-12)))
	box := m.sty.box().Render(lipgloss.JoinVertical(lipgloss.Left, lines...))
	return lipgloss.Place(w, bodyH, lipgloss.Center, lipgloss.Center, box)
}

// padCells right-pads s with spaces to w display cells (CJK-aware).
func padCells(s string, w int) string {
	pad := w - lipgloss.Width(s)
	if pad < 1 {
		pad = 1
	}
	return s + strings.Repeat(" ", pad)
}

func (m model) onOff(b bool) string {
	if b {
		return m.tr(i18n.On)
	}
	return m.tr(i18n.Off)
}

func (m model) renderCreate(w, bodyH int) string {
	c := m.create
	if c == nil {
		return ""
	}
	lines := []string{m.sty.title().Render(m.tr(i18n.CreateTitle)), ""}
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
	lines = append(lines, "", m.sty.meta().Render(m.tr(i18n.ConfirmHint)))
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

func (m model) renderDirPick(w, bodyH int) string {
	st := m.dirPick
	if st == nil {
		return ""
	}
	title := m.sty.title().Render(m.tr(i18n.DirPickTitle)) +
		m.sty.meta().Render(m.tr(i18n.DirPickHint))
	if len(st.matches) == 0 {
		return lipgloss.JoinVertical(lipgloss.Left, title, "",
			m.sty.meta().Render(m.tr(i18n.DirPickNone)))
	}
	vis := bodyH - 2
	if vis < 1 {
		vis = 1
	}
	offset := 0
	if st.cursor >= vis {
		offset = st.cursor - vis + 1
	}
	end := offset + vis
	if end > len(st.matches) {
		end = len(st.matches)
	}
	lines := []string{title, ""}
	for i := offset; i < end; i++ {
		lines = append(lines, m.pickLine(trunc(st.matches[i]+"/", w-4), i == st.cursor))
	}
	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

// pickLine renders one chooser/picker entry.
func (m model) pickLine(label string, selected bool) string {
	if selected {
		return lipgloss.NewStyle().Foreground(m.sty.accent).Bold(true).Render("▸ " + label)
	}
	return "  " + label
}
