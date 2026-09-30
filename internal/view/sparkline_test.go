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
		Rows:   []view.Row{{Label: "Coolant", Value: " 46.0", Unit: "°C"}},
		Trails: []view.Trail{{Name: "Coolant", Samples: []float64{40, 42, 44, 46}}},
	}

	lines := view.Render([]view.Section{s}, view.ArrangeStack, 40)

	joined := strings.Join(lines, "\n")
	assert.Contains(t, joined, string(view.SparkRunes[len(view.SparkRunes)-1]),
		"the newest and highest sample should reach the top of the plot")
}

// Two series on one section are two lines, and in the arrangement that names
// them they are named for themselves rather than twice for the section. The
// cooler is the case: "Cooler" against "Cooler" is two plots a reader cannot
// tell apart, and which one is the coolant is the whole question.
func TestTwoTrailsAreTwoLinesAndAreNamedApart(t *testing.T) {
	s := view.Section{Key: "cooler", Title: "Cooler",
		Rows: []view.Row{{Label: "Coolant", Value: " 46.0", Unit: "°C"}},
		Trails: []view.Trail{
			{Name: "Coolant", Samples: []float64{40, 42, 44, 46}},
			{Name: "CPU", Samples: []float64{60, 70, 80, 90}},
		},
	}

	stacked := view.Render([]view.Section{s}, view.ArrangeStack, 40)
	assert.Len(t, stacked, 4, "a title, a row and a line per trail")

	rows := strings.Join(view.Render([]view.Section{s}, view.ArrangeRow, 60), "\n")
	assert.Contains(t, rows, "Coolant")
	assert.Contains(t, rows, "CPU")
}

// Each series is scaled to its own range. The processor swings thirty-five
// degrees where the coolant moves under one, so a shared axis flattens the
// coolant to nothing -- which is the signal the plot exists for.
func TestEachTrailIsScaledToItsOwnRange(t *testing.T) {
	coolant := []float64{45.0, 45.4, 45.8}
	cpu := []float64{60, 80, 100}

	assert.Equal(t,
		view.Sparkline(coolant, 3, 0.1),
		view.Sparkline(cpu, 3, 0.1),
		"two series with the same shape should draw the same line whatever their units")
}

// A trailing mean starts on the first sample rather than after a minute of
// blank plot, and it flattens a spike rather than following it.
func TestAnAveragedSeriesStartsAtOnceAndFlattensASpike(t *testing.T) {
	a := view.NewAveraged(10, 4)

	a.Add(50)
	require.Equal(t, []float64{50}, a.Mean(), "a partial window is averaged as it stands")

	for _, v := range []float64{50, 50, 100} {
		a.Add(v)
	}
	means := a.Mean()
	require.Len(t, means, 4)
	assert.InDelta(t, 62.5, means[3], 0.001, "the spike is a quarter of the window, not all of it")
	assert.Less(t, means[3], 100.0)
}

// The window is the mean's, not the plot's. A mean over four samples kept for
// ten leaves ten points on the line, each covering the four before it.
func TestAnAveragedSeriesKeepsItsCapacityOfMeans(t *testing.T) {
	a := view.NewAveraged(3, 2)
	for _, v := range []float64{1, 2, 3, 4, 5} {
		a.Add(v)
	}

	assert.Equal(t, 3, a.Len())
	assert.Equal(t, []float64{2.5, 3.5, 4.5}, a.Mean())
}

// Against a range it is given, a series is drawn by its size, not its shape.
// A quiet interface beside a busy one is a flat line along the floor, which is
// the reading the bandwidth trend exists for (spec 021).
func TestASharedRangeDrawsAQuietSeriesFlatBesideABusyOne(t *testing.T) {
	quiet := []float64{0, 1, 0, 1}
	busy := []float64{0, 50, 100, 50}

	q := []rune(view.SparklineIn(quiet, 4, 0, 100))
	b := []rune(view.SparklineIn(busy, 4, 0, 100))

	require.Len(t, q, 4)
	for _, r := range q {
		assert.Equal(t, view.SparkRunes[0], r, "a 1-peak series under a 100 scale is the floor: %q", string(q))
	}
	assert.Equal(t, view.SparkRunes[len(view.SparkRunes)-1], b[2], "the 100 peak reaches the top: %q", string(b))

	// And on its own range the same quiet series fills the line, which is
	// what the shared range is there to prevent.
	own := []rune(view.Sparkline(quiet, 4, 0))
	assert.Equal(t, view.SparkRunes[len(view.SparkRunes)-1], own[1])
}

// Sparkline is SparklineIn over the series' own range, widened to the minimum
// span, and draws exactly what it drew before SparklineIn existed.
func TestSparklineIsItsOwnRangeDrawnThroughSparklineIn(t *testing.T) {
	series := []float64{3, 5, 4, 9}

	assert.Equal(t, view.SparklineIn(series, 4, 3, 6), view.Sparkline(series, 4, 1))
	assert.Equal(t, view.SparklineIn([]float64{46, 46}, 2, 45.5, 1), view.Sparkline([]float64{46, 46}, 2, 1))
}

func TestSparklineInWithNoRangeDrawsNothing(t *testing.T) {
	assert.Empty(t, view.SparklineIn([]float64{1, 2}, 4, 0, 0))
	assert.Empty(t, view.SparklineIn(nil, 4, 0, 1))
}

// Under ScaleShared the pane draws every trail of the section against zero
// and the section's greatest sample: the quieter interface is the flatter line
// in both arrangements that draw a plot.
func TestASharedScaleSectionDrawsItsTrailsAgainstOneRange(t *testing.T) {
	s := view.Section{Key: "bandwidth", Title: "Bandwidth", TrailScale: view.ScaleShared,
		Rows: []view.Row{{Label: "eno2", Value: "x"}},
		Trails: []view.Trail{
			{Name: "eno2 ↓", Samples: []float64{0, 1, 0, 1}},
			{Name: "wlan0 ↓", Samples: []float64{0, 50, 100, 50}, Series: 1},
		},
	}
	floor := strings.Repeat(string(view.SparkRunes[0]), 4)
	top := string(view.SparkRunes[len(view.SparkRunes)-1])

	stacked := view.Render([]view.Section{s}, view.ArrangeStack, 4)
	require.Len(t, stacked, 4, "a title, a row and a line per trail")
	assert.Equal(t, floor, stacked[2])
	assert.Contains(t, stacked[3], top)

	rows := view.Render([]view.Section{s}, view.ArrangeRow, 12)
	require.Len(t, rows, 3)
	assert.True(t, strings.HasSuffix(rows[1], floor), "the quiet trail is flat: %q", rows[1])
	assert.Contains(t, rows[2], top)

	// The same section under its own ranges draws the quiet trail as
	// movement, which is the difference the scale makes.
	s.TrailScale = view.ScaleEach
	each := view.Render([]view.Section{s}, view.ArrangeStack, 4)
	assert.NotEqual(t, floor, each[2])
}

// The shared range is measured on what the line shows: a peak that has
// scrolled off the left edge does not flatten what is still on it.
func TestASharedRangeIgnoresAPeakTheLineNoLongerShows(t *testing.T) {
	s := view.Section{Title: "Bandwidth", TrailScale: view.ScaleShared,
		Trails: []view.Trail{{Name: "eno2 ↓", Samples: []float64{1000, 0, 10}}},
	}

	lines := view.Render([]view.Section{s}, view.ArrangeStack, 2)

	require.Len(t, lines, 2)
	assert.Equal(t, string([]rune{view.SparkRunes[0], view.SparkRunes[len(view.SparkRunes)-1]}), lines[1])
}
