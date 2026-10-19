package gui

import (
	"context"
	"fmt"
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
The drawer: what a card has no room to say, beside the cards and as tall as
them (spec 060). The surface, its place, the filter and the keyboard are the
design system's (fynedesygn spec 068); what it shows is a view.Detail, which
a source reads and this draws.

A click anywhere on a card opens that section's detail, a second click on the
same card closes the drawer, and a click on another card changes what it
shows (spec 062). Where a section has several details the drawer has a switch
between them, and what was chosen is where that card opens next. A detail is
read only while it is shown, by a poll of its own that starts when the drawer
opens on it and stops when the drawer closes, moves on, or the section is
hidden.
*/
type drawer struct {
	// sw chooses between a section's details where it has several: the
	// cooler's lists. last is the detail each section last showed, which is
	// what its card opens on next; it is not kept across a restart.
	sw   *glance.Switch
	last map[string]string

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
	// rules is how many rules stand between them. name is the width a
	// line's name column is pinned to.
	parts      []drawerPart
	rules      int
	partsShape string
	name       float32

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

	// stop ends the poll of what is shown, and gen tells a result that
	// arrives after the drawer has moved on from one that is current.
	stop context.CancelFunc
	gen  int

	// desk is the compositor's half: the room beside the window, a held
	// edge, the keyboard handed back.
	desk Desk
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

// buildDrawer makes the drawer's pieces and points the panel's clicks at it.
// The pieces are built once; a detail sets them in place.
func (p *Panel) buildDrawer(a fyne.App, o Options) {
	d := &drawer{sw: glance.NewSwitch(), last: map[string]string{}, filter: glance.NewFilter(), desk: o.Desk}
	if d.desk == nil {
		d.desk = noDesk{}
	}
	d.filter.OnChanged = p.renderDrawer
	d.sw.OnChanged = p.chosen
	d.name = detailNameWidth(a, o)
	name := d.name
	// Every line starts hidden: a line is shown when a detail has something
	// to put on it.
	for range DrawerRows {
		r := glance.NewRowWidth("", "", name)
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
	p.drawer = d

	dr := p.win.Panel().Drawer()
	dr.SetFilter(d.filter)
	dr.SetSwitch(d.sw)
	// A Wayland client cannot give the keyboard away; the compositor is
	// asked, off this thread. On Windows the design system has already done
	// it and this is not called.
	dr.SetYield(func() { go d.desk.Yield(context.Background()) })
	p.win.Panel().OnTapped = p.tapped
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

// tapped opens the drawer on the card clicked, changes what it shows when
// another card's is out, or closes it on a second click of the same card
// (spec 062 R1). A click on a card with no detail does nothing.
func (p *Panel) tapped(tap glance.Tap) {
	section, opens := p.subject(tap)
	src, ok := p.source(section)
	if !ok || opens == "" {
		return
	}
	det, ok := src.(panel.Detailer)
	if !ok {
		return
	}
	d := p.drawer
	if p.win.Panel().Drawer().Open() && d.section == section {
		p.closeDrawer()
		return
	}
	if id := d.opensOn(det, section, opens); id != "" {
		p.showDetail(det, section, id)
	}
}

/*
opensOn is the detail a section's card opens on: the one it last showed while
that is still offered, then the one the section names, then the first of its
choices (spec 062 R7). Empty when the source offers none of them.
*/
func (d *drawer) opensOn(src panel.Detailer, section, opens string) string {
	offered := src.Details()
	has := func(id string) bool {
		for _, info := range offered {
			if info.ID == id {
				return id != ""
			}
		}
		return false
	}
	if last := d.last[section]; has(last) {
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

// chosen shows the detail a person chose in the drawer's switch, in place of
// the one shown (spec 062 R6).
func (p *Panel) chosen(id string) {
	d := p.drawer
	src, ok := p.source(d.section)
	if !ok {
		return
	}
	if det, ok := src.(panel.Detailer); ok && panel.Offers(src, id) {
		p.showDetail(det, d.section, id)
	}
}

// showDetail points the drawer at a detail, opening it if it is closed, and
// starts the poll that keeps it current.
func (p *Panel) showDetail(src panel.Detailer, section, id string) {
	d := p.drawer
	p.stopDetail()
	d.section, d.id = section, id
	d.last[section] = id
	d.detail = view.Detail{Section: section, ID: id, Title: title(src, id)}
	d.filter.Clear()
	// A drill keeps the switch of the list it was reached through, selected
	// on that list: a person inside one can still see which of the three
	// they are in, and leave for another without coming back out first.
	root := detailRoot(src, id)
	d.sw.SetChoices(choices(src, root))
	d.sw.Select(root)

	dr := p.win.Panel().Drawer()
	dr.SetTitle(d.detail.Title)
	start := func() {
		p.drawerOut.Store(true)
		dr.SetOpen(true)
		p.renderDrawer()
		p.pollDetail(src, id)
	}
	if dr.Open() || !d.desk.Asks() {
		start()
		return
	}
	p.placeDrawer(start)
}

/*
placeDrawer settles which side the drawer opens on where only the compositor
knows the room and only it can hold an edge, then calls open on the UI
thread.

Asked off the UI thread: each is a round trip to the compositor of a
millisecond or two, and one that has stopped answering must cost its timeout
and not the panel. A drawer nobody could place opens on the right.
*/
func (p *Panel) placeDrawer(open func()) {
	d := p.drawer
	dr := p.win.Panel().Drawer()
	auto := dr.Auto()
	gen := d.gen
	go func() {
		ctx := context.Background()
		var left, right float32
		asked := false
		if auto {
			left, right, asked = d.desk.Room(ctx)
		}
		side := make(chan glance.Side, 1)
		p.do(func() {
			if asked {
				dr.Choose(left, right)
			}
			side <- dr.Side()
		})
		held := d.desk.HoldEdge(ctx, <-side == glance.Left)
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

// pollDetail reads the detail at its interval until the drawer moves on,
// drawing each reading on the UI thread.
func (p *Panel) pollDetail(src panel.Detailer, id string) {
	d := p.drawer
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
				p.renderDrawer()
			})
			select {
			case <-ctx.Done():
				return
			case <-t.C:
			}
		}
	}()
}

// stopDetail ends the poll of what the drawer shows. Results still on their
// way are dropped by the generation.
func (p *Panel) stopDetail() {
	d := p.drawer
	d.gen++
	if d.stop != nil {
		d.stop()
		d.stop = nil
	}
}

// closeDrawer pushes the drawer back and stops reading what it showed.
func (p *Panel) closeDrawer() {
	p.stopDetail()
	p.drawer.section, p.drawer.id = "", ""
	p.win.Panel().Drawer().SetOpen(false)
	p.drawerOut.Store(false)
}

// applyDrawer brings the drawer into line with the settings: its side, and
// closed if the section it shows is no longer shown (spec 060 R4, R16).
func (p *Panel) applyDrawer(c config.Config) {
	d := p.drawer
	dr := p.win.Panel().Drawer()
	if dr.Open() && !c.Shows(d.section) {
		p.closeDrawer()
	}
	switch c.DrawerSideOrDefault() {
	case config.DrawerRight:
		dr.SetSide(glance.Right)
	case config.DrawerLeft:
		dr.SetSide(glance.Left)
	default:
		dr.SetAuto()
	}
}

/*
renderDrawer draws the detail the drawer holds: its meters, then as many of
its lines as the height fits, then its reasons and its note. It runs on the
UI thread, after each reading and whenever the filter or the cards change.

The lines are narrowed by the filter before they are cut to the height, so a
name far down a long list can be found. Nothing scrolls: the list is as long
as the cards are tall (spec 060 R5).
*/
func (p *Panel) renderDrawer() {
	d := p.drawer
	dr := p.win.Panel().Drawer()
	if d == nil || !dr.Open() {
		return
	}
	// The reading is kept as it was read and decorated on the way to being
	// drawn. Writing the decoration back would add it again at the next
	// render, and a drawer redrawn three times between readings would grow
	// three pause rows.
	p.setControls()
	detail := p.decorate(d.detail)
	if len(detail.Parts) > 0 {
		p.renderParts(detail)
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
	dr.SetRowTapped(p.tappedRow, d.rows...)
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
func (p *Panel) renderParts(detail view.Detail) {
	d := p.drawer
	dr := p.win.Panel().Drawer()
	dr.SetTitle(detail.Title)
	d.filter.Key = 0 // not a list: nothing to filter, as for one breakdown

	shape := ""
	for _, part := range detail.Parts {
		shape += fmt.Sprintf("%d/%d/%d;", len(part.Meters), len(part.Lines), len(tailLines(part)))
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
		var pieces []glance.Piece
		for i, part := range detail.Parts {
			if i > 0 {
				// A rule between one account and the next: their meters
				// look alike, and a gap alone does not say where one ends.
				pieces = append(pieces, glance.NewRule())
				d.rules++
			}
			built := drawerPart{head: glance.NewRow("", "")}
			pieces = append(pieces, built.head)
			for range part.Meters {
				m := glance.NewMeter("", MeterLabelWidth)
				built.meters = append(built.meters, m)
				pieces = append(pieces, m)
			}
			for range part.Lines {
				r := glance.NewRowWidth("", "", d.name)
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
		dr.SetRowTapped(p.tappedRow, taps...)
	}
	for i, part := range detail.Parts {
		built := d.parts[i]
		built.head.SetLabel(part.Title)
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

// restyleDrawer repaints the drawer's own pieces in the current theme.
func (p *Panel) restyleDrawer() {
	if p.drawer == nil {
		return
	}
	for _, m := range p.drawer.meters {
		m.Restyle()
	}
	for _, part := range p.drawer.parts {
		for _, m := range part.meters {
			m.Restyle()
		}
	}
	p.renderDrawer()
}
