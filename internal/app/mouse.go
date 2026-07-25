package app

import (
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/DKmiyan/tmuxgo/internal/i18n"
)

// Every keyboard flow has a mouse equivalent. The tree: click selects,
// double-click attaches, the marker toggles, drag & drop moves, the wheel
// scrolls, footer hints hover and click. Dialogs: hover preselects, click
// an item to confirm it, click outside the box to cancel (settings save),
// wheel moves the highlight, and the confirm dialog's "[y]/[n/esc]" hint
// zones are clickable buttons. Rendering and hit-testing share one layout
// per dialog (the *Lines builders plus the geometry below) so the two can
// never drift apart.

// --- centered box geometry ---

// boxGeometry maps a centered box dialog to absolute screen coordinates.
type boxGeometry struct {
	left, top     int // top-left border cell
	width, height int // cells, border included
}

// centeredBoxGeometry lays out a sty.box() dialog (1-cell border, 0/2
// padding) centered by lipgloss.Place in a w x bodyH area that begins one
// line below the header. lipgloss centers with floor-half gaps, and leaves
// the box at 0,0 when it does not fit.
func centeredBoxGeometry(w, bodyH int, lines []string) boxGeometry {
	cw := 0
	for _, l := range lines {
		if lw := lipgloss.Width(l); lw > cw {
			cw = lw
		}
	}
	g := boxGeometry{top: 1, width: cw + 6, height: len(lines) + 2}
	if gap := w - g.width; gap > 0 {
		g.left = gap / 2
	}
	if gap := bodyH - g.height; gap > 0 {
		g.top += gap / 2
	}
	return g
}

// inside reports whether the point is on the box (border included).
func (g boxGeometry) inside(x, y int) bool {
	return x >= g.left && x < g.left+g.width && y >= g.top && y < g.top+g.height
}

// lineAt returns the content line index at absolute row y, or -1 on the
// border rows and outside the box.
func (g boxGeometry) lineAt(y int) int {
	if y <= g.top || y >= g.top+g.height-1 {
		return -1
	}
	return y - g.top - 1
}

// colAt returns the content column at absolute x (0 at the first content
// cell, past the 1-cell border and 2-cell left padding).
func (g boxGeometry) colAt(x int) int {
	return x - g.left - 3
}

// pickItemAt returns the index of the list-dialog item at (x, y): items
// start at content line 2 (title, blank). -1 when the point is not on an
// item.
func (m model) pickItemAt(lines []string, x, y, n int) int {
	w, bodyH := m.renderSize()
	g := centeredBoxGeometry(w, bodyH, lines)
	if !g.inside(x, y) {
		return -1
	}
	if i := g.lineAt(y) - 2; i >= 0 && i < n {
		return i
	}
	return -1
}

// --- click ---

// handleMouseClick routes clicks by mode.
func (m model) handleMouseClick(ev tea.Mouse) (tea.Model, tea.Cmd) {
	switch m.mode {
	case modeNormal:
		return m.handleTreeClick(ev)
	case modeCreate:
		return m.handleCreateClick(ev)
	case modeMove:
		return m.handleMoveClick(ev)
	case modeConfirm:
		return m.handleConfirmClick(ev)
	case modeSettings:
		return m.handleSettingsClick(ev)
	case modeDirPick:
		return m.handleDirPickClick(ev)
	case modeHelp:
		m.mode = modeNormal // any click closes, like any key
		return m, nil
	}
	return m, nil
}

// handleTreeClick implements mouse selection in normal mode:
// click selects a row, click on an expand marker toggles it, and a quick
// second click on the same row attaches to it.
func (m model) handleTreeClick(ev tea.Mouse) (tea.Model, tea.Cmd) {
	if ev.Button != tea.MouseLeft {
		return m, nil
	}
	// the footer buttons are clickable, same as their key bindings
	if i := m.footerButtonAt(ev.X, ev.Y); i >= 0 {
		return m.dispatchAction(m.footerButtons(m.renderW())[i].act)
	}
	// rows live inside the list panel: header (1) + border (1) above, the
	// border column (1) on the left
	if ev.Y < 2 || ev.Y >= 2+m.listHeight() {
		return m, nil
	}
	if ev.X >= m.listW() {
		return m, nil // click landed on the list border or the preview
	}
	idx := m.offset + ev.Y - 2
	if idx < 0 || idx >= len(m.rows) {
		return m, nil
	}
	r := m.rows[idx]

	// expand/collapse marker zone (session/window rows only)
	if r.kind != rowPane && m.filter == "" && ev.X >= 1+2*r.depth && ev.X < 1+2*r.depth+2 {
		if m.expanded[r.id] {
			delete(m.expanded, r.id)
		} else {
			collapseSiblings(m.rows, r, m.expanded)
			m.expanded[r.id] = true
		}
		m.cursor = idx
		m.rebuild()
		return m, m.previewCmd()
	}

	doubleClick := idx == m.lastClickRow && time.Since(m.lastClickAt) < 500*time.Millisecond
	m.cursor = idx
	m.ensureVisible()
	m.lastClickRow = idx
	m.lastClickAt = time.Now()
	// window and pane rows are draggable; the drop is resolved on release
	if r.kind == rowWindow || r.kind == rowPane {
		m.dragSource = idx
	} else {
		m.dragSource = -1
	}
	m.dragTarget = -1
	if doubleClick {
		return m, m.attach(r.id)
	}
	return m, m.previewCmd()
}

// handleCreateClick: click an item to choose it, click outside to cancel.
func (m model) handleCreateClick(ev tea.Mouse) (tea.Model, tea.Cmd) {
	c := m.create
	if c == nil || ev.Button != tea.MouseLeft {
		return m, nil
	}
	w, bodyH := m.renderSize()
	g := centeredBoxGeometry(w, bodyH, m.createLines(w))
	if !g.inside(ev.X, ev.Y) {
		m.mode = modeNormal
		m.create = nil
		return m, nil
	}
	if i := g.lineAt(ev.Y) - 2; i >= 0 && i < len(c.items) {
		c.cursor = i
		return m.createConfirm()
	}
	return m, nil
}

// handleMoveClick covers the move, template and socket pickers: click a
// target to confirm it, click outside to cancel.
func (m model) handleMoveClick(ev tea.Mouse) (tea.Model, tea.Cmd) {
	st := m.move
	if st == nil || ev.Button != tea.MouseLeft {
		return m, nil
	}
	w, bodyH := m.renderSize()
	g := centeredBoxGeometry(w, bodyH, m.moveLines(w))
	if !g.inside(ev.X, ev.Y) {
		m.mode = modeNormal
		m.move = nil
		return m, nil
	}
	if i := g.lineAt(ev.Y) - 2; i >= 0 && i < len(st.labels) {
		st.cursor = i
		return m.moveConfirm()
	}
	return m, nil
}

// handleConfirmClick: the "[y] confirm" / "[n/esc] cancel" hint zones act
// as buttons; a click outside the box cancels (the safe direction).
func (m model) handleConfirmClick(ev tea.Mouse) (tea.Model, tea.Cmd) {
	c := m.confirm
	if c == nil || ev.Button != tea.MouseLeft {
		return m, nil
	}
	w, bodyH := m.renderSize()
	lines := m.confirmLines(w)
	g := centeredBoxGeometry(w, bodyH, lines)
	if !g.inside(ev.X, ev.Y) {
		return m.confirmCancel()
	}
	if g.lineAt(ev.Y) != len(lines)-1 {
		return m, nil // only the hint line's zones are clickable
	}
	switch confirmButton(m.tr(i18n.ConfirmHint), g.colAt(ev.X)) {
	case 'y':
		return m.confirmYes()
	case 'n':
		return m.confirmCancel()
	}
	return m, nil
}

// confirmButton hit-tests the "[y] confirm … [n/esc] cancel" hint line:
// the y-zone spans "[y]" up to "[n/esc]", the n-zone runs to the end.
func confirmButton(hint string, col int) rune {
	ni := strings.Index(hint, "[n/esc]")
	if ni < 0 {
		return 0
	}
	if col >= lipgloss.Width(hint[:ni]) {
		return 'n'
	}
	if yi := strings.Index(hint, "[y]"); yi >= 0 && col >= lipgloss.Width(hint[:yi]) {
		return 'y'
	}
	return 0
}

// handleSettingsClick: click a row to change it (like Enter), right-click
// to cycle theme/language backwards (like Left), click outside to save &
// close (like Esc).
func (m model) handleSettingsClick(ev tea.Mouse) (tea.Model, tea.Cmd) {
	if ev.Button != tea.MouseLeft && ev.Button != tea.MouseRight {
		return m, nil
	}
	w, bodyH := m.renderSize()
	g := centeredBoxGeometry(w, bodyH, m.settingsLines(w))
	if !g.inside(ev.X, ev.Y) {
		return m.closeSettings()
	}
	if i := g.lineAt(ev.Y) - 2; i >= 0 && i < 4 {
		m.settingsCursor = i
		if ev.Button == tea.MouseRight {
			m.settingsCycle(-1)
		} else {
			m.settingsChange()
		}
	}
	return m, nil
}

// handleDirPickClick: clicking a completion descends into it (Tab); a
// quick second click on the same row accepts the typed directory (Enter).
func (m model) handleDirPickClick(ev tea.Mouse) (tea.Model, tea.Cmd) {
	st := m.dirPick
	if st == nil || ev.Button != tea.MouseLeft {
		return m, nil
	}
	// completions render below the header, the title and a blank line
	i := ev.Y - 3 + m.dirPickOffset()
	if i == m.lastDirClickIdx && time.Since(m.lastDirClickAt) < 500*time.Millisecond {
		m.lastDirClickIdx = -1
		return m.dirPickAccept()
	}
	if ev.Y < 3 || i < 0 || i >= len(st.matches) {
		return m, nil
	}
	st.cursor = i
	m.lastDirClickIdx = i
	m.lastDirClickAt = time.Now()
	m.applyDirCompletion()
	return m, nil
}

// --- motion (hover & drag) ---

// handleMouseMotion tracks hovers (no button) and drags (left button):
// footer hints in normal mode, item preselection in the list dialogs, and
// the hovered drop row during a tree drag.
func (m model) handleMouseMotion(ev tea.Mouse) (tea.Model, tea.Cmd) {
	if ev.Button == tea.MouseNone {
		switch m.mode {
		case modeNormal:
			m.hoverFooter = m.footerButtonAt(ev.X, ev.Y)
		case modeCreate:
			if c := m.create; c != nil {
				if i := m.pickItemAt(m.createLines(m.renderW()), ev.X, ev.Y, len(c.items)); i >= 0 {
					c.cursor = i
				}
			}
		case modeMove:
			if st := m.move; st != nil {
				if i := m.pickItemAt(m.moveLines(m.renderW()), ev.X, ev.Y, len(st.labels)); i >= 0 {
					st.cursor = i
				}
			}
		case modeSettings:
			if i := m.pickItemAt(m.settingsLines(m.renderW()), ev.X, ev.Y, 4); i >= 0 {
				m.settingsCursor = i
			}
		case modeDirPick:
			if st := m.dirPick; st != nil {
				if i := ev.Y - 3 + m.dirPickOffset(); ev.Y >= 3 && i >= 0 && i < len(st.matches) {
					st.cursor = i
				}
			}
		}
		return m, nil
	}
	if m.mode != modeNormal || m.dragSource < 0 || ev.Button != tea.MouseLeft {
		return m, nil
	}
	idx := m.offset + ev.Y - 2
	if ev.Y < 2 || ev.Y >= 2+m.listHeight() || ev.X >= m.listW() || idx < 0 || idx >= len(m.rows) {
		idx = -1
	}
	m.dragTarget = idx
	if idx >= 0 {
		if _, label, ok := m.dropTarget(m.dragSource, idx); ok {
			m.setStatus(m.tr(i18n.ReleaseToMove, label), false)
		} else {
			m.setStatus(m.tr(i18n.InvalidDrop), true)
		}
	}
	return m, nil
}

// handleMouseRelease completes a drag & drop move.
func (m model) handleMouseRelease(ev tea.Mouse) (tea.Model, tea.Cmd) {
	if m.dragSource < 0 {
		return m, nil
	}
	src, dst := m.dragSource, m.dragTarget
	m.dragSource, m.dragTarget = -1, -1
	if run, label, ok := m.dropTarget(src, dst); ok {
		return m, m.runMutation(run, m.tr(i18n.MovedX, label))
	}
	return m, nil
}

// dropTarget validates dropping row src onto row dst: a window drops onto
// another session, a pane drops onto another window.
func (m model) dropTarget(srcIdx, dstIdx int) (func() error, string, bool) {
	if srcIdx < 0 || dstIdx < 0 || srcIdx >= len(m.rows) || dstIdx >= len(m.rows) {
		return nil, "", false
	}
	src, dst := m.rows[srcIdx], m.rows[dstIdx]
	switch {
	case src.kind == rowWindow && dst.kind == rowSession && dst.session.ID != src.session.ID:
		return func() error { return m.backend.MoveWindow(src.window.ID, dst.session.ID) },
			m.tr(i18n.MoveLabelWinSess, src.window.Name, dst.session.Name), true
	case src.kind == rowPane && dst.kind == rowWindow && dst.window.ID != src.window.ID:
		return func() error { return m.backend.MovePane(src.pane.ID, dst.window.ID) },
			m.tr(i18n.MoveLabelPaneWin, src.pane.CurrentCommand, dst.window.Name), true
	}
	return nil, "", false
}

// --- wheel ---

// handleMouseWheel scrolls the cursor in normal mode and moves the
// highlight in the pickers, settings and directory completion.
func (m model) handleMouseWheel(ev tea.Mouse) (tea.Model, tea.Cmd) {
	delta := 1
	if ev.Button == tea.MouseWheelUp {
		delta = -1
	}
	switch m.mode {
	case modeNormal:
		m.moveCursor(delta * 3)
		return m, m.previewCmd()
	case modeCreate:
		if c := m.create; c != nil && len(c.items) > 0 {
			c.cursor = clamp(c.cursor+delta, 0, len(c.items)-1)
		}
		return m, nil
	case modeMove:
		if st := m.move; st != nil && len(st.targets) > 0 {
			st.cursor = clamp(st.cursor+delta, 0, len(st.targets)-1)
		}
		return m, nil
	case modeDirPick:
		if st := m.dirPick; st != nil && len(st.matches) > 0 {
			st.cursor = clamp(st.cursor+delta, 0, len(st.matches)-1)
		}
		return m, nil
	case modeSettings:
		m.settingsCursor = clamp(m.settingsCursor+delta, 0, 3)
		return m, nil
	}
	return m, nil
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
