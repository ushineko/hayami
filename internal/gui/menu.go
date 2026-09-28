package gui

import (
	"fmt"

	"fyne.io/fyne/v2"

	"github.com/ushineko/hayami/internal/config"
	"github.com/ushineko/hayami/internal/desktop"
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

The opacity here is **live and not saved**: it asks KWin to fade the running
window and leaves nothing behind, which is what trying a value should do. The
one that survives a restart is in the window rule and is set in the
preferences. Two mechanisms, because Plasma has two, and the difference is
said in the interface rather than hidden.
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
			opacityItem(store),
			fyne.NewMenuItem("Quit", func() { a.Quit() }),
		)
	}, open
}

/*
opacityItem is the submenu that fades the window.

It applies **live** and does not save. That is the difference between the two
things Plasma offers and it is deliberate: a script over D-Bus changes the
running window and leaves nothing behind, which is what trying a value should
do, while the value that survives a restart lives in the window rule and is set
in the preferences.

A desktop that is not Plasma gets the item and a refusal, which is better than
a menu whose contents depend on the compositor: the reason it will not work is
worth saying once, where somebody is looking for it.
*/
func opacityItem(store *config.Store) *fyne.MenuItem {
	current := store.Config().OpacityOrDefault()

	items := make([]*fyne.MenuItem, 0, len(OpacitySteps))
	for _, step := range OpacitySteps {
		item := fyne.NewMenuItem(fmt.Sprintf("%d %%", step), func() {
			// The error is deliberately not raised to the user here. The menu
			// is dismissed by the time it arrives, there is nowhere to put it,
			// and the preferences window says plainly whether this desktop can
			// do it at all.
			_ = desktop.SetOpacity(AppID, step)
		})
		item.Checked = step == current
		items = append(items, item)
	}

	item := fyne.NewMenuItem("Opacity", nil)
	item.ChildMenu = fyne.NewMenu("", items...)
	return item
}
