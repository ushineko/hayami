package cli

import (
	"errors"
	"fmt"
	"os"
	"os/user"

	"github.com/spf13/cobra"

	"github.com/ushineko/hayami/internal/nethelper"
)

/*
helperCmd is the network helper (specs 061 and 072): the service that reads
the network by process for a panel that is not privileged, on Windows and
Linux.

It is a command of the terminal program because the service's program is a
copy of the terminal program: one binary to build, sign and trust, and the
copy in Program Files is what the service runs.
*/
func helperCmd(version string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "network-helper",
		Short: "The service that reads the network by process",
		Long: "The service that reads the network by process.\n\n" +
			"Windows gives the network by process only to an administrator, and Linux gives\n" +
			"UDP, short connections and other accounts' traffic only to a privileged program;\n" +
			"the panel runs as you. The helper is a service that reads it and serves the\n" +
			"panel and nothing else. It starts when a panel opens the list, and stops when\n" +
			"the list is closed.\n\n" + nethelper.About,
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return helperStatus(cmd)
		},
	}

	status := &cobra.Command{
		Use: "status", Short: "Say whether the helper is installed, and which version",
		Args: cobra.NoArgs, SilenceUsage: true, SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error { return helperStatus(cmd) },
	}

	var allow string
	install := &cobra.Command{
		Use:   "install",
		Short: "Install the helper, or put this version in its place",
		Long:  "Install the helper, or put this version in its place.\n\n" + nethelper.InstallSteps,
		Args:  cobra.NoArgs, SilenceUsage: true, SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if allow == "" {
				sid, err := nethelper.CurrentUser()
				if err != nil {
					return err //nolint:wrapcheck // says what it was asking
				}
				allow = sid
			}
			if !nethelper.Elevated() {
				if err := elevate(cmd, "network-helper", "install", "--allow", allow); err != nil {
					return err
				}
				state := nethelper.Probe()
				if !state.Installed || state.Version != version {
					return errors.New("the elevated step ended and the helper is not installed") //nolint:err113 // said plainly
				}
				return helperStatus(cmd)
			}
			if err := nethelper.Install(allow, version); err != nil {
				return err //nolint:wrapcheck // says which step
			}
			return helperStatus(cmd)
		},
	}
	install.Flags().StringVar(&allow, "allow", "", "the account that may read the helper (a SID on Windows, a user on Linux); empty means yours")

	remove := &cobra.Command{
		Use: "remove", Short: "Stop the helper, unregister it and remove its program",
		Args: cobra.NoArgs, SilenceUsage: true, SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if !nethelper.Elevated() {
				if err := elevate(cmd, "network-helper", "remove"); err != nil {
					return err
				}
				return helperStatus(cmd)
			}
			if err := nethelper.Remove(); err != nil {
				return err //nolint:wrapcheck // says which step
			}
			return helperStatus(cmd)
		},
	}

	var runAllow string
	run := &cobra.Command{
		Use: "run", Short: "Serve the network by process; what the service runs",
		Hidden: true,
		Args:   cobra.NoArgs, SilenceUsage: true, SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return nethelper.Run(cmd.Context(), runAllow, version) //nolint:wrapcheck // says which step
		},
	}
	run.Flags().StringVar(&runAllow, "allow", "", "the account that may read the helper")
	_ = run.MarkFlagRequired("allow")

	cmd.AddCommand(status, install, remove, run)
	return cmd
}

// elevate runs this program again as an administrator, with args.
func elevate(cmd *cobra.Command, args ...string) error {
	self, err := os.Executable()
	if err != nil {
		return fmt.Errorf("finding this program: %w", err)
	}
	return nethelper.RunElevated(cmd.Context(), self, args...) //nolint:wrapcheck // says what Windows answered
}

// helperStatus prints what there is of the helper.
func helperStatus(cmd *cobra.Command) error {
	out := cmd.OutOrStdout()
	state := nethelper.Probe()
	switch {
	case !state.Applies:
		_, _ = fmt.Fprintln(out, "not needed: this system gives the network by process without a helper")
	case !state.Installed:
		_, _ = fmt.Fprintln(out, "not installed")
		_, _ = fmt.Fprintln(out, "  hayami-tui network-helper install   installs it; "+nethelper.InstallNeeds)
	default:
		version := state.Version
		if version == "" {
			version = "an unknown version"
		}
		running := "stopped, until a panel opens the list"
		if state.Running {
			running = "running"
		}
		_, _ = fmt.Fprintf(out, "installed: %s, %s\n  %s\n", version, running, state.Path)
		if state.For != "" {
			who := "uid " + state.For
			if u, err := user.LookupId(state.For); err == nil {
				who = u.Username
			}
			_, _ = fmt.Fprintf(out, "  for %s\n", who)
		}
	}
	return nil
}
