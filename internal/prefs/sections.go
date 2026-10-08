package prefs

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"github.com/ushineko/fynedesygn/shell"
	"github.com/ushineko/fynedesygn/widgets"

	"github.com/ushineko/hayami/internal/panel"
	"github.com/ushineko/hayami/internal/view"
)

// buildSections is which sections are drawn, in what order, and how they are
// laid out.
func (w *Window) buildSections(s *shell.Shell) fyne.CanvasObject {
	c := w.opts.Store.Config()

	rows := make([]fyne.CanvasObject, 0, len(panel.Keys()))
	for i, key := range ordered(c.Sections) {
		rows = append(rows, w.sectionRow(s, key, i, len(ordered(c.Sections))))
	}

	return container.NewVBox(
		widgets.DimWrapped("Which readings the panel draws, and in what order."),
		container.NewVBox(rows...),
		widget.NewSeparator(),
		widgets.DimWrapped("How they are laid out. A grid reflows into columns when the panel "+
			"is wide enough for them; a narrow one is a stack either way."),
		w.arrangement(s),
		widget.NewSeparator(),
		// The interfaces live here rather than on a screen of their own.
		// They are one section's setting, and a whole navigation entry for a
		// list of checkboxes was more than it was worth once the list was
		// short enough to read.
		w.buildInterfaces(s),
	)
}

// ordered is every section this build has, the chosen ones first in their
// chosen order and the rest after.
//
// One list rather than two. A chosen section and an unchosen one differ by a
// tick, and a reader who had to look in two places to find a section would
// have to know which before they started looking.
func ordered(chosen []string) []string {
	out := make([]string, 0, len(panel.Keys()))
	seen := map[string]bool{}
	for _, key := range chosen {
		if known(key) && !seen[key] {
			out = append(out, key)
			seen[key] = true
		}
	}
	for _, key := range panel.Keys() {
		if !seen[key] {
			out = append(out, key)
		}
	}
	return out
}

func known(key string) bool {
	for _, k := range panel.Keys() {
		if k == key {
			return true
		}
	}
	return false
}

// sectionTitle is what a section is called on its card, which is what the
// list offers; the key is what the settings file and --sections say.
func sectionTitle(key string) string {
	if info, ok := view.SectionByKey(key); ok {
		return info.Title
	}
	return key
}

// sectionRow is one section: a tick, its name, and the two buttons that move
// it.
func (w *Window) sectionRow(s *shell.Shell, key string, at, total int) fyne.CanvasObject {
	c := w.opts.Store.Config()

	// The callback is attached *after* the initial state, not before.
	// SetChecked fires OnChanged, which saves and rebuilds this very screen,
	// which sets the check again: the first version of this was an infinite
	// recursion that crashed the program with a stack overflow, and it did it
	// on the first frame rather than subtly.
	shown := widget.NewCheck(sectionTitle(key), nil)
	shown.SetChecked(c.Shows(key))
	shown.OnChanged = func(on bool) { w.setShown(s, key, on) }

	up := widget.NewButtonWithIcon("", upIcon(), func() { w.move(s, key, -1) })
	down := widget.NewButtonWithIcon("", downIcon(), func() { w.move(s, key, +1) })
	up.Disable()
	down.Disable()
	if at > 0 {
		up.Enable()
	}
	if at < total-1 {
		down.Enable()
	}

	return container.NewBorder(nil, nil, shown, container.NewHBox(up, down))
}

// setShown adds or removes a section, keeping the order of the rest.
func (w *Window) setShown(s *shell.Shell, key string, on bool) {
	c := w.opts.Store.Config()
	if on {
		if !c.Shows(key) {
			c.Sections = append(c.Sections, key)
		}
	} else {
		kept := c.Sections[:0]
		for _, k := range c.Sections {
			if k != key {
				kept = append(kept, k)
			}
		}
		c.Sections = kept
	}
	w.save(c)
	s.Invalidate()
}

// move shifts a section one place.
//
// It works on the list as drawn, which is the chosen sections followed by the
// unchosen: moving an unchosen section would be moving something the panel is
// not drawing, so a section that is not shown is added where it was moved to.
func (w *Window) move(s *shell.Shell, key string, by int) {
	c := w.opts.Store.Config()
	if !c.Shows(key) {
		c.Sections = append(c.Sections, key)
	}

	at := -1
	for i, k := range c.Sections {
		if k == key {
			at = i
		}
	}
	to := at + by
	if at < 0 || to < 0 || to >= len(c.Sections) {
		return
	}
	c.Sections[at], c.Sections[to] = c.Sections[to], c.Sections[at]

	w.save(c)
	s.Invalidate()
}

// arrangement offers the three ways sections are laid out, described rather
// than only named: "grid" says nothing to somebody who has not seen one.
// The shell is not needed: changing the arrangement changes nothing on this
// screen, so there is nothing to rebuild.
func (w *Window) arrangement(_ *shell.Shell) fyne.CanvasObject {
	c := w.opts.Store.Config()

	labels := make([]string, 0, 3)
	byLabel := map[string]string{}
	for _, a := range []view.Arrangement{view.ArrangeStack, view.ArrangeGrid, view.ArrangeRow} {
		label := a.String() + " — " + describe(a)
		labels = append(labels, label)
		byLabel[label] = a.String()
	}

	// Selected before the callback, for the reason sectionRow gives.
	radio := widget.NewRadioGroup(labels, nil)
	for _, label := range labels {
		if byLabel[label] == c.Arrangement {
			radio.SetSelected(label)
		}
	}
	radio.OnChanged = func(chosen string) {
		name, ok := byLabel[chosen]
		if !ok {
			return
		}
		got := w.opts.Store.Config()
		if got.Arrangement == name {
			return
		}
		got.Arrangement = name
		w.save(got)
	}
	return radio
}

// describe says what an arrangement looks like.
func describe(a view.Arrangement) string {
	switch a {
	case view.ArrangeGrid:
		return "columns that reflow to the width"
	case view.ArrangeRow:
		return "one line per reading, its bar stretching to the pane (terminal only)"
	default:
		return "one section above another"
	}
}
