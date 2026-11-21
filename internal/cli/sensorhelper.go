package cli

import (
	"errors"

	"github.com/spf13/cobra"

	"github.com/ushineko/hayami/internal/nethelper"
	"github.com/ushineko/hayami/internal/sensorhelper"
)

/*
sensorHelperCmd is the sensor helper (spec 103): the Windows service that
reads the processor's temperature through PawnIO for a panel that is not an
administrator. A command of the terminal program for the network helper's
reason: the service's program is a copy of it.
*/
func sensorHelperCmd(version string) *cobra.Command {
	status := func(cmd *cobra.Command) error {
		return serviceStatus(cmd, sensorhelper.Probe(),
			"not needed: this system gives the processor's temperature without a helper",
			"hayami-tui sensor-helper install   installs it; Windows asks for an administrator",
			"stopped, until a panel reads the temperature")
	}
	cmd := &cobra.Command{
		Use:   "sensor-helper",
		Short: "The service that reads the processor's temperature",
		Long: "The service that reads the processor's temperature.\n\n" +
			"Windows keeps the processor's temperature behind a kernel driver, PawnIO, which\n" +
			"serves only an administrator; the panel runs as you. The helper is a service that\n" +
			"reads it and serves the panel and nothing else. It starts when a panel reads the\n" +
			"temperature, and stops a while after the last one closes.\n\n" + sensorhelper.About,
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.NoArgs,
		RunE:          func(cmd *cobra.Command, _ []string) error { return status(cmd) },
	}

	statusCmd := &cobra.Command{
		Use: "status", Short: "Say whether the helper is installed, and which version",
		Args: cobra.NoArgs, SilenceUsage: true, SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error { return status(cmd) },
	}

	var allow string
	install := &cobra.Command{
		Use:   "install",
		Short: "Install the helper, or put this version in its place",
		Long:  "Install the helper, or put this version in its place.\n\n" + sensorhelper.InstallSteps,
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
				if err := elevate(cmd, "sensor-helper", "install", "--allow", allow); err != nil {
					return err
				}
				if state := sensorhelper.Probe(); !state.Installed || state.Version != version {
					return errors.New("the elevated step ended and the sensor helper is not installed") //nolint:err113 // said plainly
				}
				return status(cmd)
			}
			if err := sensorhelper.Install(allow, version); err != nil {
				return err //nolint:wrapcheck // says which step
			}
			return status(cmd)
		},
	}
	install.Flags().StringVar(&allow, "allow", "", "the account that may read the helper, as a SID; empty means yours")

	remove := &cobra.Command{
		Use: "remove", Short: "Stop the helper, unregister it and remove its program",
		Args: cobra.NoArgs, SilenceUsage: true, SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if !nethelper.Elevated() {
				if err := elevate(cmd, "sensor-helper", "remove"); err != nil {
					return err
				}
				return status(cmd)
			}
			if err := sensorhelper.Remove(); err != nil {
				return err //nolint:wrapcheck // says which step
			}
			return status(cmd)
		},
	}

	var runAllow string
	run := &cobra.Command{
		Use: "run", Short: "Serve the processor's temperature; what the service runs",
		Hidden: true,
		Args:   cobra.NoArgs, SilenceUsage: true, SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return sensorhelper.Run(cmd.Context(), runAllow, version) //nolint:wrapcheck // says which step
		},
	}
	run.Flags().StringVar(&runAllow, "allow", "", "the account that may read the helper")
	_ = run.MarkFlagRequired("allow")

	cmd.AddCommand(statusCmd, install, remove, run)
	return cmd
}
