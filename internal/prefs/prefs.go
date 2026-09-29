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

	"github.com/ushineko/fynedesygn/markdown"
	"github.com/ushineko/fynedesygn/shell"
	fdtheme "github.com/ushineko/fynedesygn/theme"

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

	// Theme turns the chosen appearance into the theme to apply.
	//
	// It exists because the Appearance screen is the design system's own and
	// saves through the shell rather than through this program's settings, so
	// OnChange never sees it. The shell calls this whenever the appearance
	// changes, which makes it the one hook that does — and the panel needs it
	// twice over: to re-apply the card opacity, which setting a theme
	// replaces, and to repaint cards that draw from their own objects rather
	// than from the canvas.
	//
	// Nil means the appearance's own theme, unwrapped.
	Theme func(a fdtheme.Appearance) fyne.Theme
}

// Window is the preferences window.
type Window struct {
	shell *shell.Shell
	opts  Options

	// readme is the document pane in About while it is on screen.
	readme *markdown.Pane

	// showAll keeps the interface list expanded while the window is open. It
	// is not a setting: the long list is for finding something once, not a
	// state anybody wants restored.
	showAll bool
}

// New builds the window over an app that already exists. It is not shown.
func New(a fyne.App, o Options) *Window {
	w := &Window{opts: o}
	w.shell = shell.NewIn(a, shell.Options{
		AppID:   AppID,
		Name:    "hayami preferences",
		Version: o.Version,
		Theme:   o.Theme,

		// Not the master window. Closing a master window exits the
		// application, and this one belongs to a panel: closing the
		// preferences took the panel with it, which is how it was reported —
		// "how do I dismiss the preferences window without closing the whole
		// app?"
		Secondary: true,

		// Refresh rebuilds the current screen, which earns its place when a
		// screen is a view of something that changes elsewhere. Every screen
		// here holds settings and saves as it is changed, so there is nothing
		// to re-fetch and the button visibly did nothing.
		NoRefresh: true,

		// This window owns the application's theme, and it is the right one
		// to: it is the window with *overlays*. A font chooser, a dropdown
		// and the context menu are added to the canvas's overlay stack
		// rather than to a window's content, so nothing can override them --
		// whatever the application's theme is, an overlay wears it.
		//
		// It used to be the panel that owned it, with this window taking a
		// subtree override, and the font chooser opened in the panel's face
		// and the panel's card fade. The panel carries its own theme now
		// (fynedesygn spec 042) and needs nothing from the application, so
		// there is nothing left to compete over and no override to keep in
		// step.

		// The navigation's shape is the user's. Four sections is few enough
		// that icons alone are legible and a top strip is a reasonable choice
		// on a wide screen, so the shell draws its own control for these and
		// binds its shortcut.
		NavModes:      []shell.NavMode{shell.NavLabels, shell.NavIcons, shell.NavHidden},
		NavPlacements: []shell.NavPlacement{shell.NavLeft, shell.NavTop},
		Sections: []shell.Section{
			shell.NewSection("Sections", fynetheme.ListIcon, w.buildSections),
			shell.NewSection("Window", fynetheme.ViewFullScreenIcon, w.buildWindow),
			shell.AppearanceSection("Saved as you change it."),
			shell.NewSection("About", fynetheme.HelpIcon, w.buildAbout).
				OnDetach(w.detachAbout),
		},
	})
	w.hideOnClose()
	return w
}

// Shell is the window's shell, for a caller that wants its window.
func (w *Window) Shell() *shell.Shell { return w.shell }

/*
hideOnClose makes the close button put the window away rather than destroy it.

A closed Fyne window cannot be shown again, and the panel's menu offers
Preferences every time it is opened. Without this the first close would make
that menu item do nothing for the rest of the run, which is a worse bug than
the one it replaces.

The settings are safe either way: every screen saves as it is changed, so
there is nothing waiting to be written when the window goes away.
*/
func (w *Window) hideOnClose() {
	w.shell.Window.SetCloseIntercept(func() { w.shell.Window.Hide() })
}

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
