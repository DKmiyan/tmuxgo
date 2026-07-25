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
	w, bodyH := m.renderSize()
	header := m.renderHeader(w)
	footer := m.renderFooter(w)
	middle := m.renderPromptOrStatus(w)

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

// renderSize returns the render width (clamped to a sane minimum) and the
// body height; rendering and mouse hit-testing share it.
func (m model) renderSize() (w, bodyH int) {
	return m.renderW(), m.bodyHeight()
}

// renderW is the clamped render width.
func (m model) renderW() int {
	w := m.width
	if w < 20 {
		w = 20
	}
	return w
}

// bodyHeight is the number of rows available for the body area (tree or
// dialog), between the header and the status line.
func (m model) bodyHeight() int {
	h := m.height - 2 - m.footerHeight()
	if h < 1 {
		return 1
	}
	return h
}

// listHeight is the number of tree rows visible inside the list panel.
func (m model) listHeight() int {
	h := m.bodyHeight() - 2
	if h < 1 {
		return 1
	}
	return h
}

// listW is the outer width of the list panel: the full width, minus the
// preview column when it shows.
func (m model) listW() int {
	w := m.renderW()
	if m.showPreview() {
		return w - previewW(w)
	}
	return w
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
		return m.renderFooterBar(w)
	}
	return trunc(m.sty.meta().Render(s), w)
}

// footerButton is one clickable normal-mode hint button: a mini-box
// spanning cell range [start, end) across three text rows (top border,
// label, bottom border) in footer row band `row`.
type footerButton struct {
	label      string // e.g. "n new"
	act        action
	row        int
	start, end int
}

// footerButtons lays out the normal-mode hint buttons left to right across
// width w, wrapping to a new band when the next button does not fit —
// buttons wrap, they are never dropped. An active filter prefix occupies
// the start of the first band's middle row. The layout is pure: render and
// mouse hit-testing use the same result.
func (m model) footerButtons(w int) []footerButton {
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
	x, row := 0, 0
	if m.filter != "" {
		x = lipgloss.Width(m.tr(i18n.FilterActive, m.filter, "")) + 1
	}
	btns := make([]footerButton, 0, len(actions))
	for _, a := range actions {
		k := "?"
		if keys := m.cfg.Keys[a.name]; len(keys) > 0 {
			k = keys[0]
		}
		label := k + " " + m.tr(a.label)
		bw := lipgloss.Width(label) + 2 // mini-box hugs the text: just the borders
		if x > 0 && x+bw > w {
			row++
			x = 0
		}
		btns = append(btns, footerButton{label: label, act: a.act, row: row, start: x, end: x + bw})
		x += bw + 1
	}
	return btns
}

// footerButtonRows is the number of button bands the footer needs at
// width w.
func (m model) footerButtonRows(w int) int {
	btns := m.footerButtons(w)
	return btns[len(btns)-1].row + 1
}

// footerHeight is the footer's height in rows: each button band takes
// three rows (top border, label, bottom border); every other mode shows a
// single hint line.
func (m model) footerHeight() int {
	if m.mode != modeNormal {
		return 1
	}
	h := m.footerButtonRows(m.renderW()) * 3
	if m.height > 0 && h > m.height-4 {
		h = (m.height - 4) / 3 * 3
		if h < 3 {
			h = 3
		}
	}
	return h
}

// footerButtonAt returns the index of the footer button at absolute screen
// point (x, y), or -1 when the point is between buttons or outside the
// footer. The whole mini-box (all three rows) is clickable.
func (m model) footerButtonAt(x, y int) int {
	if m.mode != modeNormal {
		return -1
	}
	top := m.height - m.footerHeight()
	if y < top || y >= m.height {
		return -1
	}
	band := (y - top) / 3
	for i, b := range m.footerButtons(m.renderW()) {
		if b.row == band && x >= b.start && x < b.end {
			return i
		}
	}
	return -1
}

// renderFooterBar renders the normal-mode hints as mini-boxed buttons (one
// band of boxes per layout row); the hovered button's interior fills with
// the theme accent.
func (m model) renderFooterBar(w int) string {
	btns := m.footerButtons(w)
	n := m.footerButtonRows(w)
	lines := make([]string, 0, n*3)
	for band := 0; band < n; band++ {
		var top, mid, bot string
		if band == 0 && m.filter != "" {
			mid = m.sty.meta().Render(m.tr(i18n.FilterActive, m.filter, ""))
		}
		for i, b := range btns {
			if b.row != band {
				continue
			}
			bw := b.end - b.start
			border := m.sty.meta().Render("╭" + strings.Repeat("─", bw-2) + "╮")
			top = overlayAt(top, b.start, border)
			var boxMid string
			if i == m.hoverFooter {
				boxMid = m.sty.meta().Render("│") + m.sty.buttonHover().Render(b.label) + m.sty.meta().Render("│")
			} else {
				boxMid = m.sty.meta().Render("│") + m.sty.button().Render(b.label) + m.sty.meta().Render("│")
			}
			mid = overlayAt(mid, b.start, boxMid)
			border = m.sty.meta().Render("╰" + strings.Repeat("─", bw-2) + "╯")
			bot = overlayAt(bot, b.start, border)
		}
		lines = append(lines, trunc(top, w), trunc(mid, w), trunc(bot, w))
	}
	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

// overlayAt returns line with seg placed at cell x, padding the gap with
// plain spaces (segments in one band never overlap).
func overlayAt(line string, x int, seg string) string {
	if pad := x - lipgloss.Width(line); pad > 0 {
		line += strings.Repeat(" ", pad)
	}
	return line + seg
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
	listW := m.listW()
	if !m.loaded || len(m.rows) == 0 {
		msg := m.tr(i18n.Loading)
		if m.loaded {
			msg = m.tr(i18n.NoSessions)
			if m.filter != "" {
				msg = m.tr(i18n.NoMatches, m.filter)
			}
		}
		inner := lipgloss.Place(listW-2, m.listHeight(), lipgloss.Center, lipgloss.Center,
			m.sty.meta().Render(msg))
		return m.sty.frame().Width(listW).Render(inner)
	}

	end := m.offset + m.listHeight()
	if end > len(m.rows) {
		end = len(m.rows)
	}
	lines := make([]string, 0, m.listHeight())
	for i := m.offset; i < end; i++ {
		drop := false
		if m.dragSource >= 0 && i == m.dragTarget {
			_, _, drop = m.dropTarget(m.dragSource, i)
		}
		lines = append(lines, m.renderRow(m.rows[i], listW-2, i == m.cursor, drop))
	}
	for len(lines) < m.listHeight() {
		lines = append(lines, "")
	}
	body := m.sty.frame().Width(listW).Height(bodyH).
		Render(lipgloss.JoinVertical(lipgloss.Left, lines...))
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

// settingsLines builds the settings box content; rendering and mouse
// hit-testing share it (setting i sits at line 2+i).
func (m model) settingsLines(w int) []string {
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
	return lines
}

func (m model) renderSettings(w, bodyH int) string {
	box := m.sty.box().Render(lipgloss.JoinVertical(lipgloss.Left, m.settingsLines(w)...))
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

// createLines builds the create-menu box content (item i at line 2+i).
func (m model) createLines(w int) []string {
	c := m.create
	if c == nil {
		return nil
	}
	lines := []string{m.sty.title().Render(m.tr(i18n.CreateTitle)), ""}
	for i, item := range c.items {
		lines = append(lines, m.pickLine(trunc(item.label, w-12), i == c.cursor))
	}
	return lines
}

func (m model) renderCreate(w, bodyH int) string {
	lines := m.createLines(w)
	if lines == nil {
		return ""
	}
	box := m.sty.box().Render(lipgloss.JoinVertical(lipgloss.Left, lines...))
	return lipgloss.Place(w, bodyH, lipgloss.Center, lipgloss.Center, box)
}

// confirmLines builds the confirm-box content; the clickable y/n hint is
// the last line.
func (m model) confirmLines(w int) []string {
	c := m.confirm
	if c == nil {
		return nil
	}
	lines := make([]string, 0, len(c.lines)+2)
	for _, l := range c.lines {
		lines = append(lines, m.sty.dangerText().Render(trunc(l, w-10)))
	}
	lines = append(lines, "", m.sty.meta().Render(m.tr(i18n.ConfirmHint)))
	return lines
}

func (m model) renderConfirm(w, bodyH int) string {
	lines := m.confirmLines(w)
	if lines == nil {
		return ""
	}
	box := m.sty.dangerBox().Render(lipgloss.JoinVertical(lipgloss.Left, lines...))
	return lipgloss.Place(w, bodyH, lipgloss.Center, lipgloss.Center, box)
}

// moveLines builds the move/template/socket picker box content (item i at
// line 2+i).
func (m model) moveLines(w int) []string {
	st := m.move
	if st == nil {
		return nil
	}
	lines := []string{m.sty.title().Render(trunc(st.title, w-12)), ""}
	for i, label := range st.labels {
		lines = append(lines, m.pickLine(trunc(label, w-12), i == st.cursor))
	}
	return lines
}

func (m model) renderMove(w, bodyH int) string {
	lines := m.moveLines(w)
	if lines == nil {
		return ""
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
		lines := []string{title, "", m.sty.meta().Render(m.tr(i18n.DirPickNone))}
		for len(lines) < bodyH {
			lines = append(lines, "")
		}
		return lipgloss.JoinVertical(lipgloss.Left, lines...)
	}
	vis := bodyH - 2
	if vis < 1 {
		vis = 1
	}
	offset := m.dirPickOffset()
	end := offset + vis
	if end > len(st.matches) {
		end = len(st.matches)
	}
	lines := []string{title, ""}
	for i := offset; i < end; i++ {
		lines = append(lines, m.pickLine(trunc(st.matches[i]+"/", w-4), i == st.cursor))
	}
	// pad to the full body height so the footer stays at the bottom
	for len(lines) < bodyH {
		lines = append(lines, "")
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
