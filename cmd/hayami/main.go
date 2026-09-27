/*
Command hayami is the desktop panel: a frameless, always-on-top window of
cards, read without being touched.

It is linked windowed. Its sibling hayami-tui is the console application and
is where the JSON and the one-shot line live, because a windowed binary on
Windows has no console to print to.
*/
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/ushineko/hayami/internal/cli"
	"github.com/ushineko/hayami/internal/config"
	"github.com/ushineko/hayami/internal/gui"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	settings := flag.String("settings", "", "the settings file; empty means the usual place")
	flag.Parse()

	path := *settings
	if path == "" {
		p, err := config.Path()
		if err != nil {
			return err
		}
		path = p
	}
	store, err := config.Open(path)
	if err != nil {
		return err
	}

	// The window takes no --sections or --arrangement. Those are a pane's
	// arguments: a desktop panel is configured from its own settings, and a
	// flag that changed what it drew for one run would be a setting nobody
	// could find again.
	opts, err := cli.Resolve(store, "", "")
	if err != nil {
		return err
	}
	if opts.Unreadable != nil {
		fmt.Fprintf(os.Stderr, "the settings file could not be read, using defaults: %v\n",
			opts.Unreadable)
	}

	return gui.Start(gui.Options{Sources: opts.Sources(nil), Title: "hayami"})
}
