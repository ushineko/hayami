/*
Package installer is hayami's own code in its installer and uninstaller
(spec 059): what fynstall, which builds them from fynstall.yaml, cannot be
told in a config file.

It is two pages and two hooks. One page checks for LibreHardwareMonitor,
which is where the panel reads the processor's temperature on Windows (spec
036), says what it found, and offers to install and configure it. The other
does the same for hayami's network helper, the service behind the bandwidth
card's list of programs (spec 061). Neither offer is taken for the person:
each puts something on the machine that an administrator has to agree to.
After the install, a hook does what the pages were told. Before an
uninstall, another stops the panel, so Windows lets its files go, and
removes the helper when hayami is going for good.

On Linux the pages pass themselves over and the hooks do two other things
(spec 063): they put the udev rule that lets a person open their devices
where udev reads it, or say how, and they stop a panel that is running, so
an install over one leaves one panel on the desk and not two.

This file builds without a window. The page's window is in window.go, which
a command-line-only installer leaves out.
*/
package installer

import (
	"context"
	"fmt"

	"github.com/ushineko/fynstall/extension"
)

// ParamSensors is the parameter the page sets, declared in fynstall.yaml:
// Yes to install and configure LibreHardwareMonitor, No to leave it.
const ParamSensors = "sensors"

// The values of ParamSensors.
const (
	Yes = "yes"
	No  = "no"
)

// PageSensors is the page's ID.
const PageSensors = "sensors"

// ParamNetwork is the parameter the network page sets, declared in
// fynstall.yaml: Yes to install the network helper, No to leave it.
const ParamNetwork = "network"

// PageNetwork is the network page's ID.
const PageNetwork = "network"

// Extension is what fynstall links into the installer and the uninstaller.
// The name is fixed.
var Extension = extension.Extension{
	Pages: []extension.Page{{
		ID: PageSensors,
		// Before the summary, the zero place: the last question.
		Sets: []string{ParamSensors},
		CLI:  sensorsCLI,
	}, {
		ID:   PageNetwork,
		Sets: []string{ParamNetwork},
		CLI:  networkCLI,
	}},
	AfterInstall:    afterInstall,
	BeforeUninstall: beforeUninstall,
}

// probe finds what this machine has of LibreHardwareMonitor. A variable, so
// a test answers for a machine it is not on.
var probe = probeSensors

// setUp installs and configures LibreHardwareMonitor. A variable, for the
// same reason.
var setUp = setUpSensors

/*
offering is what a page that offers one thing says: what the check found,
the question, what saying yes does, and what a person is agreeing to. Both
pages are one of these, in a window and on the command line.
*/
type offering interface {
	// applies is false where there is nothing to check, and the page is
	// passed over.
	applies() bool
	// Found says what the check found, in a sentence or two.
	Found() string
	// Offer is the question, or "" when there is nothing to offer.
	Offer() string
	// Steps are what saying yes does, in order.
	Steps() []string
	why() string
	caution() string
}

/*
askCLI is a page on the command line: what was found, what saying yes does,
and one question. Where there is nothing to ask it says so and asks nothing,
and the parameter is no whatever a flag said: nothing is set up twice.
*/
func askCLI(param string, o offering, c *extension.Context, p extension.Prompter) error {
	if !o.applies() {
		return c.SetParam(param, No) //nolint:wrapcheck // names the parameter
	}
	p.Say("")
	p.Say(o.Found())
	offer := o.Offer()
	if offer == "" {
		return c.SetParam(param, No) //nolint:wrapcheck // names the parameter
	}
	p.Say("")
	p.Say(o.why())
	for _, step := range o.Steps() {
		p.Say("  - " + step)
	}
	p.Say(o.caution())
	ok, err := p.Confirm(offer + "?")
	if err != nil {
		return fmt.Errorf("asking %q: %w", offer, err)
	}
	value := No
	if ok {
		value = Yes
	}
	return c.SetParam(param, value) //nolint:wrapcheck // names the parameter
}

// sensorsCLI is the sensors page on the command line.
func sensorsCLI(ctx context.Context, c *extension.Context, p extension.Prompter) error {
	return askCLI(ParamSensors, probe(ctx), c, p)
}

// networkCLI is the network page on the command line.
func networkCLI(_ context.Context, c *extension.Context, p extension.Prompter) error {
	return askCLI(ParamNetwork, probeNet(c), c, p)
}

/*
afterInstall sets up what the pages were told to. hayami
is installed by now and works without it, so nothing here fails the install:
what went wrong is a warning in the installer's log, and the panel's own
reason line says what is missing afterwards.
*/
func afterInstall(ctx context.Context, c *extension.Context) error {
	if c.Param(ParamSensors) == Yes {
		c.Log(extension.Info, "Setting up LibreHardwareMonitor, for the processor's temperature")
		if err := setUp(ctx, c); err != nil {
			c.Logf(extension.Warn, "LibreHardwareMonitor was not set up, and hayami is installed without it: %v", err)
		}
	}
	if c.Param(ParamNetwork) == Yes {
		c.Log(extension.Info, "Installing the network helper, for the bandwidth card's list of programs")
		if err := setUpNet(ctx, c); err != nil {
			c.Logf(extension.Warn, "The network helper was not installed, and hayami is installed without it: %v", err)
		} else if n := probeNet(c); n.State.Installed {
			c.Logf(extension.Info, "The network helper is installed (%s)", n.State.Version)
		}
	}
	// Linux (spec 063). A panel that an earlier install left running is
	// stopped before the installer starts the new one, and the device rule
	// goes where it may.
	for _, name := range stopEarlier(c.Root()) {
		c.Logf(extension.Info, "Stopped %s, which was running from the earlier install", name)
	}
	placeRule(ctx, c)
	return nil
}

/*
beforeUninstall stops the panel and the terminal panel that run from the
install directory, and removes the network helper with an uninstall. Windows does not remove the file of a running program,
and an uninstall, or the first half of an upgrade, that leaves hayami.exe
behind has not removed hayami.
*/
func beforeUninstall(ctx context.Context, c *extension.Context) error {
	for _, name := range stopPrograms(c.Root()) {
		c.Logf(extension.Info, "Stopped %s, which was running", name)
	}
	// The helper outlives an upgrade: its program is a copy in Program
	// Files, and the page offers to bring it up to date. An uninstall takes
	// it too, while the program that removes it is still here.
	if !c.Upgrade() && probeNet(c).State.Installed {
		c.Log(extension.Info, "Removing the network helper; Windows asks for an administrator")
		if err := removeNet(ctx, c); err != nil {
			c.Logf(extension.Warn, "The network helper was not removed: %v. "+
				"To remove it, as an administrator: hayami-tui network-helper remove, "+
				"or sc.exe delete hayami-network", err)
		}
	}
	// The device rule goes with an uninstall and stays through an upgrade,
	// which would only put it back.
	if !c.Upgrade() {
		removeRule(ctx, c)
	}
	dropCopy(c)
	return nil
}
