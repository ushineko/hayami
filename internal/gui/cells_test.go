package gui_test

import (
	"context"
	"testing"
	"time"

	"fyne.io/fyne/v2/test"
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

	assert.Equal(t, 1, gui.ShownCells(p, "peripherals"),
		"a card built with no cells never got a grid to put one in")
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
	require.Len(t, got["peripherals"].Cells, 1)
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

	require.Len(t, got["peripherals"].Cells, 1)
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
	require.Len(t, held["peripherals"].Section.Cells, 1)
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

	// A pane's shape is not a window's: row stacks rather than doing
	// something arbitrary.
	p.Apply(config.Config{Arrangement: "row"})
	assert.Equal(t, glance.Stack, p.Window().Panel().Arrangement())
}
