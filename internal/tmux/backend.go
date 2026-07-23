package tmux

import "os/exec"

// Backend abstracts all tmux access so the UI never shells out directly.
// All targets are tmux internal IDs ($1, @2, %3), never names or indices.
type Backend interface {
	// Tree returns all sessions with nested windows and panes.
	// Sessions are sorted by recent activity (most recent first);
	// windows and panes are sorted by index. A missing tmux server
	// yields an empty tree and no error.
	Tree() ([]Session, error)

	NewSession(name string) error           // empty name lets tmux pick one
	NewWindow(sessionID, name string) error // empty name lets tmux pick one
	SplitPane(paneID string) error

	RenameSession(id, name string) error
	RenameWindow(id, name string) error

	MoveWindow(windowID, sessionID string) error
	MovePane(paneID, windowID string) error

	KillSession(id string) error
	KillWindow(id string) error
	KillPane(id string) error

	// CapturePane returns the visible pane content, trimmed and capped
	// at the last lines lines (<= 0 means the whole visible pane).
	CapturePane(paneID string, lines int) (string, error)

	// AttachCmd builds the command that takes the user to target:
	// switch-client inside tmux, attach-session outside. Stdio is left
	// for the caller (bubbletea's ExecProcess wires the terminal).
	AttachCmd(target string) *exec.Cmd
}
