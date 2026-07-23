package app

import (
	"bytes"
	"context"
	"os/exec"
	"reflect"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/DKmiyan/tmuxgo/internal/tmux"
)

// --- fake backend ---

type fakeBackend struct {
	sessions []tmux.Session
	treeErr  error

	captureContent string
	captureErr     error

	newSessions  []string
	newWindows   [][2]string
	splits       []string
	renamedS     map[string]string
	renamedW     map[string]string
	movedWindows [][2]string
	movedPanes   [][2]string
	killed       []string
}

func (f *fakeBackend) Tree() ([]tmux.Session, error) { return f.sessions, f.treeErr }

func (f *fakeBackend) NewSession(name string) error {
	f.newSessions = append(f.newSessions, name)
	return nil
}

func (f *fakeBackend) NewWindow(sessionID, name string) error {
	f.newWindows = append(f.newWindows, [2]string{sessionID, name})
	return nil
}

func (f *fakeBackend) SplitPane(paneID string) error {
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

func (f *fakeBackend) CapturePane(paneID string, lines int) (string, error) {
	return f.captureContent, f.captureErr
}

func (f *fakeBackend) AttachCmd(target string) *exec.Cmd {
	return exec.Command("true")
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
	if m.mode != modeInput {
		t.Fatalf("mode = %v, want input", m.mode)
	}
	m = typeText(m, "demo")
	m, cmd := press(m, "enter")
	if cmd == nil {
		t.Fatal("new session returned no command")
	}
	if msg := cmd().(mutationMsg); msg.err != nil {
		t.Fatalf("NewSession failed: %v", msg.err)
	}
	if !reflect.DeepEqual(fb.newSessions, []string{"demo"}) {
		t.Fatalf("newSessions = %v, want [demo]", fb.newSessions)
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
	// only the other session is offered
	if !reflect.DeepEqual(m.move.targets, []string{"$2"}) {
		t.Fatalf("targets = %v, want [$2]", m.move.targets)
	}
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
	// every window except @1
	if !reflect.DeepEqual(m.move.targets, []string{"@2", "@3"}) {
		t.Fatalf("targets = %v, want [@2 @3]", m.move.targets)
	}
	m, _ = press(m, "down") // select @3
	m, cmd := press(m, "enter")
	if msg := cmd().(mutationMsg); msg.err != nil {
		t.Fatalf("move failed: %v", msg.err)
	}
	if !reflect.DeepEqual(fb.movedPanes, [][2]string{{"%1", "@3"}}) {
		t.Fatalf("movedPanes = %v, want [[%%1 @3]]", fb.movedPanes)
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
	m = apply(m, click(5, 2)) // second row ($2); header is line 0
	if m.cursor != 1 {
		t.Fatalf("cursor = %d, want 1", m.cursor)
	}
}

func TestMouseMarkerTogglesExpand(t *testing.T) {
	m, _ := newTestModel(80, 24)
	m = apply(m, click(0, 1)) // marker of $1
	assertIDs(t, m, []string{"$1", "@1", "@2", "$2"})
	m = apply(m, click(0, 1)) // collapse again
	assertIDs(t, m, []string{"$1", "$2"})
}

func TestMouseDoubleClickAttaches(t *testing.T) {
	m, _ := newTestModel(80, 24)
	m = apply(m, click(5, 1))
	_, cmd := m.Update(click(5, 1))
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
