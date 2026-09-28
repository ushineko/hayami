package gui

import (
	"fmt"

	"fyne.io/fyne/v2"

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
			opacityItem(store, onChange),
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
