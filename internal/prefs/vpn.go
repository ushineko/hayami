package prefs

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"slices"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"github.com/ushineko/fynedesygn/shell"
	"github.com/ushineko/fynedesygn/widgets"

	"github.com/ushineko/hayami/internal/config"
	"github.com/ushineko/hayami/internal/core"
)

// vpnListTimeout bounds asking the systems what they have configured. OpenVPN3's
// configuration manager may have to start first.
const vpnListTimeout = 5 * time.Second

// vpnCheckKinds are the checks a person can choose, as a table: a type the settings
// know, the word for it, and what its fields ask for.
var vpnCheckKinds = []struct {
	Type, Label, Target string
	Prefix              bool
}{
	{Type: "ping", Label: "Ping", Target: "Host or address"},
	{Type: "dns", Label: "DNS", Target: "Name to look up", Prefix: true},
}

// vpnIntervals are the choices for how often a connected VPN is checked.
var vpnIntervals = []struct {
	Seconds int
	Label   string
}{
	{30, "Every 30 seconds"},
	{60, "Every minute"},
	{120, "Every 2 minutes"},
	{300, "Every 5 minutes"},
	{600, "Every 10 minutes"},
}

// vpnDefaultInterval is what no setting means: the panel's own default.
const vpnDefaultInterval = 120

// buildNetwork is the Network section's own preferences: the interfaces, and the
// VPNs where this platform has any (spec 085 R3, R14).
func (w *Window) buildNetwork(s *shell.Shell) fyne.CanvasObject {
	out := container.NewVBox(w.buildInterfaces(s))
	systems := w.vpnSystems()
	if len(systems) > 0 {
		out.Add(widget.NewSeparator())
		out.Add(w.buildVPNs(systems))
	}
	return out
}

func (w *Window) vpnSystems() []core.VPNBackend {
	if w.opts.VPNSystems == nil {
		return nil
	}
	return w.opts.VPNSystems()
}

/*
buildVPNs is which VPNs have a line on the Network card, and how each is checked.

**None until asked**, as with the interfaces. A name already chosen is always
listed, even where no system has it now: a setting nobody can see is worse than a
list with a stale name on it.

The list is read off the render thread, because a system may have to start to
answer, and is drawn when it comes.

**This group redraws itself and never the page.** The shell's Invalidate rebuilds
the page, which would ask for the list again, which would draw, which would
invalidate; and on a keystroke it would take the focus from the field being typed in.
*/
func (w *Window) buildVPNs(systems []core.VPNBackend) fyne.CanvasObject {
	rows := container.NewVBox(widgets.Dim("Asking which VPNs this machine has..."))
	var known []string
	var redraw func()
	redraw = func() {
		chosen := config.Bandwidth.Get(w.opts.Store.Config()).VPNs
		rows.Objects = nil
		for _, name := range orderVPNs(known, chosen) {
			rows.Add(w.vpnRow(name, known, redraw))
		}
		if len(rows.Objects) == 0 {
			rows.Add(widgets.Dim("No VPN connections were found."))
		}
		rows.Refresh()
	}
	background := w.opts.Background
	if background == nil {
		background = func(f func()) { go f() }
	}
	background(func() {
		ctx, cancel := context.WithTimeout(context.Background(), vpnListTimeout)
		defer cancel()
		names := listVPNs(ctx, systems)
		fyne.Do(func() {
			known = names
			redraw()
		})
	})

	return container.NewVBox(
		widgets.DimWrapped("Which VPNs have a line on the Network card. None, until you say. "+
			"Checks run only while a VPN is connected, from one process however many panels are open."),
		w.vpnImportOffer(func() { redraw() }),
		rows,
		w.vpnInterval(),
	)
}

// listVPNs is every configured VPN, each name once. A system that will not say is
// left out: the chosen names stay listed whatever it answers.
func listVPNs(ctx context.Context, systems []core.VPNBackend) []string {
	var out []string
	for _, system := range systems {
		if system.Configured == nil {
			continue
		}
		names, err := system.Configured(ctx)
		if err != nil {
			continue
		}
		out = append(out, names...)
	}
	slices.Sort(out)
	return slices.Compact(out)
}

// orderVPNs is the names to offer: chosen first in their chosen order, which is the
// card's, and the rest alphabetically.
func orderVPNs(known []string, chosen []config.VPNItem) []string {
	out := make([]string, 0, len(known)+len(chosen))
	for _, c := range chosen {
		out = append(out, c.Name)
	}
	for _, name := range known {
		if !slices.Contains(out, name) {
			out = append(out, name)
		}
	}
	return out
}

// chooseVPN is the settings with a VPN on or off the card. A VPN taken off keeps
// nothing: choosing it again starts it afresh.
func chooseVPN(b config.BandwidthSettings, name string, on bool) config.BandwidthSettings {
	i := slices.IndexFunc(b.VPNs, func(v config.VPNItem) bool { return v.Name == name })
	switch {
	case on && i < 0:
		b.VPNs = append(slices.Clone(b.VPNs), config.VPNItem{Name: name})
	case !on && i >= 0:
		b.VPNs = slices.Delete(slices.Clone(b.VPNs), i, i+1)
	}
	return b
}

// changeVPN saves a change to one chosen VPN. It reads the settings afresh, so a
// field being typed in cannot put back what another has changed.
func (w *Window) changeVPN(name string, change func(*config.VPNItem)) {
	c := w.opts.Store.Config()
	b := config.Bandwidth.Get(c)
	b.VPNs = slices.Clone(b.VPNs)
	i := slices.IndexFunc(b.VPNs, func(v config.VPNItem) bool { return v.Name == name })
	if i < 0 {
		return
	}
	b.VPNs[i].Checks = slices.Clone(b.VPNs[i].Checks)
	change(&b.VPNs[i])
	w.save(config.Bandwidth.Set(c, b))
}

// vpnRow is one VPN: a tick, and for a chosen one what the card calls it and its
// checks.
func (w *Window) vpnRow(name string, known []string, redraw func()) fyne.CanvasObject {
	item, chosen := findVPN(config.Bandwidth.Get(w.opts.Store.Config()).VPNs, name)
	text := name
	if len(known) > 0 && !slices.Contains(known, name) {
		text += " (not found on this machine)"
	}
	// Checked before the callback: see sectionRow.
	check := widget.NewCheck(text, nil)
	check.SetChecked(chosen)
	check.OnChanged = func(on bool) {
		c := w.opts.Store.Config()
		w.save(config.Bandwidth.Set(c, chooseVPN(config.Bandwidth.Get(c), name, on)))
		// After the event that called this, because this removes the tick that
		// is still handling it.
		fyne.Do(redraw)
	}
	if !chosen {
		return check
	}

	label := widget.NewEntry()
	label.SetPlaceHolder("What the card calls it, if not this")
	label.SetText(item.Label)
	label.OnChanged = func(v string) {
		w.changeVPN(name, func(i *config.VPNItem) { i.Label = v })
	}

	details := container.NewVBox(label)
	for at, c := range item.Checks {
		details.Add(w.vpnCheckRow(name, at, c, redraw))
	}
	add := widget.NewButton("Add a check", func() {
		w.changeVPN(name, func(i *config.VPNItem) {
			i.Checks = append(i.Checks, config.VPNCheckItem{Type: vpnCheckKinds[0].Type})
		})
		fyne.Do(redraw)
	})
	details.Add(container.NewHBox(add))
	return container.NewVBox(check, container.NewPadded(details))
}

func findVPN(vpns []config.VPNItem, name string) (config.VPNItem, bool) {
	i := slices.IndexFunc(vpns, func(v config.VPNItem) bool { return v.Name == name })
	if i < 0 {
		return config.VPNItem{}, false
	}
	return vpns[i], true
}

// vpnCheckRow is one check: its kind, what it is aimed at and a way to remove it.
func (w *Window) vpnCheckRow(name string, at int, c config.VPNCheckItem, redraw func()) fyne.CanvasObject {
	kind := slices.IndexFunc(vpnCheckKinds, func(k struct {
		Type, Label, Target string
		Prefix              bool
	}) bool {
		return k.Type == c.Type
	})
	if kind < 0 {
		kind = 0
	}
	labels := make([]string, 0, len(vpnCheckKinds))
	for _, k := range vpnCheckKinds {
		labels = append(labels, k.Label)
	}
	change := func(f func(*config.VPNCheckItem)) {
		w.changeVPN(name, func(i *config.VPNItem) {
			if at < len(i.Checks) {
				f(&i.Checks[at])
			}
		})
	}

	kinds := widget.NewSelect(labels, nil)
	kinds.SetSelectedIndex(kind)
	kinds.OnChanged = func(string) {
		chosen := vpnCheckKinds[kinds.SelectedIndex()]
		change(func(v *config.VPNCheckItem) {
			v.Type = chosen.Type
			if !chosen.Prefix {
				v.Prefix = ""
			}
		})
		fyne.Do(redraw)
	}
	target := widget.NewEntry()
	target.SetPlaceHolder(vpnCheckKinds[kind].Target)
	target.SetText(c.Target)
	target.OnChanged = func(v string) {
		change(func(i *config.VPNCheckItem) { i.Target = strings.TrimSpace(v) })
	}
	remove := widget.NewButton("Remove", func() {
		w.changeVPN(name, func(i *config.VPNItem) {
			if at < len(i.Checks) {
				i.Checks = slices.Delete(i.Checks, at, at+1)
			}
		})
		fyne.Do(redraw)
	})
	fields := []fyne.CanvasObject{target}
	if vpnCheckKinds[kind].Prefix {
		prefix := widget.NewEntry()
		prefix.SetPlaceHolder("Address starts with")
		prefix.SetText(c.Prefix)
		prefix.OnChanged = func(v string) {
			change(func(i *config.VPNCheckItem) { i.Prefix = strings.TrimSpace(v) })
		}
		fields = append(fields, prefix)
	}
	return container.NewBorder(nil, nil, kinds, remove, container.NewGridWithColumns(len(fields), fields...))
}

// vpnInterval is how often a connected VPN is checked. A value from the settings
// file that is not a choice here is offered too, rather than changed by being seen.
func (w *Window) vpnInterval() fyne.CanvasObject {
	seconds := config.Bandwidth.Get(w.opts.Store.Config()).VPNCheckSeconds
	if seconds == 0 {
		seconds = vpnDefaultInterval
	}
	type choice struct {
		seconds int
		label   string
	}
	choices := make([]choice, 0, len(vpnIntervals)+1)
	for _, i := range vpnIntervals {
		choices = append(choices, choice{i.Seconds, i.Label})
	}
	if !slices.ContainsFunc(choices, func(c choice) bool { return c.seconds == seconds }) {
		choices = append(choices, choice{seconds, fmt.Sprintf("Every %d seconds", seconds)})
	}
	labels := make([]string, 0, len(choices))
	at := 0
	for i, c := range choices {
		labels = append(labels, c.label)
		if c.seconds == seconds {
			at = i
		}
	}
	sel := widget.NewSelect(labels, nil)
	sel.SetSelectedIndex(at)
	sel.OnChanged = func(string) {
		c := w.opts.Store.Config()
		b := config.Bandwidth.Get(c)
		b.VPNCheckSeconds = choices[sel.SelectedIndex()].seconds
		w.save(config.Bandwidth.Set(c, b))
	}
	return container.NewBorder(nil, nil, widget.NewLabel("Check connected VPNs"), nil, sel)
}

// importVPNs is the settings with vpn-toggle's in them: its VPNs, its checks and
// its timing.
func importVPNs(b config.BandwidthSettings, in config.VPNImport) config.BandwidthSettings {
	b.VPNs = in.VPNs
	b.VPNCheckSeconds = in.CheckSeconds
	b.VPNGraceSeconds = in.GraceSeconds
	b.VPNImportOffered = true
	return b
}

// importOutcome says what an import did, naming what it left out (spec 085 R15).
func importOutcome(in config.VPNImport) string {
	out := fmt.Sprintf("Imported %d VPNs from vpn-toggle, whose own settings were not changed. "+
		"Each has a line on the card; untick any you do not want.", len(in.VPNs))
	if len(in.LeftOut) > 0 {
		out += " Not imported: " + strings.Join(in.LeftOut, "; ") +
			". A geolocation check asked a third party about this machine's address, and hayami does not."
	}
	return out
}

/*
vpnImportOffer is a one-time offer to take vpn-toggle's settings.

Offered while no VPN is chosen and it has not been answered: a person who said no
is not asked again, and one who has chosen VPNs by hand has answered another way.
A file that is not there is no offer. One that cannot be read says so, because
somebody with vpn-toggle's settings would otherwise wonder where the offer went.
*/
func (w *Window) vpnImportOffer(redraw func()) fyne.CanvasObject {
	b := config.Bandwidth.Get(w.opts.Store.Config())
	box := container.NewVBox()
	if b.VPNImportOffered || len(b.VPNs) > 0 {
		return box
	}
	path := w.opts.VPNToggleConfig
	if path == "" {
		var err error
		if path, err = config.VPNTogglePath(); err != nil {
			return box
		}
	}
	in, err := config.ReadVPNToggle(path)
	if errors.Is(err, fs.ErrNotExist) {
		return box
	}
	if err != nil {
		box.Add(widgets.DimWrapped("vpn-toggle's settings could not be imported: " + err.Error()))
		return box
	}

	answered := func(text string) {
		box.Objects = nil
		if text != "" {
			box.Add(widgets.DimWrapped(text))
		}
		box.Refresh()
		redraw()
	}
	yes := widget.NewButton(fmt.Sprintf("Import %d VPNs", len(in.VPNs)), func() {
		c := w.opts.Store.Config()
		w.save(config.Bandwidth.Set(c, importVPNs(config.Bandwidth.Get(c), in)))
		fyne.Do(func() { answered(importOutcome(in)) })
	})
	no := widget.NewButton("Not now", func() {
		c := w.opts.Store.Config()
		next := config.Bandwidth.Get(c)
		next.VPNImportOffered = true
		w.save(config.Bandwidth.Set(c, next))
		fyne.Do(func() { answered("") })
	})
	box.Add(widgets.DimWrapped("vpn-toggle's settings were found. Importing copies its VPNs and checks; it leaves its files as they are."))
	box.Add(container.NewHBox(yes, no))
	return box
}
