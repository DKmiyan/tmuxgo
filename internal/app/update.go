package app

import (
	"os"
	"strings"
	"time"

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
	// the footer hint segments are clickable, same as their key bindings
	if ev.Y == m.height-1 {
		segs := m.footerSegments(m.width)
		if i := m.footerHit(ev.X, m.width); i >= 0 {
			return m.dispatchAction(segs[i].act)
		}
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

// handleMouseMotion tracks footer hint hovering (no button) and drags
// (left button): remember the hovered drop row so the view can highlight
// valid targets.
func (m model) handleMouseMotion(ev tea.Mouse) (tea.Model, tea.Cmd) {
	if m.mode != modeNormal {
		return m, nil
	}
	if ev.Button == tea.MouseNone {
		hov := -1
		if ev.Y == m.height-1 {
			hov = m.footerHit(ev.X, m.width)
		}
		m.hoverFooter = hov
		return m, nil
	}
	if m.dragSource < 0 || ev.Button != tea.MouseLeft {
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
			m.setStatus(m.tr(i18n.ReleaseToMove, label), false)
		} else {
			m.setStatus(m.tr(i18n.InvalidDrop), true)
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
		return m, m.runMutation(run, m.tr(i18n.MovedX, label))
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
			m.tr(i18n.MoveLabelWinSess, src.window.Name, dst.session.Name), true
	case src.kind == rowPane && dst.kind == rowWindow && dst.window.ID != src.window.ID:
		return func() error { return m.backend.MovePane(src.pane.ID, dst.window.ID) },
			m.tr(i18n.MoveLabelPaneWin, src.pane.CurrentCommand, dst.window.Name), true
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
		m.mode = modeNormal
		cfg, path := m.cfg, m.cfgPath
		ok := m.tr(i18n.SettingsSaved)
		return m, func() tea.Msg {
			if path == "" {
				return nil
			}
			return mutationMsg{err: config.Save(path, cfg), ok: ok}
		}
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
		return m, nil
	case "right", "l":
		switch m.settingsCursor {
		case 0:
			m.cfg.Theme = nextTheme(m.cfg.Theme, 1)
			m.applyTheme()
		case 1:
			m.cfg.Language = nextLanguage(m.cfg.Language, 1)
			m.lang = i18n.Resolve(m.cfg.Language, os.Getenv)
		}
		return m, nil
	case "left", "h":
		switch m.settingsCursor {
		case 0:
			m.cfg.Theme = nextTheme(m.cfg.Theme, -1)
			m.applyTheme()
		case 1:
			m.cfg.Language = nextLanguage(m.cfg.Language, -1)
			m.lang = i18n.Resolve(m.cfg.Language, os.Getenv)
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
		m.setStatus(m.tr(i18n.Cancelled), false)
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
	return m, nil
}
