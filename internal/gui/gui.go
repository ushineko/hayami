/*
Package gui is the desktop panel: a glance window from fynedesygn, drawing the
same sections the terminal does.

The window arranges; it does not decide what a section says. Everything here
turns a view.Section into cards and rows, and nothing here reads a sensor.
*/
package gui

import (
	"context"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"

	fd "github.com/ushineko/fynedesygn"
	"github.com/ushineko/fynedesygn/glance"
	fdtheme "github.com/ushineko/fynedesygn/theme"

	"github.com/ushineko/hayami/internal/config"
	"github.com/ushineko/hayami/internal/panel"
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
}

// card is one section's card and the pieces in it, kept so a poll repaints
// rather than rebuilding. A glance window is redrawn several times a minute
// and rebuilding at that rate would fight the no-reflow rule.
type card struct {
	card   *glance.Card
	rows   []*glance.Row
	meters []*glance.Meter
}

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
		}),
		cards: map[string]*card{},
		opts:  o,
		app:   a,
	}

	for _, s := range o.Sources {
		sec := s.Section()
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
			c.AddObject(meter.Object())
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
	p.applyOpacity(c)
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
	c.card.SetAvailable(drawn)
	if !drawn {
		return
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
	a.Settings().SetTheme(fdtheme.New(fdtheme.BreezeDark, fdtheme.Options{}))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// The first reading before the first frame: a card is built with the
	// pieces its section has, so a section polled after the window is built
	// would have nowhere to put them.
	drawn := first(ctx, o.Sources)

	// The menu needs the panel it changes and the panel needs the menu before
	// its window exists, so the closure is made first and the pointer filled
	// in after. It runs on a tap, long after New has returned.
	var p *Panel
	var open func()
	if o.Store != nil {
		o.Menu, open = MenuWith(a, o.Store, o.Version, func(c config.Config) {
			if p != nil {
				p.Apply(c)
			}
		})
	}

	p = New(a, o)
	for key, ok := range drawn {
		p.Draw(key, section(o.Sources, key), ok)
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

// applyOpacity fades the cards to the setting's value.
//
// It re-wraps the app's theme rather than keeping one of its own, so the card
// opacity composes with whatever the user chose in Appearance: the scheme
// decides the colour and this decides how much of it survives.
//
// glance wraps the theme again when it shows the window, to make the window's
// own background transparent. The two compose in either order — one names the
// background and the other names the card — which is why this can be applied
// whenever the setting changes and not only before the window exists.
func (p *Panel) applyOpacity(c config.Config) {
	if p.app == nil {
		return
	}
	base := baseTheme(p.app.Settings().Theme())
	p.app.Settings().SetTheme(withCardOpacity(base, c.OpacityOrDefault()))
}

// baseTheme unwraps a theme this package has already faded, so applying a new
// opacity does not fade an already-faded card a second time.
func baseTheme(t fyne.Theme) fyne.Theme {
	if faded, ok := t.(cardOpacity); ok {
		return faded.Theme
	}
	return t
}
