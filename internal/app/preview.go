package app

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// previewMinWidth is the terminal width below which the preview stays hidden.
const previewMinWidth = 100

// previewTarget resolves which pane to preview for the current selection:
// the pane itself, or the active pane of the selected window/session.
func (m model) previewTarget() string {
	r, ok := m.currentRow()
	if !ok {
		return ""
	}
	switch r.kind {
	case rowPane:
		return r.pane.ID
	case rowWindow:
		return activePaneID(r.window)
	case rowSession:
		for i := range r.session.Windows {
			if r.session.Windows[i].Active {
				if id := activePaneID(&r.session.Windows[i]); id != "" {
					return id
				}
			}
		}
		for i := range r.session.Windows {
			if id := activePaneID(&r.session.Windows[i]); id != "" {
				return id
			}
		}
	}
	return ""
}

// previewCmd fetches fresh pane content for the current preview target.
func (m model) previewCmd() tea.Cmd {
	if !m.previewOn {
		return nil
	}
	id := m.previewTarget()
	if id == "" {
		return nil
	}
	lines := m.bodyHeight() - 2
	if lines < 1 {
		lines = 1
	}
	b := m.backend
	return func() tea.Msg {
		content, err := b.CapturePane(id, lines)
		if err != nil {
			content = "(preview unavailable)"
		}
		return previewMsg{id: id, content: content}
	}
}

// renderPreview renders the bordered preview column, exactly w cells wide
// and bodyH rows tall.
func (m model) renderPreview(w, bodyH int) string {
	if w < 10 {
		w = 10
	}
	id := m.previewTarget()
	title := "preview"
	if id != "" {
		title = "preview " + id
	}
	content := "(loading…)"
	if c, ok := m.previewCache[id]; ok && id != "" {
		content = c
	}

	innerW := w - 6 // border (2) + padding (4)
	innerH := bodyH - 4
	if innerH < 1 {
		innerH = 1
	}
	var lines []string
	for _, l := range strings.Split(content, "\n") {
		if len(lines) >= innerH {
			break
		}
		lines = append(lines, trunc(l, innerW))
	}
	body := m.sty.meta().Render(title) + "\n" + lipgloss.JoinVertical(lipgloss.Left, lines...)
	return m.sty.box().Width(w - 2).Height(bodyH - 2).Render(body)
}
