package cli

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/ushineko/hayami/internal/desktop"
)

// PanelAppID is what the window rule matches on: the desktop panel's app ID.
const PanelAppID = "io.ushineko.hayami"

/*
windowCmd is the window rule from a terminal.

It exists so that a person who has made the panel frameless can undo it
**without the panel**. A glance window has no titlebar and no controls of its
own; if the rule is installed and something goes wrong with the menu, the only
way back would otherwise be System Settings or editing kwinrulesrc by hand. A
program that can put a rule on the user's desktop should be able to take it off
from a shell.
*/
func windowCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "window",
		Short: "The panel's KWin rule: no titlebar, always on top",
		Args:  cobra.NoArgs,
		RunE:  func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	cmd.AddCommand(windowStatusCmd(), windowInstallCmd(), windowRemoveCmd())
	return cmd
}

// windowStatusCmd says whether the rule is installed.
func windowStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Say whether the rule is installed, and at what opacity",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			current, err := desktop.Current(PanelAppID)
			if err != nil {
				return err
			}
			if !current.Installed {
				cmd.Println("No window rule. The panel has its titlebar.")
				return nil
			}
			cmd.Println("Frameless and on top.")
			return nil
		},
	}
}

// windowInstallCmd puts the rule in place.
func windowInstallCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "install",
		Short: "Take the panel's titlebar away and keep it on top",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			err := desktop.Install(PanelAppID)
			if err != nil && !errors.Is(err, desktop.ErrNoKWin) {
				return err
			}
			cmd.Println("Installed. The panel has no titlebar and stays on top.")
			if err != nil {
				// The rule is written; KWin was not there to be told. It
				// applies when one starts, which is worth saying rather than
				// leaving the user to wonder why nothing changed.
				cmd.Println("KWin did not answer, so it takes effect when it next starts.")
			}
			return nil
		},
	}
	return cmd
}

// windowRemoveCmd takes the rule out again.
func windowRemoveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "remove",
		Short: "Give the panel its titlebar back",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			err := desktop.Remove(PanelAppID)
			if err != nil && !errors.Is(err, desktop.ErrNoKWin) {
				return fmt.Errorf("%w", err)
			}
			cmd.Println("Removed. The panel has its titlebar back.")
			return nil
		},
	}
}
