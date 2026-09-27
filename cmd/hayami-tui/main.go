/*
Command hayami-tui is the terminal panel.

The same sections the window draws, arranged for a pane. A session manager's
pane showing one reading is not a separate program and not a separate mode: it
is this one with --sections naming what it wants.

	hayami-tui --sections bandwidth --arrangement row

It is linked as a console application and builds with cgo off. A terminal panel
that needed a display library would be a terminal panel with a bug.
*/
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/ushineko/hayami/internal/cli"
	"github.com/ushineko/hayami/internal/config"
	"github.com/ushineko/hayami/internal/panel"
	"github.com/ushineko/hayami/internal/tui"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	sections := flag.String("sections", "",
		"comma-separated sections to draw, overriding the settings for this run ("+
			strings.Join(panel.Keys(), ", ")+")")
	arrangement := flag.String("arrangement", "",
		"stack, grid or row, overriding the settings for this run")
	once := flag.Bool("once", false, "draw one frame and exit, for a prompt or a status line")
	readings := flag.Bool("readings", false, "print the readings as JSON and exit")
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

	opts, err := cli.Resolve(store, *sections, *arrangement)
	if err != nil {
		return err
	}
	if opts.Unreadable != nil {
		// Say it and carry on. A panel that refused to start over a stray tab
		// would be a panel nobody could fix from the pane it failed in.
		fmt.Fprintf(os.Stderr, "the settings file could not be read, using defaults: %v\n",
			opts.Unreadable)
	}

	sources := opts.Sources(nil)
	if *readings {
		return cli.Readings(os.Stdout, sources, func(s panel.Source) error {
			_, err := s.Poll(context.Background())
			return err
		})
	}

	return tui.Start(tui.Options{
		Sources:     sources,
		Arrangement: opts.Arrangement,
		Once:        *once,
	})
}
