/*
Package prefs is the preferences window: the other archetype.

The panel is a glance window — no header, no navigation, no controls — and its
rules are fynedesygn's docs/glance.md. This is a shell window, and its rules
are docs/design-system.md. The two disagree about almost everything
structural, and reading the wrong page is the likely mistake here.

It is a second window of the same process rather than a second binary. The
panel and this share one settings store, and a change made here has to reach
the panel as it is made; two processes would mean a file watcher and a race for
the last write.
*/
package prefs

import (
	"fyne.io/fyne/v2"
	fynetheme "fyne.io/fyne/v2/theme"

	"github.com/ushineko/fynedesygn/shell"

	"github.com/ushineko/hayami/internal/config"
)

// AppID names the preferences window for a compositor. It is the panel's own
// with a suffix, so a window rule can tell the two apart.
const AppID = "io.ushineko.hayami.preferences"

// Options are what the window needs.
type Options struct {
	// Store is the settings, shared with the panel.
	Store *config.Store

	// Version is shown in the window's title.
	Version string

	// OnChange is called after every change, on the UI thread, so the panel
	// can follow without a restart.
	OnChange func()
}

// Window is the preferences window.
type Window struct {
	shell *shell.Shell
	opts  Options
}

// New builds the window over an app that already exists. It is not shown.
func New(a fyne.App, o Options) *Window {
	w := &Window{opts: o}
	w.shell = shell.NewIn(a, shell.Options{
		AppID:   AppID,
		Name:    "hayami preferences",
		Version: o.Version,
		Sections: []shell.Section{
			shell.NewSection("Sections", fynetheme.ListIcon, w.buildSections),
			shell.NewSection("Bandwidth", fynetheme.ComputerIcon, w.buildBandwidth),
			shell.AppearanceSection("Saved as you change it."),
		},
	})
	return w
}

// Shell is the window's shell, for a caller that wants its window.
func (w *Window) Shell() *shell.Shell { return w.shell }

// Show brings the window up, raising it when it is already there.
//
// One window, however many times the menu is used: a second copy of a
// preferences window is two views of one file, and the one nobody is looking
// at is the one that overwrites.
func (w *Window) Show() {
	w.shell.Window.Show()
	w.shell.Window.RequestFocus()
}

// save writes a changed configuration and tells the panel.
//
// The store debounces its own writes, so there is no save button and nothing
// to press: a preference that needed confirming is a preference somebody
// abandons half-set.
func (w *Window) save(c config.Config) {
	if err := w.opts.Store.SetConfig(c); err != nil {
		w.shell.Report("Saving settings", err)
		return
	}
	if w.opts.OnChange != nil {
		w.opts.OnChange()
	}
}
