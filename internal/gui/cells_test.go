package gui_test

import (
	"context"
	"testing"
	"time"

	"fyne.io/fyne/v2/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/hayami/internal/gui"
	"github.com/ushineko/hayami/internal/panel"
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
