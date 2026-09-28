package view_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/hayami/internal/view"
)

func cell(label, value, note string) view.Cell {
	return view.Cell{Label: label, Value: value, Unit: "%", Note: note}
}

func cellSection(cells ...view.Cell) view.Section {
	return view.Section{Key: "peripherals", Title: "Peripherals", Cells: cells}
}

// A cell is three lines -- the name, the reading, the state -- and they are
// the three lines the archetype draws. Two cells side by side are still three
// lines, which is the whole reason a cell is not a row.
func TestACellIsThreeLinesAndTwoCellsShareThem(t *testing.T) {
	one := view.Render([]view.Section{cellSection(cell("G502", "67", "Discharging"))},
		view.ArrangeStack, 40)
	two := view.Render([]view.Section{cellSection(
		cell("G502", "67", "Discharging"),
		cell("Keychron", "--", "No reading"),
	)}, view.ArrangeStack, 40)

	assert.Len(t, one, 4, "a title and three lines")
	assert.Len(t, two, 4, "a second cell went beside the first, not under it")
	assert.Contains(t, strings.Join(two, "\n"), "Keychron")
}

// The reading is in the middle of its column, which is what makes it the
// thing the eye lands on. A row puts it at the right margin in the same
// weight as the name, and that is the shape this replaces.
func TestTheReadingIsCentredInItsColumn(t *testing.T) {
	lines := view.Render([]view.Section{cellSection(cell("G502", "67", "Discharging"))},
		view.ArrangeStack, 40)

	value := lines[2]
	before := len(value) - len(strings.TrimLeft(value, " "))
	after := len(value) - len(strings.TrimRight(value, " "))
	assert.InDelta(t, before, after, 1, "the reading is not centred: %q", value)
}

// Every arrangement fills its width. A line that stopped at its last cell
// would leave a ragged edge down the pane, which a reader takes for a bug --
// and the last line of an odd number of devices is always short.
func TestEveryLineFillsTheWidthIncludingAShortLastOne(t *testing.T) {
	s := cellSection(
		cell("A", "10", "Discharging"),
		cell("B", "20", "Discharging"),
		cell("C", "30", "Discharging"),
	)

	for _, width := range []int{40, 60, 80} {
		for _, line := range view.Render([]view.Section{s}, view.ArrangeStack, width)[1:] {
			assert.Len(t, []rune(line), width, "at width %d: %q", width, line)
		}
	}
}

// A pane too narrow for two cells puts one to a line rather than drawing two
// nobody can read.
func TestAPaneTooNarrowForTwoCellsStacksThem(t *testing.T) {
	s := cellSection(cell("G502", "67", "Discharging"), cell("Keychron", "--", "No reading"))

	lines := view.Render([]view.Section{s}, view.ArrangeStack, view.MinCellWidth+2)

	assert.Len(t, lines, 7, "a title and three lines for each of two cells")
}

// The row arrangement has one line per reading, so a cell folds into one:
// the name and the state at the left, the reading at the right. The state is
// kept rather than dropped -- it is a word, the reading is a number, and a
// line that spent itself on the number alone would say less than the row form
// this replaced.
func TestInARowACellIsOneLineAndKeepsItsState(t *testing.T) {
	line := view.Render([]view.Section{cellSection(cell("G502", "67", "Discharging"))},
		view.ArrangeRow, 60)[0]

	assert.Contains(t, line, "G502")
	assert.Contains(t, line, "Discharging")
	assert.Contains(t, line, "67")
	assert.Len(t, []rune(line), 60)
}

// A name longer than its column is truncated rather than allowed to push the
// column along. Nothing transient may reflow the panel, and a device name is
// as transient as the device.
func TestALongNameIsTruncatedRatherThanWidening(t *testing.T) {
	s := cellSection(
		cell("Arctis Nova Pro Wireless", "15", "Discharging"),
		cell("G502", "67", "Discharging"),
	)

	lines := view.Render([]view.Section{s}, view.ArrangeStack, 40)

	require.Len(t, lines, 4)
	assert.Len(t, []rune(lines[1]), 40)
	assert.NotContains(t, lines[1], "Wireless")
}

// A section with cells and a painter is painted like any other: the name is a
// label, the reading carries the verdict, the state is quiet.
func TestACellsThreePartsArePaintedApart(t *testing.T) {
	seen := map[view.Status][]string{}
	painter := func(text string, st view.Status) string {
		seen[st] = append(seen[st], text)
		return text
	}
	s := cellSection(view.Cell{Label: "G502", Value: "15", Unit: "%",
		Note: "Discharging", Status: view.Bad})

	view.RenderWith([]view.Section{s}, view.ArrangeStack, 40, painter)

	assert.Contains(t, strings.Join(seen[view.Info], ""), "G502")
	assert.Contains(t, strings.Join(seen[view.Bad], ""), "15")
	assert.Contains(t, strings.Join(seen[view.Dim], ""), "Discharging")
}
