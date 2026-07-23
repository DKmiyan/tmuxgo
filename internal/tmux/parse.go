package tmux

import (
	"sort"
	"strconv"
	"strings"
	"time"
)

// fieldSep separates tmux format fields. It is a TAB: control characters
// such as 0x1F get vis-escaped by older tmux servers (seen on tmux 3.4),
// while TAB passes through raw on every tested version. Names containing
// tabs are pathological and simply degrade the displayed value.
const fieldSep = "\t"

var sessionFormat = strings.Join([]string{
	"#{session_id}",
	"#{session_name}",
	"#{session_attached}",
	"#{session_activity}",
}, fieldSep)

var windowFormat = strings.Join([]string{
	"#{window_id}",
	"#{session_id}",
	"#{window_index}",
	"#{window_name}",
	"#{window_active}",
}, fieldSep)

var paneFormat = strings.Join([]string{
	"#{pane_id}",
	"#{window_id}",
	"#{pane_index}",
	"#{pane_active}",
	"#{pane_current_command}",
	"#{pane_current_path}",
}, fieldSep)

// buildTree joins the three list-* outputs into a sorted session tree.
func buildTree(sessionsOut, windowsOut, panesOut string) ([]Session, error) {
	panesByWindow := make(map[string][]Pane)
	for _, line := range splitLines(panesOut) {
		p, err := parsePane(line)
		if err != nil {
			return nil, err
		}
		panesByWindow[p.WindowID] = append(panesByWindow[p.WindowID], p)
	}

	windowsBySession := make(map[string][]Window)
	for _, line := range splitLines(windowsOut) {
		w, err := parseWindow(line)
		if err != nil {
			return nil, err
		}
		w.Panes = panesByWindow[w.ID]
		sort.Slice(w.Panes, func(i, j int) bool { return w.Panes[i].Index < w.Panes[j].Index })
		windowsBySession[w.SessionID] = append(windowsBySession[w.SessionID], w)
	}

	var sessions []Session
	for _, line := range splitLines(sessionsOut) {
		s, err := parseSession(line)
		if err != nil {
			return nil, err
		}
		s.Windows = windowsBySession[s.ID]
		sort.Slice(s.Windows, func(i, j int) bool { return s.Windows[i].Index < s.Windows[j].Index })
		sessions = append(sessions, s)
	}
	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].Activity.After(sessions[j].Activity)
	})
	return sessions, nil
}

func splitLines(out string) []string {
	out = strings.TrimRight(out, "\n")
	if out == "" {
		return nil
	}
	return strings.Split(out, "\n")
}

func parseSession(line string) (Session, error) {
	f := strings.Split(line, fieldSep)
	if len(f) != 4 {
		return Session{}, errParse("session", line)
	}
	activity, _ := strconv.ParseInt(f[3], 10, 64)
	return Session{
		ID:       f[0],
		Name:     f[1],
		Attached: f[2] != "0",
		Activity: time.Unix(activity, 0),
	}, nil
}

func parseWindow(line string) (Window, error) {
	f := strings.Split(line, fieldSep)
	if len(f) != 5 {
		return Window{}, errParse("window", line)
	}
	index, _ := strconv.Atoi(f[2])
	return Window{
		ID:        f[0],
		SessionID: f[1],
		Index:     index,
		Name:      f[3],
		Active:    f[4] == "1",
	}, nil
}

func parsePane(line string) (Pane, error) {
	f := strings.Split(line, fieldSep)
	if len(f) != 6 {
		return Pane{}, errParse("pane", line)
	}
	index, _ := strconv.Atoi(f[2])
	return Pane{
		ID:             f[0],
		WindowID:       f[1],
		Index:          index,
		Active:         f[3] == "1",
		CurrentCommand: f[4],
		CurrentPath:    f[5],
	}, nil
}

func errParse(kind, line string) error {
	return &ParseError{Kind: kind, Line: line}
}

// ParseError means tmux returned a record that did not match the query format.
type ParseError struct {
	Kind string
	Line string
}

func (e *ParseError) Error() string {
	return "tmux: cannot parse " + e.Kind + " record: " + strconv.Quote(e.Line)
}
