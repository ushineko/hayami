package view_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/ushineko/hayami/internal/view"
)

// The rule the whole panel rests on: a number that changes magnitude must not
// change the width of its column, or the label beside it moves while someone
// is reading it.
func TestARateCrossingAMagnitudeKeepsItsWidth(t *testing.T) {
	unit := view.UnitWidth("B/s", "KiB/s", "MiB/s", "GiB/s", "TiB/s")

	var widths []int
	for _, bps := range []float64{0, 999, 1024, 999.9 * 1024, 1024 * 1024, 3.9 * 1024 * 1024 * 1024} {
		n, u := view.Rate(bps)
		widths = append(widths, len([]rune(n+" "+view.PadUnit(u, unit))))
	}

	for i, w := range widths {
		assert.Equal(t, widths[0], w, "the column moved at reading %d", i)
	}
}

// A value that has not arrived is the same width as one that has, so a row
// does not resize when its first reading lands.
func TestABlankIsTheWidthOfAValue(t *testing.T) {
	n, _ := view.Rate(1024)
	blank, _ := view.NoRate()

	assert.Equal(t, len([]rune(n)), len([]rune(blank)))
}

// The same rule in the rendered line: two magnitudes, one column.
func TestTheValueColumnDoesNotMoveBetweenMagnitudes(t *testing.T) {
	unit := view.UnitWidth("KiB/s", "MiB/s")
	line := func(bps float64) string {
		n, u := view.Rate(bps)
		s := view.Section{Title: "Bandwidth", Rows: []view.Row{
			{Label: "eno2 down", Value: n, Unit: view.PadUnit(u, unit)},
		}}
		return view.Render([]view.Section{s}, view.ArrangeRow, 60)[0]
	}

	kib := line(999.9 * 1024)
	mib := line(1024 * 1024)

	// The number is right-aligned in a fixed field, so what has to match is
	// where the field and its unit sit, not where the first digit falls.
	assert.Equal(t, len([]rune(kib)), len([]rune(mib)))
	assert.Equal(t, strings.Index(kib, "/s"), strings.Index(mib, "/s"),
		"the unit moved when the magnitude changed")
	assert.Equal(t, strings.Index(kib, "eno2"), strings.Index(mib, "eno2"),
		"the label moved when the magnitude changed")
}

// Below a kibibyte the value is a count of bytes: a decimal point on it is
// noise, and the width is held by the padding rather than by the decimal.
func TestBytesPerSecondHasNoDecimal(t *testing.T) {
	n, u := view.Rate(512)

	assert.Equal(t, "512", strings.TrimSpace(n))
	assert.Equal(t, "B/s", u)
}
