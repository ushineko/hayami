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

// A painted grid lines its columns up where the eye sees them. The padding
// between columns is counted in characters on screen, not in the escape codes
// a painter wraps them in: counted with the codes, a dim title padded short by
// their length and pulled the next column left on its line, which is how the
// gallery's photograph of the pane first came out (spec 027).
func TestAPaintedGridKeepsItsColumnsStraight(t *testing.T) {
	sections := []view.Section{
		section("a", "Alpha", row("one", "1", "")),
		section("b", "Beta", row("two", "2", ""), row("three", "3", "")),
	}
	painter := func(text string, _ view.Status) string { return "\x1b[90m" + text + "\x1b[0m" }
	strip := func(s string) string {
		var b strings.Builder
		for i := 0; i < len(s); i++ {
			if s[i] == '\x1b' {
				for i < len(s) && s[i] != 'm' {
					i++
				}
				continue
			}
			b.WriteByte(s[i])
		}
		return b.String()
	}

	plain := view.Render(sections, view.ArrangeGrid, 100)
	painted := view.RenderWith(sections, view.ArrangeGrid, 100, painter)

	require.Len(t, painted, len(plain))
	for i := range plain {
		assert.Equal(t, plain[i], strip(painted[i]), "line %d moved when it was painted", i)
	}
}

// A grid shares its width among the columns it fills. Four sections at a
// width with room for three columns fill two, two to a column; measured for
// three, the two it drew were a third of the pane each and the rest was empty,
// and an interface name was cut to two letters beside it (spec 027).
func TestAGridSharesItsWidthAmongTheColumnsItFills(t *testing.T) {
	sections := []view.Section{
		section("a", "Alpha", row("one", "1", "")),
		section("b", "Beta", row("two", "2", "")),
		section("c", "Gamma", row("three", "3", "")),
		section("d", "Delta", row("four", "4", "")),
	}
	width := 3*view.MinColumnWidth + 2*view.ColumnGap

	lines := view.Render(sections, view.ArrangeGrid, width)

	require.NotEmpty(t, lines)
	second := strings.Index(lines[0], "Gamma")
	require.Positive(t, second, "the third section should head the second column: %q", lines[0])
	assert.Equal(t, (width-view.ColumnGap)/2+view.ColumnGap, second,
		"the second of two columns should start half way across")
}
