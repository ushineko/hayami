package installer

import (
	"context"
	"path/filepath"

	"github.com/ushineko/fynstall/extension"

	"github.com/ushineko/hayami/internal/keepalive"
	"github.com/ushineko/hayami/internal/nethelper"
)

// Keepalive is what the installer found of the VPN keepalive helper (spec 085 R21): the
// check its page is built on. It reads a file and changes nothing.
type Keepalive struct {
	Offered, Installed bool
}

func (k Keepalive) applies() bool   { return k.Offered }
func (k Keepalive) why() string     { return keepalive.OfferWhy }
func (k Keepalive) caution() string { return keepalive.OfferCaution }

// Found says what the check found.
func (k Keepalive) Found() string {
	switch {
	case !k.Offered:
		return "This installer does not set up a VPN keepalive helper on this system. There is nothing to set up."
	case k.Installed:
		return "The VPN keepalive helper is installed, and this install restarts it on the new version. There is nothing to do."
	default:
		return "The VPN keepalive helper is not installed. " + keepalive.OfferWithout
	}
}

// Offer is the question the page asks, or "" when there is nothing to offer.
func (k Keepalive) Offer() string {
	if !k.Offered || k.Installed {
		return ""
	}
	return "Install the VPN keepalive helper"
}

// Steps are what installing it does, in order.
func (k Keepalive) Steps() []string { return keepalive.OfferSteps() }

// probeKeep finds what this machine has of the helper. A variable, so a test answers for
// a machine it is not on.
var probeKeep = func() Keepalive {
	return Keepalive{Offered: keepalive.Offered(), Installed: keepalive.Installed()}
}

// setUpKeep installs the helper from the program just installed, or restarts it on that
// program, and removeKeep removes it. Variables, for the same reason.
var (
	setUpKeep = func(ctx context.Context, c *extension.Context) error {
		return keepalive.Install(ctx, filepath.Join(c.Root(), "bin", nethelper.ProgramName)) //nolint:wrapcheck // says which step
	}
	removeKeep = func(ctx context.Context, _ *extension.Context) error {
		return keepalive.Remove(ctx) //nolint:wrapcheck // says which step
	}
)

// keepaliveCLI is the page on the command line.
func keepaliveCLI(_ context.Context, c *extension.Context, p extension.Prompter) error {
	return askCLI(ParamKeepalive, probeKeep(), c, p)
}

/*
placeKeepalive does what the page was told, and keeps an installed helper on the program
just installed: a running helper is the old program until it is restarted, and nothing
else would ever restart it. This is no new choice, and asks for no administrator. A
failure is a warning, as for the other pages: hayami works without it.
*/
func placeKeepalive(ctx context.Context, c *extension.Context) {
	asked := c.Param(ParamKeepalive) == Yes
	k := probeKeep()
	if !k.Offered || (!asked && !k.Installed) {
		return
	}
	if err := setUpKeep(ctx, c); err != nil {
		c.Logf(extension.Warn, "The VPN keepalive helper was not started on this version, and hayami is installed without it: %v", err)
		return
	}
	if asked {
		c.Log(extension.Info, "The VPN keepalive helper is installed and running")
	} else {
		c.Log(extension.Info, "The VPN keepalive helper was restarted on this version")
	}
}

// dropKeepalive removes the helper with an uninstall. It outlives an upgrade, whose
// install half restarts it.
func dropKeepalive(ctx context.Context, c *extension.Context) {
	if c.Upgrade() || !probeKeep().Installed {
		return
	}
	c.Log(extension.Info, "Removing the VPN keepalive helper")
	if err := removeKeep(ctx, c); err != nil {
		c.Logf(extension.Warn, "The VPN keepalive helper was not removed: %v. To remove it: hayami-tui vpn-keepalive remove", err)
	}
}
