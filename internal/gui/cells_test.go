package gui_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2/test"
	fynetheme "fyne.io/fyne/v2/theme"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/fynedesygn/glance"

	"github.com/ushineko/hayami/internal/config"
	"github.com/ushineko/hayami/internal/gui"
	"github.com/ushineko/hayami/internal/panel"
	"github.com/ushineko/hayami/internal/readings"
	"github.com/ushineko/hayami/internal/view"
)

// cellSource is a section of cells whose reading the test sets.
type cellSource struct{ reading view.PeripheralsReading }

func (c *cellSource) Key() string                        { return "peripherals" }
func (c *cellSource) Title() string                      { return "Peripherals" }
func (c *cellSource) Interval() time.Duration            { return time.Hour }
func (c *cellSource) Poll(context.Context) (bool, error) { return len(c.reading.Devices) > 0, nil }
func (c *cellSource) Section() view.Section              { return view.Peripherals(c.reading) }
func (c *cellSource) Data() any                          { return c.reading }

/*
AC. A card built from a section with no cells can still draw one later.

The card is built once, before the window exists, from whatever the first poll
found -- and the library takes its objects at build time. A grid built only
when there were already cells left the peripherals card empty for the life of
the program whenever that first poll came back empty, which a wireless mouse
that has been still does about one poll in fourteen.
*/
func TestACardBuiltEmptyStillDrawsACellLater(t *testing.T) {
	a := test.NewTempApp(t)
	src := &cellSource{}

	p := gui.New(a, gui.Options{Sources: []panel.Source{src}, Title: "hayami"})
	require.NotNil(t, p)

	// The device wakes up and answers.
	src.reading = view.PeripheralsReading{Devices: []view.PeripheralReading{
		{Name: "G502 X PLUS", Level: 76, Kind: view.KindMouse},
	}}
	p.Draw("peripherals", src.Section(), true)

	assert.Equal(t, view.PeripheralSlots, gui.ShownCells(p, "peripherals"),
		"a card built with no cells never got a grid to put one in")
}

/*
AC (spec 022). The card shows two cells with the headset quiet and with no
second device at all, and keeps its shape through both.

The headset switched off and on is the transition that happens on the desk,
and the card is the same size either way: the cell is the same cell, dimmed.
Against the "no device" placeholder the card is the same height -- two cells
side by side or one above the other, as before -- but not always the same
width: the design system sizes a cell to its name up to a budget, and "no
device" is shorter than "Arctis Nova Pro Wireless". That is logged in the
spec's gaps; the window is as wide as its widest card, and this is not it.
*/
func TestTheCardKeepsItsShapeWithTheHeadsetQuietOrAbsent(t *testing.T) {
	a := test.NewTempApp(t)
	at := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	mouse := view.PeripheralReading{Name: "G502 X PLUS", Level: 76, Kind: view.KindMouse, Since: at, Seen: at}
	headset := view.PeripheralReading{
		Name: "Arctis Nova Pro Wireless", Level: 47, Kind: view.KindHeadset, Since: at, Seen: at,
	}
	src := &cellSource{reading: view.PeripheralsReading{Devices: []view.PeripheralReading{mouse, headset}}}
	p := gui.New(a, gui.Options{Sources: []panel.Source{src}, Title: "hayami"})

	p.Draw("peripherals", src.Section(), true)
	require.Equal(t, 2, gui.ShownCells(p, "peripherals"))
	live := gui.CardMinSize(p, "peripherals")

	headset.Stale = true
	src.reading = view.PeripheralsReading{Devices: []view.PeripheralReading{mouse, headset}}
	p.Draw("peripherals", src.Section(), true)
	require.Equal(t, 2, gui.ShownCells(p, "peripherals"), "the quiet headset's cell was hidden")
	assert.True(t, gui.CellStale(p, "peripherals", 1), "the quiet headset was not drawn dim")
	quiet := gui.CardMinSize(p, "peripherals")
	assert.Equal(t, live, quiet, "the card changed size when the headset was switched off")

	src.reading = view.PeripheralsReading{Devices: []view.PeripheralReading{mouse}}
	p.Draw("peripherals", src.Section(), true)
	require.Equal(t, 2, gui.ShownCells(p, "peripherals"), "the empty slot was hidden")
	assert.True(t, gui.CellStale(p, "peripherals", 1), "the placeholder was not drawn dim")
	absent := gui.CardMinSize(p, "peripherals")
	assert.InDelta(t, quiet.Height, absent.Height, 0.01,
		"the card changed height when the second slot emptied")
}

// AC1. A source that gave nothing on the first poll is drawn from the cache,
// which is what a mouse asleep at startup looks like.
func TestASilentSourceIsRestoredFromTheCache(t *testing.T) {
	src := &cellSource{}
	cached := readings.Cache{"peripherals": {At: time.Now(),
		Section: view.Peripherals(view.PeripheralsReading{
			Devices: []view.PeripheralReading{{Name: "G502 X PLUS", Level: 76, Kind: view.KindMouse}},
		})}}

	drawn := map[string]bool{"peripherals": false}
	got := gui.Restore([]panel.Source{src}, drawn, cached)

	require.Contains(t, got, "peripherals")
	assert.True(t, drawn["peripherals"], "a restored section is a section to draw")
	assert.True(t, got["peripherals"].Restored)
	require.Len(t, got["peripherals"].Cells, view.PeripheralSlots)
	assert.Equal(t, "G502 X PLUS", got["peripherals"].Cells[0].Label)
}

// AC2. A source that answered ignores its cached entry entirely.
func TestALiveReadingBeatsTheCache(t *testing.T) {
	src := &cellSource{reading: view.PeripheralsReading{
		Devices: []view.PeripheralReading{{Name: "Live Mouse", Level: 50, Kind: view.KindMouse}},
	}}
	cached := readings.Cache{"peripherals": {At: time.Now(),
		Section: view.Peripherals(view.PeripheralsReading{
			Devices: []view.PeripheralReading{{Name: "Stale Mouse", Level: 76, Kind: view.KindMouse}},
		})}}

	got := gui.Restore([]panel.Source{src}, map[string]bool{"peripherals": true}, cached)

	require.Len(t, got["peripherals"].Cells, view.PeripheralSlots)
	assert.Equal(t, "Live Mouse", got["peripherals"].Cells[0].Label)
	assert.False(t, got["peripherals"].Restored, "a live reading was marked restored")
}

// AC1. A cold start with no cache leaves the section to its source, and does
// not invent one.
func TestNoCacheLeavesTheSectionAlone(t *testing.T) {
	src := &cellSource{}
	drawn := map[string]bool{"peripherals": false}

	got := gui.Restore([]panel.Source{src}, drawn, readings.Cache{})

	assert.NotContains(t, got, "peripherals")
	assert.False(t, drawn["peripherals"])
}

// AC1, AC3. A restored section's cells are dim, not just its rows.
//
// The card dims its own rows when it is told it is stale; a cell is a
// separate object holding its own reading, so without this a restored
// section drew a live-looking percentage.
func TestARestoredSectionsCellsAreDim(t *testing.T) {
	a := test.NewTempApp(t)
	src := &cellSource{reading: view.PeripheralsReading{
		Devices: []view.PeripheralReading{{Name: "G502 X PLUS", Level: 76, Kind: view.KindMouse}},
	}}
	p := gui.New(a, gui.Options{Sources: []panel.Source{src}, Title: "hayami"})

	sec := src.Section()
	sec.Restored = true
	p.Draw("peripherals", sec, true)

	assert.True(t, gui.CellStale(p, "peripherals", 0),
		"a restored section drew a cell as though it were current")
}

// AC7. A section that never reports keeps the reading it had.
//
// The write replaces the whole file with what the panel is holding, so a
// panel that started empty dropped the entry for the one section that never
// answered -- the very section the cache exists for. Found in a photograph:
// the card vanished instead of being restored.
func TestASectionThatNeverReportsKeepsItsCachedReading(t *testing.T) {
	a := test.NewTempApp(t)
	src := &cellSource{}
	p := gui.New(a, gui.Options{Sources: []panel.Source{src}, Title: "hayami"})

	was := readings.Cache{"peripherals": {At: time.Now(),
		Section: view.Peripherals(view.PeripheralsReading{
			Devices: []view.PeripheralReading{{Name: "G502 X PLUS", Level: 76, Kind: view.KindMouse}},
		})}}
	gui.Seed(p, was)

	// Another section reports; the peripherals stay silent all run.
	p.Draw("cooler", view.Section{Key: "cooler", Title: "Cooler"}, true)

	held := gui.Cached(p)
	require.Contains(t, held, "peripherals",
		"the silent section's cached reading was dropped, so the next start has nothing")
	require.Len(t, held["peripherals"].Section.Cells, view.PeripheralSlots)
	assert.Equal(t, "G502 X PLUS", held["peripherals"].Section.Cells[0].Label)
}

// AC6. A restored section is dim but not called unavailable; a section whose
// source has stopped is both.
func TestOnlyAStoppedSourceIsCalledUnavailable(t *testing.T) {
	a := test.NewTempApp(t)
	src := &cellSource{reading: view.PeripheralsReading{
		Devices: []view.PeripheralReading{{Name: "G502 X PLUS", Level: 76, Kind: view.KindMouse}},
	}}
	p := gui.New(a, gui.Options{Sources: []panel.Source{src}, Title: "hayami"})

	restored := src.Section()
	restored.Restored = true
	p.Draw("peripherals", restored, true)
	assert.False(t, gui.CardMarked(p, "peripherals"),
		"a panel that had only just started called its hardware unavailable")

	gone := src.Section()
	gone.Gone = true
	p.Draw("peripherals", gone, true)
	assert.True(t, gui.CardMarked(p, "peripherals"),
		"a source that stopped answering said nothing about it")
}

// AC. The window honours the grid setting, which it used to ignore entirely.
func TestTheWindowFollowsTheArrangementSetting(t *testing.T) {
	a := test.NewTempApp(t)
	src := &cellSource{}
	p := gui.New(a, gui.Options{Sources: []panel.Source{src}, Title: "hayami"})

	p.Apply(config.Config{Arrangement: "grid"})
	assert.Equal(t, glance.Grid, p.Window().Panel().Arrangement())

	p.Apply(config.Config{Arrangement: "stack"})
	assert.Equal(t, glance.Stack, p.Window().Panel().Arrangement())

	// Row is one line per reading, drawn by the design system's Lines
	// (spec 049). It used to stack, for want of one.
	p.Apply(config.Config{Arrangement: "row"})
	assert.Equal(t, glance.Lines, p.Window().Panel().Arrangement())
}

/*
Spec 049. In row a reading is one line in the window as in the terminal: the
detail lines a stacked card draws under a reading (a Wi-Fi interface's
totals and link) go, and come back when the window leaves row. The panel is
re-measured once for the switch, by the arrangement.
*/
func TestRowLeavesOutDetailLinesAndStackBringsThemBack(t *testing.T) {
	a := test.NewTempApp(t)
	sec := view.Bandwidth([]view.BandwidthReading{{
		Name: "Wi-Fi", RxRate: 6400, TxRate: 46000, RxTotal: 33 << 30, TxTotal: 187 << 20,
		HasRate: true, HasTotal: true,
	}})
	src := &fixed{sec}
	p := gui.New(a, gui.Options{Sources: []panel.Source{src}, Title: "hayami"})
	p.Draw(sec.Key, sec, true)

	total := sec.Rows[0].DetailLines()[0]
	text := func() string { return strings.Join(gui.CardText(p, sec.Key), " ") }
	require.Contains(t, text(), strings.Fields(total)[0], "a stacked card did not draw the totals line")

	arrange := func(a string) {
		c := config.Default()
		c.Arrangement = a
		p.Apply(c)
	}
	arrange("row")
	require.NotEmpty(t, text(), "the card is hidden, which proves nothing about its lines")
	assert.NotContains(t, text(), strings.Fields(total)[0], "row drew a reading's detail line")

	arrange("stack")
	assert.Contains(t, text(), strings.Fields(total)[0], "the detail line did not come back on leaving row")
}

/*
AC (spec 025). The window sets a bar under a level cell and clears it under a
band cell, and the card is the same height with bars and without.

The height is the point of the library's reserved row: a device that gains or
loses a level must not reflow the card.
*/
func TestTheWindowBarsALevelAndNotABand(t *testing.T) {
	a := test.NewTempApp(t)
	at := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	mouse := view.PeripheralReading{Name: "G502 X PLUS", Level: 40, Kind: view.KindMouse, Since: at, Seen: at}
	band := view.PeripheralReading{Name: "K800", Band: "Good", Segments: 3, Kind: view.KindKeyboard,
		Since: at, Seen: at}
	src := &cellSource{reading: view.PeripheralsReading{Devices: []view.PeripheralReading{mouse, band}}}
	p := gui.New(a, gui.Options{Sources: []panel.Source{src}, Title: "hayami"})

	p.Draw("peripherals", src.Section(), true)
	require.Equal(t, 2, gui.ShownCells(p, "peripherals"))
	withBar := gui.CardMinSize(p, "peripherals")

	fraction, fill, ok := gui.CellBar(p, "peripherals", 0, 200)
	require.True(t, ok, "the level cell drew no bar")
	assert.InDelta(t, 0.40, fraction, 0.01, "the bar is not the level")
	assert.Equal(t, gui.PanelTheme(p).Color(fynetheme.ColorNameWarning, a.Settings().ThemeVariant()), fill,
		"the bar is not in the cell's status")
	_, _, ok = gui.CellBar(p, "peripherals", 1, 200)
	assert.False(t, ok, "the band cell drew a bar under its segments")

	// The mouse loses its level: two band cells, no bar anywhere.
	mouse = view.PeripheralReading{Name: "G502 X PLUS", Band: "Low", Segments: 2, Kind: view.KindMouse,
		Since: at, Seen: at}
	src.reading = view.PeripheralsReading{Devices: []view.PeripheralReading{mouse, band}}
	p.Draw("peripherals", src.Section(), true)
	_, _, ok = gui.CellBar(p, "peripherals", 0, 200)
	assert.False(t, ok, "a cell that lost its level kept its bar")
	assert.Equal(t, withBar.Height, gui.CardMinSize(p, "peripherals").Height,
		"the card changed height when the bars went")
}
