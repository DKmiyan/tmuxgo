# tmuxgo

A lightweight, beginner-friendly tmux session navigator and organizer.
Browse sessions, windows, and panes in a single expandable tree and manage
them without memorizing tmux commands — from your terminal or from a tmux
popup hotkey.

![platform](https://img.shields.io/badge/platform-linux%20%7C%20macos%20%7C%20wsl-blue)
![license](https://img.shields.io/badge/license-MIT-green)

## Features

- Single-column expandable tree: sessions → windows → panes, sorted by
  recent activity with a stable selection
- Attach directly to the highlighted session, window, or pane
- Create, rename, move, and kill sessions/windows/panes (kills confirm
  first and warn about last-window/last-pane cascades)
- Live filter (`/`), optional pane preview (`p`), mouse support
- Session templates: capture a session's layout and recreate it later
- Directory-anchored sessions: new sessions start from the selected pane's
  working directory, editable with VSCode-style subdirectory completion
- Popup mode: summon the navigator inside tmux with `prefix + g`
- Configurable keys, theme, and defaults (`~/.config/tmuxgo/config.json`)
- One command, one static binary, no daemon, no config file required

Requires tmux (≥ 3.2 for the popup; ≥ 3.4 verified for everyday use).

## Install

Build from source (Go ≥ 1.25):

```sh
git clone https://github.com/DKmiyan/tmuxgo.git
cd tmuxgo
go build -o tmuxgo . && mv tmuxgo ~/.local/bin/
```

or `go install github.com/DKmiyan/tmuxgo@latest`.

Prebuilt binaries for Linux and macOS are attached to
[releases](https://github.com/DKmiyan/tmuxgo/releases).

## Quick start

```sh
tmuxgo            # open the navigator
tmuxgo setup      # optional: tmux.conf integration (popup, mouse, clipboard)
```

`tmuxgo setup` adds a managed block to `~/.tmux.conf` (with a backup):

```tmux
bind g display-popup -E -w 90% -h 85% '/path/to/tmuxgo --popup'
```

Then `prefix + g` summons the navigator anywhere inside tmux; `enter`
jumps to the target and the popup closes. (The binding uses an absolute
path on purpose: the tmux server's PATH may not include `~/.local/bin`.)

## Keys

| key | action |
| --- | --- |
| `↑/k` `↓/j` | move |
| `→/l` | expand / enter first child |
| `←/h` | collapse / go to parent |
| `enter` | attach to selected session/window/pane |
| `n` | new session / window / split pane / from template |
| `r` | rename session/window |
| `m` | move window to session, pane to window |
| `d` | kill session/window/pane (confirms first) |
| `/` | filter |
| `p` | toggle pane preview (wide terminals) |
| `,` | settings |
| `?` | help |
| `q` | quit |

Mouse: click selects, clicking an expand marker toggles it, double-click
attaches, wheel scrolls.

## CLI

```sh
tmuxgo list                      # print the tree
tmuxgo last                      # go to the previously active session
tmuxgo new [name]                # create a session and go to it
tmuxgo setup                     # tmux.conf integration
tmuxgo template save <name>      # capture the current session's layout
tmuxgo template new <name>       # recreate a session from a template
tmuxgo template list / delete
```

## Configuration

`~/.config/tmuxgo/config.json` (all keys optional):

```json
{
  "theme": "auto",
  "preview_default": false,
  "mouse": true,
  "keys": {
    "new": ["n"],
    "quit": ["q", "ctrl+c"]
  }
}
```

`theme` is `auto`, `dark`, or `light`. Actions under `keys`: `up`, `down`,
`expand`, `collapse`, `attach`, `new`, `rename`, `move`, `kill`, `filter`,
`preview`, `help`, `settings`, `quit`.

Templates live in `~/.config/tmuxgo/templates.json`.

## License

MIT
