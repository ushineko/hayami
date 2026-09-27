package view_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/hayami/internal/view"
)

// The scale is the series' own, so a climb fills the line whatever its units.
// The question a sparkline answers is "is this going up", not "how does this
// compare with that".
func TestARisingSeriesClimbsAcrossTheLine(t *testing.T) {
	got := view.Sparkline([]float64{0, 1, 2, 3, 4, 5, 6, 7}, 8, 0)

	require.Equal(t, 8, len([]rune(got)))
	assert.Equal(t, string(view.SparkRunes), got)
}

// A series that has not moved would otherwise be amplified into eight heights
// of noise, and a reader would take a hundredth of a degree for a trend.
func TestAFlatSeriesIsDrawnFlatAndNotAsNoise(t *testing.T) {
	got := view.Sparkline([]float64{46.0, 46.001, 46.0, 45.999}, 8, 1.0)

	runes := []rune(got)
	require.NotEmpty(t, runes)
	for _, r := range runes {
		assert.Equal(t, runes[0], r, "a steady series was drawn as movement: %q", got)
	}
	assert.NotEqual(t, view.SparkRunes[0], runes[0],
		"a flat series should sit in the middle; on the floor it reads as nothing at all")
}

// A plot wider than the pane takes the newest samples, because the right edge
// is now.
func TestALongSeriesKeepsItsNewestSamples(t *testing.T) {
	got := view.Sparkline([]float64{9, 9, 9, 0, 7}, 2, 0)

	assert.Equal(t, 2, len([]rune(got)))
	assert.Equal(t, string([]rune{view.SparkRunes[0], view.SparkRunes[len(view.SparkRunes)-1]}), got,
		"the last two samples are 0 and 7: a floor then a ceiling")
}

func TestAnEmptySeriesDrawsNothing(t *testing.T) {
	assert.Empty(t, view.Sparkline(nil, 20, 1))
	assert.Empty(t, view.Sparkline([]float64{1, 2}, 0, 1))
}

// A restart starts the series again. A panel that drew a line through one
// point would be inventing a trend.
func TestASeriesHoldsItsCapacityAndDropsTheOldest(t *testing.T) {
	s := view.NewSeries(3)
	for _, v := range []float64{1, 2, 3, 4, 5} {
		s.Add(v)
	}

	assert.Equal(t, []float64{3, 4, 5}, s.Samples())
	assert.Equal(t, 3, s.Len())
}

// The plot belongs under the rows it is about, in the arrangements that have
// room for it.
func TestASectionsTrailIsPlottedUnderIt(t *testing.T) {
	s := view.Section{Key: "cooler", Title: "Cooler",
		Rows:  []view.Row{{Label: "Coolant", Value: " 46.0", Unit: "°C"}},
		Trail: []float64{40, 42, 44, 46},
	}

	lines := view.Render([]view.Section{s}, view.ArrangeStack, 40)

	joined := strings.Join(lines, "\n")
	assert.Contains(t, joined, string(view.SparkRunes[len(view.SparkRunes)-1]),
		"the newest and highest sample should reach the top of the plot")
}
