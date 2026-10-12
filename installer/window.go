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
	gui.Page(PageSensors, func(c *extension.Context) wizard.Page { return &sensorsPage{c: c} })
}

// Checking is what the page says until the check answers.
const Checking = "Checking for LibreHardwareMonitor…"

// background runs the check off the window's thread, and onWindow puts its
// answer back on it. Variables, so a test runs both in line.
var (
	background = func(f func()) { go f() }
	onWindow   = fyne.Do
)

/*
sensorsPage is the sensors page's window: what the check found, and, where
there is something to offer, one check box with what ticking it does under
it. The box starts clear: the choice is the person's to make, never the
installer's.

The check runs each time the page is shown, so a person who sets
LibreHardwareMonitor up by hand and comes back sees it found.
*/
type sensorsPage struct {
	c *extension.Context

	found  *widget.Label
	offer  *widget.Check
	detail *fyne.Container
}

func (p *sensorsPage) Title() string { return "Sensors" }

// Skip passes the page over off Windows, where the kernel gives the
// processor's temperature and there is nothing to offer.
func (p *sensorsPage) Skip() bool { return runtime.GOOS != "windows" }

func (p *sensorsPage) Build(*wizard.Wizard) fyne.CanvasObject {
	p.found = wrapped(Checking)
	p.offer = widget.NewCheck("", func(on bool) {
		value := No
		if on {
			value = Yes
		}
		_ = p.c.SetParam(ParamSensors, value) // declared in fynstall.yaml
	})
	p.offer.Hide()
	p.detail = container.NewVBox()
	p.detail.Hide()
	return container.NewVBox(p.found, p.offer, p.detail)
}

// Enter runs the check and shows its answer.
func (p *sensorsPage) Enter(*wizard.Wizard) {
	background(func() {
		s := probe(context.Background())
		onWindow(func() { p.show(s) })
	})
}

// show draws what the check found.
func (p *sensorsPage) show(s Sensors) {
	if p.found == nil {
		return
	}
	p.found.SetText(s.Found())
	offer := s.Offer()
	if offer == "" {
		// Nothing to offer: whatever a flag asked for, nothing is set up.
		_ = p.c.SetParam(ParamSensors, No)
		p.offer.Hide()
		p.detail.Hide()
		return
	}
	p.offer.Text = offer
	p.offer.SetChecked(p.c.Param(ParamSensors) == Yes)
	p.offer.Refresh()
	p.offer.Show()

	// One label for the steps: a label each gave every step the height of
	// its own padding, and a list with gaps of different sizes.
	lines := s.Steps()
	for i := range lines {
		lines[i] = "-  " + lines[i]
	}
	steps := strings.Join(lines, "\n")
	p.detail.Objects = []fyne.CanvasObject{wrapped(Why), wrapped(steps), wrapped(Caution)}
	p.detail.Refresh()
	p.detail.Show()
}

func wrapped(text string) *widget.Label {
	l := widget.NewLabel(text)
	l.Wrapping = fyne.TextWrapWord
	return l
}
