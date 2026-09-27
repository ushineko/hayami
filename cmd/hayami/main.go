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

	"github.com/ushineko/hayami/internal/buildinfo"
	"github.com/ushineko/hayami/internal/cli"
	"github.com/ushineko/hayami/internal/gui"
)

func main() {
	root := cli.GUI(buildinfo.Version(), func(o cli.Options) error {
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
