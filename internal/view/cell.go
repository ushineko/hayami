package view

import "strings"

// MinCellWidth is the narrowest a cell may be drawn before the layout gives up
// and puts one to a line.
//
// Sixteen characters, which is a two-digit percentage with its sign, a state
// word like "Disconnected" truncated to something still readable, and a space
// each side. Below it the lines stop lining up with each other and the
// block stops reading as one thing.
const MinCellWidth = 16

// CellGap is the space between two cells on a line.
const CellGap = 2

// cells lays a section's cells out across a width, as many to a line as fit.
//
// The monitor draws two to a line because its panel is 260 pixels wide and it
// has four fixed slots. This fits what it can, which is the same answer at
// that width and a better one at any other -- a terminal pane is whatever the
// user dragged it to.
func cells(cs []Cell, width int, p Painter) []string {
	if len(cs) == 0 {
		return nil
	}
	columns := cellColumns(len(cs), width)

	var out []string
	for start := 0; start < len(cs); start += len(columns) {
		end := min(start+len(columns), len(cs))
		out = append(out, cellLines(cs[start:end], columns, p)...)
	}
	return out
}

/*
cellColumns is the width of each column on a line of cells.

The width is shared equally rather than measured per cell, because these are a
grid: a column that fitted its own contents would put the percentages at a
different place on every line, and a column of figures that do not line up is
the thing the fixed-width formatters exist to prevent.

Equally, and then the remainder. Integer division leaves up to one character
per column unspent, and unspent characters are a ragged right edge -- three
columns in a pane of sixty came to fifty-eight. They go to the leftmost
columns, one each, which is a difference of one character in where a reading
sits and is not visible.
*/
func cellColumns(n, width int) []int {
	perLine := (width + CellGap) / (MinCellWidth + CellGap)
	perLine = min(max(perLine, 1), n)

	usable := width - (perLine-1)*CellGap
	column := max(usable/perLine, 1)
	extra := max(usable-column*perLine, 0)

	out := make([]int, perLine)
	for i := range out {
		out[i] = column
		if i < extra {
			out[i]++
		}
	}
	return out
}

/*
cellLines draws one line of cells: four lines of text, side by side -- the
name, the reading, the state, and the bar under a level (spec 025).

The bar's line is always drawn, blank under a cell with no bar, as the
window's bar row is always reserved: a pane that gained a line when a device
gained a level would reflow over a battery. A band cell's bar line is blank
so the columns stay aligned; its segments already say its level.

columns is every column the line has room for, which is not always how many
cells are on it: the last line of an odd number of devices is short. The
empty columns are still spent, because a line that stopped at its last cell
would leave a ragged edge down the pane and the rule here is that an
arrangement fills its width rather than measuring itself.
*/
func cellLines(row []Cell, columns []int, p Painter) []string {
	names := make([]string, 0, len(columns))
	values := make([]string, 0, len(columns))
	notes := make([]string, 0, len(columns))
	bars := make([]string, 0, len(columns))

	for i, column := range columns {
		blank := strings.Repeat(" ", column)
		if i >= len(row) {
			names, values, notes, bars = append(names, blank), append(values, blank),
				append(notes, blank), append(bars, blank)
			continue
		}
		c := row[i]
		names = append(names, p.paint(centre(c.Label, column), Info))
		values = append(values, p.paint(centre(quantity(c), column), cellStatus(c)))
		notes = append(notes, p.paint(centre(c.Note, column), Dim))
		bars = append(bars, cellBar(c, column, blank, p))
	}

	gap := strings.Repeat(" ", CellGap)
	return []string{
		strings.Join(names, gap),
		strings.Join(values, gap),
		strings.Join(notes, gap),
		strings.Join(bars, gap),
	}
}

// cellBar is a cell's bar at a width, in the meter's glyphs and the cell's
// colour, or blank for a cell without one.
func cellBar(c Cell, width int, blank string, p Painter) string {
	if !c.HasBar {
		return blank
	}
	return paintBar(c.Bar, width, cellStatus(c), p)
}

// quantity is a cell's reading with its unit, as one piece: a cell centres the
// two together rather than aligning them in columns, because there is no
// column to align them in.
func quantity(c Cell) string {
	if c.Unit == "" {
		return strings.TrimSpace(c.Value)
	}
	return strings.TrimSpace(c.Value) + " " + strings.TrimSpace(c.Unit)
}

// centre pads a string into a column with the slack split either side, the
// extra character going to the right.
func centre(s string, width int) string {
	s = truncate(s, width)
	slack := width - runeLen(s)
	if slack <= 0 {
		return s
	}
	left := slack / 2
	return strings.Repeat(" ", left) + s + strings.Repeat(" ", slack-left)
}

/*
cellLine is one cell as a single line, for the arrangement that draws one
line per reading: the name at the left, the reading at the right, and the
state between them where there is room.

A cell is four lines and this arrangement has one, so something has to go.
The state goes next to the name rather than being dropped: it is a word, the
reading is a number, and a pane that spent its one line on the number alone
would say less than the row form it replaced.

With bars, the level is followed by a short bar of CellRowBar characters, and
a cell without one by as many spaces, so the readings still end in one column
down the pane. Whether there is room for it is decided for the pane, not the
line (see cellRowBars): a bar on some lines and not others would put the
readings in two columns.
*/
func cellLine(c Cell, width int, bars bool, p Painter) string {
	r := Row{
		Label:  strings.TrimSpace(c.Label + "  " + c.Note),
		Value:  c.Value,
		Unit:   c.Unit,
		Status: c.Status,
	}
	if !bars {
		return line(r, width, p)
	}
	return line(r, width-CellRowBar-1, p) + " " +
		cellBar(c, CellRowBar, strings.Repeat(" ", CellRowBar), p)
}

/*
CellRowBar is how many characters a cell's bar takes in the row arrangement.

Ten, so each is a tenth of the battery: short enough to sit after the level
without crowding the name, and a whole number of steps a reader can count.
The meter's glyphs rather than a band's segments, because a band cell draws
its level as segments and a level cell's bar beside it must not look like
another band.
*/
const CellRowBar = 10

/*
cellRowBars is whether a pane in the row arrangement has room for the cells'
bars: some cell has one, and every cell's line fits beside a bar without its
name being cut.

The name gives way to the reading in a line that is too narrow, and a bar is
worth less than either: it is the impression and the number is the reading.
So the bar goes first.
*/
func cellRowBars(sections []Section, width int) bool {
	some := false
	for _, s := range sections {
		for _, c := range s.Cells {
			some = some || c.HasBar
			label := strings.TrimSpace(c.Label + "  " + c.Note)
			if runeLen(label)+1+runeLen(quantity(c))+1+CellRowBar > width {
				return false
			}
		}
	}
	return some
}

// cellStatus is the colour a cell's reading is painted.
//
// Dimming wins over the verdict, as it does for a Row in the design system: a
// warning that is no longer being refreshed should not keep shouting. The
// pane has one dim and the window has a darker shade of each status colour;
// both say the same thing, which is that this number is the last one heard.
func cellStatus(c Cell) Status {
	if c.Stale {
		return Dim
	}
	return c.Status
}
