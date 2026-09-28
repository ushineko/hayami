package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/ushineko/hayami/internal/config"
	"github.com/ushineko/hayami/internal/panel"
	"github.com/ushineko/hayami/internal/tui"
	"github.com/ushineko/hayami/internal/view"
)

// UsageError is a misuse of the command line, as distinct from a command that
// ran and failed. It is exit 2 so a script can tell the two apart.
type UsageError struct{ Err error }

func (e *UsageError) Error() string { return e.Err.Error() }
func (e *UsageError) Unwrap() error { return e.Err }

// flags are the settings a run may override, and where they came from.
type flags struct {
	settings    string
	sections    string
	arrangement string
}

// resolve opens the settings and applies the overrides.
func (f flags) resolve() (Options, *config.Store, error) {
	path := f.settings
	if path == "" {
		p, err := config.Path()
		if err != nil {
			return Options{}, nil, err
		}
		path = p
	}
	store, err := config.Open(path)
	if err != nil {
		return Options{}, nil, err
	}
	opts, err := Resolve(store, f.sections, f.arrangement)
	opts.Store = store
	if err != nil {
		return opts, store, &UsageError{err}
	}
	return opts, store, nil
}

// TUI is the terminal panel's command tree.
//
// The panel is the root command rather than a subcommand of it: a pane in a
// session manager runs `hayami-tui --sections usage` and should not have to
// name what it wants twice. Everything that is not the panel — the readings,
// the version — is a subcommand, which is what keeps the root's flags about
// the panel and nothing else.
func TUI(version string) *cobra.Command {
	var f flags
	var once bool

	root := &cobra.Command{
		Use:   "hayami-tui",
		Short: "A panel of readings, in a terminal",
		Long: "A panel of readings, in a terminal.\n\n" +
			"The same sections the window draws, arranged for a pane. A pane showing one\n" +
			"reading is not a separate program and not a separate mode: it is this one with\n" +
			"--sections naming what it wants.\n\n" +
			"  hayami-tui --sections usage --arrangement row",
		Version:       version,
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			opts, _, err := f.resolve()
			if err != nil {
				return err
			}
			warn(cmd, opts)
			return tui.Start(tui.Options{
				Sources:     opts.Sources(nil),
				Arrangement: opts.Arrangement,
				Once:        once,
			})
		},
	}

	root.Flags().BoolVar(&once, "once", false,
		"draw one frame and exit, for a prompt or a status line")
	addPanelFlags(root, &f)
	root.AddCommand(readingsCmd(&f), arrangementsCmd())
	return root
}

// GUI is the window's command tree.
//
// It takes no --sections or --arrangement. Those are a pane's arguments: a
// desktop panel is configured from its own settings, and a flag that changed
// what it drew for one run would be a setting nobody could find again.
func GUI(version string, start func(Options) error) *cobra.Command {
	var f flags
	var preferences bool

	root := &cobra.Command{
		Use:           "hayami",
		Short:         "A panel of readings, on the desktop",
		Version:       version,
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			opts, _, err := f.resolve()
			if err != nil {
				return err
			}
			warn(cmd, opts)
			opts.Preferences = preferences
			return start(opts)
		},
	}
	root.PersistentFlags().StringVar(&f.settings, "settings", "",
		"the settings file; empty means the usual place")
	// A desktop entry's second action, and the way in for somebody whose
	// panel is somewhere they cannot right-click it.
	root.Flags().BoolVar(&preferences, "preferences", false,
		"open the preferences window as well as the panel")
	root.AddCommand(windowCmd())
	return root
}

// readingsCmd prints what the sources say, as JSON.
//
// A subcommand of the terminal binary and not the window's: a windowed binary
// has no console to print to on Windows, which is the same reason there are
// two binaries at all.
func readingsCmd(f *flags) *cobra.Command {
	return &cobra.Command{
		Use:   "readings",
		Short: "Print the readings as JSON and exit",
		Long: "Print the readings as JSON and exit.\n\n" +
			"How the data layer is debugged on a machine with no display. A rate needs two\n" +
			"samples, so a single run reports totals and no rates, which is the honest\n" +
			"answer rather than a fault.",
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			opts, _, err := f.resolve()
			if err != nil {
				return err
			}
			warn(cmd, opts)
			return Readings(cmd.OutOrStdout(), opts.Sources(nil), func(s panel.Source) error {
				_, err := s.Poll(context.Background())
				return err
			})
		},
	}
}

// arrangementsCmd lists what --arrangement takes, because a flag whose values
// are a closed set should be able to say what they are.
func arrangementsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "arrangements",
		Short: "List the ways sections can be laid out",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, _ []string) {
			for _, a := range []view.Arrangement{view.ArrangeStack, view.ArrangeGrid, view.ArrangeRow} {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%-6s %s\n", a.String(), arrangementHelp(a))
			}
		},
	}
}

func arrangementHelp(a view.Arrangement) string {
	switch a {
	case view.ArrangeGrid:
		return "columns that reflow to the width, in the manner of btop"
	case view.ArrangeRow:
		return "one line per reading, its bar stretching to the pane"
	default:
		return "one section above another; a narrow terminal, and the window"
	}
}

// addPanelFlags puts the flags a pane overrides on a command.
func addPanelFlags(cmd *cobra.Command, f *flags) {
	cmd.PersistentFlags().StringVar(&f.settings, "settings", "",
		"the settings file; empty means the usual place")
	cmd.PersistentFlags().StringVar(&f.sections, "sections", "",
		"comma-separated sections to draw, overriding the settings for this run ("+
			strings.Join(panel.Keys(), ", ")+")")
	cmd.PersistentFlags().StringVar(&f.arrangement, "arrangement", "",
		"stack, grid or row, overriding the settings for this run")
}

// warn says a settings file could not be read. The panel draws on defaults
// rather than refusing to start: a panel that would not start could not be
// fixed from the pane it failed in.
func warn(cmd *cobra.Command, o Options) {
	if o.Unreadable != nil {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(),
			"the settings file could not be read, using defaults: %v\n", o.Unreadable)
	}
}
