package app

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/DKmiyan/tmuxgo/internal/config"
	"github.com/DKmiyan/tmuxgo/internal/i18n"
	"github.com/DKmiyan/tmuxgo/internal/template"
	"github.com/DKmiyan/tmuxgo/internal/tmux"
)

// --- fake backend ---

type fakeBackend struct {
	sessions []tmux.Session
	treeErr  error

	captureContent string
	captureErr     error
	currentSession string

	newSessions    []string
	newSessionDirs []string
	newWindows     [][2]string
	newWindowDirs  []string
	splits         []string
	renamedS       map[string]string
	renamedW       map[string]string
	movedWindows   [][2]string
	movedPanes     [][2]string
	brokenPanes    []string
	killed         []string
}

func (f *fakeBackend) Tree() ([]tmux.Session, error) { return f.sessions, f.treeErr }

func (f *fakeBackend) NewSession(name string) error {
	f.newSessions = append(f.newSessions, name)
	return nil
}

func (f *fakeBackend) NewSessionID(name, dir string) (string, error) {
	f.newSessions = append(f.newSessions, name)
	f.newSessionDirs = append(f.newSessionDirs, dir)
	return "$fake", nil
}

func (f *fakeBackend) NewWindow(sessionID, name, dir string) error {
	f.newWindows = append(f.newWindows, [2]string{sessionID, name})
	f.newWindowDirs = append(f.newWindowDirs, dir)
	return nil
}

func (f *fakeBackend) SplitPane(paneID, dir string) error {
	f.splits = append(f.splits, paneID)
	return nil
}

func (f *fakeBackend) RenameSession(id, name string) error {
	if f.renamedS == nil {
		f.renamedS = map[string]string{}
	}
	f.renamedS[id] = name
	return nil
}

func (f *fakeBackend) RenameWindow(id, name string) error {
	if f.renamedW == nil {
		f.renamedW = map[string]string{}
	}
	f.renamedW[id] = name
	return nil
}

func (f *fakeBackend) MoveWindow(windowID, sessionID string) error {
	f.movedWindows = append(f.movedWindows, [2]string{windowID, sessionID})
	return nil
}

func (f *fakeBackend) MovePane(paneID, windowID string) error {
	f.movedPanes = append(f.movedPanes, [2]string{paneID, windowID})
	return nil
}

func (f *fakeBackend) BreakPane(paneID string) error {
	f.brokenPanes = append(f.brokenPanes, paneID)
	return nil
}

func (f *fakeBackend) KillSession(id string) error {
	f.killed = append(f.killed, id)
	return nil
}

func (f *fakeBackend) KillWindow(id string) error {
	f.killed = append(f.killed, id)
	return nil
}

func (f *fakeBackend) KillPane(id string) error {
	f.killed = append(f.killed, id)
	return nil
}

func (f *fakeBackend) SelectLayout(windowID, layout string) error {
	return nil
}

func (f *fakeBackend) CapturePane(paneID string, lines int) (string, error) {
	return f.captureContent, f.captureErr
}

func (f *fakeBackend) AttachCmd(target string) *exec.Cmd {
	return exec.Command("true")
}

func (f *fakeBackend) CurrentSessionID() (string, error) {
	return f.currentSession, nil
}

// --- helpers ---

func testSessions() []tmux.Session {
	return []tmux.Session{
		{
			ID: "$1", Name: "work", Attached: true, Activity: time.Now().Add(-2 * time.Minute),
			Windows: []tmux.Window{
				{ID: "@1", SessionID: "$1", Index: 1, Name: "editor", Active: true, Panes: []tmux.Pane{
					{ID: "%1", WindowID: "@1", Index: 0, Active: true, CurrentCommand: "vim", CurrentPath: "/home/u/code"},
					{ID: "%2", WindowID: "@1", Index: 1, Active: false, CurrentCommand: "bash", CurrentPath: "/home/u"},
				}},
				{ID: "@2", SessionID: "$1", Index: 2, Name: "logs", Active: false, Panes: []tmux.Pane{
					{ID: "%3", WindowID: "@2", Index: 0, Active: true, CurrentCommand: "tail", CurrentPath: "/var/log"},
				}},
			},
		},
		{
			ID: "$2", Name: "personal", Attached: false, Activity: time.Now().Add(-time.Hour),
			Windows: []tmux.Window{
				{ID: "@3", SessionID: "$2", Index: 1, Name: "mail", Active: true, Panes: []tmux.Pane{
					{ID: "%4", WindowID: "@3", Index: 0, Active: true, CurrentCommand: "mutt", CurrentPath: "/home/u"},
				}},
			},
		},
	}
}

func apply(m model, msg tea.Msg) model {
	nm, _ := m.Update(msg)
	return nm.(model)
}

func newTestModel(width, height int) (model, *fakeBackend) {
	fb := &fakeBackend{captureContent: "line one\nline two"}
	m := newModel(fb, true, false)
	m.lang = i18n.EN // hermetic: tests assert English strings
	m = apply(m, tea.WindowSizeMsg{Width: width, Height: height})
	m = apply(m, treeMsg(testSessions()))
	return m, fb
}

func key(s string) tea.KeyPressMsg {
	switch s {
	case "up":
		return tea.KeyPressMsg{Code: tea.KeyUp}
	case "down":
		return tea.KeyPressMsg{Code: tea.KeyDown}
	case "left":
		return tea.KeyPressMsg{Code: tea.KeyLeft}
	case "right":
		return tea.KeyPressMsg{Code: tea.KeyRight}
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	case "esc":
		return tea.KeyPressMsg{Code: tea.KeyEscape}
	case "backspace":
		return tea.KeyPressMsg{Code: tea.KeyBackspace}
	}
	return tea.KeyPressMsg{Code: []rune(s)[0], Text: s}
}

func press(m model, s string) (model, tea.Cmd) {
	nm, cmd := m.Update(key(s))
	return nm.(model), cmd
}

func typeText(m model, s string) model {
	for _, ch := range s {
		m, _ = press(m, string(ch))
	}
	return m
}

func rowIDs(m model) []string {
	ids := make([]string, len(m.rows))
	for i, r := range m.rows {
		ids[i] = r.id
	}
	return ids
}

func assertIDs(t *testing.T, m model, want []string) {
	t.Helper()
	if got := rowIDs(m); !reflect.DeepEqual(got, want) {
		t.Fatalf("rows = %v, want %v", got, want)
	}
}

// --- flatten / expand / collapse / cursor ---

func TestFlattenExpandAndSiblingCollapse(t *testing.T) {
	m, _ := newTestModel(80, 24)

	assertIDs(t, m, []string{"$1", "$2"})
	if m.cursor != 0 {
		t.Fatalf("initial cursor = %d, want 0", m.cursor)
	}

	m, _ = press(m, "right") // expand $1
	assertIDs(t, m, []string{"$1", "@1", "@2", "$2"})
	m, _ = press(m, "right") // $1 expanded: step into @1
	if got := m.rows[m.cursor].id; got != "@1" {
		t.Fatalf("cursor row = %s, want @1", got)
	}
	m, _ = press(m, "right") // expand @1
	assertIDs(t, m, []string{"$1", "@1", "%1", "%2", "@2", "$2"})
	m, _ = press(m, "right") // step into first pane
	if got := m.rows[m.cursor].id; got != "%1" {
		t.Fatalf("cursor row = %s, want %%1", got)
	}
	m, _ = press(m, "right") // pane: no-op
	if got := m.rows[m.cursor].id; got != "%1" {
		t.Fatalf("cursor row = %s, want %%1 (pane right must be a no-op)", got)
	}

	// Expanding @2 collapses its sibling @1.
	for m.rows[m.cursor].id != "@2" {
		m, _ = press(m, "down")
	}
	m, _ = press(m, "right")
	assertIDs(t, m, []string{"$1", "@1", "@2", "%3", "$2"})

	// Expanding $2 collapses $1 entirely.
	for m.rows[m.cursor].id != "$2" {
		m, _ = press(m, "down")
	}
	m, _ = press(m, "right")
	assertIDs(t, m, []string{"$1", "$2", "@3"})

	// Left on expanded $2 collapses it; left again is a no-op.
	m, _ = press(m, "left")
	assertIDs(t, m, []string{"$1", "$2"})
	m, _ = press(m, "left")
	if got := m.rows[m.cursor].id; got != "$2" {
		t.Fatalf("cursor row = %s, want $2", got)
	}
}

func TestLeftJumpsToParent(t *testing.T) {
	m, _ := newTestModel(80, 24)
	m, _ = press(m, "right") // expand $1
	m, _ = press(m, "down")  // @1
	m, _ = press(m, "right") // expand @1
	m, _ = press(m, "down")  // %1
	m, _ = press(m, "down")  // %2

	m, _ = press(m, "left") // pane -> parent window
	if got := m.rows[m.cursor].id; got != "@1" {
		t.Fatalf("cursor row = %s, want @1", got)
	}
	m, _ = press(m, "left") // collapse @1
	assertIDs(t, m, []string{"$1", "@1", "@2", "$2"})
	m, _ = press(m, "left") // window -> parent session
	if got := m.rows[m.cursor].id; got != "$1" {
		t.Fatalf("cursor row = %s, want $1", got)
	}
}

func TestCursorBounds(t *testing.T) {
	m, _ := newTestModel(80, 24)
	for i := 0; i < 10; i++ {
		m, _ = press(m, "down")
	}
	if m.cursor != len(m.rows)-1 {
		t.Fatalf("cursor = %d, want %d (clamped)", m.cursor, len(m.rows)-1)
	}
	for i := 0; i < 10; i++ {
		m, _ = press(m, "up")
	}
	if m.cursor != 0 {
		t.Fatalf("cursor = %d, want 0 (clamped)", m.cursor)
	}
}

// --- selection across refresh ---

func TestSelectionPreservedAcrossRefresh(t *testing.T) {
	m, fb := newTestModel(80, 24)
	m, _ = press(m, "right") // expand $1
	m, _ = press(m, "down")  // @1
	m, _ = press(m, "down")  // @2
	if got := m.rows[m.cursor].id; got != "@2" {
		t.Fatalf("cursor row = %s, want @2", got)
	}

	// Reorder: personal becomes most recent, work's windows flip order.
	reordered := []tmux.Session{
		testSessions()[1],
		{
			ID: "$1", Name: "work", Attached: true, Activity: time.Now(),
			Windows: []tmux.Window{
				testSessions()[0].Windows[1],
				testSessions()[0].Windows[0],
			},
		},
	}
	fb.sessions = reordered
	m = apply(m, treeMsg(fb.sessions))

	assertIDs(t, m, []string{"$2", "$1", "@2", "@1"})
	if got := m.rows[m.cursor].id; got != "@2" {
		t.Fatalf("selection after reorder = %s, want @2", got)
	}

	// @2 vanishes: cursor clamps to the nearest valid row.
	work := reordered[1]
	work.Windows = work.Windows[1:]
	fb.sessions = []tmux.Session{reordered[0], work}
	m = apply(m, treeMsg(fb.sessions))

	assertIDs(t, m, []string{"$2", "$1", "@1"})
	if got := m.rows[m.cursor].id; got != "@1" {
		t.Fatalf("selection after vanish = %s, want @1 (nearest valid)", got)
	}
}

// --- filter ---

func TestFilterMatchesWithAncestors(t *testing.T) {
	m, _ := newTestModel(80, 24)

	m, _ = press(m, "/")
	m = typeText(m, "Vim") // case-insensitive
	assertIDs(t, m, []string{"$1", "@1", "%1"})

	m, _ = press(m, "enter") // keep filter, back to normal
	if m.mode != modeNormal {
		t.Fatalf("mode = %v, want normal", m.mode)
	}
	assertIDs(t, m, []string{"$1", "@1", "%1"})

	got := m.View().Content
	if !strings.Contains(got, "filter:") || !strings.Contains(got, `"Vim"`) || !strings.Contains(got, "(esc clears)") {
		t.Fatalf("view missing filter notice:\n%s", got)
	}

	m, _ = press(m, "esc") // clear filter in normal mode
	if m.filter != "" {
		t.Fatalf("filter = %q, want cleared", m.filter)
	}
	assertIDs(t, m, []string{"$1", "$2"})
}

func TestFilterMatchesPanePathAndEscClears(t *testing.T) {
	m, _ := newTestModel(80, 24)

	m, _ = press(m, "/")
	m = typeText(m, "var/log")
	assertIDs(t, m, []string{"$1", "@2", "%3"})

	m, _ = press(m, "esc") // esc while typing clears the filter entirely
	if m.filter != "" {
		t.Fatalf("filter = %q, want cleared", m.filter)
	}
	assertIDs(t, m, []string{"$1", "$2"})
	if m.mode != modeNormal {
		t.Fatalf("mode = %v, want normal", m.mode)
	}
}

// --- delete confirmation ---

func TestConfirmCascadeWarnings(t *testing.T) {
	// Window @3 is the last window of personal.
	m, _ := newTestModel(80, 24)
	for m.rows[m.cursor].id != "$2" {
		m, _ = press(m, "down")
	}
	m, _ = press(m, "right") // expand $2
	m, _ = press(m, "down")  // @3
	m, _ = press(m, "d")
	if m.confirm == nil {
		t.Fatal("no confirm state after d")
	}
	joined := strings.Join(m.confirm.lines, "\n")
	if !strings.Contains(joined, "Kill window 'mail'?") {
		t.Fatalf("confirm missing window title:\n%s", joined)
	}
	if !strings.Contains(joined, "This is the last window in 'personal'; the session will be killed.") {
		t.Fatalf("confirm missing cascade warning:\n%s", joined)
	}
	m, _ = press(m, "esc")

	// Pane %3 is the last pane of logs.
	for m.rows[m.cursor].id != "$1" {
		m, _ = press(m, "up")
	}
	m, _ = press(m, "right") // expand $1
	for m.rows[m.cursor].id != "@2" {
		m, _ = press(m, "down")
	}
	m, _ = press(m, "right") // expand @2
	m, _ = press(m, "down")  // %3
	m, _ = press(m, "d")
	joined = strings.Join(m.confirm.lines, "\n")
	if !strings.Contains(joined, "This is the last pane in 'logs'; the window will be closed.") {
		t.Fatalf("confirm missing last-pane warning:\n%s", joined)
	}

	// Session row: counts, no cascade warning.
	m, _ = press(m, "esc")
	for m.rows[m.cursor].id != "$1" {
		m, _ = press(m, "up")
	}
	m, _ = press(m, "d")
	joined = strings.Join(m.confirm.lines, "\n")
	if !strings.Contains(joined, "Kill session 'work'?") ||
		!strings.Contains(joined, "2 windows") || !strings.Contains(joined, "3 panes") {
		t.Fatalf("confirm missing session summary:\n%s", joined)
	}
	if strings.Contains(joined, "last window") || strings.Contains(joined, "last pane") {
		t.Fatalf("session confirm must not warn about cascades:\n%s", joined)
	}
}

func TestConfirmKillExecutes(t *testing.T) {
	m, fb := newTestModel(80, 24)
	m, _ = press(m, "d") // on $1
	if m.mode != modeConfirm {
		t.Fatalf("mode = %v, want confirm", m.mode)
	}
	m, cmd := press(m, "y")
	if m.mode != modeNormal {
		t.Fatalf("mode = %v, want normal after confirm", m.mode)
	}
	if cmd == nil {
		t.Fatal("confirm returned no command")
	}
	msg := cmd().(mutationMsg)
	if msg.err != nil {
		t.Fatalf("kill failed: %v", msg.err)
	}
	if !reflect.DeepEqual(fb.killed, []string{"$1"}) {
		t.Fatalf("killed = %v, want [$1]", fb.killed)
	}

	m, _ = press(m, "d")
	m, _ = press(m, "n") // cancel
	if !reflect.DeepEqual(fb.killed, []string{"$1"}) {
		t.Fatalf("killed = %v after cancel, want [$1]", fb.killed)
	}
}

// --- create / rename / move ---

func TestCreateChooserOptions(t *testing.T) {
	m, fb := newTestModel(80, 24)
	m, _ = press(m, "right") // expand $1
	m, _ = press(m, "down")  // @1
	m, _ = press(m, "right") // expand @1
	m, _ = press(m, "down")  // %1

	m, _ = press(m, "n")
	if m.mode != modeCreate {
		t.Fatalf("mode = %v, want create", m.mode)
	}
	var labels []string
	for _, it := range m.create.items {
		labels = append(labels, it.label)
	}
	want := []string{"New session", "New window in 'work'", "Split pane in 'editor'"}
	if !reflect.DeepEqual(labels, want) {
		t.Fatalf("options = %v, want %v", labels, want)
	}
	if m.create.cursor != 2 {
		t.Fatalf("default option = %d, want 2 (most contextual)", m.create.cursor)
	}

	// Confirm split pane: executes immediately against the selected pane.
	m, cmd := press(m, "enter")
	if cmd == nil {
		t.Fatal("split pane returned no command")
	}
	if msg := cmd().(mutationMsg); msg.err != nil {
		t.Fatalf("split failed: %v", msg.err)
	}
	if !reflect.DeepEqual(fb.splits, []string{"%1"}) {
		t.Fatalf("splits = %v, want [%%1]", fb.splits)
	}
}

func TestNewSessionFlow(t *testing.T) {
	m, fb := newTestModel(80, 24)
	m, _ = press(m, "n")
	// first option is "New session"; default cursor is the last one.
	for m.create.cursor != 0 {
		m, _ = press(m, "up")
	}
	m, _ = press(m, "enter")
	if m.mode != modeDirPick {
		t.Fatalf("mode = %v, want dir step", m.mode)
	}
	// accept a real directory, then the name step
	dir := t.TempDir()
	m.input.SetValue(dir)
	m, _ = press(m, "enter")
	if m.mode != modeInput {
		t.Fatalf("mode = %v, want name step", m.mode)
	}
	m.input.SetValue("demo")
	m, cmd := press(m, "enter")
	if cmd == nil {
		t.Fatal("new session returned no command")
	}
	msg, ok := cmd().(dirSessionMsg)
	if !ok || msg.err != nil {
		t.Fatalf("dirSessionMsg = %#v", msg)
	}
	if !reflect.DeepEqual(fb.newSessions, []string{"demo"}) {
		t.Fatalf("newSessions = %v, want [demo]", fb.newSessions)
	}
	if !reflect.DeepEqual(fb.newSessionDirs, []string{dir}) {
		t.Fatalf("newSessionDirs = %v, want [%s]", fb.newSessionDirs, dir)
	}
}

func TestRenamePrefilledAndExecutes(t *testing.T) {
	m, fb := newTestModel(80, 24)
	m, _ = press(m, "r")
	if m.mode != modeInput {
		t.Fatalf("mode = %v, want input", m.mode)
	}
	if m.input.Value() != "work" {
		t.Fatalf("prefill = %q, want work", m.input.Value())
	}
	m = typeText(m, "-2") // appended to the prefilled name
	m, cmd := press(m, "enter")
	if msg := cmd().(mutationMsg); msg.err != nil {
		t.Fatalf("rename failed: %v", msg.err)
	}
	if fb.renamedS["$1"] != "work-2" {
		t.Fatalf("renamedS = %v, want work-2", fb.renamedS)
	}
}

func TestRenamePaneRejected(t *testing.T) {
	m, _ := newTestModel(80, 24)
	m, _ = press(m, "right")
	m, _ = press(m, "down")
	m, _ = press(m, "right")
	m, _ = press(m, "down") // %1
	m, _ = press(m, "r")
	if m.mode != modeNormal {
		t.Fatalf("mode = %v, want normal (pane rename rejected)", m.mode)
	}
	if m.status == "" {
		t.Fatal("expected a status hint for pane rename")
	}
}

func TestMoveWindowPicker(t *testing.T) {
	m, fb := newTestModel(80, 24)
	m, _ = press(m, "right") // expand $1
	m, _ = press(m, "down")  // @1
	m, _ = press(m, "m")
	if m.mode != modeMove {
		t.Fatalf("mode = %v, want move", m.mode)
	}
	// the "+ New session" pseudo-item comes first, then other sessions
	if !reflect.DeepEqual(m.move.targets, []string{"", "$2"}) {
		t.Fatalf("targets = %v, want ['' $2]", m.move.targets)
	}
	if m.move.labels[0] != "+ New session" {
		t.Fatalf("first label = %q", m.move.labels[0])
	}
	m, _ = press(m, "down") // select $2
	m, cmd := press(m, "enter")
	if msg := cmd().(mutationMsg); msg.err != nil {
		t.Fatalf("move failed: %v", msg.err)
	}
	if !reflect.DeepEqual(fb.movedWindows, [][2]string{{"@1", "$2"}}) {
		t.Fatalf("movedWindows = %v, want [[@1 $2]]", fb.movedWindows)
	}
}

func TestMovePanePicker(t *testing.T) {
	m, fb := newTestModel(80, 24)
	m, _ = press(m, "right")
	m, _ = press(m, "down")
	m, _ = press(m, "right")
	m, _ = press(m, "down") // %1 in @1
	m, _ = press(m, "m")
	// "+ New window in 'work'" first, then every window except @1
	if !reflect.DeepEqual(m.move.targets, []string{"", "@2", "@3"}) {
		t.Fatalf("targets = %v, want ['' @2 @3]", m.move.targets)
	}
	if m.move.labels[0] != "+ New window in 'work'" {
		t.Fatalf("first label = %q", m.move.labels[0])
	}
	m, _ = press(m, "down")
	m, _ = press(m, "down") // select @3
	m, cmd := press(m, "enter")
	if msg := cmd().(mutationMsg); msg.err != nil {
		t.Fatalf("move failed: %v", msg.err)
	}
	if !reflect.DeepEqual(fb.movedPanes, [][2]string{{"%1", "@3"}}) {
		t.Fatalf("movedPanes = %v, want [[%%1 @3]]", fb.movedPanes)
	}
}

func TestMovePanePickerBreaksIntoNewWindow(t *testing.T) {
	m, fb := newTestModel(80, 24)
	m, _ = press(m, "right")
	m, _ = press(m, "down")
	m, _ = press(m, "right")
	m, _ = press(m, "down") // %1
	m, _ = press(m, "m")
	// first item is the pseudo-target; confirming it breaks the pane out
	if m.move.newKind != newWindowForPane {
		t.Fatalf("newKind = %v", m.move.newKind)
	}
	m, cmd := press(m, "enter") // cursor starts at 0
	if cmd == nil {
		t.Fatal("break-pane returned no command")
	}
	if msg := cmd().(mutationMsg); msg.err != nil {
		t.Fatalf("break-pane failed: %v", msg.err)
	}
	if !reflect.DeepEqual(fb.brokenPanes, []string{"%1"}) {
		t.Fatalf("brokenPanes = %v, want [%%1]", fb.brokenPanes)
	}
}

func TestMovePanePickerWorksWithoutOtherWindows(t *testing.T) {
	m, _ := newTestModel(80, 24)
	// %4 is the only pane of @3, the only window of personal
	for m.rows[m.cursor].id != "$2" {
		m, _ = press(m, "down")
	}
	m, _ = press(m, "right") // expand $2
	m, _ = press(m, "down")  // @3
	m, _ = press(m, "right") // expand @3
	m, _ = press(m, "down")  // %4
	m, _ = press(m, "m")
	if m.mode != modeMove {
		t.Fatalf("mode = %v, want move picker even with no other window", m.mode)
	}
	if m.move.labels[0] != "+ New window in 'personal'" {
		t.Fatalf("first label = %q", m.move.labels[0])
	}
}

func TestMoveWindowPickerCreatesSession(t *testing.T) {
	m, fb := newTestModel(80, 24)
	m, _ = press(m, "right")
	m, _ = press(m, "down") // @1
	m, _ = press(m, "m")
	if m.move.newKind != newSessionForWindow {
		t.Fatalf("newKind = %v", m.move.newKind)
	}
	m, _ = press(m, "enter") // pick "+ New session"
	if m.mode != modeInput {
		t.Fatalf("mode = %v, want input for session name", m.mode)
	}
	m = typeText(m, "fresh")
	m, cmd := press(m, "enter")
	if msg := cmd().(mutationMsg); msg.err != nil {
		t.Fatalf("move-to-new-session failed: %v", msg.err)
	}
	if !reflect.DeepEqual(fb.newSessions, []string{"fresh"}) {
		t.Fatalf("newSessions = %v, want [fresh]", fb.newSessions)
	}
	if !reflect.DeepEqual(fb.movedWindows, [][2]string{{"@1", "$fake"}}) {
		t.Fatalf("movedWindows = %v, want [[@1 $fake]]", fb.movedWindows)
	}
}

// --- preview ---

func TestPreviewNarrowTerminalHint(t *testing.T) {
	m, _ := newTestModel(80, 24)
	m, _ = press(m, "p")
	if !m.previewOn {
		t.Fatal("previewOn = false, want true")
	}
	if m.status == "" {
		t.Fatal("expected narrow-terminal status hint")
	}
	if m.showPreview() {
		t.Fatal("showPreview must be false at width 80")
	}
}

func TestPreviewFetchesAndRenders(t *testing.T) {
	m, _ := newTestModel(120, 24)
	m, cmd := press(m, "p")
	if cmd == nil {
		t.Fatal("preview toggle returned no fetch command")
	}
	m = apply(m, cmd()) // delivers previewMsg
	if m.previewCache["%1"] != "line one\nline two" {
		t.Fatalf("preview cache = %q", m.previewCache["%1"])
	}
	if !m.showPreview() {
		t.Fatal("showPreview must be true at width 120")
	}
	if got := m.View().Content; !strings.Contains(got, "line one") {
		t.Fatalf("view missing preview content:\n%s", got)
	}
}

// --- attach ---

func TestAttachReturnsExecCommand(t *testing.T) {
	m, _ := newTestModel(80, 24)
	_, cmd := press(m, "enter")
	if cmd == nil {
		t.Fatal("enter returned no command")
	}
	// do not invoke the command: it would run ExecProcess against the tty
}

// --- view snapshots ---

func assertViewFits(t *testing.T, m model, width, height int) string {
	t.Helper()
	content := m.View().Content
	lines := strings.Split(content, "\n")
	for i, l := range lines {
		if got := lipgloss.Width(l); got > width {
			t.Fatalf("line %d is %d cells wide (max %d): %q", i, got, width, l)
		}
	}
	return content
}

func TestViewSnapshot80x24(t *testing.T) {
	m, _ := newTestModel(80, 24)
	m, _ = press(m, "right") // expand $1
	content := assertViewFits(t, m, 80, 24)
	for _, want := range []string{"tmuxgo", "work", "editor", "personal", "? help"} {
		if !strings.Contains(content, want) {
			t.Fatalf("view missing %q:\n%s", want, content)
		}
	}
}

func TestViewSnapshot40x10(t *testing.T) {
	m, _ := newTestModel(40, 10)
	assertViewFits(t, m, 40, 10)
}

func TestViewSnapshotsDialogs(t *testing.T) {
	m, _ := newTestModel(40, 10)
	m, _ = press(m, "?")
	assertViewFits(t, m, 40, 10)
	m, _ = press(m, "x") // close help
	m, _ = press(m, "d") // confirm dialog
	assertViewFits(t, m, 40, 10)
	m, _ = press(m, "n") // cancel -> create chooser
	assertViewFits(t, m, 40, 10)
}

func TestEmptyState(t *testing.T) {
	fb := &fakeBackend{}
	m := newModel(fb, true, false)
	m = apply(m, tea.WindowSizeMsg{Width: 80, Height: 24})
	m = apply(m, treeMsg(nil))
	content := assertViewFits(t, m, 80, 24)
	if !strings.Contains(content, "No tmux sessions") {
		t.Fatalf("empty state missing hint:\n%s", content)
	}
}

func TestTreeErrorSurfaces(t *testing.T) {
	fb := &fakeBackend{treeErr: errTest}
	m := newModel(fb, true, false)
	m = apply(m, tea.WindowSizeMsg{Width: 80, Height: 24})
	m = apply(m, m.fetchTree())
	if m.status == "" || !m.statusIsErr {
		t.Fatalf("status = %q (err=%v), want error surfaced", m.status, m.statusIsErr)
	}
}

var errTest = errString("boom")

type errString string

func (e errString) Error() string { return string(e) }

// --- mouse ---

func click(x, y int) tea.MouseClickMsg {
	return tea.MouseClickMsg(tea.Mouse{X: x, Y: y, Button: tea.MouseLeft})
}

func TestMouseClickSelectsRow(t *testing.T) {
	m, _ := newTestModel(80, 24)
	m = apply(m, click(5, 3)) // second row ($2); header 0, panel border 1
	if m.cursor != 1 {
		t.Fatalf("cursor = %d, want 1", m.cursor)
	}
}

func TestMouseMarkerTogglesExpand(t *testing.T) {
	m, _ := newTestModel(80, 24)
	m = apply(m, click(1, 2)) // marker of $1
	assertIDs(t, m, []string{"$1", "@1", "@2", "$2"})
	m = apply(m, click(1, 2)) // collapse again
	assertIDs(t, m, []string{"$1", "$2"})
}

func TestMouseDoubleClickAttaches(t *testing.T) {
	m, _ := newTestModel(80, 24)
	m = apply(m, click(5, 2))
	_, cmd := m.Update(click(5, 2))
	if cmd == nil {
		t.Fatal("double-click returned no attach command")
	}
	// do not invoke: it would run ExecProcess against the tty
}

func TestMouseWheelScrolls(t *testing.T) {
	m, _ := newTestModel(80, 24)
	wheel := tea.MouseWheelMsg(tea.Mouse{X: 5, Y: 5, Button: tea.MouseWheelDown})
	m = apply(m, wheel) // clamps to last row
	if m.cursor != len(m.rows)-1 {
		t.Fatalf("cursor = %d, want %d", m.cursor, len(m.rows)-1)
	}
	m = apply(m, tea.MouseWheelMsg(tea.Mouse{X: 5, Y: 5, Button: tea.MouseWheelUp}))
	if m.cursor != 0 {
		t.Fatalf("cursor = %d, want 0", m.cursor)
	}
}

func TestMouseIgnoredInFilterMode(t *testing.T) {
	m, _ := newTestModel(80, 24)
	m, _ = press(m, "/")
	m = apply(m, click(5, 2))
	if m.cursor != 0 {
		t.Fatalf("cursor = %d, want 0 (mouse ignored in filter mode)", m.cursor)
	}
}

// --- drag & drop move ---

func motion(x, y int) tea.MouseMotionMsg {
	return tea.MouseMotionMsg(tea.Mouse{X: x, Y: y, Button: tea.MouseLeft})
}

func release(x, y int) tea.MouseReleaseMsg {
	return tea.MouseReleaseMsg(tea.Mouse{X: x, Y: y, Button: tea.MouseLeft})
}

func TestDragMoveWindow(t *testing.T) {
	m, fb := newTestModel(80, 24)
	m, _ = press(m, "right") // expand $1: rows $1(y2) @1(y3) @2(y4) $2(y5)
	m = apply(m, click(5, 4))
	m = apply(m, motion(5, 5))
	if !strings.Contains(m.status, "release to move window 'logs' → session 'personal'") {
		t.Fatalf("drag status = %q", m.status)
	}
	_, cmd := m.Update(release(5, 5))
	if cmd == nil {
		t.Fatal("drop returned no command")
	}
	if msg := cmd().(mutationMsg); msg.err != nil {
		t.Fatalf("move failed: %v", msg.err)
	}
	if !reflect.DeepEqual(fb.movedWindows, [][2]string{{"@2", "$2"}}) {
		t.Fatalf("movedWindows = %v, want [[@2 $2]]", fb.movedWindows)
	}
}

func TestDragMovePane(t *testing.T) {
	m, fb := newTestModel(80, 24)
	m, _ = press(m, "right") // expand $1
	m, _ = press(m, "down")
	m, _ = press(m, "right") // expand @1: rows $1 @1 %1(y4) %2(y5) @2(y6) $2
	m = apply(m, click(5, 4))
	m = apply(m, motion(5, 6))
	_, cmd := m.Update(release(5, 6))
	if cmd == nil {
		t.Fatal("drop returned no command")
	}
	if msg := cmd().(mutationMsg); msg.err != nil {
		t.Fatalf("move failed: %v", msg.err)
	}
	if !reflect.DeepEqual(fb.movedPanes, [][2]string{{"%1", "@2"}}) {
		t.Fatalf("movedPanes = %v, want [[%%1 @2]]", fb.movedPanes)
	}
}

func TestDragInvalidTargetIsNoop(t *testing.T) {
	m, fb := newTestModel(80, 24)
	m, _ = press(m, "right")
	m = apply(m, click(5, 4))  // @2
	m = apply(m, motion(5, 3)) // over @1 (window: invalid drop)
	nm, cmd := m.Update(release(5, 3))
	m = nm.(model)
	if cmd != nil {
		t.Fatal("invalid drop must return no command")
	}
	if m.dragSource != -1 || m.dragTarget != -1 {
		t.Fatal("drag state must reset after release")
	}
	if len(fb.movedWindows) != 0 || len(fb.movedPanes) != 0 {
		t.Fatalf("unexpected moves: %v %v", fb.movedWindows, fb.movedPanes)
	}
}

// --- config / settings ---

func TestSettingsToggleAndSave(t *testing.T) {
	m, _ := newTestModel(80, 24)
	m.cfgPath = filepath.Join(t.TempDir(), "config.json")

	m, _ = press(m, ",")
	if m.mode != modeSettings {
		t.Fatalf("mode = %v, want settings", m.mode)
	}
	m, _ = press(m, "down") // language
	m, _ = press(m, "down") // preview default
	m, _ = press(m, "down") // mouse
	m, _ = press(m, "enter")
	if m.mouseEnabled {
		t.Fatal("mouse must be toggled off immediately")
	}
	m, cmd := press(m, "esc") // close -> save
	if cmd == nil {
		t.Fatal("closing settings returned no save command")
	}
	if msg := cmd().(mutationMsg); msg.err != nil {
		t.Fatalf("config save failed: %v", msg.err)
	}
	got, err := config.Load(m.cfgPath)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Mouse {
		t.Fatal("saved config has mouse on, want off")
	}
}

func TestSettingsThemeAppliesLive(t *testing.T) {
	m, _ := newTestModel(80, 24) // detected dark = true
	m, _ = press(m, ",")
	m, _ = press(m, "enter") // auto -> dark
	if !m.sty.dark || m.cfg.Theme != "dark" {
		t.Fatalf("theme = %q dark=%v", m.cfg.Theme, m.sty.dark)
	}
	m, _ = press(m, "enter") // dark -> light
	if m.sty.dark || m.cfg.Theme != "light" {
		t.Fatalf("theme = %q dark=%v", m.cfg.Theme, m.sty.dark)
	}
	m, _ = press(m, "enter") // light -> first named theme
	if m.cfg.Theme != themeCycle[2] || m.sty.dark != builtinThemes[themeCycle[2]].dark {
		t.Fatalf("theme = %q dark=%v", m.cfg.Theme, m.sty.dark)
	}
	m, _ = press(m, "left") // back to light
	if m.sty.dark || m.cfg.Theme != "light" {
		t.Fatalf("theme = %q dark=%v", m.cfg.Theme, m.sty.dark)
	}
	// cycling wraps all the way back to auto (detected dark)
	for m.cfg.Theme != "auto" {
		m, _ = press(m, "right")
	}
	if !m.sty.dark {
		t.Fatalf("auto on dark background must be dark, theme = %q", m.cfg.Theme)
	}
}

func TestUnknownThemeFallsBack(t *testing.T) {
	m, _ := newTestModel(80, 24) // detected dark = true
	cfg := config.Default()
	cfg.Theme = "bogus"
	m.applyConfig(cfg, "")
	if !m.sty.dark {
		t.Fatal("unknown theme must fall back to detected dark default")
	}
	if !strings.Contains(m.status, "bogus") {
		t.Fatalf("status must warn about the unknown theme, got %q", m.status)
	}
}

func TestThemeColorOverrideApplies(t *testing.T) {
	m, _ := newTestModel(80, 24)
	cfg := config.Default()
	cfg.Colors = map[string]string{"accent": "1"}
	m.applyConfig(cfg, "")
	if m.sty.accent != lipgloss.Color("1") {
		t.Fatalf("accent = %v, want override 1", m.sty.accent)
	}
}

func TestKeymapOverride(t *testing.T) {
	m, _ := newTestModel(80, 24)
	cfg := config.Default()
	cfg.Language = "en" // footer labels are asserted in English
	cfg.Keys["new"] = []string{"N"}
	m.applyConfig(cfg, "")

	m, _ = press(m, "n") // no longer bound
	if m.mode != modeNormal {
		t.Fatalf("mode = %v, want normal (n unbound)", m.mode)
	}
	// footer reflects the override
	if got := m.View().Content; !strings.Contains(got, "N new") {
		t.Fatalf("footer missing overridden key:\n%s", got)
	}
	m, _ = press(m, "N")
	if m.mode != modeCreate {
		t.Fatalf("mode = %v, want create (N bound)", m.mode)
	}
}

// --- templates in the TUI ---

func TestCreateChooserIncludesTemplates(t *testing.T) {
	dir := t.TempDir()
	store := template.NewStore(filepath.Join(dir, "templates.json"))
	if err := store.Save(template.Template{
		Name:    "dev",
		Windows: []template.Window{{Name: "editor", Panes: []template.Pane{{Dir: "/x"}}}},
	}); err != nil {
		t.Fatal(err)
	}

	m, _ := newTestModel(80, 24)
	m.templates = store
	m, _ = press(m, "right") // expand $1
	m, _ = press(m, "down")  // @1
	m, _ = press(m, "n")

	items := m.create.items
	if items[len(items)-1].kind != createFromTemplate {
		t.Fatalf("last item kind = %v, want createFromTemplate", items[len(items)-1].kind)
	}
	// default stays on the contextual item, not the template entry
	if m.create.cursor != len(items)-2 {
		t.Fatalf("default cursor = %d, want %d", m.create.cursor, len(items)-2)
	}

	// pick the template entry -> picker appears
	for m.create.cursor != len(items)-1 {
		m, _ = press(m, "down")
	}
	m, _ = press(m, "enter")
	if m.mode != modeMove || m.move == nil || !m.move.isTemplate {
		t.Fatalf("mode = %v, want template picker", m.mode)
	}
	if m.move.labels[0] != "dev (1 window)" {
		t.Fatalf("picker label = %q", m.move.labels[0])
	}

	// confirm: a mutation command is issued (its result depends on the
	// backend honoring NewSessionID; the fake cannot fully support it)
	m, cmd := press(m, "enter")
	if cmd == nil {
		t.Fatal("template create returned no command")
	}
}

func TestTemplateDeleteAndRenameInTUI(t *testing.T) {
	dir := t.TempDir()
	store := template.NewStore(filepath.Join(dir, "templates.json"))
	tpl := func(name string) template.Template {
		return template.Template{Name: name, Windows: []template.Window{{Name: "w"}}}
	}
	if err := store.Save(tpl("dev")); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(tpl("ops")); err != nil {
		t.Fatal(err)
	}
	openPicker := func(m model) model {
		m, _ = press(m, "n")
		for m.create.cursor != len(m.create.items)-1 { // template entry is last
			m, _ = press(m, "down")
		}
		m, _ = press(m, "enter")
		return m
	}

	// delete the first template ("dev") with confirmation
	m, _ := newTestModel(80, 24)
	m.templates = store
	m = openPicker(m)
	m, _ = press(m, "d")
	if m.mode != modeConfirm || !strings.Contains(strings.Join(m.confirm.lines, "\n"), "Delete template 'dev'?") {
		t.Fatalf("mode = %v, confirm = %+v", m.mode, m.confirm)
	}
	m, cmd := press(m, "y")
	if msg := cmd().(mutationMsg); msg.err != nil {
		t.Fatalf("delete failed: %v", msg.err)
	}
	if _, err := store.Get("dev"); err == nil {
		t.Fatal("template 'dev' must be deleted")
	}
	if m.mode != modeNormal {
		t.Fatalf("mode = %v, want normal after delete", m.mode)
	}

	// rename "ops" to "prod": input is prefilled, erase then type
	m = openPicker(m)
	m, _ = press(m, "r")
	if m.mode != modeInput || m.input.Value() != "ops" {
		t.Fatalf("mode = %v, prefill = %q", m.mode, m.input.Value())
	}
	for i := 0; i < 3; i++ {
		m, _ = press(m, "backspace")
	}
	m = typeText(m, "prod")
	m, cmd = press(m, "enter")
	if msg := cmd().(mutationMsg); msg.err != nil {
		t.Fatalf("rename failed: %v", msg.err)
	}
	if _, err := store.Get("prod"); err != nil {
		t.Fatal("template 'prod' must exist after rename")
	}
	if _, err := store.Get("ops"); err == nil {
		t.Fatal("template 'ops' must be gone after rename")
	}

	// rename conflict surfaces an error
	m = openPicker(m)
	m, _ = press(m, "r")
	for i := 0; i < 4; i++ {
		m, _ = press(m, "backspace")
	}
	m = typeText(m, "prod")
	_, cmd = press(m, "enter")
	if msg := cmd().(mutationMsg); msg.err == nil {
		t.Fatal("conflicting rename must fail")
	}
}

// --- directory step of new-session ---

func TestSessionNameForDir(t *testing.T) {
	if got := sessionNameForDir("/a/b.c:d e"); got != "b_c_d_e" {
		t.Fatalf("sessionNameForDir = %q", got)
	}
}

func TestNewSessionDirFlow(t *testing.T) {
	// real directories for the completion engine
	root := t.TempDir()
	for _, d := range []string{"alpha", "beta", ".hidden"} {
		if err := os.Mkdir(filepath.Join(root, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "afile"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	m, fb := newTestModel(80, 24)
	m, _ = press(m, "right") // expand $1
	m, _ = press(m, "down")  // @1
	m, _ = press(m, "right") // expand @1
	m, _ = press(m, "down")  // %1 (cwd /home/u/code)
	m, _ = press(m, "n")
	for m.create.cursor != 0 {
		m, _ = press(m, "up") // "New session"
	}
	m, _ = press(m, "enter")

	// dir input prefilled with the selected pane's cwd
	if m.mode != modeDirPick {
		t.Fatalf("mode = %v, want dir step", m.mode)
	}
	if m.input.Value() != "/home/u/code" {
		t.Fatalf("prefill = %q, want /home/u/code", m.input.Value())
	}

	// completion: "/al" in root matches only alpha (not .hidden or afile)
	m.input.SetValue(root + "/al")
	m.refreshDirCompletions()
	if !reflect.DeepEqual(m.dirPick.matches, []string{"alpha"}) {
		t.Fatalf("matches = %v, want [alpha]", m.dirPick.matches)
	}
	m, _ = press(m, "tab") // complete into the directory
	if m.input.Value() != root+"/alpha/" {
		t.Fatalf("after tab = %q", m.input.Value())
	}

	// accept dir -> name step prefilled with the basename
	m, _ = press(m, "enter")
	if m.mode != modeInput || m.input.Value() != "alpha" {
		t.Fatalf("mode = %v, name prefill = %q", m.mode, m.input.Value())
	}

	// accept name -> session created anchored at the dir, then attached
	m, cmd := press(m, "enter")
	msg, ok := cmd().(dirSessionMsg)
	if !ok || msg.err != nil {
		t.Fatalf("dirSessionMsg = %#v", msg)
	}
	if msg.name != "alpha" || msg.id != "$fake" {
		t.Fatalf("dirSessionMsg = %+v", msg)
	}
	if !reflect.DeepEqual(fb.newSessions, []string{"alpha"}) {
		t.Fatalf("newSessions = %v", fb.newSessions)
	}
	_, attachCmd := m.Update(msg)
	if attachCmd == nil {
		t.Fatal("no attach command after create")
	}
}

func TestDirPickRejectsInvalidPath(t *testing.T) {
	m, _ := newTestModel(80, 24)
	m, _ = press(m, "n")
	for m.create.cursor != 0 {
		m, _ = press(m, "up")
	}
	m, _ = press(m, "enter")
	m.input.SetValue("/nonexistent/xyz")
	m, _ = press(m, "enter")
	if m.mode != modeDirPick {
		t.Fatalf("mode = %v, want to stay in dir step", m.mode)
	}
	if m.status == "" || !m.statusIsErr {
		t.Fatalf("status = %q (err=%v)", m.status, m.statusIsErr)
	}
}

func TestNewWindowDirFlow(t *testing.T) {
	m, fb := newTestModel(80, 24)
	m, _ = press(m, "right") // expand $1
	m, _ = press(m, "down")  // @1
	m, _ = press(m, "right") // expand @1
	m, _ = press(m, "down")  // %1 (cwd /home/u/code)
	m, _ = press(m, "n")
	// choose "New window in 'work'" (index 1)
	for m.create.cursor != 1 {
		if m.create.cursor < 1 {
			m, _ = press(m, "down")
		} else {
			m, _ = press(m, "up")
		}
	}
	m, _ = press(m, "enter")

	// same dir step, prefilled with the pane's cwd
	if m.mode != modeDirPick || m.input.Value() != "/home/u/code" {
		t.Fatalf("mode = %v, prefill = %q", m.mode, m.input.Value())
	}
	dir := t.TempDir()
	m.input.SetValue(dir)
	m, _ = press(m, "enter")

	// name step prefilled with the directory basename
	if m.mode != modeInput || m.input.Value() != filepath.Base(dir) {
		t.Fatalf("mode = %v, name prefill = %q, want %q", m.mode, m.input.Value(), filepath.Base(dir))
	}
	m.input.SetValue("api")
	m, cmd := press(m, "enter")
	if msg := cmd().(mutationMsg); msg.err != nil {
		t.Fatalf("NewWindow failed: %v", msg.err)
	}
	if !reflect.DeepEqual(fb.newWindows, [][2]string{{"$1", "api"}}) {
		t.Fatalf("newWindows = %v, want [[$1 api]]", fb.newWindows)
	}
	if !reflect.DeepEqual(fb.newWindowDirs, []string{dir}) {
		t.Fatalf("newWindowDirs = %v, want [%s]", fb.newWindowDirs, dir)
	}
}

// --- socket switcher ---

func TestSocketSwitchResetsState(t *testing.T) {
	m, fb := newTestModel(80, 24)
	switched := []string{}
	m.backendFor = func(socket string) tmux.Backend {
		switched = append(switched, socket)
		return fb
	}
	m.listSockets = func() ([]string, error) { return []string{"other", "third"}, nil }

	m, _ = press(m, "right") // expand $1
	m.filter = "vim"
	m.rebuild()

	m, _ = press(m, "S")
	if m.mode != modeMove || m.move == nil || !m.move.isSocket {
		t.Fatalf("mode = %v, want socket picker", m.mode)
	}
	if !reflect.DeepEqual(m.move.targets, []string{"other", "third"}) {
		t.Fatalf("targets = %v", m.move.targets)
	}
	m, cmd := press(m, "enter") // cursor 0 = "other"
	if cmd == nil {
		t.Fatal("socket switch returned no fetch command")
	}
	if !reflect.DeepEqual(switched, []string{"other"}) {
		t.Fatalf("backendFor = %v, want [other]", switched)
	}
	if m.socket != "other" {
		t.Fatalf("socket = %q, want other", m.socket)
	}
	if len(m.expanded) != 0 || m.filter != "" || m.cursor != 0 {
		t.Fatalf("state not reset: expanded=%v filter=%q cursor=%d", m.expanded, m.filter, m.cursor)
	}
	if got := m.View().Content; !strings.Contains(got, "socket: other") {
		t.Fatalf("header missing socket name:\n%s", got)
	}

	// the current socket is excluded next time, and "default" (target "")
	// appears only when not on the default socket
	m2, _ := press(m, "S")
	if !reflect.DeepEqual(m2.move.targets, []string{"", "third"}) {
		t.Fatalf("targets after switch = %v", m2.move.targets)
	}
	if m2.move.labels[0] != "default" {
		t.Fatalf("first label = %q, want default", m2.move.labels[0])
	}
}

// --- popup mode (display-popup: quit after successful attach) ---

func TestPopupQuitsAfterAttach(t *testing.T) {
	fb := &fakeBackend{}
	m := newModel(fb, true, true)
	m = apply(m, tea.WindowSizeMsg{Width: 80, Height: 24})
	m = apply(m, treeMsg(testSessions()))

	_, cmd := m.Update(attachMsg{err: nil})
	if cmd == nil {
		t.Fatal("popup attach returned no command")
	}
	if _, isQuit := cmd().(tea.QuitMsg); !isQuit {
		t.Fatal("popup mode must quit after a successful attach")
	}
}

func TestNormalModeStaysAfterAttach(t *testing.T) {
	fb := &fakeBackend{}
	m := newModel(fb, true, false)
	m = apply(m, tea.WindowSizeMsg{Width: 80, Height: 24})
	m = apply(m, treeMsg(testSessions()))

	_, cmd := m.Update(attachMsg{err: nil})
	if cmd == nil {
		t.Fatal("attach returned no command")
	}
	if _, isQuit := cmd().(tea.QuitMsg); isQuit {
		t.Fatal("normal mode must keep running after attach")
	}
	// a failed attach in popup mode must also keep running and show the error
	m.popup = true
	m2, cmd2 := m.Update(attachMsg{err: errTest})
	m = m2.(model)
	if _, isQuit := cmd2().(tea.QuitMsg); isQuit {
		t.Fatal("popup mode must not quit on a failed attach")
	}
	if m.status == "" || !m.statusIsErr {
		t.Fatalf("status = %q (err=%v), want error surfaced", m.status, m.statusIsErr)
	}
}

func TestPopupDefaultsEnablePreview(t *testing.T) {
	fb := &fakeBackend{}
	m := newModel(fb, true, true)
	m.applyPopupDefaults()
	if !m.previewOn {
		t.Fatal("popup mode must default the preview on")
	}
}

func TestPopupFocusesCurrentSession(t *testing.T) {
	fb := &fakeBackend{currentSession: "$2"}
	m := newModel(fb, true, true)
	m = apply(m, tea.WindowSizeMsg{Width: 80, Height: 24})
	m = apply(m, treeMsg(testSessions()))
	m = apply(m, m.focusCurrent()) // delivers focusMsg{"$2"}
	// focus arrived after the tree: applied immediately
	if got := m.rows[m.cursor].id; got != "$2" {
		t.Fatalf("focused row = %s, want $2", got)
	}
	assertIDs(t, m, []string{"$1", "$2", "@3"})

	// and when the focus arrives before the tree it stays pending
	m2 := newModel(fb, true, true)
	m2 = apply(m2, tea.WindowSizeMsg{Width: 80, Height: 24})
	m2 = apply(m2, m2.focusCurrent())
	m2 = apply(m2, treeMsg(testSessions()))
	if got := m2.rows[m2.cursor].id; got != "$2" {
		t.Fatalf("pending-focused row = %s, want $2", got)
	}
}

func TestPopupEscQuits(t *testing.T) {
	fb := &fakeBackend{}
	m := newModel(fb, true, true)
	m = apply(m, tea.WindowSizeMsg{Width: 80, Height: 24})
	m = apply(m, treeMsg(testSessions()))

	_, cmd := press(m, "esc")
	if cmd == nil {
		t.Fatal("esc in popup mode returned no command")
	}
	if _, isQuit := cmd().(tea.QuitMsg); !isQuit {
		t.Fatal("esc in popup mode must quit")
	}

	// with an active filter, esc clears the filter instead of quitting
	m.filter = "vim"
	m.rebuild()
	m, cmd = press(m, "esc")
	if m.filter != "" {
		t.Fatal("esc must clear the filter first")
	}
	if cmd != nil {
		if _, isQuit := cmd().(tea.QuitMsg); isQuit {
			t.Fatal("esc with active filter must not quit")
		}
	}
}

// --- end-to-end: real Program, scripted keystrokes, real renderer ---

func TestProgramSmoke(t *testing.T) {
	fb := &fakeBackend{captureContent: "line one\nline two"}
	fb.sessions = testSessions()

	// right (expand $1), down (@1), ? (help), x (close help), q (quit)
	input := strings.NewReader("\x1b[C\x1b[B?xq")
	var out bytes.Buffer

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// preload the tree before Run so scripted keys act on loaded rows
	// (in the real program the fetch races the first keystrokes)
	m := newModel(fb, true, false)
	m = apply(m, tea.WindowSizeMsg{Width: 80, Height: 24})
	m = apply(m, treeMsg(fb.sessions))

	p := tea.NewProgram(m,
		tea.WithContext(ctx),
		tea.WithInput(input),
		tea.WithOutput(&out),
		tea.WithWindowSize(80, 24),
	)
	final, err := p.Run()
	if err != nil {
		t.Fatalf("program run: %v", err)
	}
	m, ok := final.(model)
	if !ok {
		t.Fatalf("final model type = %T, want model", final)
	}
	if !m.expanded["$1"] {
		t.Fatalf("expanded = %v, want $1 expanded", m.expanded)
	}
	if !strings.Contains(out.String(), "tmuxgo") {
		t.Fatalf("rendered output missing header:\n%q", out.String())
	}
}

// --- i18n ---

func TestSettingsLanguageCyclesLive(t *testing.T) {
	m, _ := newTestModel(80, 24)
	m, _ = press(m, ",")
	m, _ = press(m, "down") // language row
	m, _ = press(m, "enter")
	if m.cfg.Language != "en" || m.lang != i18n.EN {
		t.Fatalf("language = %q lang = %q", m.cfg.Language, m.lang)
	}
	m, _ = press(m, "enter")
	if m.cfg.Language != "zh" || m.lang != i18n.ZH {
		t.Fatalf("language = %q lang = %q", m.cfg.Language, m.lang)
	}
	// the settings screen itself re-renders in Chinese
	if got := m.View().Content; !strings.Contains(got, "语言") {
		t.Fatalf("settings screen not in Chinese:\n%s", got)
	}
	m, _ = press(m, "left") // zh -> en
	if m.cfg.Language != "en" || m.lang != i18n.EN {
		t.Fatalf("language = %q lang = %q", m.cfg.Language, m.lang)
	}
}

func TestChineseViewFitsWidth(t *testing.T) {
	for _, w := range []int{40, 80, 140} {
		m, _ := newTestModel(w, 24)
		m.lang = i18n.ZH
		for i, line := range strings.Split(m.View().Content, "\n") {
			if got := lipgloss.Width(line); got > w {
				t.Fatalf("w=%d line %d is %d cells: %q", w, i, got, line)
			}
		}
	}
}

func TestChineseFooterAndDialogs(t *testing.T) {
	m, _ := newTestModel(80, 24)
	m.lang = i18n.ZH
	if got := m.View().Content; !strings.Contains(got, "n 新建") {
		t.Fatalf("footer not in Chinese:\n%s", got)
	}
	// kill confirm dialog in Chinese
	m, _ = press(m, "d")
	if got := m.View().Content; !strings.Contains(got, "删除会话") {
		t.Fatalf("confirm dialog not in Chinese:\n%s", got)
	}
	// header counts use Chinese units
	m, _ = press(m, "n") // cancel confirm
	if got := m.View().Content; !strings.Contains(got, "个会话") {
		t.Fatalf("header not in Chinese:\n%s", got)
	}
}

// --- clickable footer button bar ---

// barButtonY returns the absolute y of the middle (label) row of button
// band r in the footer.
func barButtonY(m model, r int) int {
	return m.height - m.footerHeight() + r*3 + 1
}

func TestFooterButtonsLayout(t *testing.T) {
	m, _ := newTestModel(100, 24)
	btns := m.footerButtons(100)
	if len(btns) != 9 {
		t.Fatalf("buttons = %d, want 9", len(btns))
	}
	if btns[0].start != 0 || btns[0].row != 0 {
		t.Fatalf("first button at start %d row %d", btns[0].start, btns[0].row)
	}
	for i := 1; i < len(btns); i++ {
		if btns[i].row != btns[i-1].row {
			if btns[i].start != 0 {
				t.Fatalf("button %d wraps but does not start at 0", i)
			}
			continue
		}
		if btns[i].start != btns[i-1].end+1 {
			t.Fatalf("button %d not spaced: prev end %d, start %d", i, btns[i-1].end, btns[i].start)
		}
	}
	for _, b := range btns {
		if b.end > 100 {
			t.Fatalf("button %q overflows width (end %d)", b.label, b.end)
		}
	}
	// a narrow width wraps buttons to more bands but never drops them
	narrow := m.footerButtons(30)
	if len(narrow) != 9 {
		t.Fatalf("narrow buttons = %d, want all 9 wrapped", len(narrow))
	}
	rows := 1
	for _, b := range narrow {
		if b.end > 30 {
			t.Fatalf("button %q overflows width (end %d)", b.label, b.end)
		}
		if b.row+1 > rows {
			rows = b.row + 1
		}
	}
	if rows < 2 {
		t.Fatal("narrow width must wrap to multiple bands")
	}
	// an active filter shifts the first button right
	m.filter = "wo"
	if withFilter := m.footerButtons(100); withFilter[0].start == 0 {
		t.Fatal("filter prefix must shift buttons right")
	}
}

func TestFooterClickDispatches(t *testing.T) {
	m, _ := newTestModel(100, 24)
	btns := m.footerButtons(100)
	help := -1
	for i, b := range btns {
		if b.act == actHelp {
			help = i
		}
	}
	if help < 0 {
		t.Fatal("no help button")
	}
	m = apply(m, click(btns[help].start+2, barButtonY(m, btns[help].row)))
	if m.mode != modeHelp {
		t.Fatalf("mode = %v, want help after footer click", m.mode)
	}
	// clicking the top border row of a button box also dispatches
	m, _ = newTestModel(100, 24)
	m = apply(m, click(btns[help].start+2, barButtonY(m, btns[help].row)-1))
	if m.mode != modeHelp {
		t.Fatalf("mode = %v, want help after box-border click", m.mode)
	}
	// clicking the gap between buttons does nothing
	m, _ = newTestModel(100, 24)
	m = apply(m, click(btns[0].end, barButtonY(m, 0)))
	if m.mode != modeNormal {
		t.Fatalf("gap click changed mode to %v", m.mode)
	}
	// the quit button returns tea.Quit
	m, _ = newTestModel(100, 24)
	quit := btns[len(btns)-1]
	if quit.act != actQuit {
		t.Fatalf("last button = %v, want quit", quit.act)
	}
	_, cmd := m.handleMouseClick(tea.Mouse(click(quit.start+2, barButtonY(m, quit.row))))
	if cmd == nil {
		t.Fatal("quit click returned no command")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatalf("quit click cmd = %T, want tea.QuitMsg", cmd())
	}
}

func TestFooterHoverHighlights(t *testing.T) {
	m, _ := newTestModel(80, 24)
	btns := m.footerButtons(80)
	m = apply(m, tea.MouseMotionMsg(tea.Mouse{X: btns[1].start + 2, Y: barButtonY(m, btns[1].row), Button: tea.MouseNone}))
	if m.hoverFooter != 1 {
		t.Fatalf("hoverFooter = %d, want 1", m.hoverFooter)
	}
	if out := m.View().Content; !strings.Contains(out, "rename") {
		t.Fatal("footer missing rename label")
	}
	// moving off the bar clears the hover
	m = apply(m, tea.MouseMotionMsg(tea.Mouse{X: 5, Y: 5, Button: tea.MouseNone}))
	if m.hoverFooter != -1 {
		t.Fatalf("hoverFooter = %d, want -1", m.hoverFooter)
	}
}

func TestFooterButtonMotionIsNotHover(t *testing.T) {
	// a left-button motion is a potential drag, never a footer hover
	m, _ := newTestModel(80, 24)
	btns := m.footerButtons(80)
	m = apply(m, motion(btns[1].start+2, barButtonY(m, btns[1].row)))
	if m.hoverFooter != -1 {
		t.Fatal("button motion must not set hover")
	}
}

// --- dialog mouse support ---

func hover(x, y int) tea.MouseMotionMsg {
	return tea.MouseMotionMsg(tea.Mouse{X: x, Y: y, Button: tea.MouseNone})
}

func rightClick(x, y int) tea.MouseClickMsg {
	return tea.MouseClickMsg(tea.Mouse{X: x, Y: y, Button: tea.MouseRight})
}

// TestCenteredBoxGeometryMatchesRender pins the hit-test geometry to what
// lipgloss.Place actually draws: the selected settings row's ▸ marker must
// land exactly where centeredBoxGeometry predicts.
func TestCenteredBoxGeometryMatchesRender(t *testing.T) {
	m, _ := newTestModel(80, 24)
	m.mode = modeSettings
	w, bodyH := m.renderSize()
	g := centeredBoxGeometry(w, bodyH, m.settingsLines(w))

	lines := strings.Split(ansi.Strip(m.View().Content), "\n")
	foundY, foundX := -1, -1
	for i, l := range lines {
		if j := strings.Index(l, "▸"); j >= 0 {
			foundY, foundX = i, lipgloss.Width(l[:j])
			break
		}
	}
	if wantY := g.top + 1 + 2; foundY != wantY { // border, title, blank
		t.Fatalf("selected row at y=%d, geometry predicted %d", foundY, wantY)
	}
	if wantX := g.left + 3; foundX != wantX { // border + padding
		t.Fatalf("selected marker at x=%d, geometry predicted %d", foundX, wantX)
	}
}

func TestSettingsMouseClickChangesValue(t *testing.T) {
	m, _ := newTestModel(80, 24)
	m.mode = modeSettings
	w, bodyH := m.renderSize()
	g := centeredBoxGeometry(w, bodyH, m.settingsLines(w))
	rowY := func(i int) int { return g.top + 1 + 2 + i }
	x := g.left + 5

	// left-click the language row: selects it and cycles forward (Enter)
	before := m.cfg.Language
	m = apply(m, click(x, rowY(1)))
	if m.settingsCursor != 1 {
		t.Fatalf("settingsCursor = %d, want 1", m.settingsCursor)
	}
	if m.cfg.Language == before {
		t.Fatal("click did not change the language")
	}
	// right-click cycles back (Left)
	m = apply(m, rightClick(x, rowY(1)))
	if m.cfg.Language != before {
		t.Fatalf("right-click did not cycle back: %q, want %q", m.cfg.Language, before)
	}
	// boolean rows toggle on click
	prev := m.cfg.PreviewDefault
	m = apply(m, click(x, rowY(2)))
	if m.cfg.PreviewDefault == prev {
		t.Fatal("click did not toggle the preview default")
	}
}

func TestSettingsMouseHoverAndWheel(t *testing.T) {
	m, _ := newTestModel(80, 24)
	m.mode = modeSettings
	w, bodyH := m.renderSize()
	g := centeredBoxGeometry(w, bodyH, m.settingsLines(w))

	m = apply(m, hover(g.left+5, g.top+1+2+3)) // hover the mouse row
	if m.settingsCursor != 3 {
		t.Fatalf("hover settingsCursor = %d, want 3", m.settingsCursor)
	}
	m = apply(m, tea.MouseWheelMsg(tea.Mouse{X: 5, Y: 5, Button: tea.MouseWheelUp}))
	if m.settingsCursor != 2 {
		t.Fatalf("wheel settingsCursor = %d, want 2", m.settingsCursor)
	}
}

func TestSettingsMouseClickOutsideSavesAndCloses(t *testing.T) {
	m, _ := newTestModel(80, 24)
	m.mode = modeSettings
	m = apply(m, click(0, 0)) // header, outside the box
	if m.mode != modeNormal {
		t.Fatalf("mode = %v, want normal after outside click", m.mode)
	}
}

func TestCreateMouseClickChoosesItem(t *testing.T) {
	m, fb := newTestModel(80, 24)
	m, _ = press(m, "right") // expand $1
	m, _ = press(m, "down")  // @1
	m, _ = press(m, "n")     // items: session, window, split (cursor on split)
	w, bodyH := m.renderSize()
	g := centeredBoxGeometry(w, bodyH, m.createLines(w))

	// hover preselects without confirming
	m = apply(m, hover(g.left+4, g.top+1+2+0))
	if m.create.cursor != 0 {
		t.Fatalf("hover create.cursor = %d, want 0", m.create.cursor)
	}
	// click the split item: same as Enter on it
	_, cmd := m.Update(click(g.left+4, g.top+1+2+2))
	if cmd == nil {
		t.Fatal("click on split item returned no command")
	}
	if msg := cmd().(mutationMsg); msg.err != nil {
		t.Fatalf("split failed: %v", msg.err)
	}
	if !reflect.DeepEqual(fb.splits, []string{"%1"}) {
		t.Fatalf("splits = %v, want [%%1]", fb.splits)
	}
}

func TestCreateMouseClickOutsideCancels(t *testing.T) {
	m, _ := newTestModel(80, 24)
	m, _ = press(m, "n")
	m = apply(m, click(0, 0))
	if m.mode != modeNormal || m.create != nil {
		t.Fatalf("mode = %v create = %v, want cancelled", m.mode, m.create)
	}
}

func TestMoveMouseClickChoosesTarget(t *testing.T) {
	m, fb := newTestModel(80, 24)
	m, _ = press(m, "right") // expand $1
	m, _ = press(m, "down")  // @1
	m, _ = press(m, "m")     // move @1: [0] = new session, [1] = personal
	w, bodyH := m.renderSize()
	g := centeredBoxGeometry(w, bodyH, m.moveLines(w))

	_, cmd := m.Update(click(g.left+4, g.top+1+2+1)) // "personal"
	if cmd == nil {
		t.Fatal("click on target returned no command")
	}
	if msg := cmd().(mutationMsg); msg.err != nil {
		t.Fatalf("move failed: %v", msg.err)
	}
	if !reflect.DeepEqual(fb.movedWindows, [][2]string{{"@1", "$2"}}) {
		t.Fatalf("movedWindows = %v, want [[@1 $2]]", fb.movedWindows)
	}
}

func TestConfirmMouseButtons(t *testing.T) {
	m, fb := newTestModel(80, 24)
	m, _ = press(m, "d") // delete session $1
	w, bodyH := m.renderSize()
	lines := m.confirmLines(w)
	g := centeredBoxGeometry(w, bodyH, lines)
	hintY := g.top + 1 + len(lines) - 1
	hint := m.tr(i18n.ConfirmHint)
	nCol := g.left + 3 + lipgloss.Width(hint[:strings.Index(hint, "[n/esc]")]) + 1

	// [y] confirm zone executes the kill
	_, cmd := m.Update(click(g.left+3+1, hintY))
	if cmd == nil {
		t.Fatal("click on [y] returned no command")
	}
	if msg := cmd().(mutationMsg); msg.err != nil {
		t.Fatalf("kill failed: %v", msg.err)
	}
	if !reflect.DeepEqual(fb.killed, []string{"$1"}) {
		t.Fatalf("killed = %v, want [$1]", fb.killed)
	}

	// [n/esc] cancel zone aborts
	m, _ = press(m, "d")
	m = apply(m, click(nCol, hintY))
	if m.mode != modeNormal || m.confirm != nil {
		t.Fatalf("mode = %v, want cancelled", m.mode)
	}
	if m.status != m.tr(i18n.Cancelled) {
		t.Fatalf("status = %q, want cancelled", m.status)
	}
	if len(fb.killed) != 1 {
		t.Fatalf("killed = %v, cancel must not kill", fb.killed)
	}

	// a click outside the box also cancels (never confirms)
	m, _ = press(m, "d")
	m = apply(m, click(0, 0))
	if m.mode != modeNormal || m.confirm != nil {
		t.Fatalf("mode = %v, want cancelled on outside click", m.mode)
	}
	if len(fb.killed) != 1 {
		t.Fatalf("killed = %v, outside click must not kill", fb.killed)
	}
}

func TestHelpMouseClickCloses(t *testing.T) {
	m, _ := newTestModel(80, 24)
	m, _ = press(m, "?")
	if m.mode != modeHelp {
		t.Fatalf("mode = %v, want help", m.mode)
	}
	m = apply(m, click(40, 12))
	if m.mode != modeNormal {
		t.Fatalf("mode = %v, want normal after click", m.mode)
	}
}

func TestDirPickMouseCompleteAndAccept(t *testing.T) {
	root := t.TempDir()
	for _, d := range []string{"alpha", "beta"} {
		if err := os.Mkdir(filepath.Join(root, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	m, _ := newTestModel(80, 24)
	m, _ = press(m, "n")
	for m.create.cursor != 0 {
		m, _ = press(m, "up") // "New session"
	}
	m, _ = press(m, "enter")
	if m.mode != modeDirPick {
		t.Fatalf("mode = %v, want dir step", m.mode)
	}
	m.input.SetValue(root + "/")
	m.refreshDirCompletions() // matches: alpha (y=3), beta (y=4)

	// hover preselects
	m = apply(m, hover(2, 4))
	if m.dirPick.cursor != 1 {
		t.Fatalf("hover dirPick.cursor = %d, want 1", m.dirPick.cursor)
	}
	// single click completes into the directory (Tab)
	m = apply(m, click(2, 4))
	if m.input.Value() != root+"/beta/" {
		t.Fatalf("click complete = %q, want %q", m.input.Value(), root+"/beta/")
	}
	// a quick second click on the same row accepts the directory (Enter)
	m = apply(m, click(2, 4))
	if m.mode != modeInput {
		t.Fatalf("double-click accept: mode = %v, want name step", m.mode)
	}
	if m.input.Value() != "beta" {
		t.Fatalf("name prefill = %q, want beta", m.input.Value())
	}
	if m.pendingDir != root+"/beta/" {
		t.Fatalf("pendingDir = %q, want %q", m.pendingDir, root+"/beta/")
	}
}

// --- preview: window-level sections and sizing ---

func TestPreviewWClamp(t *testing.T) {
	if got := previewW(104); got != 41 {
		t.Fatalf("previewW(104) = %d, want 41", got)
	}
	if got := previewW(200); got != 56 {
		t.Fatalf("previewW(200) = %d, want 56 (capped)", got)
	}
	if got := previewW(40); got != 20 {
		t.Fatalf("previewW(40) = %d, want 20 (floor)", got)
	}
}

func TestPreviewMultiPaneSections(t *testing.T) {
	m, _ := newTestModel(120, 24)
	m.previewOn = true
	m, _ = press(m, "right") // expand $1
	m, _ = press(m, "down")  // @1 (two panes)
	if win := m.previewWindow(); win == nil || win.ID != "@1" {
		t.Fatalf("previewWindow = %v, want @1", m.previewWindow())
	}
	// deliver both panes' content via the fetch command
	m, cmd := press(m, "p") // toggles off
	m, cmd = press(m, "p")  // and back on, returning a fetch cmd
	if cmd == nil {
		t.Fatal("no fetch command")
	}
	m = apply(m, cmd())
	if m.previewCache["%1"] == "" || m.previewCache["%2"] == "" {
		t.Fatalf("cache missing panes: %v", m.previewCache)
	}
	// give the panes distinguishable content
	m.previewCache["%1"] = "vim-content"
	m.previewCache["%2"] = "bash-content"
	got := ansi.Strip(m.View().Content)
	if !strings.Contains(got, "vim-content") || !strings.Contains(got, "bash-content") {
		t.Fatalf("multi-pane preview missing content:\n%s", got)
	}
	if !strings.Contains(got, "─") || !strings.Contains(got, "%2 bash") {
		t.Fatalf("preview missing labeled divider:\n%s", got)
	}
	if !strings.Contains(got, "preview @1") {
		t.Fatalf("preview title should be the window:\n%s", got)
	}
}

func TestPreviewSinglePaneNoDivider(t *testing.T) {
	m, _ := newTestModel(120, 24)
	m.previewOn = true
	// session $2 has one window with one pane
	m, _ = press(m, "down") // $2
	m.previewCache["%4"] = "mutt-content"
	got := ansi.Strip(m.View().Content)
	if !strings.Contains(got, "mutt-content") {
		t.Fatalf("single-pane preview missing content:\n%s", got)
	}
	if strings.Contains(got, "%4 mutt ─") {
		t.Fatal("single pane must not render a divider")
	}
}
