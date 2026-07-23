// Package template captures and recreates tmux session layouts:
// window names, split structure (via tmux layout strings), and pane
// working directories. Templates are stored as JSON.
package template

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/DKmiyan/tmuxgo/internal/tmux"
)

// Pane is a pane's restorable state.
type Pane struct {
	Dir string `json:"dir,omitempty"`
}

// Window is a window's restorable state.
type Window struct {
	Name   string `json:"name"`
	Layout string `json:"layout,omitempty"`
	Panes  []Pane `json:"panes,omitempty"`
}

// Template describes how to recreate a session's structure.
type Template struct {
	Name    string   `json:"name"`
	Windows []Window `json:"windows"`
}

// Capture builds a Template from a live session.
func Capture(name string, s tmux.Session) Template {
	t := Template{Name: name}
	for _, w := range s.Windows {
		tw := Window{Name: w.Name, Layout: w.Layout}
		for _, p := range w.Panes {
			tw.Panes = append(tw.Panes, Pane{Dir: p.CurrentPath})
		}
		t.Windows = append(t.Windows, tw)
	}
	return t
}

func dirOf(w Window, i int) string {
	if i >= 0 && i < len(w.Panes) {
		return w.Panes[i].Dir
	}
	return ""
}

// Create builds a live session from the template and returns its tmux ID.
// Processes are not restored; only structure, names, and directories.
func (t Template) Create(b tmux.Backend) (string, error) {
	if len(t.Windows) == 0 {
		return "", errors.New("template has no windows")
	}
	sessID, err := b.NewSessionID(t.Name, dirOf(t.Windows[0], 0))
	if err != nil {
		return "", err
	}
	for i, w := range t.Windows {
		winID, err := ensureWindow(b, sessID, i, w)
		if err != nil {
			return sessID, err
		}
		if len(w.Panes) > 1 {
			paneID, err := firstPaneID(b, winID)
			if err != nil {
				return sessID, err
			}
			for j := 1; j < len(w.Panes); j++ {
				if err := b.SplitPane(paneID, w.Panes[j].Dir); err != nil {
					return sessID, err
				}
			}
		}
		if w.Layout != "" {
			if err := b.SelectLayout(winID, w.Layout); err != nil {
				return sessID, err
			}
		}
	}
	return sessID, nil
}

// ensureWindow returns the ID of the session's window at position i,
// creating and naming it from the template when needed.
func ensureWindow(b tmux.Backend, sessID string, i int, w Window) (string, error) {
	if i > 0 {
		if err := b.NewWindow(sessID, "", dirOf(w, 0)); err != nil {
			return "", err
		}
	}
	tree, err := b.Tree()
	if err != nil {
		return "", err
	}
	for _, s := range tree {
		if s.ID != sessID {
			continue
		}
		// windows come back sorted by index; position i matches creation order
		if i >= len(s.Windows) {
			return "", fmt.Errorf("window %d not found in new session", i)
		}
		win := s.Windows[i]
		if win.Name != w.Name {
			if err := b.RenameWindow(win.ID, w.Name); err != nil {
				return "", err
			}
		}
		return win.ID, nil
	}
	return "", fmt.Errorf("new session %s not found", sessID)
}

func firstPaneID(b tmux.Backend, winID string) (string, error) {
	tree, err := b.Tree()
	if err != nil {
		return "", err
	}
	for _, s := range tree {
		for _, w := range s.Windows {
			if w.ID == winID && len(w.Panes) > 0 {
				return w.Panes[0].ID, nil
			}
		}
	}
	return "", fmt.Errorf("window %s not found", winID)
}

// Store persists templates as one JSON array in a file.
type Store struct {
	path string
}

// NewStore returns a Store backed by the given file.
func NewStore(path string) *Store {
	return &Store{path: path}
}

// DefaultStore stores templates at ~/.config/tmuxgo/templates.json.
func DefaultStore() (*Store, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	return NewStore(filepath.Join(home, ".config", "tmuxgo", "templates.json")), nil
}

// Path returns the backing file path.
func (s *Store) Path() string { return s.path }

// List returns all stored templates (missing file means none).
func (s *Store) List() ([]Template, error) {
	raw, err := os.ReadFile(s.path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var ts []Template
	if err := json.Unmarshal(raw, &ts); err != nil {
		return nil, fmt.Errorf("parse %s: %w", s.path, err)
	}
	return ts, nil
}

// Get returns the named template.
func (s *Store) Get(name string) (Template, error) {
	ts, err := s.List()
	if err != nil {
		return Template{}, err
	}
	for _, t := range ts {
		if t.Name == name {
			return t, nil
		}
	}
	return Template{}, fmt.Errorf("template %q not found", name)
}

// Save inserts or replaces the template with the same name.
func (s *Store) Save(t Template) error {
	if t.Name == "" {
		return errors.New("template name cannot be empty")
	}
	ts, err := s.List()
	if err != nil {
		return err
	}
	replaced := false
	for i := range ts {
		if ts[i].Name == t.Name {
			ts[i] = t
			replaced = true
		}
	}
	if !replaced {
		ts = append(ts, t)
	}
	return s.write(ts)
}

// Delete removes the named template.
func (s *Store) Delete(name string) error {
	ts, err := s.List()
	if err != nil {
		return err
	}
	out := ts[:0]
	found := false
	for _, t := range ts {
		if t.Name == name {
			found = true
			continue
		}
		out = append(out, t)
	}
	if !found {
		return fmt.Errorf("template %q not found", name)
	}
	return s.write(out)
}

func (s *Store) write(ts []Template) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(ts, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, raw, 0o644)
}
