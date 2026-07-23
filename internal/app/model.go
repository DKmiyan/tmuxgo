package app

import (
	"os"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

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
)

// inputPurpose identifies what a text input is collecting.
type inputPurpose int

const (
	inputNewSession inputPurpose = iota
	inputNewWindow
	inputRenameSession
	inputRenameWindow
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

	previewOn    bool
	previewCache map[string]string

	status      string
	statusIsErr bool

	// popup mode (tmux display-popup): quit right after a successful
	// attach so the popup closes into the attach target.
	popup bool

	width  int
	height int
}

func newModel(b tmux.Backend, dark, popup bool) model {
	in := textinput.New()
	in.CharLimit = 120
	return model{
		backend:      b,
		sty:          newStyles(dark),
		expanded:     make(map[string]bool),
		previewCache: make(map[string]string),
		input:        in,
		popup:        popup,
		width:        80,
		height:       24,
	}
}

// Run starts the interactive TUI against the given backend. popup enables
// attach-and-exit mode for use from a tmux display-popup binding.
func Run(b tmux.Backend, popup bool) error {
	dark := lipgloss.HasDarkBackground(os.Stdin, os.Stdout)
	p := tea.NewProgram(newModel(b, dark, popup))
	_, err := p.Run()
	return err
}

func (m model) Init() tea.Cmd {
	return tea.Batch(m.fetchTree, tick())
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
