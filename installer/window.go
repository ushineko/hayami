//go:build !nogui

package installer

import (
	"context"
	"runtime"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"github.com/ushineko/fynedesygn/wizard"

	"github.com/ushineko/fynstall/extension"
	"github.com/ushineko/fynstall/extension/gui"
)

func init() { //nolint:gochecknoinits // how fynstall is given a page's window
	gui.Page(PageSensors, func(c *extension.Context) wizard.Page {
		return &offerPage{c: c, title: "Sensors", param: ParamSensors, checking: Checking,
			check: func(ctx context.Context) offering { return probe(ctx) },
			// The kernel gives the processor's temperature off Windows.
			shown: func() bool { return runtime.GOOS == "windows" }}
	})
	gui.Page(PageNetwork, func(c *extension.Context) wizard.Page {
		return &offerPage{c: c, title: "Network", param: ParamNetwork, checking: CheckingNetwork,
			check: func(context.Context) offering { return probeNet(c) },
			// Wherever the installer can offer it: Windows, and Linux with
			// polkit's pkexec (spec 078).
			shown: func() bool { return probeNet(c).applies() }}
	})
}

// Checking is what the sensors page says until the check answers.
const Checking = "Checking for LibreHardwareMonitor…"

// CheckingNetwork is what the network page says until the check answers.
const CheckingNetwork = "Checking for the network helper…"

// background runs the check off the window's thread, and onWindow puts its
// answer back on it. Variables, so a test runs both in line.
var (
	background = func(f func()) { go f() }
	onWindow   = fyne.Do
)

/*
offerPage is the window of a page that offers one thing: what the check
found, and, where there is something to offer, one check box with what
ticking it does under it. The box starts clear: the choice is the person's
to make, never the installer's.

The check runs each time the page is shown, so a person who sets the thing
up by hand and comes back sees it found.
*/
type offerPage struct {
	c *extension.Context

	title    string
	param    string
	checking string
	check    func(context.Context) offering
	// shown is whether the page has anything to say here; asked before the
	// page is drawn, so it is quick.
	shown func() bool

	found  *widget.Label
	offer  *widget.Check
	detail *fyne.Container
}

func (p *offerPage) Title() string { return p.title }

// Skip passes the page over where it has nothing to offer. It was "off
// Windows" for both pages, which skipped the network page on Linux whatever
// its own check said (#257).
func (p *offerPage) Skip() bool { return !p.shown() }

func (p *offerPage) Build(*wizard.Wizard) fyne.CanvasObject {
	p.found = wrapped(p.checking)
	p.offer = widget.NewCheck("", func(on bool) {
		value := No
		if on {
			value = Yes
		}
		_ = p.c.SetParam(p.param, value) // declared in fynstall.yaml
	})
	p.offer.Hide()
	p.detail = container.NewVBox()
	p.detail.Hide()
	return container.NewVBox(p.found, p.offer, p.detail)
}

// Enter runs the check and shows its answer.
func (p *offerPage) Enter(*wizard.Wizard) {
	background(func() {
		o := p.check(context.Background())
		onWindow(func() { p.show(o) })
	})
}

// show draws what the check found.
func (p *offerPage) show(o offering) {
	if p.found == nil {
		return
	}
	p.found.SetText(o.Found())
	offer := o.Offer()
	if offer == "" {
		// Nothing to offer: whatever a flag asked for, nothing is set up.
		_ = p.c.SetParam(p.param, No)
		p.offer.Hide()
		p.detail.Hide()
		return
	}
	p.offer.Text = offer
	p.offer.SetChecked(p.c.Param(p.param) == Yes)
	p.offer.Refresh()
	p.offer.Show()

	// One label for the steps: a label each gave every step the height of
	// its own padding, and a list with gaps of different sizes.
	lines := o.Steps()
	for i := range lines {
		lines[i] = "-  " + lines[i]
	}
	steps := strings.Join(lines, "\n")
	p.detail.Objects = []fyne.CanvasObject{wrapped(o.why()), wrapped(steps), wrapped(o.caution())}
	p.detail.Refresh()
	p.detail.Show()
}

func wrapped(text string) *widget.Label {
	l := widget.NewLabel(text)
	l.Wrapping = fyne.TextWrapWord
	return l
}
