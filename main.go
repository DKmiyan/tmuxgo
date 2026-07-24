package main

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/DKmiyan/tmuxgo/internal/app"
	"github.com/DKmiyan/tmuxgo/internal/config"
	"github.com/DKmiyan/tmuxgo/internal/i18n"
	"github.com/DKmiyan/tmuxgo/internal/setup"
	"github.com/DKmiyan/tmuxgo/internal/template"
	"github.com/DKmiyan/tmuxgo/internal/tmux"
)

// lang is the CLI interface language, resolved once from the config file
// (language: auto|en|zh) and the environment.
var lang = i18n.EN

// resolveLang reads the configured language best-effort (defaults + env
// apply when the config is missing or invalid).
func resolveLang() i18n.Lang {
	cfgLang := ""
	if path, err := config.DefaultPath(); err == nil {
		if cfg, err := config.Load(path); err == nil {
			cfgLang = cfg.Language
		}
	}
	return i18n.Resolve(cfgLang, os.Getenv)
}

// tr formats a CLI message in the resolved language.
func tr(id i18n.ID, args ...any) string {
	return i18n.T(lang, id, args...)
}

// plural formats a counted unit ("1 window" / "2 windows"; Chinese has no
// plural form).
func plural(n int, unit i18n.ID) string {
	return i18n.Plural(lang, n, unit)
}

func main() {
	lang = resolveLang()
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
		fmt.Print(tr(i18n.Usage))
	default:
		fmt.Fprintf(os.Stderr, tr(i18n.UnknownCommand, args[0])+"\n\n%s", tr(i18n.Usage))
		os.Exit(2)
	}
}

func cmdList(b *tmux.Tmux) {
	tree, err := b.Tree()
	if err != nil {
		fatal(err)
	}
	if len(tree) == 0 {
		fmt.Println(tr(i18n.NoSessionsCLI))
		return
	}
	for _, s := range tree {
		attached := ""
		if s.Attached {
			attached = tr(i18n.AttachedMark)
		}
		fmt.Printf(tr(i18n.ListLine),
			s.Name, attached, plural(len(s.Windows), i18n.UnitWindow), tmux.HumanAge(s.Activity))
		for _, w := range s.Windows {
			fmt.Printf("  %d: %s - %s\n", w.Index, w.Name, plural(len(w.Panes), i18n.UnitPane))
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
		fatal(errors.New(tr(i18n.NoSessionsCLI)))
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
		fmt.Printf(tr(i18n.SetupWrote), conf, conf)
	} else {
		fmt.Printf(tr(i18n.SetupUpToDate), conf)
	}
	fmt.Print(tr(i18n.SetupPopupHint))
}

func cmdTemplate(b *tmux.Tmux, args []string) {
	store, err := template.DefaultStore()
	if err != nil {
		fatal(err)
	}
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: tmuxgo template save|list|new|delete ...")
		fmt.Print(tr(i18n.Usage))
		os.Exit(2)
	}
	switch args[0] {
	case "save":
		if len(args) < 2 {
			fatal(errors.New("usage: tmuxgo template save <name> [session]"))
		}
		templateSave(b, store, args[1], args[2:])
	case "list":
		ts, err := store.List()
		if err != nil {
			fatal(err)
		}
		if len(ts) == 0 {
			fmt.Println(tr(i18n.NoTemplatesCLI))
			return
		}
		for _, t := range ts {
			fmt.Printf("%s - %s\n", t.Name, plural(len(t.Windows), i18n.UnitWindow))
		}
	case "new":
		if len(args) < 2 {
			fatal(errors.New("usage: tmuxgo template new <name>"))
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
			fatal(errors.New("usage: tmuxgo template delete <name>"))
		}
		if err := store.Delete(args[1]); err != nil {
			fatal(err)
		}
		fmt.Printf(tr(i18n.TemplateDeletedCLI), args[1])
	default:
		fmt.Fprintf(os.Stderr, tr(i18n.UnknownTemplateCmd, args[0])+"\n\n%s", tr(i18n.Usage))
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
		fatal(errors.New(tr(i18n.NoSessionsCLI)))
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
			fatal(fmt.Errorf(tr(i18n.SessionNotFound), want))
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
	fmt.Printf(tr(i18n.TemplateSavedCLI),
		name, plural(len(tpl.Windows), i18n.UnitWindow), found.Name, store.Path())
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "tmuxgo:", err)
	os.Exit(1)
}
