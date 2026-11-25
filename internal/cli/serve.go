package cli

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/ushineko/hayami/internal/api"
	"github.com/ushineko/hayami/internal/config"
)

/*
serveCmd runs the service other programs read (spec 113 R1): the readings,
their details and drills as JSON over a socket only this account can open.
It runs in the foreground until stopped; it is what the systemd unit runs,
and what a person runs by hand to try it.
*/
func serveCmd(f *flags, version string) *cobra.Command {
	var listen string
	var idle time.Duration
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Serve the readings to programs of this account, as JSON over a local socket",
		Long: "Serve the readings to programs of this account, as JSON over a local socket.\n\n" +
			"The readings, their details and drills, under the contract in api/openapi.yaml.\n" +
			"Only this account can open the socket. A section is read only while a client\n" +
			"reads it. Runs until stopped; started by its systemd socket, it exits after\n" +
			"five minutes with nobody reading.",
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			path := f.settings
			if path == "" {
				p, err := config.Path()
				if err != nil {
					return err
				}
				path = p
			}
			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			return api.Serve(ctx, api.Options{
				Listen: listen, SettingsPath: path, Idle: idle, Version: version,
				Log: slog.New(slog.NewTextHandler(cmd.ErrOrStderr(), nil)),
			})
		},
	}
	cmd.Flags().StringVar(&listen, "listen", "", "the socket to make, in place of the default address")
	cmd.Flags().DurationVar(&idle, "idle", 0, "exit after this long with nobody reading (default: 5m when started by its socket, never otherwise)")
	return cmd
}
