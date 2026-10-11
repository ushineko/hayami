/*
Package installer is hayami's own code in its installer and uninstaller
(spec 059): what fynstall, which builds them from fynstall.yaml, cannot be
told in a config file.

It is one page and two hooks. The page checks for LibreHardwareMonitor,
which is where the panel reads the processor's temperature on Windows (spec
036), says what it found, and offers to install and configure it. The offer
is never taken for the person: LibreHardwareMonitor is a second program, it
runs as administrator, and it loads a kernel driver. After the install, a
hook does what the page was told. Before an uninstall, another stops the
panel, so Windows lets its files go.

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

// Extension is what fynstall links into the installer and the uninstaller.
// The name is fixed.
var Extension = extension.Extension{
	Pages: []extension.Page{{
		ID: PageSensors,
		// Before the summary, the zero place: the last question.
		Sets: []string{ParamSensors},
		CLI:  sensorsCLI,
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
sensorsCLI is the page on the command line: what was found, what setting it
up does, and one question. Where there is nothing to ask (it is set up
already, or this is not Windows) it says so and asks nothing.
*/
func sensorsCLI(ctx context.Context, c *extension.Context, p extension.Prompter) error {
	s := probe(ctx)
	if !s.Applies {
		return c.SetParam(ParamSensors, No) //nolint:wrapcheck // names the parameter
	}
	p.Say("")
	p.Say(s.Found())
	offer := s.Offer()
	if offer == "" {
		return c.SetParam(ParamSensors, No) //nolint:wrapcheck // names the parameter
	}
	p.Say("")
	p.Say(Why)
	for _, step := range s.Steps() {
		p.Say("  - " + step)
	}
	p.Say(Caution)
	ok, err := p.Confirm(offer + "?")
	if err != nil {
		return fmt.Errorf("asking about LibreHardwareMonitor: %w", err)
	}
	value := No
	if ok {
		value = Yes
	}
	return c.SetParam(ParamSensors, value) //nolint:wrapcheck // names the parameter
}

/*
afterInstall sets LibreHardwareMonitor up when the page was told to. hayami
is installed by now and works without it, so nothing here fails the install:
what went wrong is a warning in the installer's log, and the panel's own
reason line says what is missing afterwards.
*/
func afterInstall(ctx context.Context, c *extension.Context) error {
	if c.Param(ParamSensors) != Yes {
		return nil
	}
	c.Log(extension.Info, "Setting up LibreHardwareMonitor, for the processor's temperature")
	if err := setUp(ctx, c); err != nil {
		c.Logf(extension.Warn, "LibreHardwareMonitor was not set up, and hayami is installed without it: %v", err)
	}
	return nil
}

/*
beforeUninstall stops the panel and the terminal panel that run from the
install directory. Windows does not remove the file of a running program,
and an uninstall, or the first half of an upgrade, that leaves hayami.exe
behind has not removed hayami.
*/
func beforeUninstall(_ context.Context, c *extension.Context) error {
	for _, name := range stopPrograms(c.Root()) {
		c.Logf(extension.Info, "Stopped %s, which was running", name)
	}
	return nil
}
