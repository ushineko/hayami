package cli

import (
	"errors"
	"fmt"
	"os"
	"runtime"

	"github.com/spf13/cobra"

	"github.com/ushineko/hayami/internal/api/unit"
	"github.com/ushineko/hayami/internal/nethelper"
)

/*
apiServiceCmd is the installer's step for the service other programs read on
Windows (spec 113 R6, R25), and the install scripts': `api-service install`
registers the service or updates it on this program, `remove` removes it.
Each needs an administrator, and run without one asks Windows for one (one
UAC prompt), as `network-helper` does. Hidden: install and removal are the
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
		if runtime.GOOS != "windows" {
			// Elsewhere the installer sets the service up as the person,
			// never as an administrator.
			return errors.New("this is the installer's step on Windows; here it sets the service up as you") //nolint:err113 // said plainly
		}
		if !nethelper.Elevated() {
			return elevate(cmd, "api-service", name)
		}
		return do()
	}
	install := &cobra.Command{
		Use: "install", Short: "Register the service to run a copy of this program, or update it",
		Args: cobra.NoArgs, SilenceUsage: true, SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return step(cmd, "install", func() error {
				self, err := os.Executable()
				if err != nil {
					return fmt.Errorf("finding this program: %w", err)
				}
				return unit.Install(cmd.Context(), self) //nolint:wrapcheck // says which step
			})
		},
	}
	remove := &cobra.Command{
		Use: "remove", Short: "Stop and unregister the service, and remove its program",
		Args: cobra.NoArgs, SilenceUsage: true, SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return step(cmd, "remove", func() error {
				return unit.Remove(cmd.Context()) //nolint:wrapcheck // says which step
			})
		},
	}
	cmd.AddCommand(install, remove)
	return cmd
}
