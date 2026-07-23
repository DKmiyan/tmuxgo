package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/DKmiyan/tmuxgo/internal/app"
	"github.com/DKmiyan/tmuxgo/internal/setup"
	"github.com/DKmiyan/tmuxgo/internal/template"
	"github.com/DKmiyan/tmuxgo/internal/tmux"
)

const usage = `tmuxgo - a tmux session navigator

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
	case "setup":
		cmdSetup()
	case "template":
		cmdTemplate(b, args[1:])
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
	id, err := b.NewSessionID(name, "")
	if err != nil {
		fatal(err)
	}
	if err := b.Attach(id); err != nil {
		fatal(err)
	}
}

func cmdSetup() {
	home, err := os.UserHomeDir()
	if err != nil {
		fatal(err)
	}
	conf, changed, err := setup.Run(home)
	if err != nil {
		fatal(err)
	}
	if changed {
		fmt.Printf("tmuxgo: wrote %s (backup: %s.tmuxgo-bak)\n", conf, conf)
	} else {
		fmt.Printf("tmuxgo: %s already up to date\n", conf)
	}
	fmt.Println("tmuxgo: prefix + g opens the navigator popup")
}

func cmdTemplate(b *tmux.Tmux, args []string) {
	store, err := template.DefaultStore()
	if err != nil {
		fatal(err)
	}
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: tmuxgo template save|list|new|delete ...")
		fmt.Print(usage)
		os.Exit(2)
	}
	switch args[0] {
	case "save":
		if len(args) < 2 {
			fatal(fmt.Errorf("usage: tmuxgo template save <name> [session]"))
		}
		templateSave(b, store, args[1], args[2:])
	case "list":
		ts, err := store.List()
		if err != nil {
			fatal(err)
		}
		if len(ts) == 0 {
			fmt.Println("no templates (use 'tmuxgo template save <name>')")
			return
		}
		for _, t := range ts {
			fmt.Printf("%s - %s\n", t.Name, plural(len(t.Windows), "window"))
		}
	case "new":
		if len(args) < 2 {
			fatal(fmt.Errorf("usage: tmuxgo template new <name>"))
		}
		tpl, err := store.Get(args[1])
		if err != nil {
			fatal(err)
		}
		id, err := tpl.Create(b)
		if err != nil {
			fatal(err)
		}
		if err := b.Attach(id); err != nil {
			fatal(err)
		}
	case "delete":
		if len(args) < 2 {
			fatal(fmt.Errorf("usage: tmuxgo template delete <name>"))
		}
		if err := store.Delete(args[1]); err != nil {
			fatal(err)
		}
		fmt.Printf("tmuxgo: template %q deleted\n", args[1])
	default:
		fmt.Fprintf(os.Stderr, "unknown template command %q\n\n%s", args[0], usage)
		os.Exit(2)
	}
}

// templateSave captures a session into a named template. With no session
// argument it uses the current session inside tmux, else the most recently
// active one.
func templateSave(b *tmux.Tmux, store *template.Store, name string, sessionArgs []string) {
	tree, err := b.Tree()
	if err != nil {
		fatal(err)
	}
	if len(tree) == 0 {
		fatal(fmt.Errorf("no tmux sessions"))
	}
	var found *tmux.Session
	switch {
	case len(sessionArgs) > 0:
		want := strings.Join(sessionArgs, " ")
		for i := range tree {
			if tree[i].Name == want {
				found = &tree[i]
			}
		}
		if found == nil {
			fatal(fmt.Errorf("session %q not found", want))
		}
	case tmux.InsideTmux():
		id, err := b.CurrentSessionID()
		if err != nil {
			fatal(err)
		}
		for i := range tree {
			if tree[i].ID == id {
				found = &tree[i]
			}
		}
	default:
		found = &tree[0] // most recently active
	}
	tpl := template.Capture(name, *found)
	if err := store.Save(tpl); err != nil {
		fatal(err)
	}
	fmt.Printf("tmuxgo: template %q saved (%s, from session %q) to %s\n",
		name, plural(len(tpl.Windows), "window"), found.Name, store.Path())
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
