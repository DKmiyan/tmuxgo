package app

import (
	"os"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/DKmiyan/tmuxgo/internal/config"
	"github.com/DKmiyan/tmuxgo/internal/template"
	"github.com/DKmiyan/tmuxgo/internal/tmux"
)

type mode int

const (
	modeNormal mode = iota
	modeFilter
	modeInput
	modeCreate
	modeConfirm
	modeMove
	modeHelp
	modeSettings
	modeDirPick
)

// inputPurpose identifies what a text input is collecting.
type inputPurpose int

const (
	inputNewSession inputPurpose = iota
	inputNewWindow
	inputRenameSession
	inputRenameWindow
	inputRenameTemplate
	inputMoveWindowNewSession
)

const refreshInterval = 2 * time.Second

type model struct {
	backend tmux.Backend
	sty     *styles

	tree     []tmux.Session
	rows     []row
	cursor   int
	offset   int
	expanded map[string]bool
	filter   string
	loaded   bool

	mode         mode
	input        textinput.Model
	inputPurpose inputPurpose
	inputTarget  string

	create  *createState
	confirm *confirmState
	move    *moveState
	dirPick *dirPickState
	dirList func() ([]string, error) // test hook; nil = zoxide/fallback

	previewOn    bool
	previewCache map[string]string

	status      string
	statusIsErr bool

	// popup mode (tmux display-popup): quit right after a successful
	// attach so the popup closes into the attach target.
	popup bool
	// pendingFocus selects+expands this session ID on the next tree load
	// (popup mode focuses the client's current session)
	pendingFocus string

	// templates enables "new session from template" in the create menu
	// (nil = feature unavailable)
	templates *template.Store

	// socket is the tmux server socket the backend is bound to
	// ("" = default); backendFor and listSockets are injectable for tests
	socket      string
	backendFor  func(socket string) tmux.Backend
	listSockets func() ([]string, error)

	// user configuration (theme, defaults, key bindings)
	cfg          config.Config
	cfgPath      string
	detectedDark bool
	keyActions   map[string]action

	// settings screen cursor (0 = theme, 1 = preview default, 2 = mouse)
	settingsCursor int

	// mouse support: click selects, double-click attaches, wheel scrolls
	mouseEnabled bool
	lastClickRow int
	lastClickAt  time.Time

	// drag & drop move: dragSource is the row index where the current
	// drag started (-1 = no drag), dragTarget is the hovered drop row
	dragSource int
	dragTarget int

	width  int
	height int
}

func newModel(b tmux.Backend, dark, popup bool) model {
	in := textinput.New()
	in.CharLimit = 120
	cfg := config.Default()
	return model{
		backend:      b,
		sty:          newStyles(dark),
		expanded:     make(map[string]bool),
		previewCache: make(map[string]string),
		input:        in,
		popup:        popup,
		cfg:          cfg,
		detectedDark: dark,
		keyActions:   buildKeyActions(cfg),
		mouseEnabled: cfg.Mouse,
		lastClickRow: -1,
		dragSource:   -1,
		dragTarget:   -1,
		width:        80,
		height:       24,
	}
}

// applyConfig overlays the loaded configuration onto the model.
func (m *model) applyConfig(cfg config.Config, path string) {
	m.cfg = cfg
	m.cfgPath = path
	m.keyActions = buildKeyActions(cfg)
	m.applyTheme()
	m.mouseEnabled = cfg.Mouse
	m.previewOn = cfg.PreviewDefault
}

// applyTheme rebuilds the palette from the configured theme. "auto" keeps
// the palette detected from the terminal background.
func (m *model) applyTheme() {
	switch m.cfg.Theme {
	case "dark":
		m.sty = newStyles(true)
	case "light":
		m.sty = newStyles(false)
	default:
		m.sty = newStyles(m.detectedDark)
	}
}

// action identifies a bindable normal-mode action.
type action int

const (
	actUp action = iota
	actDown
	actExpand
	actCollapse
	actAttach
	actNew
	actRename
	actMove
	actKill
	actFilter
	actPreview
	actHelp
	actSettings
	actQuit
	actSocket
)

var actionByName = map[string]action{
	"up": actUp, "down": actDown, "expand": actExpand, "collapse": actCollapse,
	"attach": actAttach, "new": actNew, "rename": actRename, "move": actMove,
	"kill": actKill, "filter": actFilter, "preview": actPreview, "help": actHelp,
	"settings": actSettings, "quit": actQuit, "socket": actSocket,
}

// buildKeyActions flattens the action -> keys config into key -> action.
func buildKeyActions(cfg config.Config) map[string]action {
	out := make(map[string]action, len(cfg.Keys))
	for name, keys := range cfg.Keys {
		act, ok := actionByName[name]
		if !ok {
			continue
		}
		for _, k := range keys {
			out[k] = act
		}
	}
	return out
}

// Run starts the interactive TUI against the given backend. popup enables
// attach-and-exit mode for use from a tmux display-popup binding.
func Run(b tmux.Backend, popup bool) error {
	dark := lipgloss.HasDarkBackground(os.Stdin, os.Stdout)
	m := newModel(b, dark, popup)
	if store, err := template.DefaultStore(); err == nil {
		m.templates = store
	}
	if path, err := config.DefaultPath(); err == nil {
		if cfg, err := config.Load(path); err == nil {
			m.applyConfig(cfg, path)
		}
	}
	m.backendFor = func(socket string) tmux.Backend { return tmux.NewWithSocket(socket) }
	m.listSockets = tmux.ListSockets
	m.applyPopupDefaults()
	p := tea.NewProgram(m)
	_, err := p.Run()
	return err
}

func (m model) Init() tea.Cmd {
	cmds := []tea.Cmd{m.fetchTree, tick()}
	if m.popup {
		cmds = append(cmds, m.focusCurrent)
	}
	return tea.Batch(cmds...)
}

// applyPopupDefaults enables popup-mode conveniences: preview on and
// focusing the client's current session on load.
func (m *model) applyPopupDefaults() {
	if !m.popup {
		return
	}
	m.previewOn = true
}

// --- messages ---

type treeMsg []tmux.Session
type tickMsg time.Time

type errMsg struct{ err error }

type mutationMsg struct {
	err error
	ok  string
}

type attachMsg struct{ err error }

// focusMsg carries the client's current session ID for popup focus.
type focusMsg struct{ id string }

// focusCurrent resolves the client's current session (popup mode).
func (m model) focusCurrent() tea.Msg {
	id, err := m.backend.CurrentSessionID()
	if err != nil {
		return errMsg{err}
	}
	return focusMsg{id: id}
}

type previewMsg struct {
	id      string
	content string
}

// --- commands ---

func (m model) fetchTree() tea.Msg {
	tree, err := m.backend.Tree()
	if err != nil {
		return errMsg{err}
	}
	return treeMsg(tree)
}

func tick() tea.Cmd {
	return tea.Tick(refreshInterval, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func (m model) runMutation(run func() error, ok string) tea.Cmd {
	return func() tea.Msg {
		return mutationMsg{err: run(), ok: ok}
	}
}

func (m model) attach(target string) tea.Cmd {
	return tea.ExecProcess(m.backend.AttachCmd(target), func(err error) tea.Msg {
		return attachMsg{err: err}
	})
}

// --- shared state helpers ---

func (m *model) setStatus(s string, isErr bool) {
	m.status = s
	m.statusIsErr = isErr
}

// rebuild recomputes visible rows, preserving the selected tmux ID when it
// still exists and clamping the cursor otherwise.
func (m *model) rebuild() {
	prevIdx := m.cursor
	selID := ""
	if r, ok := m.currentRow(); ok {
		selID = r.id
	}
	if m.filter != "" {
		m.rows = flattenFiltered(m.tree, m.filter)
	} else {
		m.rows = flatten(m.tree, m.expanded)
	}
	switch {
	case indexOfRow(m.rows, selID) >= 0:
		m.cursor = indexOfRow(m.rows, selID)
	case len(m.rows) == 0:
		m.cursor = 0
	case prevIdx >= len(m.rows):
		m.cursor = len(m.rows) - 1
	default:
		m.cursor = prevIdx
	}
	m.ensureVisible()
}

func (m model) currentRow() (row, bool) {
	if m.cursor >= 0 && m.cursor < len(m.rows) {
		return m.rows[m.cursor], true
	}
	return row{}, false
}

func (m *model) moveCursor(delta int) {
	if len(m.rows) == 0 {
		return
	}
	m.cursor += delta
	if m.cursor < 0 {
		m.cursor = 0
	}
	if m.cursor > len(m.rows)-1 {
		m.cursor = len(m.rows) - 1
	}
	m.ensureVisible()
}

func (m *model) ensureVisible() {
	vis := m.bodyHeight()
	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	if m.cursor >= m.offset+vis {
		m.offset = m.cursor - vis + 1
	}
	if m.offset < 0 {
		m.offset = 0
	}
}

// focusSession selects and expands the session with the given tmux ID.
func (m *model) focusSession(id string) {
	idx := indexOfRow(m.rows, id)
	if idx < 0 {
		return
	}
	collapseSiblings(m.rows, m.rows[idx], m.expanded)
	m.expanded[id] = true
	m.rebuild()
	if idx := indexOfRow(m.rows, id); idx >= 0 {
		m.cursor = idx
		m.ensureVisible()
	}
}

// expandOrChild implements Right: expand a collapsed node (collapsing its
// siblings), or step into the first child of an expanded one.
func (m *model) expandOrChild() {
	r, ok := m.currentRow()
	if !ok || r.kind == rowPane || m.filter != "" {
		return
	}
	if !m.expanded[r.id] {
		collapseSiblings(m.rows, r, m.expanded)
		m.expanded[r.id] = true
		m.rebuild()
		return
	}
	if m.cursor+1 < len(m.rows) && m.rows[m.cursor+1].depth > r.depth {
		m.moveCursor(1)
	}
}

// collapseOrParent implements Left: collapse an expanded node, or jump to
// the parent row.
func (m *model) collapseOrParent() {
	r, ok := m.currentRow()
	if !ok || m.filter != "" {
		return
	}
	if r.kind != rowPane && m.expanded[r.id] {
		delete(m.expanded, r.id)
		m.rebuild()
		return
	}
	if r.parentID != "" {
		if idx := indexOfRow(m.rows, r.parentID); idx >= 0 {
			m.cursor = idx
			m.ensureVisible()
		}
	}
}
