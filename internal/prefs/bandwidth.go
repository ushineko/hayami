package prefs

import (
	"sort"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"github.com/ushineko/fynedesygn/shell"
	"github.com/ushineko/fynedesygn/widgets"

	"github.com/ushineko/hayami/internal/core"
)

// buildBandwidth is which interfaces the bandwidth section watches.
//
// The machine's own interfaces, listed. Nobody should have to know that a
// virtual network is called wg0 to be able to watch it, and until this screen
// existed the only way to name one was to find the settings file and know its
// keys — which is the clearest case for this window.
func (w *Window) buildBandwidth(s *shell.Shell) fyne.CanvasObject {
	c := w.opts.Store.Config()

	names, err := interfaces()
	if err != nil {
		return container.NewVBox(widgets.Dim("The interface table could not be read: " + err.Error()))
	}

	rows := make([]fyne.CanvasObject, 0, len(names))
	for _, name := range names {
		rows = append(rows, w.interfaceRow(s, name, watched(c.Interfaces, name)))
	}

	return container.NewVBox(
		widgets.Dim("Which interfaces the bandwidth section watches. None, until you say."),
		container.NewVBox(rows...),
	)
}

// interfaceRow is one interface and whether it is watched.
func (w *Window) interfaceRow(s *shell.Shell, name string, on bool) fyne.CanvasObject {
	// Checked before the callback: see sectionRow.
	check := widget.NewCheck(name, nil)
	check.SetChecked(on)
	check.OnChanged = func(watch bool) {
		c := w.opts.Store.Config()
		if watch {
			if !watched(c.Interfaces, name) {
				c.Interfaces = append(c.Interfaces, name)
				sort.Strings(c.Interfaces)
			}
		} else {
			kept := c.Interfaces[:0]
			for _, got := range c.Interfaces {
				if got != name {
					kept = append(kept, got)
				}
			}
			c.Interfaces = kept
		}
		w.save(c)
		s.Invalidate()
	}
	return check
}

// interfaces are the machine's own, from the kernel's table.
func interfaces() ([]string, error) {
	counters, err := core.ReadNetDev()
	if err != nil {
		return nil, err
	}
	return core.InterfaceNames(counters), nil
}

// watched reports whether a name is in the list.
func watched(list []string, name string) bool {
	for _, got := range list {
		if got == name {
			return true
		}
	}
	return false
}
