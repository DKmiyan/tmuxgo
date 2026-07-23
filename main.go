package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/DKmiyan/tmuxgo/internal/app"
	"github.com/DKmiyan/tmuxgo/internal/tmux"
)

const usage = `tmuxgo - a tmux session navigator

usage:
  tmuxgo            open the interactive navigator
  tmuxgo --popup    navigator for tmux display-popup: exits after attach
  tmuxgo list       print the session/window/pane tree
  tmuxgo last       go to the previously active session
  tmuxgo new [name] create a session and go to it

environment:
  TMUXGO_SOCKET     use a non-default tmux socket name

popup binding (~/.tmux.conf):
  bind g display-popup -E -w 90% -h 85% '/path/to/tmuxgo --popup'

  use an absolute path: the tmux server's PATH may not include ~/.local/bin
`

func main() {
	b := tmux.New()
	args := os.Args[1:]

	popup := false
	rest := args[:0]
	for _, a := range args {
		if a == "--popup" || a == "-p" {
			popup = true
		} else {
			rest = append(rest, a)
		}
	}
	args = rest

	if len(args) == 0 {
		if err := app.Run(b, popup); err != nil {
			fatal(err)
		}
		return
	}

	switch args[0] {
	case "list":
		cmdList(b)
	case "last":
		cmdLast(b)
	case "new":
		name := ""
		if len(args) > 1 {
			name = strings.Join(args[1:], " ")
		}
		cmdNew(b, name)
	case "-h", "--help", "help":
		fmt.Print(usage)
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n%s", args[0], usage)
		os.Exit(2)
	}
}

func cmdList(b *tmux.Tmux) {
	tree, err := b.Tree()
	if err != nil {
		fatal(err)
	}
	if len(tree) == 0 {
		fmt.Println("no tmux sessions")
		return
	}
	for _, s := range tree {
		attached := ""
		if s.Attached {
			attached = " (attached)"
		}
		fmt.Printf("%s%s - %s, active %s ago\n",
			s.Name, attached, plural(len(s.Windows), "window"), tmux.HumanAge(s.Activity))
		for _, w := range s.Windows {
			fmt.Printf("  %d: %s - %s\n", w.Index, w.Name, plural(len(w.Panes), "pane"))
			for _, p := range w.Panes {
				fmt.Printf("      %d: %s (%s)\n", p.Index, p.CurrentCommand, p.CurrentPath)
			}
		}
	}
}

func cmdLast(b *tmux.Tmux) {
	if tmux.InsideTmux() {
		if err := b.SwitchToLast(); err != nil {
			fatal(err)
		}
		return
	}
	tree, err := b.Tree()
	if err != nil {
		fatal(err)
	}
	if len(tree) == 0 {
		fatal(fmt.Errorf("no tmux sessions"))
	}
	// Tree is sorted by recent activity, so [0] is the last active session.
	if err := b.Attach(tree[0].ID); err != nil {
		fatal(err)
	}
}

func cmdNew(b *tmux.Tmux, name string) {
	id, err := b.NewSessionID(name)
	if err != nil {
		fatal(err)
	}
	if err := b.Attach(id); err != nil {
		fatal(err)
	}
}

func plural(n int, word string) string {
	if n == 1 {
		return fmt.Sprintf("1 %s", word)
	}
	return fmt.Sprintf("%d %ss", n, word)
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "tmuxgo:", err)
	os.Exit(1)
}
