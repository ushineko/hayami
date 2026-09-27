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
}

// Panel is the window and the cards in it.
type Panel struct {
	win   *glance.Window
	cards map[string]*card
	opts  Options
}

// card is one section's card and the rows in it, kept so a poll repaints
// rather than rebuilding. A glance window is redrawn several times a minute
// and rebuilding at that rate would fight the no-reflow rule.
type card struct {
	card *glance.Card
	rows []*glance.Row
}

// New builds the window with a card per source. Cards are not drawn until
// their source answers, so the window does not flash empty on the way up.
func New(a fyne.App, o Options) *Panel {
	if o.Title == "" {
		o.Title = "hayami"
	}
	p := &Panel{
		win: glance.NewWindow(a, glance.Options{
			Title: o.Title,
			OnTop: true,
		}),
		cards: map[string]*card{},
		opts:  o,
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
		p.cards[s.Key()] = holder
		p.win.Panel().Add(c)
	}
	return p
}

// Window is the glance window, for a caller that wants the menu or the icon.
func (p *Panel) Window() *glance.Window { return p.win }

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
		return
	}
	for i, r := range rows {
		c.rows[i].SetLabel(r.Label)
		c.rows[i].Set(reading(r))
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

// Start builds the window, starts the polls and runs until it closes.
func Start(o Options) error {
	a := app.NewWithID(AppID)
	a.Settings().SetTheme(fdtheme.New(fdtheme.BreezeDark, fdtheme.Options{}))

	p := New(a, o)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	p.Poll(ctx)

	p.win.ShowAndRun()
	return nil
}
