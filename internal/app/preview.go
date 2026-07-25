package app

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/DKmiyan/tmuxgo/internal/tmux"
)

// previewMinWidth is the terminal width below which the preview stays hidden.
const previewMinWidth = 100

// previewW is the preview column's outer width: two fifths of the terminal,
// clamped to [20, 56] so it stays a sidebar even on wide terminals.
func previewW(total int) int {
	w := total * 2 / 5
	if w < 20 {
		w = 20
	}
	if w > 56 {
		w = 56
	}
	return w
}

// previewWindow resolves which window the preview shows: the selected
// window, the selected pane's window, or the selected session's active
// window. The preview works at window granularity: every pane of it gets a
// section.
func (m model) previewWindow() *tmux.Window {
	r, ok := m.currentRow()
	if !ok {
		return nil
	}
	switch r.kind {
	case rowPane, rowWindow:
		return r.window
	case rowSession:
		for i := range r.session.Windows {
			if r.session.Windows[i].Active {
				return &r.session.Windows[i]
			}
		}
		if len(r.session.Windows) > 0 {
			return &r.session.Windows[0]
		}
	}
	return nil
}

// previewCmd fetches fresh content for every pane of the preview window.
func (m model) previewCmd() tea.Cmd {
	if !m.previewOn {
		return nil
	}
	win := m.previewWindow()
	if win == nil || len(win.Panes) == 0 {
		return nil
	}
	lines := m.bodyHeight() - 3
	if lines < 1 {
		lines = 1
	}
	b := m.backend
	ids := make([]string, 0, len(win.Panes))
	for i := range win.Panes {
		ids = append(ids, win.Panes[i].ID)
	}
	return func() tea.Msg {
		panes := make(map[string]string, len(ids))
		for _, id := range ids {
			content, err := b.CapturePane(id, lines)
			if err != nil {
				content = "(preview unavailable)"
			}
			panes[id] = content
		}
		return previewMsg{panes: panes}
	}
}

// renderPreview renders the bordered preview column, exactly w cells wide
// and bodyH rows tall (lipgloss Width/Height include the border). Every
// pane of the window gets a section separated by a labeled divider; the
// section height adapts to the pane count (terminals have one font size,
// so "smaller" means fewer lines per pane).
func (m model) renderPreview(w, bodyH int) string {
	if w < 10 {
		w = 10
	}
	innerW := w - 6 // border (2) + padding (4)
	innerH := bodyH - 3
	if innerH < 1 {
		innerH = 1
	}

	win := m.previewWindow()
	title := "preview"
	if win != nil {
		title = "preview " + win.ID
	}

	var lines []string
	switch {
	case win == nil || len(win.Panes) == 0:
		lines = append(lines, m.sty.meta().Render("(no pane)"))
	case len(win.Panes) == 1:
		lines = m.previewPaneLines(win.Panes[0].ID, innerW, innerH)
	default:
		// split the height fairly; dividers take one row each
		avail := innerH - (len(win.Panes) - 1)
		share := avail / len(win.Panes)
		if share < 1 {
			share = 1
		}
		for i := range win.Panes {
			p := &win.Panes[i]
			if i > 0 {
				lines = append(lines, m.previewDivider(p, innerW))
			}
			n := share
			if i == len(win.Panes)-1 {
				n = innerH - len(lines) // last pane takes what is left
			}
			lines = append(lines, m.previewPaneLines(p.ID, innerW, n)...)
			if len(lines) >= innerH {
				break
			}
		}
	}
	if len(lines) > innerH {
		lines = lines[:innerH]
	}
	body := m.sty.meta().Render(title) + "\n" + lipgloss.JoinVertical(lipgloss.Left, lines...)
	return m.sty.box().Width(w).Height(bodyH).Render(body)
}

// previewPaneLines returns up to n clipped content lines of one pane.
func (m model) previewPaneLines(id string, innerW, n int) []string {
	content, ok := m.previewCache[id]
	if !ok {
		content = "(loading…)"
	}
	var out []string
	for _, l := range strings.Split(content, "\n") {
		if len(out) >= n {
			break
		}
		out = append(out, trunc(l, innerW))
	}
	return out
}

// previewDivider renders the horizontal bar separating two pane sections,
// labeled with the next pane's id and command.
func (m model) previewDivider(p *tmux.Pane, innerW int) string {
	label := " " + p.ID + " " + p.CurrentCommand + " "
	rule := innerW - lipgloss.Width(label) - 1
	if rule < 1 {
		rule = 1
	}
	return m.sty.meta().Render("─" + trunc(label, innerW-1) + strings.Repeat("─", rule))
}
