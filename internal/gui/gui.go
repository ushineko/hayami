/*
Package gui is the desktop panel: a glance window from fynedesygn, drawing the
same sections the terminal does.

The window arranges; it does not decide what a section says. Everything here
turns a view.Section into cards and rows, and nothing here reads a sensor.
*/
package gui

import (
	"context"
	"image/color"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	fynetheme "fyne.io/fyne/v2/theme"

	fd "github.com/ushineko/fynedesygn"
	"github.com/ushineko/fynedesygn/glance"
	fdtheme "github.com/ushineko/fynedesygn/theme"
	"github.com/ushineko/fynedesygn/widgets"

	"github.com/ushineko/hayami/internal/config"
	"github.com/ushineko/hayami/internal/panel"
	"github.com/ushineko/hayami/internal/readings"
	"github.com/ushineko/hayami/internal/view"
)

// AppID is the window's identity. The same string is the Wayland app_id Fyne
// derives from it, the desktop file's basename, and what a KWin rule matches
// on, so all three stay in step.
const AppID = "io.ushineko.hayami"

// Options are what the window needs to start.
type Options struct {
	// Sources are the sections, in the order to draw them.
	Sources []panel.Source

	// Title names the window for the taskbar and for a compositor rule.
	Title string

	// Menu builds the panel's context menu. Nil means no menu, which for a
	// glance window means no interface at all.
	Menu func() *fyne.Menu

	// Version is what the preferences window shows in its title.
	Version string

	// Store is the settings, shared with the preferences window. Nil means a
	// panel with nothing to follow, which is what a test has.
	Store *config.Store

	// Preferences opens the preferences window at start as well as the panel.
	Preferences bool

	// Sections are the sections to build the cards from, by key, where they
	// are already known: a live first reading, or what the cache restored.
	// A key with no entry falls back to asking its source, which is what a
	// test that builds a panel by hand does.
	Sections map[string]view.Section
}

// Panel is the window and the cards in it.
type Panel struct {
	win   *glance.Window
	cards map[string]*card
	opts  Options

	// app is kept so the card opacity can be re-applied when the setting
	// changes: it is a property of the theme, and the theme belongs to the
	// app rather than to the window.
	app fyne.App

	// cache is what each section last read, written out on a timer so the
	// next run opens showing it rather than a blank (spec 013).
	cacheMu sync.Mutex
	cache   readings.Cache

	// live is the sources that have answered this run. Until one has, its
	// card shows what the cache remembered.
	live map[string]bool
}

// card is one section's card and the pieces in it, kept so a poll repaints
// rather than rebuilding. A glance window is redrawn several times a minute
// and rebuilding at that rate would fight the no-reflow rule.
type card struct {
	card   *glance.Card
	rows   []*glance.Row
	meters []*glance.Meter

	// spark is the section's trend, for a section that has one. Nil for the
	// rest, which is most of them.
	spark *glance.Sparkline

	// grid is the section's cells, for a section whose readings are blocks
	// rather than lines. Nil for the rest, which is every section but the
	// peripherals.
	grid  *glance.CellGrid
	cells []*glance.Cell
}

// SparkCapacity is how many samples the window's plot holds.
//
// The same as the pane's, because they are drawing the same series: a trend
// that covered a different span in each shell would be two different readings
// with one name.
const SparkCapacity = panel.CoolerTrail

/*
CellSlack is how many spare cells a card of cells is built with.

The library takes objects at build time and a card rebuilt on a poll would
reflow the window several times a minute, so a cell that was not built cannot
be added. For a meter that is livable -- an account appearing is rare. For a
peripheral it is not: a mouse is switched on, a headset comes off its cradle,
a pair of earbuds is taken out of the case, and a window that had to be
restarted to see any of it is a window nobody would keep open.

Four, which is what the archetype has slots for and more than this machine has
ever had at once. They cost nothing while they are hidden.
*/
const CellSlack = 4

// MeterLabelWidth pins a card's meter labels to one column, so several meters
// stacked in a card line their captions up and two windows of the same quota
// can be compared at a glance.
const MeterLabelWidth float32 = 84

// New builds the window with a card per source. Cards are not drawn until
// their source answers, so the window does not flash empty on the way up.
//
// **Poll the sources before calling this.** A card is built with the rows and
// meters its section has at that moment, and the library takes objects at
// build time: a card built from an empty section stays empty. Start does the
// first poll for this reason, and the window is the real size on its first
// frame rather than growing into it.
func New(a fyne.App, o Options) *Panel {
	if o.Title == "" {
		o.Title = "hayami"
	}
	p := &Panel{
		win: glance.NewWindow(a, glance.Options{
			Title: o.Title,
			OnTop: true,
			Menu:  o.Menu,

			// The one of the three the toolkit can grant. GLFW gives the
			// window a framebuffer with an alpha channel and the design
			// system makes the window's own background transparent, so the
			// desktop shows through the space between cards. The titlebar is
			// KWin's to remove and nothing here can ask for it.
			Translucent: true,

			// Where the panel sits and how big it is are the user's business.
			// A fixed-size window tells the window manager it will not take a
			// resize, so a frameless panel's own Resize menu item is greyed
			// out and there is no way at all to ask for a wider one.
			//
			// It cannot be made *narrower* than its content — Fyne clamps to
			// the minimum size — which is why spec 011 had to narrow the
			// content rather than rely on this. The two are complementary:
			// that one was about the panel being the wrong size, this one is
			// about who gets to change it.
			Resizable: true,

			// No floor: the window is as wide as its widest card and not a
			// pixel more. The design system's default floor is 260, which is
			// a guard against a panel of one short reading looking like a
			// chip; these cards measure 94, 180, 216 and 218, so the floor
			// was buying nothing and costing forty-two pixels of empty panel
			// down the right-hand side. A panel the user can resize does not
			// need protecting from being narrow, either.
			MinWidth: glance.NoMinWidth,
		}),
		cards: map[string]*card{},
		cache: readings.Cache{},
		opts:  o,
		app:   a,
	}

	for _, s := range o.Sources {
		// The section the card is built from decides its shape for the life
		// of the program, because the library takes its objects now. Where
		// Start restored one from the cache, that is the shape to build.
		sec, ok := o.Sections[s.Key()]
		if !ok {
			sec = s.Section()
		}
		c := glance.NewCard(sec.Title)
		holder := &card{card: c}
		for _, r := range flatten(sec.Rows) {
			row := glance.NewRow(r.Label, r.Value)
			holder.rows = append(holder.rows, row)
			c.AddRow(row)
		}
		for _, m := range sec.Meters {
			// The window arranges in one column, not the pane's three: the
			// design system's meter pins its label to a width of its own and
			// the card is narrow. Name() is the three parts as one string.
			meter := glance.NewMeter(m.Name(), MeterLabelWidth)
			holder.meters = append(holder.meters, meter)
			c.Add(meter)
		}

		// Every card gets a grid, whether or not it has cells yet. Cells and
		// rows are separate for the reason the view keeps them separate: they
		// are laid out differently, and nothing so far has both.
		//
		// **Whether or not**, because the alternative was a card that could
		// never hold one. The library takes objects at build time, this runs
		// once before the window exists, and a first poll that found nothing
		// therefore left the peripherals card empty for the life of the
		// program -- no grid, and no CellSlack either, because the slack pads
		// a grid that was created. A wireless mouse that has been still
		// answers nothing about one poll in fourteen, so restarting the panel
		// at the wrong moment was enough to do it.
		//
		// An empty grid costs nothing: the library's layout reports a zero
		// size while no cell is shown, so a card of rows is unchanged.
		holder.grid = glance.NewCellGrid()
		for i := range len(sec.Cells) + CellSlack {
			blank := view.NoQuantity()
			if i < len(sec.Cells) {
				blank = sec.Cells[i].Value
			}
			cell := glance.NewCell("", blank)
			cell.SetShown(i < len(sec.Cells))
			holder.cells = append(holder.cells, cell)
			holder.grid.Add(cell)
		}
		c.Add(holder.grid)

		// A section with a trend to plot gets one. The pane has drawn these
		// since spec 006 and the window never has, which is a parity gap the
		// parity test could not see: it compares which sections each shell
		// draws, not what they draw in them.
		//
		// One plot however many series: the design system's sparkline holds
		// several and scales each to its own range, which is what makes the
		// coolant and the processor readable on one line.
		if len(sec.Trails) > 0 {
			holder.spark = glance.NewSparkline(SparkCapacity)
			c.AddObject(holder.spark)
		}

		p.cards[s.Key()] = holder
		p.win.Panel().Add(c)
	}
	return p
}

// Window is the glance window, for a caller that wants the menu or the icon.
func (p *Panel) Window() *glance.Window { return p.win }

/*
Apply brings the window into line with a changed configuration.

Hiding and showing a section is live: a card is drawn when the user allows it
*and* its source has something to say, and this is the first of those. The
card's own callback stops the poll, so a cooler section switched off stops
running liquidctl every five seconds for nobody.

**Reordering is not live.** The design system's panel adds cards and never
removes or moves one, so the stack's order is fixed when the window is built.
A reorder therefore takes effect at the next start, which the preferences
window says. It is in this spec's gaps.
*/
func (p *Panel) Apply(c config.Config) {
	p.applyTheme(c)

	// And repaint in it. A card restyles its title and its rows; a meter and
	// a sparkline go in as plain canvas objects and have to be told, and
	// without this a panel given a size of its own drew its meters' labels in
	// the size the application had when they were built.
	p.restyle()
	for key, card := range p.cards {
		card.card.SetAllowed(c.Shows(key))
	}
	for _, s := range p.opts.Sources {
		if b, ok := s.(*panel.Bandwidth); ok {
			b.SetInterfaces(c.Interfaces)
		}
	}
}

// Draw brings a section's card up to date. It is called on the UI thread.
//
// A card whose row count changed is rebuilt; otherwise the rows are set in
// place. The count changes when the user edits the watched interfaces, which
// is a real event, and not when a number does.
func (p *Panel) Draw(key string, sec view.Section, drawn bool) {
	c, ok := p.cards[key]
	if !ok {
		return
	}

	// A source that has not answered yet this run shows what the panel last
	// knew instead of nothing, until it does. Not for a source that *has*
	// answered: then an empty poll means the section has gone, and a reading
	// kept indefinitely would be the panel insisting on hardware that is no
	// longer there. Fifteen seconds of blank while a mouse wakes is the
	// whole of what this is for (spec 013).
	if drawn && !sec.Restored {
		// A live reading, and only a live one. A restored section arrives
		// here with drawn set as well -- it is a section to draw -- and
		// counting it as having been heard would stop the cache standing in
		// on the *next* empty poll, which is a second or two later and hid
		// the card again.
		p.heard(key)
	} else if !drawn {
		if restored, err := p.lastKnown(key); err == nil {
			sec, drawn = restored, true
		}
	}
	c.card.SetAvailable(drawn)
	if !drawn {
		return
	}

	// A source that was answering and has stopped keeps its last values and
	// draws them dim, marker and all. The card does the whole of it; this
	// only has to say which state it is in, and to say it before the rows are
	// set so a row written afterwards is not left bright.
	// Dim either way, but only a source that has *stopped* is unavailable.
	// A restored reading is the last one heard by a panel that has not asked
	// yet, and saying otherwise reports a fault where there is a device that
	// has not woken up.
	if sec.Gone {
		c.card.SetStale(true)
	} else {
		c.card.SetLastKnown(sec.Restored)
	}
	rows := flatten(sec.Rows)
	if len(rows) != len(c.rows) {
		p.rebuild(c, rows)
	} else {
		for i, r := range rows {
			c.rows[i].SetLabel(r.Label)
			c.rows[i].Set(reading(r))
		}
	}

	// A cell the card was not built with cannot be added now, for the reason
	// the meters below give: the library takes objects at build time. A
	// peripheral appearing is not rare, though, which is why a card is built
	// with room and the surplus cells are hidden rather than missing -- see
	// drawCells.
	p.drawCells(c, sec.Cells, sec.Dimmed())

	// A meter the card was not built with cannot be added now: the library
	// takes objects at build time and a card rebuilt on a poll would reflow
	// the window several times a minute. A section that gains a meter after
	// the window is up is a restart, and the only thing that does that today
	// is an account appearing, which is rare enough to live with. It is in
	// the spec's gaps.
	for i, m := range sec.Meters {
		if i >= len(c.meters) {
			break
		}
		c.meters[i].SetLabel(m.Name())

		// The figures go in the slots rather than into one caption. A row
		// with a stretch in it is as wide as its widest pair; the same
		// figures concatenated made this window 655 px wide against 268 px
		// for the rest of the panel.
		c.meters[i].Set(m.Fraction, m.Caption, status(m.Status))
		c.meters[i].SetTrailing(m.Reset)
		c.meters[i].SetStats(m.StatsLeft, m.StatsRight)
	}

	// The plot is given the whole series rather than the newest sample: the
	// section keeps the trails and this is a view of them, so a window that
	// missed a poll or was rebuilt still draws the same shape the pane does.
	//
	// The series are registered on every draw because their colour follows
	// the reading -- the coolant's band, and Dim for a section whose source
	// has stopped answering -- and because a section that gains a trail
	// should not have to wait for a restart to plot it.
	if c.spark != nil && len(sec.Trails) > 0 {
		c.spark.Clear()
		for _, t := range sec.Trails {
			c.spark.AddSeries(t.Name, p.trailColour(t, sec.Gone), view.SparkMinSpan)
			for _, v := range t.Samples {
				c.spark.Add(t.Name, v)
			}
		}
	}

	/*
		Re-measure, because a card can get shorter.

		The design system resizes the window when a card is added or hidden,
		which is the case its own comment is about (quirk 34: a Fyne window
		grows to fit and never shrinks back on its own). It cannot know about
		a card whose *contents* shrank — a detail line that went away, a
		peripheral that was unplugged, a meter's stats row that emptied — and
		those happen on a poll, several times a minute.

		Without this the window keeps its high-water mark and draws panel
		background below the last card. It shows as a band of empty window at
		the bottom that never goes away, because nothing ever lowers the
		requested size again.
	*/
	p.win.Panel().Resize()
}

/*
drawCells brings a card's cells up to date.

A cell the card was not built with cannot be added: the library takes objects
at build time, and a card rebuilt on a poll would reflow the window several
times a minute. Unlike a meter, though, a section gaining one is an ordinary
event -- a mouse is switched on, a headset comes off its cradle -- so the
surplus is drawn as a hidden cell rather than dropped, and a device that
appears fills one. A device that goes away hides its own again.

CellSlack is how many spare there are. A window that has to be restarted to see
a peripheral is a window nobody would keep open.
*/
func (p *Panel) drawCells(c *card, cells []view.Cell, dim bool) {
	for i, cell := range c.cells {
		if i >= len(cells) {
			cell.SetShown(false)
			continue
		}
		cl := cells[i]
		cell.SetName(cl.Label)
		cell.SetNote(cl.Note)
		rd := reading(view.Row{Value: cl.Value, Unit: cl.Unit, Status: cl.Status})
		// The section's dimming reaches its cells. A card dims its own rows
		// when it is told it is stale, and a cell is not one of them: it is
		// a separate object holding its own reading, so a restored section
		// drew a live-looking percentage until this was here.
		rd.Stale = cl.Stale || dim
		cell.Set(rd)
		cell.SetShown(true)
	}
}

// flatten turns a row with a detail line into two rows.
//
// The design system's card has no second line under a row: a glance.Row is one
// label and one value. The terminal draws the detail right-aligned under the
// value and the window draws it as its own quiet row, which keeps both shells
// showing the same numbers. A row for it in the library would be better and is
// in this spec's gaps.
func flatten(rows []view.Row) []view.Row {
	out := make([]view.Row, 0, len(rows))
	for _, r := range rows {
		bare := r
		bare.Detail = ""
		out = append(out, bare)
		if r.Detail != "" {
			out = append(out, view.Row{Value: r.Detail, Status: view.Info})
		}
	}
	return out
}

// rebuild replaces a card's rows. It is the slow path and is taken only when
// the shape of the section changed.
func (p *Panel) rebuild(c *card, want []view.Row) {
	rows := c.card.Rows()
	for i, r := range want {
		if i < len(rows) {
			rows[i].SetLabel(r.Label)
			rows[i].Set(reading(r))
			rows[i].SetShown(true)
			continue
		}
		row := glance.NewRow(r.Label, r.Value)
		row.Set(reading(r))
		c.card.AddRow(row)
		c.rows = append(c.rows, row)
	}
	// A row the section no longer has is hidden rather than removed: the
	// library has no remove, and a hidden row costs nothing.
	for i := len(want); i < len(c.rows); i++ {
		c.rows[i].SetShown(false)
	}
}

// reading turns a view row's value into what the library draws.
func reading(r view.Row) glance.Reading {
	text := r.Value
	if r.Unit != "" {
		text += " " + r.Unit
	}
	return glance.Known(text, status(r.Status))
}

// trailColour is what a plot's line is drawn in: the trail's own status, or
// the theme's disabled colour for a section whose source has stopped
// answering.
//
// **Resolved in the panel's theme, not the application's.** A sparkline is
// given colours rather than a status, so this is the one place the panel
// picks a colour by hand -- and the application's theme is the preferences
// window's, which would put that window's scheme on the panel's plot.
//
// The dim case is the design system's disabled role rather than a status,
// because there is no dim status: fd.Status is a verdict, and "we have not
// heard from this in a minute" is not one. The pane makes the same choice in
// its own vocabulary, and the two shells must not disagree about what a stale
// plot looks like.
func (p *Panel) trailColour(t view.Trail, gone bool) color.Color {
	th := p.win.Panel().Theme()
	variant := fynetheme.VariantDark
	if p.app != nil {
		variant = p.app.Settings().ThemeVariant()
	}
	if gone {
		return th.Color(fynetheme.ColorNameDisabled, variant)
	}
	return th.Color(widgets.StatusColorName(status(t.Status)), variant)
}

// status maps this program's verdict onto the design system's. They are the
// same vocabulary; the view keeps its own so that it and the terminal panel
// stay free of Fyne.
func status(s view.Status) fd.Status {
	switch s {
	case view.Good:
		return fd.StatusGood
	case view.Warn:
		return fd.StatusWarn
	case view.Bad:
		return fd.StatusBad
	default:
		return fd.StatusInfo
	}
}

// Poll runs every source on its own cadence until the context is done, drawing
// each answer on the UI thread.
func (p *Panel) Poll(ctx context.Context) {
	for _, s := range p.opts.Sources {
		go pollOne(ctx, s, p)
	}
	go p.keepCache(ctx)
}

/*
restore decides what each card is built from, and marks the ones drawn from
the cache.

A live reading always wins. The cache fills in only for a source that gave
nothing on the first poll, which is what a sleeping mouse looks like: it
answers nothing about one poll in fourteen, and without this the section is
blank for an interval.

drawn is updated in place, because a section restored from the cache is a
section to draw.
*/
func restore(sources []panel.Source, drawn map[string]bool, cached readings.Cache) map[string]view.Section {
	out := make(map[string]view.Section, len(sources))
	for _, src := range sources {
		key := src.Key()
		if drawn[key] {
			out[key] = section(sources, key)
			continue
		}
		if restored, err := cached.Restore(key); err == nil {
			out[key], drawn[key] = restored, true
		}
	}
	return out
}

// CacheInterval is how often the readings are written out.
//
// Not on every poll: the bandwidth section polls every two seconds and a
// glance panel has no business writing to disk thirty times a minute. Fifteen
// seconds is longer than every interval but that one, so most polls are
// written promptly and the busiest is coalesced.
const CacheInterval = 15 * time.Second

// loadCache reads what the panel last knew. A cache that is not there, cannot
// be read or does not parse is a cold start: the panel works without it.
func loadCache() readings.Cache {
	path, err := readings.Path()
	if err != nil {
		return readings.Cache{}
	}
	return readings.Load(path, time.Now())
}

// seed starts the panel off with what was already on disk, so a section that
// has not reported this run keeps the reading it had.
func (p *Panel) seed(c readings.Cache) {
	p.cacheMu.Lock()
	defer p.cacheMu.Unlock()
	if p.cache == nil {
		p.cache = readings.Cache{}
	}
	for key, e := range c {
		p.cache[key] = e
	}
}

// heard notes that a source has answered this run, which ends the cache
// standing in for it.
func (p *Panel) heard(key string) {
	p.cacheMu.Lock()
	defer p.cacheMu.Unlock()
	if p.live == nil {
		p.live = map[string]bool{}
	}
	p.live[key] = true
}

// lastKnown is the cached reading for a source that has not answered yet.
func (p *Panel) lastKnown(key string) (view.Section, error) {
	p.cacheMu.Lock()
	defer p.cacheMu.Unlock()
	if p.live[key] {
		return view.Section{}, readings.ErrNoEntry
	}
	return p.cache.Restore(key)
}

// remember records a section for the next write.
func (p *Panel) remember(key string, sec view.Section) {
	p.cacheMu.Lock()
	defer p.cacheMu.Unlock()
	if p.cache == nil {
		p.cache = readings.Cache{}
	}
	// Restored is not written back: what goes in the cache is a reading, and
	// whether it is being *drawn* as a last-known one is the next run's
	// question rather than this one's.
	sec.Restored = false
	p.cache[key] = readings.Entry{At: time.Now(), Section: sec}
}

// writeCache puts the readings on disk, coalesced across every section.
//
// A failure is not reported anywhere. It is a cache: the panel drew the
// readings already, and a user who cannot write to their own cache directory
// has a problem this panel is not going to tell them about usefully.
func (p *Panel) writeCache() {
	path, err := readings.Path()
	if err != nil {
		return
	}
	p.cacheMu.Lock()
	snapshot := make(readings.Cache, len(p.cache))
	for k, v := range p.cache {
		snapshot[k] = v
	}
	p.cacheMu.Unlock()

	if len(snapshot) == 0 {
		return
	}
	_ = readings.Save(path, snapshot)
}

// keepCache writes the readings on a timer until the context is done.
func (p *Panel) keepCache(ctx context.Context) {
	t := time.NewTicker(CacheInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			p.writeCache()
			return
		case <-t.C:
			p.writeCache()
		}
	}
}

// pollOne is one source's loop. Each keeps its own interval: a byte counter
// and a thermal probe share a window and nothing else.
func pollOne(ctx context.Context, s panel.Source, p *Panel) {
	t := time.NewTicker(s.Interval())
	defer t.Stop()
	for {
		drawn, err := s.Poll(ctx)
		if err != nil {
			drawn = false
		}
		sec := s.Section()
		if drawn {
			p.remember(s.Key(), sec)
		}
		fyne.Do(func() { p.Draw(s.Key(), sec, drawn) })

		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// first polls every source once and reports which of them answered.
//
// It runs before the window exists, so it blocks: there is nothing to keep
// responsive yet, and a panel that opened before its first reading would
// resize in front of the person who opened it.
func first(ctx context.Context, sources []panel.Source) map[string]bool {
	out := make(map[string]bool, len(sources))
	for _, s := range sources {
		drawn, err := s.Poll(ctx)
		if err != nil {
			drawn = false
		}
		out[s.Key()] = drawn
	}
	return out
}

// section finds a source's current drawing by key.
func section(sources []panel.Source, key string) view.Section {
	for _, s := range sources {
		if s.Key() == key {
			return s.Section()
		}
	}
	return view.Section{}
}

// Start builds the window, starts the polls and runs until it closes.
func Start(o Options) error {
	a := app.NewWithID(AppID)
	Appearance(a, o.Store).Apply(a)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// What the panel last knew, before anything is asked. A section whose
	// first live poll comes back empty is drawn from this instead of blank,
	// and a card is built the size of it -- the card takes its objects at
	// build time, so its shape is decided here or not at all (spec 013).
	cached := loadCache()

	// The first reading before the first frame: a card is built with the
	// pieces its section has, so a section polled after the window is built
	// would have nowhere to put them.
	drawn := first(ctx, o.Sources)

	sections := restore(o.Sources, drawn, cached)

	// The menu needs the panel it changes and the panel needs the menu before
	// its window exists, so the closure is made first and the pointer filled
	// in after. It runs on a tap, long after New has returned.
	var p *Panel
	var open func()
	if o.Store != nil {
		o.Menu, open = MenuWith(a, o.Store, o.Version, func(c config.Config) {
			if p != nil {
				p.Apply(c)
				p.restyle()
			}
		})
	}

	o.Sections = sections
	a.SetIcon(appIcon())

	p = New(a, o)

	// The panel starts out remembering what it already knew. Without this
	// the first write replaces the file with only the sections that reported
	// *this* run, which drops the entry for the one section that did not --
	// the very one the cache exists for.
	p.seed(cached)
	for key, ok := range drawn {
		sec, found := sections[key]
		if !found {
			sec = section(o.Sources, key)
		}
		p.Draw(key, sec, ok)
	}
	if o.Store != nil {
		p.Apply(o.Store.Config())
	}
	if o.Preferences && open != nil {
		open()
	}
	p.Poll(ctx)

	p.win.ShowAndRun()
	return nil
}

/*
Appearance is the scheme, the fonts and the text size the user chose.

The panel used to hard-code BreezeDark and a default face, which meant the
Appearance screen in the preferences window changed the preferences window and
nothing else — a font chooser that had no effect on the thing it was next to.
It is the same file and the design system already keeps the section, reads it
and writes it; all this does is ask.

A store that is not there is a panel started before its settings exist, and
the design system's own defaults are the right answer to that.
*/
func Appearance(a fyne.App, store *config.Store) fdtheme.Appearance {
	if store == nil {
		return fdtheme.DefaultAppearance()
	}
	return fdtheme.LoadAppearanceFrom(store.Settings(), a.Preferences())
}

/*
applyTheme gives the panel its own face and leaves the application's alone.

**The application's belongs to the preferences window, deliberately.** That is
the window with *overlays* — a font chooser, a dropdown, the context menu —
and an overlay is added to the canvas's overlay stack rather than to a window's
content, so nothing can override one. Whatever the application's theme is, an
overlay wears it.

It used to be the other way round, on the reasoning that a card is a canvas
object and a subtree override does not reach one. That was true and it was the
wrong conclusion: it meant the application's theme was the panel's, so opening
the font chooser from the preferences window drew the whole dialog in the
panel's face and the panel's card fade — a see-through list of font names at
eight points. The design system carries the panel's face explicitly now
(fynedesygn spec 042), so the panel needs nothing from the application.

The card fade goes here, on the panel's own theme, for the same reason: it is
the cards' and has no business anywhere else.
*/
func (p *Panel) applyTheme(c config.Config) {
	if p.app == nil {
		return
	}
	a := c.PanelAppearance(Appearance(p.app, p.opts.Store))
	p.win.Panel().SetTheme(withCardOpacity(a.Theme(), c.OpacityOrDefault()))
}

/*
restyle repaints everything in the panel in the current theme.

The design system's card restyles its title and its rows, and it cannot do
more: a meter and a sparkline are added to it through AddObject, which takes a
plain canvas object, so the card has no way to know they have a Restyle of
their own. This program added them and holds them, so this is where they are
told.

The symptom when they are not is oddly specific and was reported as one: a
change of text size takes effect everywhere in the panel *except* the usage
section, because that is the only section whose readings are meters rather
than rows.
*/
func (p *Panel) restyle() {
	for _, c := range p.cards {
		for _, m := range c.meters {
			m.Restyle()
		}
		if c.grid != nil {
			c.grid.Restyle()
		}
		if c.spark != nil {
			c.spark.Refresh()
		}
	}
	// The panel last, so it measures the cards after they have been resized
	// by whatever the new face and size are.
	p.win.Panel().Restyle()
}
