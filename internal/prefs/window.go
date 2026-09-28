package prefs

import (
	"errors"
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	fd "github.com/ushineko/fynedesygn"
	"github.com/ushineko/fynedesygn/shell"
	"github.com/ushineko/fynedesygn/widgets"

	"github.com/ushineko/hayami/internal/desktop"
)

// PanelAppID is the app ID the window rule matches on.
//
// It is the desktop panel's, not this window's: the rule is about the panel,
// and the preferences window is an ordinary window that should keep its
// titlebar. They are different strings for that reason and the difference is
// worth stating, because a rule that matched this window too would strip the
// controls off the screen the user configures it from.
const PanelAppID = "io.ushineko.hayami"

/*
buildWindow is what the compositor grants: frameless, on top, translucent.

None of the three is the toolkit's to give, and all three arrive together in
one KWin rule, so they are one control rather than three. The rule is a write
into the user's own kwinrulesrc — the file holding every window rule they
have — so it happens here, when they ask, and not on a first run.
*/
func (w *Window) buildWindow(s *shell.Shell) fyne.CanvasObject {
	current, err := desktop.Current(PanelAppID)
	if err != nil {
		return container.NewVBox(widgets.Dim("The window rules could not be read: " + err.Error()))
	}

	c := w.opts.Store.Config()
	opacity := c.OpacityOrDefault()

	value := widget.NewLabel(fmt.Sprintf("%d %%", opacity))
	value.Importance = widget.LowImportance
	slider := widget.NewSlider(50, 100)
	slider.Step = 5
	slider.SetValue(float64(opacity))

	rule := widget.NewCheck("Frameless, always on top, and translucent", nil)
	rule.SetChecked(current.Installed)

	// The state is read back from the rule file rather than kept in the
	// settings, because the rule is the truth: a user who removes it in System
	// Settings has removed it, and a checkbox remembering otherwise would be
	// this program disagreeing with the desktop about what is on screen.
	rule.OnChanged = func(on bool) { w.setRule(s, on) }

	slider.OnChanged = func(v float64) { value.SetText(fmt.Sprintf("%d %%", int(v))) }
	slider.OnChangeEnded = func(v float64) { w.setOpacity(int(v)) }

	return container.NewVBox(
		widgets.Dim("What the compositor grants. A glance window is read without being touched, "+
			"so it has no titlebar and sits above other windows."),
		rule,
		widgets.Dim("Installs a KWin rule you can see and remove in System Settings. "+
			"Plasma only, and only for the titlebar: nothing else here needs it."),
		widget.NewSeparator(),
		widgets.Dim("How solid the cards are. The space around them is always clear, "+
			"so the desktop shows through the panel whatever this says."),
		container.NewBorder(nil, nil, nil, value, slider),
		widgets.Dim("Drawn by the panel itself, so it works on any desktop."),
	)
}

// setRule installs or removes the rule and says what happened.
func (w *Window) setRule(s *shell.Shell, on bool) {
	var err error
	if on {
		err = desktop.Install(PanelAppID)
	} else {
		err = desktop.Remove(PanelAppID)
	}
	w.report(s, err, on)
}

// setOpacity saves the opacity, which is all it has to do.
//
// The panel watches its own settings and re-fades its cards, and nothing about
// this reaches the compositor: the opacity is the toolkit's and works on any
// desktop, unlike the rule above it on this screen.
func (w *Window) setOpacity(opacity int) {
	c := w.opts.Store.Config()
	c.Opacity = opacity
	w.save(c)
}

// report says how a compositor call went.
//
// A desktop without KWin is told plainly, and told as a *fact* rather than as
// a failure: nothing is broken, the feature is not available here, and that is
// the same answer the cooler gives a machine with no liquidctl. It goes
// through Flash rather than Report for exactly that reason — Report renders
// everything as "... failed".
func (w *Window) report(s *shell.Shell, err error, on bool) {
	switch {
	case err == nil && on:
		s.OK("The panel is frameless and on top.")
	case err == nil:
		s.OK("The panel has its titlebar back.")
	case errors.Is(err, desktop.ErrNoKWin):
		s.Flash("This desktop is not Plasma, so the panel is unchanged.", fd.StatusWarn)
	default:
		s.Report("Changing the window", err)
	}
}
