package cli

import (
	"context"
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
	cmd.AddCommand(run)
	return cmd
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
