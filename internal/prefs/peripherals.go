package prefs

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"github.com/ushineko/fynedesygn/shell"
	"github.com/ushineko/fynedesygn/widgets"

	"github.com/ushineko/hayami/internal/config"
)

// OneLineLabel is the peripherals section's one setting, as the check says it.
const OneLineLabel = "Every device on one line"

/*
buildPeripherals is which way the peripherals card grows past two devices
(spec 071): wrapping, as many across as the panel's width fits, or every
device on one line, with the panel growing wider instead of taller.

The window's alone. A terminal pane cannot grow, and already puts every device
on one line when it is wide enough; the note says so, and says that row, which
is one reading per line, draws one device per line either way.
*/
func (w *Window) buildPeripherals(_ *shell.Shell) fyne.CanvasObject {
	check := widget.NewCheck(OneLineLabel, func(on bool) {
		c := w.opts.Store.Config()
		w.save(config.Peripherals.Set(c, config.PeripheralsSettings{OneLine: on}))
	})
	// Set before the handler would fire on it: a check set after its handler
	// saves the value it was just given.
	check.Checked = config.Peripherals.Get(w.opts.Store.Config()).OneLine

	return container.NewVBox(
		widgets.DimWrapped("How the peripherals section grows past two devices. On one line, the "+
			"panel grows wider instead of taller: in the window, in stack and grid. A terminal "+
			"pane wraps to its width."),
		check,
	)
}
