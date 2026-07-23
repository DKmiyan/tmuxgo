package tmux

import "time"

// Session is a tmux session with its windows nested inside.
type Session struct {
	ID       string // tmux internal ID, e.g. "$0"
	Name     string
	Attached bool      // at least one client is attached
	Activity time.Time // last activity timestamp
	Windows  []Window
}

// Window is a tmux window with its panes nested inside.
type Window struct {
	ID        string // tmux internal ID, e.g. "@1"
	SessionID string // owning session ID
	Index     int
	Name      string
	Active    bool // current window of its session
	Panes     []Pane
}

// Pane is a tmux pane.
type Pane struct {
	ID             string // tmux internal ID, e.g. "%2"
	WindowID       string // owning window ID
	Index          int
	Active         bool // active pane of its window
	CurrentCommand string
	CurrentPath    string
}
