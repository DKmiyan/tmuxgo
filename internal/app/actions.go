package app

import (
	"fmt"

	tea "charm.land/bubbletea/v2"
)

// --- create ---

type createKind int

const (
	createSession createKind = iota
	createWindow
	createSplit
	createFromTemplate
)

type createItem struct {
	label    string
	kind     createKind
	targetID string // session ID for createWindow, pane ID for createSplit
}

type createState struct {
	items  []createItem
	cursor int
}

func (m model) startCreate() (tea.Model, tea.Cmd) {
	items := []createItem{{label: "New session", kind: createSession}}
	if r, ok := m.currentRow(); ok {
		items = append(items, createItem{
			label:    fmt.Sprintf("New window in '%s'", r.session.Name),
			kind:     createWindow,
			targetID: r.session.ID,
		})
		if r.kind != rowSession {
			paneID := activePaneID(r.window)
			if r.kind == rowPane {
				paneID = r.pane.ID
			}
			if paneID != "" {
				items = append(items, createItem{
					label:    fmt.Sprintf("Split pane in '%s'", r.window.Name),
					kind:     createSplit,
					targetID: paneID,
				})
			}
		}
	}
	// default to the most contextual (last) option
	def := len(items) - 1
	if m.templates != nil {
		if ts, err := m.templates.List(); err == nil && len(ts) > 0 {
			items = append(items, createItem{label: "New session from template…", kind: createFromTemplate})
		}
	}
	m.create = &createState{items: items, cursor: def}
	m.mode = modeCreate
	return m, nil
}

// --- rename ---

func (m model) startRename() (tea.Model, tea.Cmd) {
	r, ok := m.currentRow()
	if !ok {
		return m, nil
	}
	switch r.kind {
	case rowPane:
		m.setStatus("cannot rename a pane", true)
		return m, nil
	case rowSession:
		m.inputPurpose = inputRenameSession
		m.inputTarget = r.session.ID
		m.input.Reset()
		m.input.SetValue(r.session.Name)
		m.input.Prompt = "rename session: "
	case rowWindow:
		m.inputPurpose = inputRenameWindow
		m.inputTarget = r.window.ID
		m.input.Reset()
		m.input.SetValue(r.window.Name)
		m.input.Prompt = "rename window: "
	}
	m.input.CursorEnd()
	m.mode = modeInput
	return m, m.input.Focus()
}

// --- move ---

type moveNewKind int

const (
	newNone             moveNewKind = iota
	newWindowForPane                // first picker item breaks the pane into a new window
	newSessionForWindow             // first picker item creates a session for the window
)

type moveState struct {
	title      string
	labels     []string
	targets    []string
	cursor     int
	sourceID   string
	isWindow   bool // moving a window between sessions; false = pane between windows
	isTemplate bool // picking a session template to instantiate
	isSocket   bool // picking a tmux server socket to switch to
	newKind    moveNewKind
}

func (m model) startMove() (tea.Model, tea.Cmd) {
	r, ok := m.currentRow()
	if !ok {
		return m, nil
	}
	switch r.kind {
	case rowSession:
		m.setStatus("sessions cannot be moved", true)
		return m, nil
	case rowWindow:
		st := &moveState{
			title:    fmt.Sprintf("Move window '%s' to session:", r.window.Name),
			sourceID: r.window.ID,
			isWindow: true,
			newKind:  newSessionForWindow,
		}
		st.labels = append(st.labels, "+ New session")
		st.targets = append(st.targets, "")
		for i := range m.tree {
			s := &m.tree[i]
			if s.ID == r.session.ID {
				continue
			}
			st.labels = append(st.labels, fmt.Sprintf("%s (%s)", s.Name, plural(len(s.Windows), "window")))
			st.targets = append(st.targets, s.ID)
		}
		m.move = st
		m.mode = modeMove
	case rowPane:
		st := &moveState{
			title:    fmt.Sprintf("Move pane '%s' to window:", r.pane.CurrentCommand),
			sourceID: r.pane.ID,
			newKind:  newWindowForPane,
		}
		st.labels = append(st.labels, fmt.Sprintf("+ New window in '%s'", r.session.Name))
		st.targets = append(st.targets, "")
		for i := range m.tree {
			s := &m.tree[i]
			for j := range s.Windows {
				w := &s.Windows[j]
				if w.ID == r.window.ID {
					continue
				}
				st.labels = append(st.labels, fmt.Sprintf("%s: %d %s", s.Name, w.Index, w.Name))
				st.targets = append(st.targets, w.ID)
			}
		}
		m.move = st
		m.mode = modeMove
	}
	return m, nil
}

// --- template picker ---

// startTemplatePicker lists stored templates to instantiate as new sessions.
func (m model) startTemplatePicker() (tea.Model, tea.Cmd) {
	ts, err := m.templates.List()
	if err != nil || len(ts) == 0 {
		m.setStatus("no templates saved", true)
		return m, nil
	}
	st := &moveState{title: "New session from template:", isTemplate: true}
	for _, t := range ts {
		st.labels = append(st.labels, fmt.Sprintf("%s (%s)", t.Name, plural(len(t.Windows), "window")))
		st.targets = append(st.targets, t.Name)
	}
	m.move = st
	m.mode = modeMove
	return m, nil
}

// startDeleteTemplate asks for confirmation before deleting the picked
// template from the store.
func (m model) startDeleteTemplate() (tea.Model, tea.Cmd) {
	st := m.move
	if st == nil || !st.isTemplate {
		return m, nil
	}
	name := st.targets[st.cursor]
	store := m.templates
	m.move = nil
	m.confirm = &confirmState{
		lines: []string{fmt.Sprintf("Delete template '%s'?", name),
			"The JSON entry is removed; tmux sessions are unaffected."},
		action: func() error { return store.Delete(name) },
		okMsg:  "template '" + name + "' deleted",
	}
	m.mode = modeConfirm
	return m, nil
}

// startRenameTemplate opens the input to rename the picked template.
func (m model) startRenameTemplate() (tea.Model, tea.Cmd) {
	st := m.move
	if st == nil || !st.isTemplate {
		return m, nil
	}
	name := st.targets[st.cursor]
	m.move = nil
	m.inputPurpose = inputRenameTemplate
	m.inputTarget = name
	m.input.Reset()
	m.input.SetValue(name)
	m.input.Prompt = "rename template: "
	m.input.CursorEnd()
	m.mode = modeInput
	return m, m.input.Focus()
}

// --- socket switcher ---

// startSocketPicker lists the user's tmux server sockets to switch to.
func (m model) startSocketPicker() (tea.Model, tea.Cmd) {
	if m.listSockets == nil || m.backendFor == nil {
		m.setStatus("socket switching unavailable", true)
		return m, nil
	}
	names, err := m.listSockets()
	if err != nil {
		m.setStatus(err.Error(), true)
		return m, nil
	}
	st := &moveState{title: "Switch tmux server socket:", isSocket: true}
	if m.socket != "" {
		st.labels = append(st.labels, "default")
		st.targets = append(st.targets, "")
	}
	for _, n := range names {
		if n == m.socket {
			continue
		}
		st.labels = append(st.labels, n)
		st.targets = append(st.targets, n)
	}
	if len(st.targets) == 0 {
		m.setStatus("no other tmux server sockets", false)
		return m, nil
	}
	m.move = st
	m.mode = modeMove
	return m, nil
}

// switchSocket rebinds the model to another tmux server socket and resets
// all server-dependent state.
func (m *model) switchSocket(socket string) {
	m.backend = m.backendFor(socket)
	m.socket = socket
	m.tree = nil
	m.rows = nil
	m.cursor = 0
	m.offset = 0
	m.expanded = make(map[string]bool)
	m.filter = ""
	m.previewCache = make(map[string]string)
	if socket == "" {
		m.setStatus("socket: default", false)
	} else {
		m.setStatus("socket: "+socket, false)
	}
}

// --- delete ---

type confirmState struct {
	lines  []string
	action func() error
	okMsg  string
}

func (m model) startDelete() (tea.Model, tea.Cmd) {
	r, ok := m.currentRow()
	if !ok {
		return m, nil
	}
	c := &confirmState{}
	switch r.kind {
	case rowSession:
		s := r.session
		nPanes := 0
		for _, w := range s.Windows {
			nPanes += len(w.Panes)
		}
		c.lines = []string{
			fmt.Sprintf("Kill session '%s'?", s.Name),
			fmt.Sprintf("%s and %s will be killed.",
				plural(len(s.Windows), "window"), plural(nPanes, "pane")),
		}
		id := s.ID
		c.action = func() error { return m.backend.KillSession(id) }
		c.okMsg = "session killed"
	case rowWindow:
		w := r.window
		c.lines = []string{
			fmt.Sprintf("Kill window '%s'?", w.Name),
			fmt.Sprintf("%s will be killed.", plural(len(w.Panes), "pane")),
		}
		if len(r.session.Windows) == 1 {
			c.lines = append(c.lines,
				fmt.Sprintf("This is the last window in '%s'; the session will be killed.", r.session.Name))
		}
		id := w.ID
		c.action = func() error { return m.backend.KillWindow(id) }
		c.okMsg = "window killed"
	case rowPane:
		p := r.pane
		c.lines = []string{
			fmt.Sprintf("Kill pane '%s' (%s)?", p.CurrentCommand, p.ID),
		}
		if len(r.window.Panes) == 1 {
			c.lines = append(c.lines,
				fmt.Sprintf("This is the last pane in '%s'; the window will be closed.", r.window.Name))
			if len(r.session.Windows) == 1 {
				c.lines = append(c.lines,
					fmt.Sprintf("It is also the last window in '%s'; the session will be killed.", r.session.Name))
			}
		}
		id := p.ID
		c.action = func() error { return m.backend.KillPane(id) }
		c.okMsg = "pane killed"
	}
	m.confirm = c
	m.mode = modeConfirm
	return m, nil
}

func plural(n int, word string) string {
	if n == 1 {
		return fmt.Sprintf("1 %s", word)
	}
	return fmt.Sprintf("%d %ss", n, word)
}
