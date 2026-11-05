package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/ushineko/hayami/internal/config"
	"github.com/ushineko/hayami/internal/core"
	"github.com/ushineko/hayami/internal/keepalive"
	"github.com/ushineko/hayami/internal/panel"
)

// notifyDeadline bounds a notification, so a desktop that has stopped answering costs
// a line in the journal and not the helper's loop.
const notifyDeadline = 5 * time.Second

/*
keepaliveCmd is the VPN keepalive helper (spec 085 R21): the service that keeps a chosen
tunnel working when no panel is open. It is a command of the terminal program, which
reads, because the helper has no window; what it acts on is settled by the settings a
person chose, and the terminal panel itself still has no way to act on a VPN.
*/
func keepaliveCmd(f *flags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "vpn-keepalive",
		Short: "The service that keeps a chosen VPN working",
		Long: "The service that keeps a chosen VPN working when no panel is open.\n\n" +
			"For a VPN it is switched on for in preferences, a failed check bounces the\n" +
			"tunnel, three in a row disconnect it until you re-enable it, and a tunnel that\n" +
			"drops is re-dialled. An OpenVPN3 tunnel is only notified on, because its\n" +
			"connections need a person at a sign-in page.",
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.NoArgs,
	}
	run := &cobra.Command{
		Use: "run", Short: "Keep the VPNs working, until stopped; what the service runs",
		Args: cobra.NoArgs, SilenceUsage: true, SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			return runKeepalive(ctx, cmd, f)
		},
	}
	status := &cobra.Command{
		Use: "status", Short: "Say whether the helper is installed and running",
		Args: cobra.NoArgs, SilenceUsage: true, SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error { return keepaliveDoctor(cmd, f) },
	}
	install := &cobra.Command{
		Use:   "install",
		Short: "Install the helper as your own service, and start it",
		Long: "Install the helper as your own service, and start it. Run again, it puts this\n" +
			"program in place of the one running. It needs no administrator.\n\n" +
			keepalive.OfferCaution,
		Args: cobra.NoArgs, SilenceUsage: true, SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if f.settings != "" {
				return &UsageError{errKeepaliveSettings}
			}
			self, err := os.Executable()
			if err != nil {
				return fmt.Errorf("finding this program: %w", err)
			}
			if err := keepalive.Install(cmd.Context(), self); err != nil {
				return err //nolint:wrapcheck // says which step
			}
			return keepaliveDoctor(cmd, f)
		},
	}
	remove := &cobra.Command{
		Use: "remove", Aliases: []string{"uninstall"}, Short: "Stop the helper and remove its service",
		Args: cobra.NoArgs, SilenceUsage: true, SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			there := keepalive.Installed()
			if err := keepalive.Remove(cmd.Context()); err != nil {
				return err //nolint:wrapcheck // says which step
			}
			said := "not installed"
			if there {
				said = "removed"
			}
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), said)
			return nil
		},
	}
	cmd.RunE = func(cmd *cobra.Command, _ []string) error { return keepaliveDoctor(cmd, f) }
	cmd.AddCommand(status, install, remove, run)
	return cmd
}

// errKeepaliveSettings is an install asked to use settings the service would not read.
var errKeepaliveSettings = errors.New("the helper's service reads your own settings, so install takes no --settings")

// vpnToggleService is the monitor hayami replaces (spec 085 R27).
const vpnToggleService = "vpn-toggle.service"

// doctorDeadline bounds asking the service manager for a line of a report.
const doctorDeadline = 3 * time.Second

// keepaliveStanding is what a report says of the helper.
type keepaliveStanding struct {
	installed, running bool

	// on is how many VPNs are switched on for it.
	on int

	// toggleActive is vpn-toggle's service running, and toggleErr that it could not be
	// asked.
	toggleActive bool
	toggleErr    error
}

// keepaliveLines is that in words: the state, what is missing for what was asked for,
// and vpn-toggle when two monitors could be running (spec 085 R27).
func keepaliveLines(s keepaliveStanding) []string {
	var state string
	switch {
	case s.running && s.installed:
		state = "running"
	case s.running:
		state = "running, started by hand"
	case s.installed:
		state = "installed, not running"
	default:
		state = "not installed"
	}
	lines := []string{fmt.Sprintf("vpn keepalive: %s, switched on for %d VPN(s)", state, s.on)}
	if s.on > 0 && !s.running {
		lines = append(lines, "  warning: nothing is keeping those VPNs working. hayami-tui vpn-keepalive install starts it.")
	}
	if !s.installed && !s.running {
		return lines
	}
	switch {
	case s.toggleErr != nil:
		lines = append(lines, "  could not ask whether vpn-toggle is running: "+s.toggleErr.Error())
	case s.toggleActive:
		lines = append(lines, "  warning: vpn-toggle's service is running too, and two monitors on one VPN bounce each "+
			"other's tunnel. systemctl --user disable --now vpn-toggle stops it.")
	}
	return lines
}

// keepaliveDoctor prints that, where the helper can be installed.
func keepaliveDoctor(cmd *cobra.Command, f *flags) error {
	if !keepalive.Offered() {
		return nil
	}
	_, store, err := f.resolve()
	if err != nil {
		return err
	}
	defer func() { _ = store.Close() }()
	files, err := keepalive.Here()
	if err != nil {
		return err //nolint:wrapcheck // says what it was asking
	}
	s := keepaliveStanding{installed: keepalive.Installed(), running: files.Running()}
	for _, v := range config.Bandwidth.Get(store.Config()).VPNs {
		if v.Keepalive {
			s.on++
		}
	}
	if s.installed || s.running {
		ctx, cancel := context.WithTimeout(cmd.Context(), doctorDeadline)
		defer cancel()
		s.toggleActive, s.toggleErr = core.UserServiceActive(ctx, vpnToggleService)
	}
	for _, line := range keepaliveLines(s) {
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), line)
	}
	return nil
}

func runKeepalive(ctx context.Context, cmd *cobra.Command, f *flags) error {
	_, store, err := f.resolve()
	if err != nil {
		return err
	}
	path := store.Settings().Path()
	_ = store.Close()
	files, err := keepalive.Here()
	if err != nil {
		return err //nolint:wrapcheck // says what it was asking
	}
	host := core.NewHost(core.HostConfig{})
	log := func(text string) { _, _ = fmt.Fprintln(cmd.ErrOrStderr(), text) }

	return panel.RunKeepalive(ctx, panel.KeepaliveOptions{ //nolint:wrapcheck // says which step
		Systems: host.VPNs, Checkers: host.VPNCheckers, Files: files, Log: log,
		Settings: followBandwidth(path, log),
		Notify: func(ctx context.Context, text string) {
			limited, cancel := context.WithTimeout(ctx, notifyDeadline)
			defer cancel()
			if err := core.Notify(limited, "hayami: VPN", text); err != nil {
				log(err.Error())
			}
		},
	})
}

// followBandwidth is the Network section's settings as they are now: read again when
// the file changes, and as they were when it cannot be read, so that a save half done
// by preferences does not make the helper forget every VPN. Each version of the file
// is read once, so one that cannot be read is said once.
func followBandwidth(path string, log func(string)) func() config.BandwidthSettings {
	var seen time.Time
	var last config.BandwidthSettings
	return func() config.BandwidthSettings {
		info, err := os.Stat(path)
		if err != nil || info.ModTime().Equal(seen) {
			return last
		}
		seen = info.ModTime()
		store, err := config.Open(path)
		if err != nil {
			log("the settings could not be read: " + err.Error())
			return last
		}
		defer func() { _ = store.Close() }()
		if err := store.Unreadable(); err != nil {
			log("the settings could not be read: " + err.Error())
			return last
		}
		last = config.Bandwidth.Get(store.Config())
		return last
	}
}
