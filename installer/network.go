package installer

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/ushineko/fynstall/extension"

	"github.com/ushineko/hayami/internal/nethelper"
)

/*
Network is what the installer found of hayami's network helper (spec 061):
the check the network page is built on. It reads the service manager and
changes nothing, and needs no administrator.
*/
type Network struct {
	// State is the helper as it is on this machine.
	State nethelper.State
	// Version is the hayami this installer carries.
	Version string
}

func (n Network) applies() bool   { return n.State.Applies }
func (n Network) why() string     { return NetworkWhy }
func (n Network) caution() string { return NetworkCaution }

// Current reports whether the helper is this installer's version: there is
// nothing to do.
func (n Network) Current() bool { return n.State.Installed && n.State.Version == n.Version }

// Found says what the check found.
func (n Network) Found() string {
	switch {
	case !n.State.Applies:
		return "This system gives hayami the network by program itself. There is nothing to set up."
	case n.Current():
		return fmt.Sprintf("The network helper is installed (%s): the bandwidth card's list of programs is read from it. There is nothing to do.", n.Version)
	case n.State.Installed:
		was := n.State.Version
		if was == "" {
			was = "an unknown version"
		}
		return fmt.Sprintf("The network helper is installed (%s), and this installer carries %s. The one installed goes on working.", was, n.Version)
	default:
		return "The network helper is not installed. Without it the bandwidth card's list of programs says it needs the helper, and the rest of the panel is as it is."
	}
}

// Offer is the question the page asks, or "" when there is nothing to
// offer.
func (n Network) Offer() string {
	switch {
	case !n.State.Applies || n.Current():
		return ""
	case n.State.Installed:
		return "Update the network helper"
	default:
		return "Install the network helper"
	}
}

// NetworkWhy is why the page exists at all, said once above the steps.
const NetworkWhy = "Windows gives the network by program only to an administrator, and the panel runs as you. " +
	"The helper is a small Windows service that reads it and hands the panel program IDs and byte counts, and nothing else. Installing it:"

// NetworkCaution is what a person should know before saying yes.
const NetworkCaution = "It is a service, and stays until it is removed: uninstalling hayami removes it, and asks for an administrator to. " +
	"The panel works without it, and you can install it later by running this installer again."

// Steps are what installing it does, in order.
func (n Network) Steps() []string {
	return []string{
		"copies hayami's terminal program to Program Files, where only an administrator can change it",
		"registers the service hayami-network to run that copy as Local Service, which is not an administrator",
		"lets your account, and no other, start the service and read what it serves",
		"starts nothing: the service runs while a panel shows the list and stops when it is closed",
		"Windows asks for an administrator once, for all of this",
	}
}

// probeNet finds what this machine has of the helper. A variable, so a test
// answers for a machine it is not on.
var probeNet = func(c *extension.Context) Network {
	return Network{State: nethelper.Probe(), Version: c.App().Version}
}

// setUpNet installs the helper, and removeNet removes it. Variables, for
// the same reason.
var (
	setUpNet  = setUpNetwork
	removeNet = removeNetwork
)

// helperProgram is the installed terminal program, which is what installs
// and removes the helper.
func helperProgram(c *extension.Context) string {
	return filepath.Join(c.Root(), "bin", "hayami-tui.exe")
}

/*
setUpNetwork installs the helper by running the installed hayami-tui's own
`network-helper install` as an administrator: the steps that command has
always taken, so there is one description of them. The account that may read
the helper is the one running this installer, named to the elevated program
because an administrator's consent may be given by another account.
*/
func setUpNetwork(ctx context.Context, c *extension.Context) error {
	sid, err := nethelper.CurrentUser()
	if err != nil {
		return err //nolint:wrapcheck // says what it was asking
	}
	if err := nethelper.RunElevated(ctx, helperProgram(c), "network-helper", "install", "--allow", sid); err != nil {
		return err //nolint:wrapcheck // says what Windows answered
	}
	if !nethelper.Probe().Installed {
		return errors.New("the elevated step ended and the service is not registered") //nolint:err113 // said plainly
	}
	return nil
}

// removeNetwork removes the helper the same way.
func removeNetwork(ctx context.Context, c *extension.Context) error {
	if err := nethelper.RunElevated(ctx, helperProgram(c), "network-helper", "remove"); err != nil {
		return err //nolint:wrapcheck // says what Windows answered
	}
	if nethelper.Probe().Installed {
		return errors.New("the elevated step ended and the service is still registered") //nolint:err113 // said plainly
	}
	return nil
}
