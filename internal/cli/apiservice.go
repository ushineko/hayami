package cli

import (
	"errors"
	"fmt"
	"os"
	"runtime"

	"github.com/spf13/cobra"

	"github.com/ushineko/hayami/internal/api/tcpkey"
	"github.com/ushineko/hayami/internal/api/unit"
	"github.com/ushineko/hayami/internal/nethelper"
)

/*
apiServiceCmd is the installer's step for the service other programs read on
Windows (spec 113 R6, R25), and the install scripts': `api-service install`
registers the service or updates it on this program, `remove` removes it.
Each needs an administrator there, and run without one asks Windows for one
(one UAC prompt), as `network-helper` does. On macOS it writes and loads the
person's launchd agent, or removes it, as the person (R7): the route there
until fynstall builds a macOS installer. Hidden: install and removal are the
installer's, and there is no `api install` for a person to run.
*/
func apiServiceCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use: "api-service", Short: "The installer's step for the service other programs read",
		Hidden: true, Args: cobra.NoArgs, SilenceUsage: true, SilenceErrors: true,
	}
	// step runs do as an administrator: here, or in this program run again
	// as one.
	step := func(cmd *cobra.Command, name string, do func() error) error {
		switch runtime.GOOS {
		case "darwin":
			// The agent is the person's own, and needs no administrator.
			// Until fynstall builds an installer for macOS, this is how it
			// is set up there (spec 113, phase 5a).
			return do()
		case "windows":
		default:
			// On Linux the installer sets the service up as the person,
			// never as an administrator.
			return errors.New("this is the installer's step on Windows and macOS; here the installer sets the service up as you") //nolint:err113 // said plainly
		}
		if !nethelper.Elevated() {
			return elevate(cmd, "api-service", name)
		}
		return do()
	}
	var withTCP bool
	install := &cobra.Command{
		Use: "install", Short: "Register the service to run a copy of this program, or update it",
		Args: cobra.NoArgs, SilenceUsage: true, SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			first := !unit.Installed()
			return step(cmd, "install", func() error {
				self, err := os.Executable()
				if err != nil {
					return fmt.Errorf("finding this program: %w", err)
				}
				if err := unit.Install(cmd.Context(), self); err != nil {
					return err //nolint:wrapcheck // says which step
				}
				// On macOS this is the installer's route, and the port is on
				// by default there as the installer's page has it (spec 115
				// R12): set up on a first install, and an install over one
				// leaves the port as it found it.
				if runtime.GOOS == "darwin" && first && withTCP {
					return unit.InstallTCP(cmd.Context(), self, currentPort(), false) //nolint:wrapcheck // says which step
				}
				return nil
			})
		},
	}
	install.Flags().BoolVar(&withTCP, "tcp", true, "on macOS, also set up the loopback TCP port on a first install (spec 115)")
	remove := &cobra.Command{
		Use: "remove", Short: "Stop and unregister the service, and remove its program",
		Args: cobra.NoArgs, SilenceUsage: true, SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return step(cmd, "remove", func() error {
				return unit.Remove(cmd.Context()) //nolint:wrapcheck // says which step
			})
		},
	}
	cmd.AddCommand(install, remove, apiTCPCmd())
	return cmd
}

/*
apiTCPCmd sets up, moves or removes the service's loopback TCP port (spec 115
R13), or gives it a new token. It runs as the person on every system and
never asks for an administrator: the port's key belongs in the person's own
settings directory. On Linux and macOS it acts on the installed service,
written again with its own program and the port; on Windows it writes the
key, which the account's instance reads when it next starts.
*/
func apiTCPCmd() *cobra.Command {
	var port int
	var remove, renew bool
	cmd := &cobra.Command{
		Use:   "tcp",
		Short: "Set up the service's loopback TCP port, move it, give it a new token, or remove it",
		Args:  cobra.NoArgs, SilenceUsage: true, SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			program, err := unit.InstalledProgram()
			if err != nil {
				return err //nolint:wrapcheck // says the service is not installed
			}
			if remove {
				if err := unit.RemoveTCP(cmd.Context(), program); err != nil {
					return err //nolint:wrapcheck // says which step
				}
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), "The TCP port is removed.")
				return nil
			}
			if port == 0 {
				port = currentPort()
			}
			if port < 1 || port > 65535 {
				return fmt.Errorf("%d is not a port", port) //nolint:err113 // said plainly
			}
			if err := unit.InstallTCP(cmd.Context(), program, port, renew); err != nil {
				return err //nolint:wrapcheck // says which step
			}
			path, _ := tcpkey.Path()
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "The service listens on %s; its token is in %s.\n", tcpkey.Address(port), path)
			return nil
		},
	}
	cmd.Flags().IntVar(&port, "port", 0, fmt.Sprintf("the port; the one set up already, or %d", tcpkey.DefaultPort))
	cmd.Flags().BoolVar(&remove, "remove", false, "remove the port and its key; the socket stays")
	cmd.Flags().BoolVar(&renew, "new-token", false, "replace the token; programs holding the old one are refused")
	return cmd
}

// currentPort is the port already set up, or the default.
func currentPort() int {
	path, err := tcpkey.Path()
	if err != nil {
		return tcpkey.DefaultPort
	}
	if k, err := tcpkey.Read(path); err == nil {
		return k.Port()
	}
	return tcpkey.DefaultPort
}
