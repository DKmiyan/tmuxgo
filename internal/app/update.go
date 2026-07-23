package app

import (
	"strings"

	tea "charm.land/bubbletea/v2"
)

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.input.SetWidth(msg.Width - 8)
		m.ensureVisible()
		return m, nil
	case treeMsg:
		m.tree = msg
		m.loaded = true
		m.rebuild()
		return m, nil
	case errMsg:
		m.setStatus(msg.err.Error(), true)
		return m, nil
	case mutationMsg:
		if msg.err != nil {
			m.setStatus(msg.err.Error(), true)
		} else {
			m.setStatus(msg.ok, false)
		}
		return m, m.fetchTree
	case attachMsg:
		if msg.err != nil {
			m.setStatus("attach failed: "+msg.err.Error(), true)
			return m, m.fetchTree
		}
		if m.popup {
			// popup mode: the switch already happened; exit so the
			// display-popup closes into the attach target
			return m, tea.Quit
		}
		return m, m.fetchTree
	case previewMsg:
		m.previewCache[msg.id] = msg.content
		return m, nil
	case tickMsg:
		return m, tea.Batch(m.fetchTree, tick(), m.previewCmd())
	case tea.KeyPressMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m model) handleKey(k tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch m.mode {
	case modeFilter:
		return m.handleFilterKey(k)
	case modeInput:
		return m.handleInputKey(k)
	case modeCreate:
		return m.handleCreateKey(k)
	case modeConfirm:
		return m.handleConfirmKey(k)
	case modeMove:
		return m.handleMoveKey(k)
	case modeHelp:
		m.mode = modeNormal
		return m, nil
	default:
		return m.handleNormalKey(k)
	}
}

func (m model) handleNormalKey(k tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch k.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "esc":
		if m.filter != "" {
			m.filter = ""
			m.rebuild()
		}
		return m, nil
	case "up", "k":
		m.moveCursor(-1)
		return m, m.previewCmd()
	case "down", "j":
		m.moveCursor(1)
		return m, m.previewCmd()
	case "right", "l":
		m.expandOrChild()
		return m, m.previewCmd()
	case "left", "h":
		m.collapseOrParent()
		return m, m.previewCmd()
	case "enter":
		if r, ok := m.currentRow(); ok {
			return m, m.attach(r.id)
		}
		return m, nil
	case "n":
		return m.startCreate()
	case "r":
		return m.startRename()
	case "m":
		return m.startMove()
	case "d":
		return m.startDelete()
	case "/":
		m.mode = modeFilter
		m.input.Reset()
		m.input.Prompt = "/"
		m.input.Placeholder = "filter"
		m.filter = ""
		m.rebuild()
		return m, m.input.Focus()
	case "p":
		m.previewOn = !m.previewOn
		if m.previewOn && m.width < previewMinWidth {
			m.setStatus("preview hidden: terminal too narrow", true)
		}
		return m, m.previewCmd()
	case "?":
		m.mode = modeHelp
		return m, nil
	}
	return m, nil
}

func (m model) handleFilterKey(k tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch k.String() {
	case "enter":
		m.mode = modeNormal
		m.input.Blur()
		return m, nil
	case "esc":
		m.mode = modeNormal
		m.input.Blur()
		m.filter = ""
		m.rebuild()
		return m, nil
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(k)
	m.filter = m.input.Value()
	m.rebuild()
	m.cursor = 0
	m.offset = 0
	return m, cmd
}

func (m model) handleInputKey(k tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch k.String() {
	case "esc":
		m.mode = modeNormal
		m.input.Blur()
		return m, nil
	case "enter":
		value := strings.TrimSpace(m.input.Value())
		purpose, target := m.inputPurpose, m.inputTarget
		m.mode = modeNormal
		m.input.Blur()
		switch purpose {
		case inputNewSession:
			return m, m.runMutation(func() error { return m.backend.NewSession(value) }, "session created")
		case inputNewWindow:
			return m, m.runMutation(func() error { return m.backend.NewWindow(target, value) }, "window created")
		case inputRenameSession:
			if value == "" {
				m.setStatus("name cannot be empty", true)
				return m, nil
			}
			return m, m.runMutation(func() error { return m.backend.RenameSession(target, value) }, "session renamed")
		case inputRenameWindow:
			if value == "" {
				m.setStatus("name cannot be empty", true)
				return m, nil
			}
			return m, m.runMutation(func() error { return m.backend.RenameWindow(target, value) }, "window renamed")
		}
		return m, nil
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(k)
	return m, cmd
}

func (m model) handleCreateKey(k tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	c := m.create
	if c == nil {
		m.mode = modeNormal
		return m, nil
	}
	switch k.String() {
	case "esc", "q":
		m.mode = modeNormal
		m.create = nil
		return m, nil
	case "up", "k":
		if c.cursor > 0 {
			c.cursor--
		}
		return m, nil
	case "down", "j":
		if c.cursor < len(c.items)-1 {
			c.cursor++
		}
		return m, nil
	case "enter":
		item := c.items[c.cursor]
		m.create = nil
		switch item.kind {
		case createSplit:
			m.mode = modeNormal
			return m, m.runMutation(func() error { return m.backend.SplitPane(item.targetID) }, "pane split")
		case createSession:
			m.mode = modeInput
			m.inputPurpose = inputNewSession
			m.input.Reset()
			m.input.Prompt = "session name (empty = auto): "
			return m, m.input.Focus()
		case createWindow:
			m.mode = modeInput
			m.inputPurpose = inputNewWindow
			m.inputTarget = item.targetID
			m.input.Reset()
			m.input.Prompt = "window name (empty = auto): "
			return m, m.input.Focus()
		}
	}
	return m, nil
}

func (m model) handleConfirmKey(k tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	c := m.confirm
	if c == nil {
		m.mode = modeNormal
		return m, nil
	}
	switch k.String() {
	case "y", "Y":
		action := c.action
		okMsg := c.okMsg
		m.mode = modeNormal
		m.confirm = nil
		return m, m.runMutation(action, okMsg)
	default:
		m.mode = modeNormal
		m.confirm = nil
		m.setStatus("cancelled", false)
		return m, nil
	}
}

func (m model) handleMoveKey(k tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	st := m.move
	if st == nil {
		m.mode = modeNormal
		return m, nil
	}
	switch k.String() {
	case "esc", "q":
		m.mode = modeNormal
		m.move = nil
		return m, nil
	case "up", "k":
		if st.cursor > 0 {
			st.cursor--
		}
		return m, nil
	case "down", "j":
		if st.cursor < len(st.targets)-1 {
			st.cursor++
		}
		return m, nil
	case "enter":
		target := st.targets[st.cursor]
		source, isWindow := st.sourceID, st.isWindow
		m.mode = modeNormal
		m.move = nil
		if isWindow {
			return m, m.runMutation(func() error { return m.backend.MoveWindow(source, target) }, "window moved")
		}
		return m, m.runMutation(func() error { return m.backend.MovePane(source, target) }, "pane moved")
	}
	return m, nil
}
