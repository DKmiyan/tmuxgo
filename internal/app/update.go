package app

import (
	"os"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/DKmiyan/tmuxgo/internal/config"
	"github.com/DKmiyan/tmuxgo/internal/i18n"
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
	case dirSessionMsg:
		if msg.err != nil {
			m.setStatus(msg.err.Error(), true)
			return m, m.fetchTree
		}
		m.setStatus(m.tr(i18n.SessionCreated, msg.name), false)
		return m, m.attach(msg.id)
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
			m.setStatus(m.tr(i18n.AttachFailed, msg.err.Error()), true)
			return m, m.fetchTree
		}
		if m.popup {
			// popup mode: the switch already happened; exit so the
			// display-popup closes into the attach target
			return m, tea.Quit
		}
		return m, m.fetchTree
	case previewMsg:
		for id, content := range msg.panes {
			m.previewCache[id] = content
		}
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
	case modeDirPick:
		return m.handleDirPickKey(k)
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
	return m.dispatchAction(act)
}

// dispatchAction runs a normal-mode action, whether it came from a key
// binding or from clicking a footer hint segment.
func (m model) dispatchAction(act action) (tea.Model, tea.Cmd) {
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
		m.input.Placeholder = m.tr(i18n.FilterPlaceholder)
		m.filter = ""
		m.rebuild()
		return m, m.input.Focus()
	case actPreview:
		m.previewOn = !m.previewOn
		if m.previewOn && m.width < previewMinWidth {
			m.setStatus(m.tr(i18n.PreviewTooNarrow), true)
		}
		return m, m.previewCmd()
	case actHelp:
		m.mode = modeHelp
		return m, nil
	case actSettings:
		m.mode = modeSettings
		m.settingsCursor = 0
		return m, nil
	case actSocket:
		return m.startSocketPicker()
	}
	return m, nil
}

// handleSettingsKey drives the settings screen: values persisted on close.
// Theme and language changes apply immediately.
func (m model) handleSettingsKey(k tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch k.String() {
	case "esc", "q":
		return m.closeSettings()
	case "up", "k":
		if m.settingsCursor > 0 {
			m.settingsCursor--
		}
		return m, nil
	case "down", "j":
		if m.settingsCursor < 3 {
			m.settingsCursor++
		}
		return m, nil
	case "enter", "space":
		m.settingsChange()
		return m, nil
	case "right", "l":
		m.settingsCycle(1)
		return m, nil
	case "left", "h":
		m.settingsCycle(-1)
		return m, nil
	}
	return m, nil
}

// closeSettings leaves the settings screen, persisting the config.
func (m model) closeSettings() (tea.Model, tea.Cmd) {
	m.mode = modeNormal
	cfg, path := m.cfg, m.cfgPath
	ok := m.tr(i18n.SettingsSaved)
	return m, func() tea.Msg {
		if path == "" {
			return nil
		}
		return mutationMsg{err: config.Save(path, cfg), ok: ok}
	}
}

// settingsChange applies Enter/Space (or a click): cycle theme/language
// forward, toggle the boolean rows.
func (m *model) settingsChange() {
	switch m.settingsCursor {
	case 0:
		m.cfg.Theme = nextTheme(m.cfg.Theme, 1)
		m.applyTheme()
	case 1:
		m.cfg.Language = nextLanguage(m.cfg.Language, 1)
		m.lang = i18n.Resolve(m.cfg.Language, os.Getenv)
	case 2:
		m.cfg.PreviewDefault = !m.cfg.PreviewDefault
	case 3:
		m.cfg.Mouse = !m.cfg.Mouse
		m.mouseEnabled = m.cfg.Mouse
	}
}

// settingsCycle steps the cyclic rows (theme, language) by dir, like
// Left/Right (or a right-click) on the row.
func (m *model) settingsCycle(dir int) {
	switch m.settingsCursor {
	case 0:
		m.cfg.Theme = nextTheme(m.cfg.Theme, dir)
		m.applyTheme()
	case 1:
		m.cfg.Language = nextLanguage(m.cfg.Language, dir)
		m.lang = i18n.Resolve(m.cfg.Language, os.Getenv)
	}
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
			dir := m.pendingDir
			m.pendingDir = ""
			b := m.backend
			return m, func() tea.Msg {
				id, err := b.NewSessionID(value, dir)
				return dirSessionMsg{id: id, name: value, err: err}
			}
		case inputNewWindow:
			dir := m.pendingDir
			m.pendingDir = ""
			return m, m.runMutation(func() error { return m.backend.NewWindow(target, value, dir) }, m.tr(i18n.WindowCreated))
		case inputRenameSession:
			if value == "" {
				m.setStatus(m.tr(i18n.NameEmpty), true)
				return m, nil
			}
			return m, m.runMutation(func() error { return m.backend.RenameSession(target, value) }, m.tr(i18n.SessionRenamed))
		case inputRenameWindow:
			if value == "" {
				m.setStatus(m.tr(i18n.NameEmpty), true)
				return m, nil
			}
			return m, m.runMutation(func() error { return m.backend.RenameWindow(target, value) }, m.tr(i18n.WindowRenamed))
		case inputRenameTemplate:
			if value == "" {
				m.setStatus(m.tr(i18n.NameEmpty), true)
				return m, nil
			}
			return m, m.runMutation(func() error { return m.templates.Rename(target, value) }, m.tr(i18n.TemplateRenamed))
		case inputMoveWindowNewSession:
			return m, m.runMutation(func() error {
				sessID, err := m.backend.NewSessionID(value, "")
				if err != nil {
					return err
				}
				return m.backend.MoveWindow(target, sessID)
			}, m.tr(i18n.WindowMovedNewSession))
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
		return m.createConfirm()
	}
	return m, nil
}

// createConfirm activates the highlighted create-menu item (Enter or a
// mouse click).
func (m model) createConfirm() (tea.Model, tea.Cmd) {
	c := m.create
	item := c.items[c.cursor]
	m.create = nil
	switch item.kind {
	case createSplit:
		m.mode = modeNormal
		return m, m.runMutation(func() error { return m.backend.SplitPane(item.targetID, "") }, m.tr(i18n.PaneSplit))
	case createFromTemplate:
		return m.startTemplatePicker()
	case createSession:
		return m.startDirStep(inputNewSession, "", m.tr(i18n.PromptSessionName))
	case createWindow:
		return m.startDirStep(inputNewWindow, item.targetID, m.tr(i18n.PromptWindowName))
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
		return m.confirmYes()
	default:
		return m.confirmCancel()
	}
}

// confirmYes runs the confirmed destructive action (y key or a click on
// the "[y] confirm" zone).
func (m model) confirmYes() (tea.Model, tea.Cmd) {
	c := m.confirm
	action, okMsg := c.action, c.okMsg
	m.mode = modeNormal
	m.confirm = nil
	return m, m.runMutation(action, okMsg)
}

// confirmCancel dismisses the confirm dialog (any other key, a click on
// the "[n/esc] cancel" zone, or a click outside the box).
func (m model) confirmCancel() (tea.Model, tea.Cmd) {
	m.mode = modeNormal
	m.confirm = nil
	m.setStatus(m.tr(i18n.Cancelled), false)
	return m, nil
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
	case "d":
		if st.isTemplate {
			return m.startDeleteTemplate()
		}
		return m, nil
	case "r":
		if st.isTemplate {
			return m.startRenameTemplate()
		}
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
		return m.moveConfirm()
	}
	return m, nil
}

// moveConfirm activates the highlighted move/template/socket target
// (Enter or a mouse click).
func (m model) moveConfirm() (tea.Model, tea.Cmd) {
	st := m.move
	// pseudo-item first: create the missing home for the move
	if st.newKind != newNone && st.cursor == 0 {
		source := st.sourceID
		m.mode = modeNormal
		m.move = nil
		switch st.newKind {
		case newWindowForPane:
			return m, m.runMutation(func() error { return m.backend.BreakPane(source) }, m.tr(i18n.PaneMovedNewWindow))
		case newSessionForWindow:
			m.inputPurpose = inputMoveWindowNewSession
			m.inputTarget = source
			m.input.Reset()
			m.input.Prompt = m.tr(i18n.PromptNewSessionAuto)
			m.mode = modeInput
			return m, m.input.Focus()
		}
	}
	target := st.targets[st.cursor]
	source, isWindow, isTemplate, isSocket := st.sourceID, st.isWindow, st.isTemplate, st.isSocket
	m.mode = modeNormal
	m.move = nil
	if isSocket {
		m.switchSocket(target)
		return m, m.fetchTree
	}
	if isTemplate {
		tpl, err := m.templates.Get(target)
		if err != nil {
			m.setStatus(err.Error(), true)
			return m, nil
		}
		return m, m.runMutation(func() error {
			_, err := tpl.Create(m.backend)
			return err
		}, m.tr(i18n.SessionFromTemplate, target))
	}
	if isWindow {
		return m, m.runMutation(func() error { return m.backend.MoveWindow(source, target) }, m.tr(i18n.WindowMoved))
	}
	return m, m.runMutation(func() error { return m.backend.MovePane(source, target) }, m.tr(i18n.PaneMoved))
}
