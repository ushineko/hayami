/*
Command hayami-tui is the terminal panel.

The same sections the window draws, arranged for a pane. A session manager's
pane showing one reading is not a separate program and not a separate mode: it
is this one with --sections naming what it wants.

	hayami-tui --sections usage --arrangement row

It is linked as a console application and builds with cgo off. A terminal panel
that needed a display library would be a terminal panel with a bug.
*/
package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/ushineko/hayami/internal/buildinfo"
	"github.com/ushineko/hayami/internal/cli"
)

func main() {
	if err := cli.TUI(buildinfo.Version()).Execute(); err != nil {
		// The command tree silences cobra's own reporting so this is the
		// single place a failure is printed; leaving it on prints every
		// failure twice.
		fmt.Fprintln(os.Stderr, "hayami-tui:", err)
		var usage *cli.UsageError
		if errors.As(err, &usage) {
			// A misuse of the command line is exit 2, distinct from a command
			// that ran and failed, so a script can tell them apart.
			os.Exit(2)
		}
		os.Exit(1)
	}
}
