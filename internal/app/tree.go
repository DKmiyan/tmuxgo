package app

import (
	"strings"

	"github.com/DKmiyan/tmuxgo/internal/tmux"
)

type rowKind int

const (
	rowSession rowKind = iota
	rowWindow
	rowPane
)

// row is one visible line in the flattened tree. It points into the tree
// slice; rows are rebuilt whenever the tree or expansion changes.
type row struct {
	kind     rowKind
	id       string // tmux ID of this node
	depth    int
	parentID string // "" for sessions

	session *tmux.Session // set for every row (owning session)
	window  *tmux.Window  // set for window and pane rows
	pane    *tmux.Pane    // set for pane rows
}

// flatten builds visible rows from the tree honoring the expanded set.
func flatten(tree []tmux.Session, expanded map[string]bool) []row {
	var rows []row
	for i := range tree {
		s := &tree[i]
		rows = append(rows, row{kind: rowSession, id: s.ID, depth: 0, session: s})
		if !expanded[s.ID] {
			continue
		}
		for j := range s.Windows {
			w := &s.Windows[j]
			rows = append(rows, row{kind: rowWindow, id: w.ID, depth: 1, parentID: s.ID, session: s, window: w})
			if !expanded[w.ID] {
				continue
			}
			for k := range w.Panes {
				p := &w.Panes[k]
				rows = append(rows, row{kind: rowPane, id: p.ID, depth: 2, parentID: w.ID, session: s, window: w, pane: p})
			}
		}
	}
	return rows
}

// flattenFiltered builds rows matching the filter, ignoring the expanded
// set. Ancestors of a match are included for context even when they do not
// match themselves. A matching node includes its whole subtree.
func flattenFiltered(tree []tmux.Session, filter string) []row {
	f := strings.ToLower(filter)
	var rows []row
	for i := range tree {
		s := &tree[i]
		sMatch := strings.Contains(strings.ToLower(s.Name), f)
		var winRows []row
		for j := range s.Windows {
			w := &s.Windows[j]
			wMatch := sMatch || strings.Contains(strings.ToLower(w.Name), f)
			var paneRows []row
			for k := range w.Panes {
				p := &w.Panes[k]
				if wMatch || paneMatches(p, f) {
					paneRows = append(paneRows, row{kind: rowPane, id: p.ID, depth: 2, parentID: w.ID, session: s, window: w, pane: p})
				}
			}
			if wMatch || len(paneRows) > 0 {
				winRows = append(winRows, row{kind: rowWindow, id: w.ID, depth: 1, parentID: s.ID, session: s, window: w})
				winRows = append(winRows, paneRows...)
			}
		}
		if sMatch || len(winRows) > 0 {
			rows = append(rows, row{kind: rowSession, id: s.ID, depth: 0, session: s})
			rows = append(rows, winRows...)
		}
	}
	return rows
}

func paneMatches(p *tmux.Pane, lowerFilter string) bool {
	return strings.Contains(strings.ToLower(p.CurrentCommand), lowerFilter) ||
		strings.Contains(strings.ToLower(p.CurrentPath), lowerFilter)
}

// indexOfRow returns the index of the row with the given tmux ID, or -1.
func indexOfRow(rows []row, id string) int {
	if id == "" {
		return -1
	}
	for i, r := range rows {
		if r.id == id {
			return i
		}
	}
	return -1
}

// collapseSiblings removes every node sharing r's parent and kind from the
// expanded set, keeping only the selected branch open.
func collapseSiblings(rows []row, r row, expanded map[string]bool) {
	for _, other := range rows {
		if other.kind == r.kind && other.parentID == r.parentID && other.id != r.id {
			delete(expanded, other.id)
		}
	}
}

// activePaneID returns the active pane of a window (first pane as fallback).
func activePaneID(w *tmux.Window) string {
	for _, p := range w.Panes {
		if p.Active {
			return p.ID
		}
	}
	if len(w.Panes) > 0 {
		return w.Panes[0].ID
	}
	return ""
}
