package app

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/DKmiyan/tmuxgo/internal/config"
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
		if m.pendingFocus != "" {
			m.focusSession(m.pendingFocus)
			m.pendingFocus = ""
		}
		return m, nil
	case focusMsg:
		if msg.id != "" {
			m.pendingFocus = msg.id
			if m.loaded {
				m.pendingFocus = ""
				m.focusSession(msg.id)
			}
		}
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
	case tea.MouseClickMsg:
		return m.handleMouseClick(tea.Mouse(msg))
	case tea.MouseMotionMsg:
		return m.handleMouseMotion(tea.Mouse(msg))
	case tea.MouseReleaseMsg:
		return m.handleMouseRelease(tea.Mouse(msg))
	case tea.MouseWheelMsg:
		return m.handleMouseWheel(tea.Mouse(msg))
	case tea.KeyPressMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

// handleMouseWheel scrolls the cursor in normal mode and in the
// create/move pickers.
func (m model) handleMouseWheel(ev tea.Mouse) (tea.Model, tea.Cmd) {
	delta := 3
	if ev.Button == tea.MouseWheelUp {
		delta = -3
	}
	switch m.mode {
	case modeNormal:
		m.moveCursor(delta)
		return m, m.previewCmd()
	case modeCreate:
		if c := m.create; c != nil {
			c.cursor += delta / 3
			if c.cursor < 0 {
				c.cursor = 0
			}
			if c.cursor > len(c.items)-1 {
				c.cursor = len(c.items) - 1
			}
		}
		return m, nil
	case modeMove:
		if st := m.move; st != nil {
			st.cursor += delta / 3
			if st.cursor < 0 {
				st.cursor = 0
			}
			if st.cursor > len(st.targets)-1 {
				st.cursor = len(st.targets) - 1
			}
		}
		return m, nil
	}
	return m, nil
}

// handleMouseClick implements mouse selection in normal mode:
// click selects a row, click on an expand marker toggles it, and a quick
// second click on the same row attaches to it.
func (m model) handleMouseClick(ev tea.Mouse) (tea.Model, tea.Cmd) {
	if m.mode != modeNormal || ev.Button != tea.MouseLeft {
		return m, nil
	}
	// the body starts one line below the header
	idx := m.offset + ev.Y - 1
	if idx < 0 || idx >= len(m.rows) {
		return m, nil
	}
	if m.showPreview() && ev.X >= m.width*3/5 {
		return m, nil // click landed in the preview column
	}
	r := m.rows[idx]

	// expand/collapse marker zone (session/window rows only)
	if r.kind != rowPane && m.filter == "" && ev.X >= 2*r.depth && ev.X < 2*r.depth+2 {
		if m.expanded[r.id] {
			delete(m.expanded, r.id)
		} else {
			collapseSiblings(m.rows, r, m.expanded)
			m.expanded[r.id] = true
		}
		m.cursor = idx
		m.rebuild()
		return m, m.previewCmd()
	}

	doubleClick := idx == m.lastClickRow && time.Since(m.lastClickAt) < 500*time.Millisecond
	m.cursor = idx
	m.ensureVisible()
	m.lastClickRow = idx
	m.lastClickAt = time.Now()
	// window and pane rows are draggable; the drop is resolved on release
	if r.kind == rowWindow || r.kind == rowPane {
		m.dragSource = idx
	} else {
		m.dragSource = -1
	}
	m.dragTarget = -1
	if doubleClick {
		return m, m.attach(r.id)
	}
	return m, m.previewCmd()
}

// handleMouseMotion tracks a drag: remember the hovered drop row so the
// view can highlight valid targets.
func (m model) handleMouseMotion(ev tea.Mouse) (tea.Model, tea.Cmd) {
	if m.mode != modeNormal || m.dragSource < 0 || ev.Button != tea.MouseLeft {
		return m, nil
	}
	idx := m.offset + ev.Y - 1
	if idx < 0 || idx >= len(m.rows) {
		idx = -1
	}
	if m.showPreview() && ev.X >= m.width*3/5 {
		idx = -1
	}
	m.dragTarget = idx
	if idx >= 0 {
		if _, label, ok := m.dropTarget(m.dragSource, idx); ok {
			m.setStatus("release to move "+label, false)
		} else {
			m.setStatus("not a valid drop target", true)
		}
	}
	return m, nil
}

// handleMouseRelease completes a drag & drop move.
func (m model) handleMouseRelease(ev tea.Mouse) (tea.Model, tea.Cmd) {
	if m.dragSource < 0 {
		return m, nil
	}
	src, dst := m.dragSource, m.dragTarget
	m.dragSource, m.dragTarget = -1, -1
	if run, label, ok := m.dropTarget(src, dst); ok {
		return m, m.runMutation(run, "moved "+label)
	}
	return m, nil
}

// dropTarget validates dropping row src onto row dst: a window drops onto
// another session, a pane drops onto another window.
func (m model) dropTarget(srcIdx, dstIdx int) (func() error, string, bool) {
	if srcIdx < 0 || dstIdx < 0 || srcIdx >= len(m.rows) || dstIdx >= len(m.rows) {
		return nil, "", false
	}
	src, dst := m.rows[srcIdx], m.rows[dstIdx]
	switch {
	case src.kind == rowWindow && dst.kind == rowSession && dst.session.ID != src.session.ID:
		return func() error { return m.backend.MoveWindow(src.window.ID, dst.session.ID) },
			fmt.Sprintf("window '%s' → session '%s'", src.window.Name, dst.session.Name), true
	case src.kind == rowPane && dst.kind == rowWindow && dst.window.ID != src.window.ID:
		return func() error { return m.backend.MovePane(src.pane.ID, dst.window.ID) },
			fmt.Sprintf("pane '%s' → window '%s'", src.pane.CurrentCommand, dst.window.Name), true
	}
	return nil, "", false
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
	case modeSettings:
		return m.handleSettingsKey(k)
	case modeHelp:
		m.mode = modeNormal
		return m, nil
	default:
		return m.handleNormalKey(k)
	}
}

func (m model) handleNormalKey(k tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	// esc is fixed: it clears an active filter (and is never bindable);
	// in popup mode with no filter it closes the popup
	if k.String() == "esc" {
		if m.filter != "" {
			m.filter = ""
			m.rebuild()
		} else if m.popup {
			return m, tea.Quit
		}
		return m, nil
	}
	act, ok := m.keyActions[k.String()]
	if !ok {
		return m, nil
	}
	switch act {
	case actQuit:
		return m, tea.Quit
	case actUp:
		m.moveCursor(-1)
		return m, m.previewCmd()
	case actDown:
		m.moveCursor(1)
		return m, m.previewCmd()
	case actExpand:
		m.expandOrChild()
		return m, m.previewCmd()
	case actCollapse:
		m.collapseOrParent()
		return m, m.previewCmd()
	case actAttach:
		if r, ok := m.currentRow(); ok {
			return m, m.attach(r.id)
		}
		return m, nil
	case actNew:
		return m.startCreate()
	case actRename:
		return m.startRename()
	case actMove:
		return m.startMove()
	case actKill:
		return m.startDelete()
	case actFilter:
		m.mode = modeFilter
		m.input.Reset()
		m.input.Prompt = "/"
		m.input.Placeholder = "filter"
		m.filter = ""
		m.rebuild()
		return m, m.input.Focus()
	case actPreview:
		m.previewOn = !m.previewOn
		if m.previewOn && m.width < previewMinWidth {
			m.setStatus("preview hidden: terminal too narrow", true)
		}
		return m, m.previewCmd()
	case actHelp:
		m.mode = modeHelp
		return m, nil
	case actSettings:
		m.mode = modeSettings
		m.settingsCursor = 0
		return m, nil
	}
	return m, nil
}

// handleSettingsKey drives the settings screen: three toggles persisted on
// close. Theme changes apply immediately.
func (m model) handleSettingsKey(k tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch k.String() {
	case "esc", "q":
		m.mode = modeNormal
		cfg, path := m.cfg, m.cfgPath
		return m, func() tea.Msg {
			if path == "" {
				return nil
			}
			return mutationMsg{err: config.Save(path, cfg), ok: "settings saved"}
		}
	case "up", "k":
		if m.settingsCursor > 0 {
			m.settingsCursor--
		}
		return m, nil
	case "down", "j":
		if m.settingsCursor < 2 {
			m.settingsCursor++
		}
		return m, nil
	case "enter", "space":
		switch m.settingsCursor {
		case 0:
			switch m.cfg.Theme {
			case "auto":
				m.cfg.Theme = "dark"
			case "dark":
				m.cfg.Theme = "light"
			default:
				m.cfg.Theme = "auto"
			}
			m.applyTheme()
		case 1:
			m.cfg.PreviewDefault = !m.cfg.PreviewDefault
		case 2:
			m.cfg.Mouse = !m.cfg.Mouse
			m.mouseEnabled = m.cfg.Mouse
		}
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
			return m, m.runMutation(func() error { return m.backend.NewWindow(target, value, "") }, "window created")
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
			return m, m.runMutation(func() error { return m.backend.SplitPane(item.targetID, "") }, "pane split")
		case createFromTemplate:
			return m.startTemplatePicker()
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
		source, isWindow, isTemplate := st.sourceID, st.isWindow, st.isTemplate
		m.mode = modeNormal
		m.move = nil
		if isTemplate {
			tpl, err := m.templates.Get(target)
			if err != nil {
				m.setStatus(err.Error(), true)
				return m, nil
			}
			return m, m.runMutation(func() error {
				_, err := tpl.Create(m.backend)
				return err
			}, "session created from template '"+target+"'")
		}
		if isWindow {
			return m, m.runMutation(func() error { return m.backend.MoveWindow(source, target) }, "window moved")
		}
		return m, m.runMutation(func() error { return m.backend.MovePane(source, target) }, "pane moved")
	}
	return m, nil
}
