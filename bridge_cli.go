package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/DKmiyan/tmuxgo/internal/bridge"
	"github.com/DKmiyan/tmuxgo/internal/i18n"
	"github.com/DKmiyan/tmuxgo/internal/tmux"
)

func cmdBridge(args []string) int {
	flags := flag.NewFlagSet(args[0], flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	help := flags.Bool("help", false, "")
	ticket := flags.String("ticket", "", "")
	socket := flags.String("socket", "default", "")
	interval := flags.Int("interval-ms", 1000, "")
	version := flags.Bool("protocol-version", false, "")
	if err := flags.Parse(args[1:]); err != nil || flags.NArg() != 0 {
		fmt.Fprintln(os.Stderr, tr(i18n.BridgeUsage))
		return 2
	}
	invalidFlags := false
	flags.Visit(func(f *flag.Flag) {
		if args[0] == "bridge-attach" && f.Name != "ticket" && f.Name != "help" {
			invalidFlags = true
		}
		if args[0] == "bridge" && f.Name == "ticket" {
			invalidFlags = true
		}
	})
	if invalidFlags {
		fmt.Fprintln(os.Stderr, tr(i18n.BridgeUsage))
		return 2
	}
	if *help {
		fmt.Print(tr(i18n.BridgeUsage))
		return 0
	}
	if *version && args[0] == "bridge" {
		fmt.Println(`{"version":0}`)
		return 0
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
	defer cancel()
	var err error
	if args[0] == "bridge-attach" {
		if *ticket == "" {
			fmt.Fprintln(os.Stderr, tr(i18n.BridgeUsage))
			return 2
		}
		err = bridge.Attach(ctx, *ticket)
	} else {
		backend, e := tmux.NewBridge(*socket)
		if e != nil || *ticket != "" {
			fmt.Fprintln(os.Stderr, tr(i18n.BridgeUsage))
			return 2
		}
		err = bridge.Run(ctx, os.Stdin, os.Stdout, backend, lang, time.Duration(*interval)*time.Millisecond)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, tr(i18n.BridgeOperationFailed, bridge.PublicErrorCode(err)))
		return 1
	}
	return 0
}
