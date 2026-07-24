// Package i18n holds tmuxgo's user-visible strings in English and Chinese.
// Chinese is compiled into the binary; no language packs are needed.
package i18n

import (
	"fmt"
	"strings"
)

// Lang is a resolved interface language.
type Lang string

const (
	EN Lang = "en"
	ZH Lang = "zh"
)

// Resolve maps a configured language ("auto", "en", "zh") to a concrete
// one. "auto" (and anything unrecognized) follows LC_ALL/LC_CTYPE/LANG.
func Resolve(cfg string, getenv func(string) string) Lang {
	switch strings.ToLower(strings.TrimSpace(cfg)) {
	case "en":
		return EN
	case "zh":
		return ZH
	}
	for _, key := range []string{"LC_ALL", "LC_CTYPE", "LANG"} {
		if strings.HasPrefix(strings.ToLower(getenv(key)), "zh") {
			return ZH
		}
	}
	return EN
}

// ID identifies a message in the table.
type ID int

const (
	// units (used by Plural)
	UnitSession ID = iota
	UnitWindow
	UnitPane

	// footer hints per mode
	FooterApplyCancel
	FooterChooseConfirm
	FooterTemplate
	FooterConfirmYN
	FooterDirPick
	FooterHelpClose
	FooterSettings
	FilterActive

	// normal-mode footer action labels
	ActNew
	ActRename
	ActMove
	ActKill
	ActFilter
	ActPreview
	ActHelp
	ActSettings
	ActQuit

	// tree body
	Loading
	NoSessions
	NoMatches
	SessionMeta

	// help screen
	HelpTitle
	HelpUp
	HelpDown
	HelpExpand
	HelpCollapse
	HelpAttach
	HelpNew
	HelpRename
	HelpMove
	HelpKill
	HelpFilter
	HelpPreview
	HelpSettings
	HelpSocket
	HelpHelp
	HelpQuit
	HelpMouse1
	HelpMouse2
	HelpMouse3
	HelpEscClear

	// settings screen
	SettingsTitle
	SetTheme
	SetLanguage
	SetPreviewStart
	SetMouse
	On
	Off
	SettingsHint
	SettingsKeysIn
	SettingsSaved

	// confirm dialog
	ConfirmHint

	// create menu
	CreateTitle
	NewSession
	NewWindowIn
	SplitPaneIn
	NewFromTemplate

	// rename
	CannotRenamePane
	PromptRenameSession
	PromptRenameWindow
	PromptRenameTemplate

	// move picker
	MoveWindowTitle
	NewSessionItem
	MovePaneTitle
	NewWindowInItem
	SessionsCannotMove

	// templates
	NoTemplates
	TemplatePickerTitle
	DeleteTemplateQ
	DeleteTemplateNote
	TemplateDeleted
	TemplateRenamed
	SessionFromTemplate

	// socket switcher
	SocketUnavailable
	SocketPickerTitle
	SocketDefault
	NoOtherSockets
	SocketNowDefault

	// kill confirmations
	KillSessionQ
	WillBeKilled2
	SessionKilled
	KillWindowQ
	WillBeKilled1
	LastWindowCascade
	WindowKilled
	KillPaneQ
	LastPaneCascade
	AlsoLastWindowCascade
	PaneKilled

	// status messages and prompts
	SessionCreated
	AttachFailed
	ReleaseToMove
	InvalidDrop
	MoveLabelWinSess
	MoveLabelPaneWin
	MovedX
	FilterPlaceholder
	PreviewTooNarrow
	WindowCreated
	SessionRenamed
	WindowRenamed
	NameEmpty
	WindowMovedNewSession
	PromptSessionName
	PromptWindowName
	PromptNewSessionAuto
	PromptDir
	PaneSplit
	Cancelled
	PaneMovedNewWindow
	WindowMoved
	PaneMoved
	NotADirectory
	UnknownTheme
	DirPickTitle
	DirPickHint
	DirPickNone

	// CLI (main package)
	Usage
	NoSessionsCLI
	AttachedMark
	ListLine
	SetupWrote
	SetupUpToDate
	SetupPopupHint
	NoTemplatesCLI
	TemplateDeletedCLI
	TemplateSavedCLI
	SessionNotFound
	UnknownCommand
	UnknownTemplateCmd

	idCount
)

// table maps a message ID to its {en, zh} forms. %verbs are fmt verbs.
var table = map[ID][2]string{
	UnitSession: {"session", "会话"},
	UnitWindow:  {"window", "窗口"},
	UnitPane:    {"pane", "面板"},

	FooterApplyCancel:   {"enter apply · esc cancel", "enter 确认 · esc 取消"},
	FooterChooseConfirm: {"↑/↓ choose · enter confirm · esc cancel", "↑/↓ 选择 · enter 确认 · esc 取消"},
	FooterTemplate:      {"↑/↓ choose · enter create · d delete · r rename · esc cancel", "↑/↓ 选择 · enter 创建 · d 删除 · r 改名 · esc 取消"},
	FooterConfirmYN:     {"y confirm · n cancel", "y 确认 · n 取消"},
	FooterDirPick:       {"tab/→ complete · ↑/↓ choose · enter accept · esc cancel", "tab/→ 补全 · ↑/↓ 选择 · enter 接受 · esc 取消"},
	FooterHelpClose:     {"any key to close", "按任意键关闭"},
	FooterSettings:      {"↑/↓ move · enter change · esc save & close", "↑/↓ 移动 · enter 修改 · esc 保存并关闭"},
	FilterActive:        {"filter: %q (esc clears) · %s", "过滤: %q (esc 清除) · %s"},

	ActNew:      {"new", "新建"},
	ActRename:   {"rename", "改名"},
	ActMove:     {"move", "移动"},
	ActKill:     {"kill", "删除"},
	ActFilter:   {"filter", "过滤"},
	ActPreview:  {"preview", "预览"},
	ActHelp:     {"help", "帮助"},
	ActSettings: {"settings", "设置"},
	ActQuit:     {"quit", "退出"},

	Loading:     {"loading…", "加载中…"},
	NoSessions:  {"No tmux sessions. Press n to create one.", "没有 tmux 会话。按 n 新建一个。"},
	NoMatches:   {"No matches for %q.", "没有匹配 %q 的结果。"},
	SessionMeta: {"%s · %s ago", "%s · %s 前"},

	HelpTitle:    {"keys", "按键"},
	HelpUp:       {"move up", "上移"},
	HelpDown:     {"move down", "下移"},
	HelpExpand:   {"expand / enter first child", "展开 / 进入第一个子项"},
	HelpCollapse: {"collapse / go to parent", "折叠 / 返回父级"},
	HelpAttach:   {"attach to selected session/window/pane", "attach 到选中的会话/窗口/面板"},
	HelpNew:      {"new session / window / split pane / from template", "新建会话 / 窗口 / 拆分面板 / 从模板创建"},
	HelpRename:   {"rename session/window", "会话/窗口改名"},
	HelpMove:     {"move window/pane", "移动窗口/面板"},
	HelpKill:     {"kill session/window/pane (confirms first)", "删除会话/窗口/面板（先确认）"},
	HelpFilter:   {"filter", "过滤"},
	HelpPreview:  {"toggle pane preview (wide terminals)", "切换面板预览（宽终端）"},
	HelpSettings: {"settings", "设置"},
	HelpSocket:   {"switch tmux server socket", "切换 tmux server socket"},
	HelpHelp:     {"this help", "本帮助"},
	HelpQuit:     {"quit", "退出"},
	HelpMouse1:   {"click select, marker click expands,", "点击选中，点击 ▸/▾ 标记展开，"},
	HelpMouse2:   {"double-click attaches, wheel scrolls,", "双击 attach，滚轮滚动，"},
	HelpMouse3:   {"drag window/pane to move it", "拖拽窗口/面板即可移动"},
	HelpEscClear: {"clear filter", "清除过滤"},

	SettingsTitle:   {"settings", "设置"},
	SetTheme:        {"theme", "主题"},
	SetLanguage:     {"language", "语言"},
	SetPreviewStart: {"preview on start", "启动时预览"},
	SetMouse:        {"mouse", "鼠标"},
	On:              {"on", "开"},
	Off:             {"off", "关"},
	SettingsHint:    {"enter/space/arrows change, esc saves", "enter/space/方向键 修改，esc 保存"},
	SettingsKeysIn:  {"keys are configurable in:", "按键可在以下文件中配置："},
	SettingsSaved:   {"settings saved", "设置已保存"},

	ConfirmHint: {"[y] confirm    [n/esc] cancel", "[y] 确认    [n/esc] 取消"},

	CreateTitle:     {"create", "新建"},
	NewSession:      {"New session", "新建会话"},
	NewWindowIn:     {"New window in '%s'", "在 '%s' 中新建窗口"},
	SplitPaneIn:     {"Split pane in '%s'", "在 '%s' 中拆分面板"},
	NewFromTemplate: {"New session from template…", "从模板新建会话…"},

	CannotRenamePane:     {"cannot rename a pane", "面板不能改名"},
	PromptRenameSession:  {"rename session: ", "会话改名: "},
	PromptRenameWindow:   {"rename window: ", "窗口改名: "},
	PromptRenameTemplate: {"rename template: ", "模板改名: "},

	MoveWindowTitle:    {"Move window '%s' to session:", "移动窗口 '%s' 到会话:"},
	NewSessionItem:     {"+ New session", "+ 新建会话"},
	MovePaneTitle:      {"Move pane '%s' to window:", "移动面板 '%s' 到窗口:"},
	NewWindowInItem:    {"+ New window in '%s'", "+ 在 '%s' 中新建窗口"},
	SessionsCannotMove: {"sessions cannot be moved", "会话不能移动"},

	NoTemplates:         {"no templates saved", "还没有保存的模板"},
	TemplatePickerTitle: {"New session from template:", "从模板新建会话:"},
	DeleteTemplateQ:     {"Delete template '%s'?", "删除模板 '%s'？"},
	DeleteTemplateNote:  {"The JSON entry is removed; tmux sessions are unaffected.", "仅删除 JSON 记录；tmux 会话不受影响。"},
	TemplateDeleted:     {"template '%s' deleted", "模板 '%s' 已删除"},
	TemplateRenamed:     {"template renamed", "模板已改名"},
	SessionFromTemplate: {"session created from template '%s'", "已从模板 '%s' 创建会话"},

	SocketUnavailable: {"socket switching unavailable", "无法切换 socket"},
	SocketPickerTitle: {"Switch tmux server socket:", "切换 tmux server socket:"},
	SocketDefault:     {"default", "默认"},
	NoOtherSockets:    {"no other tmux server sockets", "没有其他 tmux server socket"},
	SocketNowDefault:  {"socket: default", "socket: 默认"},

	KillSessionQ:          {"Kill session '%s'?", "删除会话 '%s'？"},
	WillBeKilled2:         {"%s and %s will be killed.", "%s 和 %s 将被一并删除。"},
	SessionKilled:         {"session killed", "会话已删除"},
	KillWindowQ:           {"Kill window '%s'?", "删除窗口 '%s'？"},
	WillBeKilled1:         {"%s will be killed.", "%s 将被一并删除。"},
	LastWindowCascade:     {"This is the last window in '%s'; the session will be killed.", "这是 '%s' 的最后一个窗口；会话将被删除。"},
	WindowKilled:          {"window killed", "窗口已删除"},
	KillPaneQ:             {"Kill pane '%s' (%s)?", "删除面板 '%s' (%s)？"},
	LastPaneCascade:       {"This is the last pane in '%s'; the window will be closed.", "这是 '%s' 的最后一个面板；窗口将被关闭。"},
	AlsoLastWindowCascade: {"It is also the last window in '%s'; the session will be killed.", "它也是 '%s' 的最后一个窗口；会话将被删除。"},
	PaneKilled:            {"pane killed", "面板已删除"},

	SessionCreated:        {"session '%s' created", "会话 '%s' 已创建"},
	AttachFailed:          {"attach failed: %s", "attach 失败: %s"},
	ReleaseToMove:         {"release to move %s", "松开以移动 %s"},
	InvalidDrop:           {"not a valid drop target", "不是有效的放置目标"},
	MoveLabelWinSess:      {"window '%s' → session '%s'", "窗口 '%s' → 会话 '%s'"},
	MoveLabelPaneWin:      {"pane '%s' → window '%s'", "面板 '%s' → 窗口 '%s'"},
	MovedX:                {"moved %s", "已移动 %s"},
	FilterPlaceholder:     {"filter", "过滤"},
	PreviewTooNarrow:      {"preview hidden: terminal too narrow", "预览已隐藏：终端太窄"},
	WindowCreated:         {"window created", "窗口已创建"},
	SessionRenamed:        {"session renamed", "会话已改名"},
	WindowRenamed:         {"window renamed", "窗口已改名"},
	NameEmpty:             {"name cannot be empty", "名称不能为空"},
	WindowMovedNewSession: {"window moved to a new session", "窗口已移动到新会话"},
	PromptSessionName:     {"session name: ", "会话名称: "},
	PromptWindowName:      {"window name: ", "窗口名称: "},
	PromptNewSessionAuto:  {"new session name (empty = auto): ", "新会话名称（留空 = 自动）: "},
	PromptDir:             {"dir: ", "目录: "},
	PaneSplit:             {"pane split", "面板已拆分"},
	Cancelled:             {"cancelled", "已取消"},
	PaneMovedNewWindow:    {"pane moved to a new window", "面板已移动到新窗口"},
	WindowMoved:           {"window moved", "窗口已移动"},
	PaneMoved:             {"pane moved", "面板已移动"},
	NotADirectory:         {"not a directory: %s", "不是目录: %s"},
	UnknownTheme:          {"unknown theme '%s', using default", "未知主题 '%s'，使用默认主题"},
	DirPickTitle:          {"new session — directory", "新建会话 — 选择目录"},
	DirPickHint:           {"  (tab/→ complete, enter accept)", "  (tab/→ 补全，enter 接受)"},
	DirPickNone:           {"  (no matching subdirectories)", "  (没有匹配的子目录)"},

	Usage:              {usageEN, usageZH},
	NoSessionsCLI:      {"no tmux sessions", "没有 tmux 会话"},
	AttachedMark:       {" (attached)", " (已连接)"},
	ListLine:           {"%s%s - %s, active %s ago\n", "%s%s - %s, %s 前活跃\n"},
	SetupWrote:         {"tmuxgo: wrote %s (backup: %s.tmuxgo-bak)\n", "tmuxgo: 已写入 %s（备份: %s.tmuxgo-bak）\n"},
	SetupUpToDate:      {"tmuxgo: %s already up to date\n", "tmuxgo: %s 已是最新\n"},
	SetupPopupHint:     {"tmuxgo: prefix + g opens the navigator popup\n", "tmuxgo: prefix + g 打开导航弹窗\n"},
	NoTemplatesCLI:     {"no templates (use 'tmuxgo template save <name>')", "没有模板（用 'tmuxgo template save <name>' 保存）"},
	TemplateDeletedCLI: {"tmuxgo: template %q deleted\n", "tmuxgo: 模板 %q 已删除\n"},
	TemplateSavedCLI:   {"tmuxgo: template %q saved (%s, from session %q) to %s\n", "tmuxgo: 模板 %q 已保存（%s，来自会话 %q）到 %s\n"},
	SessionNotFound:    {"session %q not found", "会话 %q 不存在"},
	UnknownCommand:     {"unknown command %q", "未知命令 %q"},
	UnknownTemplateCmd: {"unknown template command %q", "未知 template 子命令 %q"},
}

const usageEN = `tmuxgo - a tmux session navigator

usage:
  tmuxgo            open the interactive navigator
  tmuxgo --popup    navigator for tmux display-popup: exits after attach
  tmuxgo list       print the session/window/pane tree
  tmuxgo last       go to the previously active session
  tmuxgo new [name] create a session and go to it
  tmuxgo setup      install tmux.conf integration (popup, mouse, clipboard)

templates (session layouts: window names, splits, working dirs):
  tmuxgo template save <name> [session]   capture a session as a template
  tmuxgo template list                    show templates
  tmuxgo template new <name>              create a session from a template
  tmuxgo template delete <name>           remove a template

environment:
  TMUXGO_SOCKET     use a non-default tmux socket name

popup binding (~/.tmux.conf):
  bind g display-popup -E -w 90% -h 85% '/path/to/tmuxgo --popup'

  use an absolute path: the tmux server's PATH may not include ~/.local/bin
`

const usageZH = `tmuxgo - tmux 会话导航器

用法:
  tmuxgo            打开交互式导航器
  tmuxgo --popup    用于 tmux display-popup 的导航器: attach 后自动退出
  tmuxgo list       打印会话/窗口/面板树
  tmuxgo last       切换到上一个活跃会话
  tmuxgo new [name] 新建会话并进入
  tmuxgo setup      安装 tmux.conf 集成（弹窗、鼠标、剪贴板）

模板（会话布局: 窗口名、拆分、工作目录）:
  tmuxgo template save <name> [session]   把会话捕获为模板
  tmuxgo template list                    列出模板
  tmuxgo template new <name>              从模板创建会话
  tmuxgo template delete <name>           删除模板

环境变量:
  TMUXGO_SOCKET     使用非默认的 tmux socket 名

弹窗绑定 (~/.tmux.conf):
  bind g display-popup -E -w 90% -h 85% '/path/to/tmuxgo --popup'

  用绝对路径: tmux server 的 PATH 可能没有 ~/.local/bin
`

// T formats the message id in lang ("en"/"zh") with optional fmt args.
func T(l Lang, id ID, args ...any) string {
	e, ok := table[id]
	if !ok {
		return fmt.Sprintf("!i18n:%d!", int(id))
	}
	s := e[0]
	if l == ZH {
		s = e[1]
	}
	if len(args) > 0 {
		return fmt.Sprintf(s, args...)
	}
	return s
}

// Plural formats a counted unit: "1 window" / "2 windows" in English;
// Chinese has no plural ("2 个窗口").
func Plural(l Lang, n int, unit ID) string {
	if l == ZH {
		return fmt.Sprintf("%d 个%s", n, T(l, unit))
	}
	if n == 1 {
		return "1 " + T(l, unit)
	}
	return fmt.Sprintf("%d %ss", n, T(l, unit))
}
