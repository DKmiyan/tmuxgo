package app

import (
	"fmt"

	tea "charm.land/bubbletea/v2"

	"github.com/DKmiyan/tmuxgo/internal/i18n"
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
	items := []createItem{{label: m.tr(i18n.NewSession), kind: createSession}}
	if r, ok := m.currentRow(); ok {
		items = append(items, createItem{
			label:    m.tr(i18n.NewWindowIn, r.session.Name),
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
					label:    m.tr(i18n.SplitPaneIn, r.window.Name),
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
			items = append(items, createItem{label: m.tr(i18n.NewFromTemplate), kind: createFromTemplate})
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
		m.setStatus(m.tr(i18n.CannotRenamePane), true)
		return m, nil
	case rowSession:
		m.inputPurpose = inputRenameSession
		m.inputTarget = r.session.ID
		m.input.Reset()
		m.input.SetValue(r.session.Name)
		m.input.Prompt = m.tr(i18n.PromptRenameSession)
	case rowWindow:
		m.inputPurpose = inputRenameWindow
		m.inputTarget = r.window.ID
		m.input.Reset()
		m.input.SetValue(r.window.Name)
		m.input.Prompt = m.tr(i18n.PromptRenameWindow)
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
		m.setStatus(m.tr(i18n.SessionsCannotMove), true)
		return m, nil
	case rowWindow:
		st := &moveState{
			title:    m.tr(i18n.MoveWindowTitle, r.window.Name),
			sourceID: r.window.ID,
			isWindow: true,
			newKind:  newSessionForWindow,
		}
		st.labels = append(st.labels, m.tr(i18n.NewSessionItem))
		st.targets = append(st.targets, "")
		for i := range m.tree {
			s := &m.tree[i]
			if s.ID == r.session.ID {
				continue
			}
			st.labels = append(st.labels, fmt.Sprintf("%s (%s)", s.Name, m.plural(len(s.Windows), i18n.UnitWindow)))
			st.targets = append(st.targets, s.ID)
		}
		m.move = st
		m.mode = modeMove
	case rowPane:
		st := &moveState{
			title:    m.tr(i18n.MovePaneTitle, r.pane.CurrentCommand),
			sourceID: r.pane.ID,
			newKind:  newWindowForPane,
		}
		st.labels = append(st.labels, m.tr(i18n.NewWindowInItem, r.session.Name))
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
		m.setStatus(m.tr(i18n.NoTemplates), true)
		return m, nil
	}
	st := &moveState{title: m.tr(i18n.TemplatePickerTitle), isTemplate: true}
	for _, t := range ts {
		st.labels = append(st.labels, fmt.Sprintf("%s (%s)", t.Name, m.plural(len(t.Windows), i18n.UnitWindow)))
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
		lines: []string{m.tr(i18n.DeleteTemplateQ, name),
			m.tr(i18n.DeleteTemplateNote)},
		action: func() error { return store.Delete(name) },
		okMsg:  m.tr(i18n.TemplateDeleted, name),
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
	m.input.Prompt = m.tr(i18n.PromptRenameTemplate)
	m.input.CursorEnd()
	m.mode = modeInput
	return m, m.input.Focus()
}

// --- socket switcher ---

// startSocketPicker lists the user's tmux server sockets to switch to.
func (m model) startSocketPicker() (tea.Model, tea.Cmd) {
	if m.listSockets == nil || m.backendFor == nil {
		m.setStatus(m.tr(i18n.SocketUnavailable), true)
		return m, nil
	}
	names, err := m.listSockets()
	if err != nil {
		m.setStatus(err.Error(), true)
		return m, nil
	}
	st := &moveState{title: m.tr(i18n.SocketPickerTitle), isSocket: true}
	if m.socket != "" {
		st.labels = append(st.labels, m.tr(i18n.SocketDefault))
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
		m.setStatus(m.tr(i18n.NoOtherSockets), false)
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
		m.setStatus(m.tr(i18n.SocketNowDefault), false)
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
			m.tr(i18n.KillSessionQ, s.Name),
			m.tr(i18n.WillBeKilled2,
				m.plural(len(s.Windows), i18n.UnitWindow), m.plural(nPanes, i18n.UnitPane)),
		}
		id := s.ID
		c.action = func() error { return m.backend.KillSession(id) }
		c.okMsg = m.tr(i18n.SessionKilled)
	case rowWindow:
		w := r.window
		c.lines = []string{
			m.tr(i18n.KillWindowQ, w.Name),
			m.tr(i18n.WillBeKilled1, m.plural(len(w.Panes), i18n.UnitPane)),
		}
		if len(r.session.Windows) == 1 {
			c.lines = append(c.lines, m.tr(i18n.LastWindowCascade, r.session.Name))
		}
		id := w.ID
		c.action = func() error { return m.backend.KillWindow(id) }
		c.okMsg = m.tr(i18n.WindowKilled)
	case rowPane:
		p := r.pane
		c.lines = []string{
			m.tr(i18n.KillPaneQ, p.CurrentCommand, p.ID),
		}
		if len(r.window.Panes) == 1 {
			c.lines = append(c.lines, m.tr(i18n.LastPaneCascade, r.window.Name))
			if len(r.session.Windows) == 1 {
				c.lines = append(c.lines, m.tr(i18n.AlsoLastWindowCascade, r.session.Name))
			}
		}
		id := p.ID
		c.action = func() error { return m.backend.KillPane(id) }
		c.okMsg = m.tr(i18n.PaneKilled)
	}
	m.confirm = c
	m.mode = modeConfirm
	return m, nil
}
