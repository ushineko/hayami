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

// A cell is four lines -- the name, the reading, the state and the bar's
// line (spec 025) -- and two cells side by side are still four lines, which is
// the whole reason a cell is not a row.
func TestACellIsFourLinesAndTwoCellsShareThem(t *testing.T) {
	one := view.Render([]view.Section{cellSection(cell("G502", "67", "Discharging"))},
		view.ArrangeStack, 40)
	two := view.Render([]view.Section{cellSection(
		cell("G502", "67", "Discharging"),
		cell("Keychron", "--", "No reading"),
	)}, view.ArrangeStack, 40)

	assert.Len(t, one, 5, "a title and four lines")
	assert.Len(t, two, 5, "a second cell went beside the first, not under it")
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

	assert.Len(t, lines, 9, "a title and four lines for each of two cells")
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

	require.Len(t, lines, 5)
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

// barCell is a level cell with its bar, as the peripherals section builds one.
func barCell(label string, level int, st view.Status) view.Cell {
	c := cell(label, "", "Discharging")
	c.Value = strings.TrimSpace(view.Count(level))
	c.Status, c.Bar, c.HasBar = st, float64(level)/100, true
	return c
}

// bandCell is a cell whose level is segments, with no bar.
func bandCell(label string) view.Cell {
	return view.Cell{Label: label, Value: "▮▮▮▯", Note: "Good", Status: view.Good}
}

// AC (spec 025). Under a level cell the pane draws a bar the cell's width in
// the meter's glyphs, filled to the level; under a band cell the line is
// blank, so the columns stay aligned.
func TestAPaneDrawsABarUnderALevelAndNoneUnderABand(t *testing.T) {
	s := cellSection(barCell("G502", 70, view.Good), bandCell("K800"))

	lines := view.Render([]view.Section{s}, view.ArrangeStack, 42)

	require.Len(t, lines, 5, "a title and four lines")
	bars := []rune(lines[4])
	require.Len(t, bars, 42, "the bar line does not fill the width")
	column := (42 - view.CellGap) / 2
	left, right := string(bars[:column]), string(bars[column+view.CellGap:])

	assert.Equal(t, strings.Repeat(string(view.BarFull), 14)+strings.Repeat(string(view.BarEmpty), 6), left,
		"the level cell's bar is not 70 %% of its column")
	assert.Equal(t, strings.Repeat(" ", column), right, "a band cell drew a bar")
}

// The bar line is there even when no cell has a bar, as the window's bar row
// is reserved: a pane that grew a line when a device gained a level would
// reflow over a battery.
func TestThePanesBarLineIsReservedWithoutABar(t *testing.T) {
	with := view.Render([]view.Section{cellSection(barCell("G502", 70, view.Good), bandCell("K800"))},
		view.ArrangeStack, 40)
	without := view.Render([]view.Section{cellSection(bandCell("K800"), bandCell("K810"))},
		view.ArrangeStack, 40)

	assert.Len(t, without, len(with), "the pane changed height with the bars")
}

// The bar is painted in the cell's status and its track dim, and a stale
// cell's bar is dim with it.
func TestACellsBarIsPaintedInItsStatus(t *testing.T) {
	seen := map[view.Status]string{}
	painter := func(text string, st view.Status) string {
		seen[st] += text
		return text
	}
	view.RenderWith([]view.Section{cellSection(barCell("G502", 15, view.Bad))}, view.ArrangeStack, 40, painter)
	assert.Contains(t, seen[view.Bad], string(view.BarFull))
	assert.Contains(t, seen[view.Dim], string(view.BarEmpty))

	seen = map[view.Status]string{}
	stale := barCell("G502", 15, view.Bad)
	stale.Stale = true
	view.RenderWith([]view.Section{cellSection(stale)}, view.ArrangeStack, 40, painter)
	assert.NotContains(t, seen[view.Bad], string(view.BarFull), "a stale cell's bar kept its verdict")
	assert.Contains(t, seen[view.Dim], string(view.BarFull))
}

// In a row a level cell's bar is a short run after the level, and a band
// cell's line has as many spaces there, so the readings end in one column.
func TestInARowTheBarFollowsTheLevel(t *testing.T) {
	s := cellSection(barCell("G502", 70, view.Good), bandCell("K800"))

	lines := view.Render([]view.Section{s}, view.ArrangeRow, 60)

	require.Len(t, lines, 2)
	bar := strings.Repeat(string(view.BarFull), 7) + strings.Repeat(string(view.BarEmpty), 3)
	assert.True(t, strings.HasSuffix(lines[0], "70 % "+bar), "%q", lines[0])
	assert.True(t, strings.HasSuffix(lines[1], "▮▮▮▯"+strings.Repeat(" ", view.CellRowBar+1)), "%q", lines[1])
	for _, l := range lines {
		assert.Len(t, []rune(l), 60)
	}
}

// Too narrow for the bar beside every name, the row drops the bar rather than
// cutting a name: the bar is the impression and the name says which device.
func TestInANarrowRowTheBarGoesFirst(t *testing.T) {
	s := cellSection(barCell("G502 X PLUS", 70, view.Good), bandCell("K800"))

	lines := view.Render([]view.Section{s}, view.ArrangeRow, 30)

	joined := strings.Join(lines, "\n")
	assert.NotContains(t, joined, string(view.BarFull))
	assert.Contains(t, joined, "G502 X PLUS  Discharging")
}
