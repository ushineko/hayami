package gui

import (
	"context"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	fynetheme "fyne.io/fyne/v2/theme"

	"github.com/ushineko/fynedesygn/glance"

	"github.com/ushineko/hayami/internal/config"
	"github.com/ushineko/hayami/internal/panel"
	"github.com/ushineko/hayami/internal/view"
)

/*
The drawers: what a card has no room to say, beside the cards and as tall as
them (spec 060). The surface, its place, the filter and the keyboard are the
design system's (fynedesygn spec 068, 079); what a drawer shows is a
view.Detail, which a source reads and this draws.

A click anywhere on a card opens that section's detail, a second click on the
same card closes the drawer, and a click on another card changes what it
shows (spec 062). Where a section has several details the drawer has a switch
between them, and what was chosen is where that card opens next. A detail is
read only while it is shown, by a poll of its own that starts when a drawer
opens on it and stops when the drawer closes, moves on, or the section is
hidden.

A Shift-click opens another drawer beside the ones out, or closes one, and a
plain click with several out closes them all (spec 088). Each drawer has its
own detail, poll, pause, drill, filter and switch.
*/
type drawer struct {
	// dr is the design system's drawer this one draws in, and seq when it
	// opened, which orders the open drawers as the row does.
	dr  *glance.Drawer
	seq int

	// sw chooses between a section's details where it has several: the
	// cooler's lists.
	sw     *glance.Switch
	filter *glance.Filter
	meters []*glance.Meter
	rows   []*glance.Row

	// notes are the lines under the list: its reasons and its note. They are
	// rows of their own because a line of the list has its name column
	// pinned, and a sentence in a pinned column is cut to a name's width.
	notes []*glance.Row

	// section and id are what the drawer is open on, and detail is the last
	// reading of it. shownMeters is how many meters the content was built
	// with, and listed whether the filter is in it: the content is replaced
	// when either changes and not otherwise.
	section, id string
	detail      view.Detail
	shownMeters int
	listed      bool
	built       bool

	// parts are the pieces of a detail made of details, every account's
	// usage, built to its shape and set in place until the shape changes.
	// rules is how many rules stand between them.
	parts      []drawerPart
	rules      int
	partsShape string

	// paused stops the reading of the open detail; confirming is the kill
	// waiting for its second click; killed is what the last one did, said
	// on the page until the drill is left.
	// paused is read by the poll on its own goroutine and written by a
	// click on the UI thread, so it is atomic and not a plain bool: the
	// first attempt asked the UI thread for it and read the answer before
	// it arrived, and the pause did nothing at all.
	paused     atomic.Bool
	confirming bool
	killed     string

	// vpn is the presses on a VPN's rows.
	vpn vpnPresses

	// stop ends the poll of what is shown, and gen tells a result that
	// arrives after the drawer has moved on from one that is current.
	stop context.CancelFunc
	gen  int
}

/*
drawerKit is what every drawer shares: what each card last showed, which is
what it opens on next (it is not kept across a restart), the width a line's
name column is pinned to, the compositor, and where a sign-in address goes.
*/
type drawerKit struct {
	last map[string]string
	name float32
	desk Desk

	// openURL gives a sign-in address to the person's browser; a test
	// replaces it, so no test opens one.
	openURL func(*url.URL) error

	// seq counts openings, for drawer.seq.
	seq int
}

/*
Desk is what the drawer asks of the compositor, where only the compositor
can answer (spec 060 R18): how much room is beside the window, to hold the
window's right edge so a drawer can open on the left, and to give the
keyboard back. desktop.Drawer is the real one.

It is a seam and not a call into the desktop package for a reason learned
the hard way: a test that builds a panel must not reach the desk. The first
run of this spec's tests on a Plasma desktop loaded a KWin script into the
session of the person sitting at it, against the panel they were running.
A panel built without a Desk asks nothing of anybody.
*/
type Desk interface {
	// Asks reports whether this desktop needs asking at all. Where the
	// window can do all three itself, on Windows, nothing is asked.
	Asks() bool
	Room(ctx context.Context) (left, right float32, ok bool)
	HoldEdge(ctx context.Context, on bool) bool
	Yield(ctx context.Context)
	Stop()
}

// drawerPart is one part of a detail made of details, as drawn: its heading,
// its meters, its lines, and the lines for its reasons and its note.
type drawerPart struct {
	head   *glance.Row
	plot   *glance.Sparkline
	traces map[string]bool
	meters []*glance.Meter
	rows   []*glance.Row
	notes  []*glance.Row
}

// noDesk is no compositor to ask: a test's panel, or a caller that named
// none.
type noDesk struct{}

func (noDesk) Asks() bool                                    { return false }
func (noDesk) Room(context.Context) (float32, float32, bool) { return 0, 0, false }
func (noDesk) HoldEdge(context.Context, bool) bool           { return false }
func (noDesk) Yield(context.Context)                         {}
func (noDesk) Stop()                                         {}

// DrawerRows is how many lines the drawer is built with; it shows as many as
// its height fits. More than a panel of every section at the largest text
// size has room for.
const DrawerRows = 60

// DrawerMeters is how many meters the drawer is built with: more than any
// account has limits.
const DrawerMeters = 8

// DrawerNotes is how many lines the drawer has for a detail's reasons and
// its note.
const DrawerNotes = 4

// buildDrawer makes the first drawer's pieces and points the panel's clicks
// at the drawers. More are made as Shift-clicks ask for them.
func (p *Panel) buildDrawer(a fyne.App, o Options) {
	p.kit = drawerKit{last: map[string]string{}, desk: o.Desk, openURL: a.OpenURL, name: detailNameWidth(a, o)}
	if p.kit.desk == nil {
		p.kit.desk = noDesk{}
	}
	p.newDrawer(p.win.Panel().Drawer())
	p.win.Panel().OnTapped = p.tapped
}

// newDrawer gives one of the panel's drawers its pieces, built once; a
// detail sets them in place.
func (p *Panel) newDrawer(dr *glance.Drawer) *drawer {
	d := &drawer{dr: dr, sw: glance.NewSwitch(), filter: glance.NewFilter()}
	d.vpn = vpnPresses{running: map[string]*vpnRun{}, said: map[string]view.DetailLine{}}
	d.filter.OnChanged = func() { p.renderOne(d) }
	d.sw.OnChanged = func(id string) { p.chosen(d, id) }
	d.sw.OnShifted = func(id string) { p.shiftChosen(d, id) }
	// Every line starts hidden: a line is shown when a detail has something
	// to put on it.
	for range DrawerRows {
		r := glance.NewRowWidth("", "", p.kit.name)
		r.SetShown(false)
		d.rows = append(d.rows, r)
	}
	for range DrawerNotes {
		r := glance.NewRow("", "")
		r.SetShown(false)
		d.notes = append(d.notes, r)
	}
	for range DrawerMeters {
		d.meters = append(d.meters, glance.NewMeter("", MeterLabelWidth))
	}
	dr.SetFilter(d.filter)
	dr.SetSwitch(d.sw)
	// A Wayland client cannot give the keyboard away; the compositor is
	// asked, off this thread. On Windows the design system has already done
	// it and this is not called.
	dr.SetYield(func() { go p.kit.desk.Yield(context.Background()) })
	p.drawers = append(p.drawers, d)
	return d
}

// detailNameWidth is the width a detail line's name column is pinned to:
// view.DetailNameWidth characters in the panel's face, measured as
// nameColumnWidth measures a hardware name's.
func detailNameWidth(a fyne.App, o Options) float32 {
	c := config.Default()
	if o.Store != nil {
		c = o.Store.Config()
	}
	var th fyne.Theme
	if a != nil {
		th = c.PanelAppearance(Appearance(a, o.Store)).Theme()
	}
	if th == nil {
		th = fynetheme.DefaultTheme()
	}
	t := canvas.NewText(strings.Repeat("n", view.DetailNameWidth), nil)
	t.TextSize = th.Size(fynetheme.SizeNameText)
	t.FontSource = th.Font(fyne.TextStyle{})
	return t.MinSize().Width
}

// subject is the section a click asks about and the detail its card opens.
// What on the card was clicked makes no difference (spec 062 R1).
func (p *Panel) subject(tap glance.Tap) (section, id string) {
	for key, c := range p.cards {
		if c.card == tap.Card {
			return key, c.opens
		}
	}
	return "", ""
}

// source is the section's source, and whether there is one.
func (p *Panel) source(key string) (panel.Source, bool) {
	for _, s := range p.opts.Sources {
		if s.Key() == key {
			return s, true
		}
	}
	return nil, false
}

// detailer is the section's source as one with details.
func (p *Panel) detailer(section string) (panel.Detailer, bool) {
	src, ok := p.source(section)
	if !ok {
		return nil, false
	}
	det, ok := src.(panel.Detailer)
	return det, ok
}

// openDrawers are the drawers out, in the order they opened, which is the
// row's order from the cards outwards.
func (p *Panel) openDrawers() []*drawer {
	var out []*drawer
	for _, d := range p.drawers {
		if d.dr.Open() {
			out = append(out, d)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].seq < out[j].seq })
	return out
}

// showing is the open drawer showing a section's detail, or the list a drill
// shown in it was reached through, or nil.
func (p *Panel) showing(section, id string) *drawer {
	det, ok := p.detailer(section)
	if !ok {
		return nil
	}
	for _, d := range p.openDrawers() {
		if d.section == section && detailRoot(det, d.id) == id {
			return d
		}
	}
	return nil
}

/*
tapped answers a click on a card (spec 062 R1, spec 088 R1-R4, R7).

A plain click with one drawer out or none opens the card's detail, changes
what the drawer shows, or closes it on a second click of the same card; with
several out it closes them all. A Shift-click is shiftTapped. A click on a
card with no detail does nothing.
*/
func (p *Panel) tapped(tap glance.Tap) {
	section, opens := p.subject(tap)
	det, ok := p.detailer(section)
	if !ok || opens == "" {
		return
	}
	if tap.Shift {
		p.shiftTapped(det, section, opens)
		return
	}
	open := p.openDrawers()
	switch {
	case len(open) > 1:
		p.closeAll()
	case len(open) == 1 && open[0].section == section:
		p.closeDrawer(open[0])
	default:
		id := p.opensOn(det, section, opens)
		if id == "" {
			return
		}
		d := p.drawers[0]
		if len(open) == 1 {
			d = open[0]
		}
		p.showDetail(d, det, section, id)
	}
}

/*
shiftTapped adds a drawer for the card's next detail not already out, in the
order its switch reads starting with the one it opens on, or closes the
card's drawer opened last when every one of its details is out (spec 088 R3,
R4). For a card with one detail that is a toggle.
*/
func (p *Panel) shiftTapped(det panel.Detailer, section, opens string) {
	first := p.opensOn(det, section, opens)
	if first == "" {
		return
	}
	order := []string{first}
	for _, c := range choices(det, first) {
		order = append(order, c.ID)
	}
	for _, id := range order {
		if p.showing(section, id) == nil {
			p.addDrawer(det, section, id)
			return
		}
	}
	open := p.openDrawers()
	for i := len(open) - 1; i >= 0; i-- {
		if open[i].section == section {
			p.closeDrawer(open[i])
			return
		}
	}
}

/*
addDrawer shows a detail in a drawer of its own beside those out, where the
screen has room for one, and in the outermost where it has not (spec 088 R8,
R9). With none out it is the first drawer, which opens whatever the room.

The room is the window's to read on Windows. On Plasma the compositor is
asked, off the UI thread as the first drawer's place is; elsewhere the row is
held to glance.DrawersWithoutRoom.
*/
func (p *Panel) addDrawer(det panel.Detailer, section, id string) {
	pn := p.win.Panel()
	if pn.Opened() == 0 {
		p.showDetail(p.drawers[0], det, section, id)
		return
	}
	fits, known := pn.FitsHere()
	if known || !p.kit.desk.Asks() {
		if !known {
			fits = pn.Opened() < glance.DrawersWithoutRoom
		}
		p.placeNew(fits, det, section, id)
		return
	}
	go func() {
		left, right, ok := p.kit.desk.Room(context.Background())
		p.do(func() {
			fits := pn.Opened() < glance.DrawersWithoutRoom
			if ok {
				fits = pn.Fits(left, right)
			}
			p.placeNew(fits, det, section, id)
		})
	}()
}

// placeNew shows a detail in a closed drawer, made if need be, or in the
// outermost when another would not fit. Nothing is resized for the second.
func (p *Panel) placeNew(fits bool, det panel.Detailer, section, id string) {
	if p.showing(section, id) != nil {
		return // opened meanwhile, by another click
	}
	if !fits {
		out := p.win.Panel().Outermost()
		for _, d := range p.drawers {
			if d.dr == out {
				p.showDetail(d, det, section, id)
				return
			}
		}
	}
	for _, d := range p.drawers {
		if !d.dr.Open() {
			p.showDetail(d, det, section, id)
			return
		}
	}
	p.showDetail(p.newDrawer(p.win.Panel().AddDrawer()), det, section, id)
}

/*
opensOn is the detail a section's card opens on: the one it last showed while
that is still offered, then the one the section names, then the first of its
choices (spec 062 R7). Empty when the source offers none of them.
*/
func (p *Panel) opensOn(src panel.Detailer, section, opens string) string {
	offered := src.Details()
	has := func(id string) bool {
		for _, info := range offered {
			if info.ID == id {
				return id != ""
			}
		}
		return false
	}
	if last := p.kit.last[section]; has(last) {
		return last
	}
	if has(opens) {
		return opens
	}
	for _, info := range offered {
		if info.Label != "" {
			return info.ID
		}
	}
	return ""
}

// choices are the details a person chooses between where id is one of them:
// those the source offers with a label. A detail that is not one of a choice
// has no switch.
func choices(src panel.Detailer, id string) []glance.Choice {
	var out []glance.Choice
	among := false
	for _, info := range src.Details() {
		if info.Label == "" {
			continue
		}
		c := glance.Choice{ID: info.ID, Label: info.Label}
		for _, r := range info.Key {
			c.Key = r
			break
		}
		out = append(out, c)
		among = among || info.ID == id
	}
	if !among {
		return nil
	}
	return out
}

// detailRoot is the detail a drill was reached through, and the ID itself
// for anything that is not one.
func detailRoot(src panel.Detailer, id string) string {
	dr, ok := src.(panel.Driller)
	if !ok || !dr.Drills(id) {
		return id
	}
	root, _ := panel.DrillRoot(id)
	return root
}

// chosen shows the detail a person chose in a drawer's switch, in place of
// the one shown (spec 062 R6). A detail is out in one drawer at a time: one
// that shows it already closes (spec 088 R6).
func (p *Panel) chosen(d *drawer, id string) {
	src, _ := p.source(d.section)
	det, ok := p.detailer(d.section)
	if !ok || !panel.Offers(src, id) {
		return
	}
	if other := p.showing(d.section, id); other != nil && other != d {
		p.closeDrawer(other)
	}
	p.showDetail(d, det, d.section, id)
}

// shiftChosen opens a choice of a drawer's switch in a drawer of its own, or
// closes the drawer that shows it, this one included (spec 088 R5).
func (p *Panel) shiftChosen(d *drawer, id string) {
	section := d.section
	src, _ := p.source(section)
	det, ok := p.detailer(section)
	if !ok || !panel.Offers(src, id) {
		return
	}
	if other := p.showing(section, id); other != nil {
		p.closeDrawer(other)
		return
	}
	p.addDrawer(det, section, id)
}

// showDetail points a drawer at a detail, opening it if it is closed, and
// starts the poll that keeps it current.
func (p *Panel) showDetail(d *drawer, src panel.Detailer, section, id string) {
	p.stopDetail(d)
	d.section, d.id = section, id
	p.kit.last[section] = id
	d.detail = view.Detail{Section: section, ID: id, Title: title(src, id)}
	d.filter.Clear()
	// A drill keeps the switch of the list it was reached through, selected
	// on that list: a person inside one can still see which of the three
	// they are in, and leave for another without coming back out first.
	root := detailRoot(src, id)
	d.sw.SetChoices(choices(src, root))
	d.sw.Select(root)

	dr := d.dr
	dr.SetTitle(d.detail.Title)
	start := func() {
		if !dr.Open() {
			p.kit.seq++
			d.seq = p.kit.seq
		}
		dr.SetOpen(true)
		p.drawerOut.Store(true)
		p.renderOne(d)
		p.pollDetail(d, src, id)
	}
	// Only the first drawer out settles the side and the held edge; the
	// rest go beyond it, on its side.
	if dr.Open() || p.win.Panel().Opened() > 0 || !p.kit.desk.Asks() {
		start()
		return
	}
	p.placeDrawer(d, start)
}

/*
placeDrawer settles which side the drawer opens on where only the compositor
knows the room and only it can hold an edge, then calls open on the UI
thread.

Asked off the UI thread: each is a round trip to the compositor of a
millisecond or two, and one that has stopped answering must cost its timeout
and not the panel. A drawer nobody could place opens on the right.
*/
func (p *Panel) placeDrawer(d *drawer, open func()) {
	dr := d.dr
	auto := dr.Auto()
	gen := d.gen
	desk := p.kit.desk
	go func() {
		ctx := context.Background()
		var left, right float32
		asked := false
		if auto {
			left, right, asked = desk.Room(ctx)
		}
		side := make(chan glance.Side, 1)
		p.do(func() {
			if asked {
				dr.Choose(left, right)
			}
			side <- dr.Side()
		})
		held := desk.HoldEdge(ctx, <-side == glance.Left)
		p.do(func() {
			if d.gen != gen {
				return // closed, or moved on, while the compositor was asked
			}
			dr.SetEdgeHeld(held)
			open()
		})
	}()
}

// title is the heading the source gives a detail.
func title(src panel.Detailer, id string) string {
	for _, info := range src.Details() {
		if info.ID == id {
			return info.Title
		}
	}
	// A drill-down is not in the list a section offers, so its heading
	// comes from the detail it was reached through. Without this the
	// drawer is headed with the path itself, which is plumbing.
	if root, _, ok := strings.Cut(id, "/"); ok {
		for _, info := range src.Details() {
			if info.ID == root {
				return info.Title
			}
		}
	}
	return id
}

// pollDetail reads a drawer's detail at its interval until the drawer moves
// on, drawing each reading on the UI thread.
func (p *Panel) pollDetail(d *drawer, src panel.Detailer, id string) {
	ctx, cancel := context.WithCancel(context.Background())
	d.stop = func() {
		cancel()
		src.CloseDetail(id)
	}
	gen := d.gen
	go func() {
		t := time.NewTicker(src.DetailInterval(id))
		defer t.Stop()
		// The first two readings are taken whether or not the reading is
		// stopped. Pause holds a page still; it does not leave a page a
		// person has just opened blank. Two and not one because a rate is
		// a difference: opening a drill builds a reader with no history,
		// whose first answer is empty, and a page stopped on that one
		// reading would report a busy process as idle.
		reads := 0
		for {
			// A paused detail is not read. The ticker keeps running, so
			// unpausing picks the reading up at the next tick rather than
			// needing the poll restarted.
			if d.paused.Load() && reads >= 2 {
				select {
				case <-ctx.Done():
					return
				case <-t.C:
				}
				continue
			}
			reads++
			detail, _ := src.PollDetail(ctx, id)
			if ctx.Err() != nil {
				return
			}
			p.do(func() {
				if d.gen != gen {
					return
				}
				// The heading a failed read carries is the one it was asked
				// under.
				if detail.Title == "" {
					detail.Title = d.detail.Title
				}
				d.detail = detail
				p.renderOne(d)
			})
			select {
			case <-ctx.Done():
				return
			case <-t.C:
			}
		}
	}()
}

// stopDetail ends the poll of what a drawer shows. Results still on their
// way are dropped by the generation.
func (p *Panel) stopDetail(d *drawer) {
	d.gen++
	if d.stop != nil {
		d.stop()
		d.stop = nil
	}
}

// stopDetails ends every drawer's poll.
func (p *Panel) stopDetails() {
	for _, d := range p.drawers {
		p.stopDetail(d)
	}
}

// closeDrawer pushes a drawer back and stops reading what it showed. Those
// beyond it move in.
func (p *Panel) closeDrawer(d *drawer) {
	p.stopDetail(d)
	d.section, d.id = "", ""
	d.dr.SetOpen(false)
	p.drawerOut.Store(p.win.Panel().Opened() > 0)
}

// closeAll pushes every drawer back in one change of shape. Closed one at a
// time they were a resize each, and on Plasma the window kept the drawers'
// width with the cards stretched across it (fynedesygn spec 079).
func (p *Panel) closeAll() {
	for _, d := range p.openDrawers() {
		p.stopDetail(d)
		d.section, d.id = "", ""
	}
	p.win.Panel().CloseDrawers()
	p.drawerOut.Store(false)
}

// applyDrawer brings the drawers into line with the settings: their side,
// and each closed whose section is no longer shown (spec 060 R4, R16; spec
// 088 R11, R13).
func (p *Panel) applyDrawer(c config.Config) {
	for _, d := range p.openDrawers() {
		if !c.Shows(d.section) {
			p.closeDrawer(d)
		}
	}
	dr := p.win.Panel().Drawer()
	switch c.DrawerSideOrDefault() {
	case config.DrawerRight:
		dr.SetSide(glance.Right)
	case config.DrawerLeft:
		dr.SetSide(glance.Left)
	default:
		dr.SetAuto()
	}
}

// renderDrawer draws every drawer out. It runs on the UI thread, after a
// reading and whenever the cards change.
func (p *Panel) renderDrawer() {
	for _, d := range p.drawers {
		p.renderOne(d)
	}
}

/*
renderOne draws the detail a drawer holds: its meters, then as many of its
lines as the height fits, then its reasons and its note. It runs on the UI
thread, after each reading and whenever the filter or the cards change.

The lines are narrowed by the filter before they are cut to the height, so a
name far down a long list can be found. Nothing scrolls: the list is as long
as the cards are tall (spec 060 R5).
*/
func (p *Panel) renderOne(d *drawer) {
	if d == nil || !d.dr.Open() {
		return
	}
	dr := d.dr
	// The reading is kept as it was read and decorated on the way to being
	// drawn. Writing the decoration back would add it again at the next
	// render, and a drawer redrawn three times between readings would grow
	// three pause rows.
	p.setControls(d)
	detail := p.decorate(d, d.detail)
	if len(detail.Parts) > 0 {
		p.renderParts(d, detail)
		return
	}
	d.partsShape, d.parts = "", nil
	dr.SetScrolling(false)
	if detail.Filterable {
		detail = detail.Matching(d.filter.Matches)
	}
	dr.SetTitle(detail.Title)

	// The filter's key works only on a list. The library has no way to take
	// a filter out of a drawer once it has one, so on a breakdown its key is
	// one nobody can type.
	d.filter.Key = 0
	if detail.Filterable {
		d.filter.Key = glance.DefaultFilterKey
	}

	meters := min(len(detail.Meters), len(d.meters))
	if !d.built || meters != d.shownMeters || detail.Filterable != d.listed {
		d.built, d.shownMeters, d.listed = true, meters, detail.Filterable
		pieces := []glance.Piece{d.sw}
		if detail.Filterable {
			pieces = append(pieces, d.filter)
		}
		for _, m := range d.meters[:meters] {
			pieces = append(pieces, m)
		}
		for _, r := range d.rows {
			pieces = append(pieces, r)
		}
		for _, r := range d.notes {
			pieces = append(pieces, r)
		}
		dr.SetContent(pieces...)
	}
	for i, m := range detail.Meters[:meters] {
		setMeter(d.meters[i], m)
	}

	// What stands above the list, and the lines under it that are always
	// drawn: the reasons and the note come out of the room the list has.
	above := []glance.Piece{d.sw}
	if detail.Filterable {
		above = append(above, d.filter)
	}
	for _, m := range d.meters[:meters] {
		above = append(above, m)
	}
	tail := tailLines(detail)
	fit := dr.Fit(d.rows[0], above...)
	tail = tail[:min(len(tail), len(d.notes), max(fit, 0))]
	lines := detail.Top(min(max(fit-len(tail), 0), len(d.rows))).Lines

	for i, r := range d.rows {
		if i >= len(lines) {
			r.SetShown(false)
			continue
		}
		p.setLine(r, lines[i])
		r.SetShown(true)
	}
	// A list of lines is clickable on the same terms a breakdown of parts
	// is: the row carries what it asks for and the drawer reports which was
	// hit. A row with nothing to ask does nothing.
	dr.SetRowTapped(func(r *glance.Row) { p.tappedRow(d, r) }, d.rows...)
	for i, r := range d.notes {
		if i >= len(tail) {
			r.SetShown(false)
			continue
		}
		r.SetLabel(tail[i])
		r.SetShown(true)
	}
}

// setMeter draws a detail's meter on one of the drawer's.
func setMeter(to *glance.Meter, m view.Meter) {
	to.SetLabel(m.Name())
	to.Set(m.Fraction, m.Caption, status(m.Status))
	to.SetTrailing(m.Reset)
	to.SetStats(m.StatsLeft, m.StatsRight)
}

// setLine draws a detail's line on one of the drawer's rows.
func (p *Panel) setLine(r *glance.Row, l view.DetailLine) {
	r.SetID(l.Open)
	r.SetLabel(l.Name)
	r.Set(p.value(view.Row{Value: l.Value, Unit: l.Unit, Parts: l.Parts, Status: l.Status}))
}

// plotPiece is the sparkline as the drawer takes its content. The library's
// sparkline is a canvas object with colours set per trace, and has no theme of its
// own to be told about.
type plotPiece struct{ *glance.Sparkline }

// Object is the sparkline itself.
func (s plotPiece) Object() fyne.CanvasObject { return s.Sparkline }

// SetTheme does nothing: drawPlot colours every trace on each draw.
func (plotPiece) SetTheme(fyne.Theme) {}

// DrawerPlotHeight is a plot's height in the drawer: twice a card's, because a
// drawer is opened to look at the history a card only hints at.
const DrawerPlotHeight = 2 * glance.SparklineHeight

// drawPlot gives one part's plot its trails, as a card's plot is given them (spec
// 064, 086): a new sample drifts into place over the time between samples, and a
// trace the part no longer has is emptied.
func (p *Panel) drawPlot(built *drawerPart, part view.Detail) {
	built.plot.SetDrift(part.TrailEvery)
	trails := part.Trails
	samples := make(map[string][]float64, len(built.traces)+len(trails))
	for name := range built.traces {
		samples[name] = nil
	}
	for _, t := range trails {
		colour := p.trailColour(t, view.Section{})
		if built.traces[t.Name] {
			built.plot.SetColour(t.Name, colour)
		} else {
			built.plot.AddSeries(t.Name, colour, view.SparkMinSpan)
			built.traces[t.Name] = true
		}
		samples[t.Name] = t.Samples
	}
	built.plot.SetSamples(samples)
}

// tailLines are the lines under a detail's list: its reasons, then its note.
func tailLines(detail view.Detail) []string {
	var tail []string
	for _, r := range detail.Reasons {
		text := r.Text
		if r.Label != "" {
			text = r.Label + ": " + text
		}
		tail = append(tail, text)
	}
	if detail.Note != "" {
		tail = append(tail, detail.Note)
	}
	return tail
}

/*
renderParts draws a detail made of details, one under another: every
account's usage (spec 062 R9). Each part is its heading, its meters, its
lines, then its reasons and its note.

Nothing is cut to the height: the whole of it is drawn and the drawer scrolls
when it is taller than the cards (R11). The pieces are built when the shape
changes, an account or a limit arriving or going, and set in place otherwise,
so a reading leaves a scrolled breakdown where it is (R12).
*/
func (p *Panel) renderParts(d *drawer, detail view.Detail) {
	dr := d.dr
	dr.SetTitle(detail.Title)
	d.filter.Key = 0 // not a list: nothing to filter, as for one breakdown

	shape := ""
	for _, part := range detail.Parts {
		shape += fmt.Sprintf("%d/%d/%d/%t;", len(part.Meters), len(part.Lines), len(tailLines(part)), len(part.Trails) > 0)
	}
	if shape != d.partsShape {
		// A rebuild starts the content again at the top. A live breakdown
		// changes shape whenever a row comes or goes, so without putting the
		// offset back the drawer could not be scrolled at all: each reading
		// would take the reader to the top again.
		was := dr.Offset()
		defer func() { fyne.Do(func() { dr.SetOffset(was) }) }()
		d.partsShape, d.built = shape, false
		d.parts, d.rules = d.parts[:0], 0
		// The switch heads a breakdown as it heads a list: the peers are one
		// of two bandwidth lists, and without it there was no way back to the
		// other but a key nobody could see (#226). A breakdown with nothing to
		// choose between has a switch that draws nothing.
		pieces := []glance.Piece{d.sw}
		for i, part := range detail.Parts {
			if i > 0 {
				// A rule between one account and the next: their meters
				// look alike, and a gap alone does not say where one ends.
				pieces = append(pieces, glance.NewRule())
				d.rules++
			}
			built := drawerPart{head: glance.NewRow("", "")}
			pieces = append(pieces, built.head)
			if len(part.Trails) > 0 {
				built.plot = glance.NewSparkline(SparkCapacity)
				built.plot.SetHeight(DrawerPlotHeight)
				built.plot.SetScale(glance.ScaleShared)
				built.traces = map[string]bool{}
				pieces = append(pieces, plotPiece{built.plot})
			}
			for range part.Meters {
				m := glance.NewMeter("", MeterLabelWidth)
				built.meters = append(built.meters, m)
				pieces = append(pieces, m)
			}
			for range part.Lines {
				r := glance.NewRowWidth("", "", p.kit.name)
				built.rows = append(built.rows, r)
				pieces = append(pieces, r)
			}
			for range tailLines(part) {
				r := glance.NewRow("", "")
				built.notes = append(built.notes, r)
				pieces = append(pieces, r)
			}
			d.parts = append(d.parts, built)
		}
		dr.SetScrolling(true)
		dr.SetContent(pieces...)
		var taps []*glance.Row
		for _, built := range d.parts {
			taps = append(taps, built.rows...)
		}
		dr.SetRowTapped(func(r *glance.Row) { p.tappedRow(d, r) }, taps...)
	}
	for i, part := range detail.Parts {
		built := d.parts[i]
		built.head.SetLabel(part.Title)
		if len(part.TitleParts) > 0 {
			built.head.Set(p.value(view.JoinedRow(part.TitleParts)))
		}
		if built.plot != nil {
			p.drawPlot(&built, part)
		}
		for j, m := range part.Meters {
			setMeter(built.meters[j], m)
		}
		for j, l := range part.Lines {
			p.setLine(built.rows[j], l)
		}
		for j, text := range tailLines(part) {
			built.notes[j].SetLabel(text)
		}
	}
}

// restyleDrawer repaints the drawers' own pieces in the current theme.
func (p *Panel) restyleDrawer() {
	for _, d := range p.drawers {
		for _, m := range d.meters {
			m.Restyle()
		}
		for _, part := range d.parts {
			for _, m := range part.meters {
				m.Restyle()
			}
		}
	}
	p.renderDrawer()
}
