package gui

import (
	"context"
	"strings"
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

A click on a card opens that section's detail, and on a row or a meter with
one of its own, that one. A click on what is already shown closes the drawer;
a click on something else changes what it shows. A detail is read only while
it is shown, by a poll of its own that starts when the drawer opens on it and
stops when the drawer closes, moves on, or the section is hidden.
*/
type drawer struct {
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
	d := &drawer{filter: glance.NewFilter(), desk: o.Desk}
	if d.desk == nil {
		d.desk = noDesk{}
	}
	d.filter.OnChanged = p.renderDrawer
	name := detailNameWidth(a, o)
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

// subject is the section and the detail a click asks for: the row's own, the
// meter's own, or the section's.
func (p *Panel) subject(tap glance.Tap) (section, id string) {
	for key, c := range p.cards {
		if c.card != tap.Card {
			continue
		}
		id = c.opens
		if tap.Row != nil {
			if opens, ok := c.rowOpens[tap.Row.ID()]; ok && opens != "" {
				id = opens
			}
		}
		if tap.Meter != nil {
			for i, m := range c.meters {
				if m == tap.Meter && i < len(c.meterOpens) && c.meterOpens[i] != "" {
					id = c.meterOpens[i]
				}
			}
		}
		return key, id
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

// tapped opens the drawer on what was clicked, changes what it shows, or
// closes it on a second click of the same thing (spec 060 R3). A click on a
// card with no detail does nothing.
func (p *Panel) tapped(tap glance.Tap) {
	section, id := p.subject(tap)
	src, ok := p.source(section)
	if !ok || id == "" || !panel.Offers(src, id) {
		return
	}
	d := p.drawer
	dr := p.win.Panel().Drawer()
	if dr.Open() && d.section == section && d.id == id {
		p.closeDrawer()
		return
	}
	p.showDetail(src.(panel.Detailer), section, id)
}

// showDetail points the drawer at a detail, opening it if it is closed, and
// starts the poll that keeps it current.
func (p *Panel) showDetail(src panel.Detailer, section, id string) {
	d := p.drawer
	p.stopDetail()
	d.section, d.id = section, id
	d.detail = view.Detail{Section: section, ID: id, Title: title(src, id)}
	d.filter.Clear()

	dr := p.win.Panel().Drawer()
	dr.SetTitle(d.detail.Title)
	start := func() {
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
		for {
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
	detail := d.detail
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
		var pieces []glance.Piece
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
		d.meters[i].SetLabel(m.Name())
		d.meters[i].Set(m.Fraction, m.Caption, status(m.Status))
		d.meters[i].SetTrailing(m.Reset)
		d.meters[i].SetStats(m.StatsLeft, m.StatsRight)
	}

	// What stands above the list, and the lines under it that are always
	// drawn: the reasons and the note come out of the room the list has.
	var above []glance.Piece
	if detail.Filterable {
		above = append(above, d.filter)
	}
	for _, m := range d.meters[:meters] {
		above = append(above, m)
	}
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
	fit := dr.Fit(d.rows[0], above...)
	tail = tail[:min(len(tail), len(d.notes), max(fit, 0))]
	lines := detail.Top(min(max(fit-len(tail), 0), len(d.rows))).Lines

	for i, r := range d.rows {
		if i >= len(lines) {
			r.SetShown(false)
			continue
		}
		l := lines[i]
		r.SetLabel(l.Name)
		r.Set(p.value(view.Row{Value: l.Value, Unit: l.Unit, Parts: l.Parts, Status: l.Status}))
		r.SetShown(true)
	}
	for i, r := range d.notes {
		if i >= len(tail) {
			r.SetShown(false)
			continue
		}
		r.SetLabel(tail[i])
		r.SetShown(true)
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
	p.renderDrawer()
}
