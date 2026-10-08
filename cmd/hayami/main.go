/*
Command hayami is the desktop panel: a frameless, always-on-top window of
cards, read without being touched.

It is linked windowed. Its sibling hayami-tui is the console application and is
where the JSON and the one-shot line live, because a windowed binary on Windows
has no console to print to.
*/
package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/ushineko/hayami/internal/buildinfo"
	"github.com/ushineko/hayami/internal/cli"
	"github.com/ushineko/hayami/internal/gui"
	"github.com/ushineko/hayami/internal/prefs"
)

func main() {
	// cobra takes any program Explorer starts for a console tool somebody
	// double-clicked: it prints a note, waits five seconds and exits 1 (its
	// "mousetrap", Windows only). The panel is that program from the Start
	// menu, a desktop shortcut and the login shortcut, and is linked windowed,
	// so the note went nowhere and the person saw an hourglass and then
	// nothing. Turned off here, for the windowed binary only: hayami-tui is a
	// console program and the note is right for it.
	cobra.MousetrapHelpText = ""
	tuneGC(os.Getenv)

	root := cli.GUI(buildinfo.Version(), prefs.Pages(), func(o cli.Options) error {
		return gui.Start(gui.Options{
			Sources:     o.Sources(nil),
			Title:       "hayami",
			Version:     buildinfo.Version(),
			Store:       o.Store,
			Preferences: o.Preferences,
		})
	})
	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "hayami:", err)
		var usage *cli.UsageError
		if errors.As(err, &usage) {
			os.Exit(2)
		}
		os.Exit(1)
	}
}
