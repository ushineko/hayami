package prefs

import (
	"fmt"
	"sort"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"github.com/ushineko/fynedesygn/shell"
	"github.com/ushineko/fynedesygn/widgets"

	"github.com/ushineko/hayami/internal/config"

	"github.com/ushineko/hayami/internal/core"
)

/*
buildInterfaces is which interfaces the bandwidth section watches.

**The real ones first, and the rest behind a switch.** The machine this was
written for reports seventy-seven interfaces: two are ethernet and wireless,
two are overlays, and seventy-three are the veth pairs and bridges a container
runtime leaves lying about. Seventy-seven checkboxes to find two is not a
choice, and it is what this screen was.

Nothing already chosen is ever hidden -- a watched interface is listed
whatever it is, because a setting you cannot see is worse than a long list --
and "Show every interface" is there for the case the heuristic reads a name
wrongly.
*/
func (w *Window) buildInterfaces(s *shell.Shell) fyne.CanvasObject {
	names, err := interfaces()
	if err != nil {
		return container.NewVBox(widgets.Dim("The interface table could not be read: " + err.Error()))
	}

	rows := container.NewVBox()
	redraw := func() {
		watching := config.Bandwidth.Get(w.opts.Store.Config()).Interfaces
		rows.Objects = nil
		for _, name := range order(names, watching, w.showAll) {
			rows.Add(w.interfaceRow(s, name, watched(watching, name)))
		}
		rows.Refresh()
	}
	redraw()

	all := widget.NewCheck("Show every interface", func(on bool) {
		w.showAll = on
		redraw()
	})
	all.SetChecked(w.showAll)

	watching := config.Bandwidth.Get(w.opts.Store.Config()).Interfaces
	note := "Which interfaces the bandwidth section watches. None, until you say."
	if hidden := len(names) - len(order(names, watching, false)); hidden > 0 {
		note += fmt.Sprintf(" %d are hidden: container and virtual interfaces.", hidden)
	}

	return container.NewVBox(widgets.DimWrapped(note), rows, all)
}

/*
order is the interfaces to offer, in the order to offer them.

Watched first, so a choice already made is never buried; then the ordinary
interfaces, then the overlays, then -- only when asked for -- the virtual
churn. Alphabetical within each group: the alternative is ordering by traffic,
and a list that reorders itself while somebody reads it is worse than one in
an arbitrary but stable order.
*/
func order(names, watching []string, showAll bool) []string {
	rank := func(name string) int {
		if watched(watching, name) {
			return 0
		}
		switch core.ClassifyInterface(name) {
		case core.KindOrdinary:
			return 1
		case core.KindTunnel:
			return 2
		default:
			return 3
		}
	}

	out := make([]string, 0, len(names))
	for _, name := range names {
		if !showAll && rank(name) == 3 {
			continue
		}
		out = append(out, name)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if a, b := rank(out[i]), rank(out[j]); a != b {
			return a < b
		}
		return out[i] < out[j]
	})
	return out
}

// interfaceRow is one interface and whether it is watched.
func (w *Window) interfaceRow(s *shell.Shell, name string, on bool) fyne.CanvasObject {
	// Checked before the callback: see sectionRow.
	check := widget.NewCheck(name, nil)
	check.SetChecked(on)
	check.OnChanged = func(watch bool) {
		c := w.opts.Store.Config()
		b := config.Bandwidth.Get(c)
		if watch {
			if !watched(b.Interfaces, name) {
				b.Interfaces = append(b.Interfaces, name)
				sort.Strings(b.Interfaces)
			}
		} else {
			kept := make([]string, 0, len(b.Interfaces))
			for _, got := range b.Interfaces {
				if got != name {
					kept = append(kept, got)
				}
			}
			b.Interfaces = kept
		}
		w.save(config.Bandwidth.Set(c, b))
		s.Invalidate()
	}
	return check
}

// interfaces are the machine's own, from the kernel's table.
func interfaces() ([]string, error) {
	counters, err := core.ReadCounters()
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
