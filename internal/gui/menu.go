package gui

import (
	"fyne.io/fyne/v2"

	"github.com/ushineko/hayami/internal/config"
	"github.com/ushineko/hayami/internal/prefs"
)

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
*/
func Menu(a fyne.App, store *config.Store, version string, onChange func(config.Config)) func() *fyne.Menu {
	menu, _ := MenuWith(a, store, version, onChange)
	return menu
}

// MenuWith is Menu, and also the function that opens the preferences window,
// for a caller that offers another way in: a command-line flag, a desktop
// entry's second action, or a panel somewhere a person cannot right-click.
func MenuWith(a fyne.App, store *config.Store, version string, onChange func(config.Config)) (func() *fyne.Menu, func()) {
	var window *prefs.Window

	open := func() {
		// One window, however many times the menu is used. A second copy is
		// two views of one file, and the one nobody is looking at is the one
		// that overwrites.
		if window == nil {
			window = prefs.New(a, prefs.Options{
				Store:   store,
				Version: version,
				OnChange: func() {
					if onChange != nil {
						onChange(store.Config())
					}
				},
			})
		}
		window.Show()
	}

	return func() *fyne.Menu {
		return fyne.NewMenu("",
			fyne.NewMenuItem("Preferences…", open),
			fyne.NewMenuItem("Quit", func() { a.Quit() }),
		)
	}, open
}
