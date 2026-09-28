package prefs

import (
	"errors"
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	fd "github.com/ushineko/fynedesygn"
	"github.com/ushineko/fynedesygn/dialogs"
	"github.com/ushineko/fynedesygn/shell"
	fdtheme "github.com/ushineko/fynedesygn/theme"
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

	panel := c.PanelAppearance(fdtheme.LoadAppearanceFrom(
		w.opts.Store.Settings(), fyne.CurrentApp().Preferences()))

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
		widgets.DimWrapped("The panel's own faces and size. This window keeps its own."),
		w.faces(s, panel),
		widget.NewSeparator(),
		widgets.DimWrapped("What the compositor grants."),
		rule,
		widgets.DimWrapped("A KWin rule, in System Settings. Plasma only."),
		widgets.DimWrapped("On now; off at the next start."),
		widget.NewSeparator(),
		widgets.DimWrapped("How solid the cards are; the space around them is always clear."),
		container.NewBorder(nil, nil, nil, value, slider),
		widgets.DimWrapped("Drawn by the panel, on any desktop."),
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

// setFontSize saves the panel's own text size.
//
// The panel watches its settings and re-themes itself; nothing here reaches
// the preferences window, which is the point of the setting.
func (w *Window) setFontSize(size float32) {
	c := w.opts.Store.Config()
	c.FontSize = size
	w.save(c)
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
		// Deliberately not "the titlebar is back". KWin takes decoration away
		// from a window already on screen and will not give it back: that
		// happens when the window is next created. Saying otherwise sends the
		// user looking for a change that is not going to arrive.
		s.OK("Removed. The panel gets its titlebar back when it next starts.")
	case errors.Is(err, desktop.ErrNoKWin):
		s.Flash("This desktop is not Plasma, so the panel is unchanged.", fd.StatusWarn)
	default:
		s.Report("Changing the window", err)
	}
}

/*
faces is the panel's two font choosers and its size.

**Two faces, not one.** The panel draws labels in the interface family and
readings in the monospace one, and they are chosen separately for the reason
the design system keeps them apart: a label is read as words and a reading is
read as a column, and a column needs every digit the same width. A
proportional family chosen as the monospace face is not a matter of taste, it
is a mistake — which is why the chooser for it offers only the families that
measured as monospace.

The choosers are the design system's own, so they show a sample in the
highlighted family and change nothing until Choose. A dropdown of three
hundred names in a face that tells you nothing about any of them is not a
choice, it is a lottery.
*/
func (w *Window) faces(s *shell.Shell, panel fdtheme.Appearance) fyne.CanvasObject {
	var face, mono *widget.Button

	face = widget.NewButton(faceLabel(panel.Font), func() {
		dialogs.ChooseFont(s.Window, "The panel's interface font", panel.Font, panel, false,
			func(name string) {
				face.SetText(faceLabel(name))
				w.setFace(name, "")
			})
	})
	mono = widget.NewButton(faceLabel(panel.Mono), func() {
		dialogs.ChooseFont(s.Window, "The panel's monospace font", panel.Mono, panel, true,
			func(name string) {
				mono.SetText(faceLabel(name))
				w.setFace("", name)
			})
	})

	value := widget.NewLabel(fmt.Sprintf("%g pt", panel.TextSize))
	value.Importance = widget.LowImportance

	sizes := fdtheme.TextSizes()
	names := make([]string, 0, len(sizes))
	for _, v := range sizes {
		names = append(names, fmt.Sprintf("%g", v))
	}
	size := widget.NewSelect(names, func(name string) {
		for _, v := range sizes {
			if fmt.Sprintf("%g", v) == name {
				value.SetText(fmt.Sprintf("%g pt", v))
				w.setFontSize(v)
			}
		}
	})
	size.SetSelected(fmt.Sprintf("%g", panel.TextSize))

	return container.NewVBox(
		container.NewBorder(nil, nil, widgets.Dim("Interface"), nil, face),
		container.NewBorder(nil, nil, widgets.Dim("Monospace"), nil, mono),
		container.NewBorder(nil, nil, widgets.Dim("Size"), value, size),
	)
}

// faceLabel names a family on a chooser's button, as the design system's own
// appearance screen does: the ellipsis says the button opens something.
func faceLabel(name string) string {
	if name == "" {
		name = fdtheme.DefaultFontName
	}
	return name + "…"
}

// setFace saves one of the panel's two families. An empty name leaves that one
// alone, so the two callers do not have to read the config first.
func (w *Window) setFace(face, mono string) {
	c := w.opts.Store.Config()
	if face != "" {
		c.Font = face
	}
	if mono != "" {
		c.Mono = mono
	}
	w.save(c)
}
