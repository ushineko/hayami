package view_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/hayami/internal/view"
)

func section(key, title string, rows ...view.Row) view.Section {
	return view.Section{Key: key, Title: title, Rows: rows}
}

func row(label, value, unit string) view.Row {
	return view.Row{Label: label, Value: value, Unit: unit}
}

// The same section, three ways. This is the spec's load-bearing claim — that
// an arrangement is a property of the view and not a mode of the program — and
// if it is ever untrue it will be untrue here first.
func TestOneSectionRendersInAllThreeArrangements(t *testing.T) {
	s := section("bandwidth", "Bandwidth", row("eno2 down", "317.1", "KiB/s"))

	for _, a := range []view.Arrangement{view.ArrangeStack, view.ArrangeGrid, view.ArrangeRow} {
		lines := view.Render([]view.Section{s}, a, 40)

		require.NotEmpty(t, lines, "%s drew nothing", a)
		assert.Contains(t, strings.Join(lines, "\n"), "317.1 KiB/s",
			"%s lost the reading", a)
	}
}

// Row stretches. The pane's width is the panel's width; a line that measured
// itself would leave a ragged edge a person reads as a bug.
func TestARowStretchesToThePanesWidth(t *testing.T) {
	s := section("bandwidth", "Bandwidth", row("eno2 down", "317.1", "KiB/s"))

	narrow := view.Render([]view.Section{s}, view.ArrangeRow, 40)
	wide := view.Render([]view.Section{s}, view.ArrangeRow, 120)

	require.Len(t, narrow, 1)
	require.Len(t, wide, 1)
	assert.Equal(t, 40, len([]rune(narrow[0])))
	assert.Equal(t, 120, len([]rune(wide[0])))
	assert.True(t, strings.HasSuffix(narrow[0], "317.1 KiB/s"))
	assert.True(t, strings.HasSuffix(wide[0], "317.1 KiB/s"),
		"the value left the right edge when the pane grew")
}

// A pane of three lines has no room for a heading on each one. A row that is
// already labelled keeps its own label and nothing else; the section's name is
// spent only where a row has none.
func TestARowDoesNotRepeatTheSectionsName(t *testing.T) {
	s := section("bandwidth", "Bandwidth", row("eno2 down", "317.1", "KiB/s"))

	lines := view.Render([]view.Section{s}, view.ArrangeRow, 60)

	assert.True(t, strings.HasPrefix(lines[0], "eno2 down"),
		"the section's name was repeated on a row that is already labelled: %q", lines[0])
}

func TestARowWithNoLabelTakesTheSectionsName(t *testing.T) {
	s := section("bandwidth", "Bandwidth", row("", "317.1", "KiB/s"))

	lines := view.Render([]view.Section{s}, view.ArrangeRow, 60)

	assert.True(t, strings.HasPrefix(lines[0], "Bandwidth"))
}

// Grid reflows and drops nothing. The second half of that is the one that
// matters: a layout that loses a section when the window narrows loses the
// reading the person was watching.
func TestTheGridReflowsWithTheWidthAndDropsNothing(t *testing.T) {
	sections := []view.Section{
		section("a", "Alpha", row("one", "1", "")),
		section("b", "Beta", row("two", "2", "")),
		section("c", "Gamma", row("three", "3", "")),
		section("d", "Delta", row("four", "4", "")),
	}

	wide := strings.Join(view.Render(sections, view.ArrangeGrid, 120), "\n")
	narrow := strings.Join(view.Render(sections, view.ArrangeGrid, 60), "\n")

	for _, title := range []string{"Alpha", "Beta", "Gamma", "Delta"} {
		assert.Contains(t, wide, title)
		assert.Contains(t, narrow, title)
	}
	assert.NotEqual(t, wide, narrow, "the grid ignored the width")
	assert.Less(t, len(strings.Split(wide, "\n")), len(strings.Split(narrow, "\n")),
		"a wider grid should be a shorter one")
}

// One column is a stack. A grid that drew a single narrow column would be a
// stack with worse spacing and a different code path to maintain.
func TestAGridTooNarrowForTwoColumnsIsAStack(t *testing.T) {
	sections := []view.Section{
		section("a", "Alpha", row("one", "1", "")),
		section("b", "Beta", row("two", "2", "")),
	}

	grid := view.Render(sections, view.ArrangeGrid, view.MinColumnWidth+1)
	stack := view.Render(sections, view.ArrangeStack, view.MinColumnWidth+1)

	assert.Equal(t, stack, grid)
}

// A name the user typed wrongly is told about, not silently ignored.
func TestAnUnknownArrangementIsRefused(t *testing.T) {
	_, err := view.ParseArrangement("colums")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "stack, grid or row")
}

func TestAnArrangementRoundTripsThroughItsName(t *testing.T) {
	for _, a := range []view.Arrangement{view.ArrangeStack, view.ArrangeGrid, view.ArrangeRow} {
		got, err := view.ParseArrangement(a.String())
		require.NoError(t, err)
		assert.Equal(t, a, got)
	}
}

// A section whose source has stopped keeps its values and says so in its
// title. Blanking it would answer a question the reader did not ask.
func TestASectionWhoseSourceIsGoneSaysSoAndKeepsItsValues(t *testing.T) {
	s := section("bandwidth", "Bandwidth", row("eno2 down", "317.1", "KiB/s"))
	s.Gone = true

	lines := strings.Join(view.Render([]view.Section{s}, view.ArrangeStack, 40), "\n")

	assert.Contains(t, lines, "(unavailable)")
	assert.Contains(t, lines, "317.1 KiB/s")
}

// And the verdict is dropped with it. A green row for a temperature nobody
// has measured this minute asserts something the panel does not know, which
// is the same reason a stale peripheral keeps its number and loses its
// colour.
func TestASectionWhoseSourceIsGoneDrawsItsValuesDim(t *testing.T) {
	seen := map[view.Status][]string{}
	painter := func(text string, st view.Status) string {
		seen[st] = append(seen[st], text)
		return text
	}
	s := view.Section{Key: "cooler", Title: "Cooler",
		Rows:   []view.Row{{Label: "Coolant", Value: " 38.9", Unit: "°C", Status: view.Good}},
		Trails: []view.Trail{{Name: "Coolant", Samples: []float64{38, 39}, Status: view.Good}},
	}

	view.RenderWith([]view.Section{s}, view.ArrangeStack, 40, painter)
	require.NotEmpty(t, seen[view.Good], "a live section carries its verdict")

	s.Gone = true
	seen = map[view.Status][]string{}
	view.RenderWith([]view.Section{s}, view.ArrangeStack, 40, painter)

	assert.Empty(t, seen[view.Good], "a stale reading keeps its number and loses its colour")
	assert.Contains(t, strings.Join(seen[view.Dim], ""), "38.9")
}
