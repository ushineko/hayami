package gui

import (
	"fmt"

	"fyne.io/fyne/v2"

	fdtheme "github.com/ushineko/fynedesygn/theme"

	"github.com/ushineko/hayami/internal/config"
	"github.com/ushineko/hayami/internal/prefs"
)

// OpacitySteps are the opacities the menu offers.
//
// A short list, not a slider. A menu is for trying a value and seeing the
// desktop come through; the exact number is the preferences' business, and a
// slider in a context menu is a thing nobody can hit.
var OpacitySteps = []int{100, 95, 90, 85, 80, 70}

/*
Menu is the panel's whole interface: preferences, opacity, quit.

Three items, deliberately. The program this replaces grew six nested submenus —
opacity, font size, devices and their slots and their types, two providers and
their intervals, every interface and what to do with it — which is a settings
dialog wearing a menu's clothes. Everything that is a setting lives in the
preferences window; what is left here is the window's own state and the way
out.

A glance window has no controls of its own (docs/glance.md), so this menu is
the only thing a person can press. That is the reason to keep it short rather
than the reason to fill it.

The opacity here fades the cards and saves as it goes, which is the same thing
the preferences' slider does — one setting, one mechanism, drawn by the
toolkit. The titlebar is the only thing on this panel that needs a compositor.
*/
func Menu(a fyne.App, store *config.Store, version string, onChange func(config.Config)) func() *fyne.Menu {
	menu, _ := MenuWith(a, store, version, onChange)
	return menu
}

// MenuWith is Menu, and also the function that opens the preferences window,
// for a caller that offers another way in: a command-line flag, a desktop
// entry's second action, or a panel somewhere a person cannot right-click.
func MenuWith(a fyne.App, store *config.Store, version string, onChange func(config.Config)) (func() *fyne.Menu, func()) { //nolint:revive // the callback is the panel's only way back
	var window *prefs.Window

	/*
		applied is every route by which the panel's theme changes.

		All of them go through here because all of them end in
		app.Settings().SetTheme, and a theme set on the application rebuilds
		every window from it -- taking with it the subtree override that
		keeps the preferences window out of the panel's face. Whatever
		changed, that override has to be made again afterwards.

		The opacity submenu is the case that is easy to miss: it never
		touches a font, but it fades the cards by re-wrapping the
		application's theme, which is the same event as far as the other
		window is concerned.
	*/
	applied := func(c config.Config) {
		if onChange != nil {
			onChange(c)
		}
		relayout(window)
	}

	open := func() {
		// One window, however many times the menu is used. A second copy is
		// two views of one file, and the one nobody is looking at is the one
		// that overwrites.
		if window == nil {
			window = prefs.New(a, prefs.Options{
				Store:    store,
				Version:  version,
				Theme:    themeFor(store, applied),
				OnChange: func() { applied(store.Config()) },
			})
		}
		window.Show()
	}

	return func() *fyne.Menu {
		return fyne.NewMenu("",
			fyne.NewMenuItem("Preferences…", open),
			opacityItem(store, applied),
			fyne.NewMenuItem("Quit", func() { a.Quit() }),
		)
	}, open
}

/*
opacityItem is the submenu that fades the cards.

It saves, and the panel re-fades as soon as it does. There is no second
mechanism and no compositor involved: the window's background is already
transparent and the cards are drawn by the toolkit, so this is a theme being
rebuilt and it works on any desktop.

An earlier version of this applied live over KWin's D-Bus and deliberately did
not save, on the reasoning that a menu is for trying a value. That was built on
a wrong premise — that the toolkit could not be translucent and KWin had to do
it — and once the premise went, so did the reason for two mechanisms.
*/
func opacityItem(store *config.Store, onChange func(config.Config)) *fyne.MenuItem {
	current := store.Config().OpacityOrDefault()

	items := make([]*fyne.MenuItem, 0, len(OpacitySteps))
	for _, step := range OpacitySteps {
		item := fyne.NewMenuItem(fmt.Sprintf("%d %%", step), func() {
			c := store.Config()
			c.Opacity = step
			if err := store.SetConfig(c); err != nil {
				return
			}
			if onChange != nil {
				onChange(c)
			}
		})
		item.Checked = step == current
		items = append(items, item)
	}

	item := fyne.NewMenuItem("Opacity", nil)
	item.ChildMenu = fyne.NewMenu("", items...)
	return item
}

/*
themeFor builds the theme hook the preferences window hands to the shell.

Two things happen here and both are necessary.

The appearance decides the scheme, the face and the size, and the card opacity
is this program's own, applied over the top — because setting a theme replaces
whatever was wrapped around the last one, so without this, choosing a font
would quietly undo the fade.

Then the panel is told. A glance window paints from its own objects rather
than from the canvas, so a new theme reaches the preferences window and leaves
the panel drawn in the old face until something else rebuilds it. The
notification is queued rather than made here: this runs *while* the shell is
working out what theme to apply, and a panel that restyled at that moment
would restyle to the theme it already had.

**It notifies only when the appearance has changed**, and that is load-bearing
twice over.

The shell asks for a theme whenever it lays itself out, which includes the
moment the preferences window is built — so an unconditional notification
made the panel set the application's theme immediately after this window had
wrapped itself in its own, and a theme set on the application rebuilds every
window and takes the wrapping with it. The symptom was the whole preferences
window drawn in the panel's text size: 8 pt against the 12 pt on its own
Appearance screen, or 20 against 12, always the panel's.

And the notification now puts this window's wrapping back, which would be a
loop if every layout notified.
*/
func themeFor(store *config.Store, notify func(config.Config)) func(fdtheme.Appearance) fyne.Theme {
	return themeWith(store, notify, func(f func()) { go fyne.Do(f) })
}

// themeWith is themeFor with the queue named, so a test can watch what would
// be queued without a running event loop. The queue is the only thing about
// this that needs Fyne, and it is the only thing a test cannot have.
func themeWith(
	store *config.Store,
	notify func(config.Config),
	queue func(func()),
) func(fdtheme.Appearance) fyne.Theme {
	var last fdtheme.Appearance
	first := true

	return func(a fdtheme.Appearance) fyne.Theme {
		changed := first || a != last
		last, first = a, false

		if notify != nil && changed {
			queue(func() { notify(store.Config()) })
		}
		return withCardOpacity(a.Theme(), store.Config().OpacityOrDefault())
	}
}

// relayout draws the preferences window in its own appearance again.
//
// A window that owns its appearance keeps it in a subtree override, and an
// override is built from the objects that were there when it was built. The
// application's theme changing rebuilds the window from the application's
// theme, so the override has to be made again afterwards; asking the shell
// for the appearance it already has is what does that.
func relayout(w *prefs.Window) {
	if w == nil {
		return
	}
	s := w.Shell()
	s.SetAppearance(s.Appearance())
}
