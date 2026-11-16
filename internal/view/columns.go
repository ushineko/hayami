package view

import (
	"fmt"
	"slices"
	"strings"
)

/*
The columns of a process list (spec 092).

A list is a name and one figure, which is what a drawer at its usual width
has room for. A wider drawer has room for more, and what the list already
read and threw away is enough to fill it: how many processes share the name,
the other figure of the same reading, the share of all memory. So a detail
says which columns it has, in the order a narrowing drawer drops them, last
first, and each line carries a cell for each; a shell shows as many as fit.

**Every cell is a fixed width**, the widest it can be, for the reason every
figure is: a reading must not move the columns beside it. What decides how
many are shown is the room, never a value.
*/

// The columns a process list can have, by key.
const (
	ColumnCount  = "procs"
	ColumnCPU    = "cpu"
	ColumnMemory = "memory"
	ColumnShare  = "share"
)

// Column is one of a detail's extra columns: its key, the heading drawn over
// it, and its width in characters, which every cell under it is.
type Column struct {
	Key     string `json:"key"`
	Heading string `json:"heading"`
	Width   int    `json:"width"`
}

// FigureGap is the space between a line's figure and its first cell, and
// between one cell and the next.
const FigureGap = 2

// columnKind is how one kind of column is read from a use and drawn.
type columnKind struct {
	heading string
	// has reports whether a use carries the figure; nil for one every use
	// carries.
	has  func(ProcessUse) bool
	cell func(ProcessUse) string
}

// percentWidth is a share's cell, "100.0 %"; sizeWidth a byte count's,
// "999.9 MiB"; countWidth the count's, at its heading's width.
var (
	percentWidth = NumberWidth + len(" %")
	sizeWidth    = NumberWidth + 1 + UnitWidth(byteUnits...)
)

var columnKinds = map[string]columnKind{
	ColumnCount: {heading: "procs", cell: func(u ProcessUse) string {
		return fmt.Sprintf("×%d", min(u.processes(), 999))
	}},
	ColumnCPU: {heading: "CPU", has: func(u ProcessUse) bool { return u.CPU.OK }, cell: func(u ProcessUse) string {
		return percentCell(u.CPU)
	}},
	ColumnMemory: {heading: "Mem", has: func(u ProcessUse) bool { return u.Memory.OK }, cell: func(u ProcessUse) string {
		if !u.Memory.OK {
			n, _ := NoSize()
			return n + strings.Repeat(" ", sizeWidth-NumberWidth)
		}
		n, unit := Size(u.Memory.V)
		return n + " " + PadUnit(unit, UnitWidth(byteUnits...))
	}},
	ColumnShare: {heading: "of mem", has: func(u ProcessUse) bool { return u.Share.OK }, cell: func(u ProcessUse) string {
		return percentCell(u.Share)
	}},
}

// columnWidths are the cells' widths, each the widest its cell and its
// heading can be.
var columnWidths = map[string]int{
	ColumnCount:  max(len("×999"), len("procs")),
	ColumnCPU:    percentWidth,
	ColumnMemory: sizeWidth,
	ColumnShare:  percentWidth,
}

// percentCell is a share, or its empty form at the same width.
func percentCell(v Opt[float64]) string {
	if !v.OK {
		return NoQuantity() + "  "
	}
	return Quantity(min(max(v.V, 0), 999.9)) + " %"
}

// processes is how many processes a use is: a use read from one process says
// nothing and is one.
func (u ProcessUse) processes() int { return max(u.Count, 1) }

// plus is two uses of one name summed: their figures, and each other figure
// that either has.
func (u ProcessUse) plus(o ProcessUse) ProcessUse {
	u.Value += o.Value
	u.Second += o.Second
	u.Count = u.processes() + o.processes()
	u.CPU, u.Memory, u.Share = addOpt(u.CPU, o.CPU), addOpt(u.Memory, o.Memory), addOpt(u.Share, o.Share)
	return u
}

// Sum is uses added together, as a list adds the processes of one name.
func Sum(uses []ProcessUse) ProcessUse {
	if len(uses) == 0 {
		return ProcessUse{}
	}
	out := uses[0]
	for _, u := range uses[1:] {
		out = out.plus(u)
	}
	return out
}

func addOpt(a, b Opt[float64]) Opt[float64] {
	return Opt[float64]{V: a.V + b.V, OK: a.OK || b.OK}
}

/*
OfferColumns are the columns a list of detail's kind has for these uses, in
the kind's order. A column none of the uses carries is not offered: a list
with no memory total has no share of it, a card's list read without the
processes has no processor share. count leaves the count out, for a page of
one name's processes, each of which is one.
*/
func OfferColumns(detail string, uses []ProcessUse, count bool) []Column {
	var out []Column
	for _, key := range processKinds[detail].columns {
		kind := columnKinds[key]
		switch {
		case key == ColumnCount && !count:
			continue
		case kind.has != nil && !slices.ContainsFunc(uses, kind.has):
			continue
		}
		out = append(out, Column{Key: key, Heading: kind.heading, Width: columnWidths[key]})
	}
	return out
}

/*
ColumnsWant is how many characters a list's value takes with every column
its kind can carry: the figure, then each column and its gap (spec 093 R1).
It is the kind's, not a reading's, so no reading changes it; a list without
columns wants none, and its drawer the usual width.
*/
func ColumnsWant(detail string) int {
	kind := processKinds[detail]
	if len(kind.columns) == 0 {
		return 0
	}
	w := kind.figure
	for _, key := range kind.columns {
		w += FigureGap + columnWidths[key]
	}
	return w
}

// Cells are a use's cells under columns, each at its column's width.
func Cells(columns []Column, u ProcessUse) []string {
	if len(columns) == 0 {
		return nil
	}
	out := make([]string, len(columns))
	for i, c := range columns {
		out[i] = padLeft(columnKinds[c.Key].cell(u), c.Width)
	}
	return out
}

// FigureWidth is the width of the lines' figure, value and unit, which the
// fixed-width formatters make the same on every line of a list.
func FigureWidth(lines []DetailLine) int {
	w := 0
	for _, l := range lines {
		if !l.Heading {
			w = max(w, runeLen(l.figure()))
		}
	}
	return w
}

// figure is the line's value with its unit.
func (l DetailLine) figure() string {
	if l.Unit == "" {
		return l.Value
	}
	return l.Value + " " + l.Unit
}

// ColumnsWidth is how many characters the figure and the first n columns
// take together.
func ColumnsWidth(figure int, columns []Column, n int) int {
	w := figure
	for _, c := range columns[:min(n, len(columns))] {
		w += FigureGap + c.Width
	}
	return w
}

/*
Row is the line as a row's value with its first n cells after the figure,
for a shell that draws a value as one right-aligned run. With none it is the
line as it always was; with some, the figure and its unit become parts so
the cells can follow them, each in the plain colour of a fact.
*/
func (l DetailLine) Row(n int) Row {
	r := Row{Value: l.Value, Unit: l.Unit, Parts: l.Parts, Status: l.Status}
	n = min(n, len(l.Cells))
	if n <= 0 {
		return r
	}
	parts := slices.Clone(l.Parts)
	if len(parts) == 0 {
		parts = []Part{{Text: l.Value, Status: l.Status}}
	}
	if l.Unit != "" {
		parts = append(parts, Part{Text: " " + l.Unit, Status: Info})
	}
	for _, c := range l.Cells[:n] {
		parts = append(parts, Part{Text: strings.Repeat(" ", FigureGap) + c, Status: Info})
	}
	row := JoinedRow(parts)
	row.Status = l.Status
	return row
}

// ColumnHeads is the line over the first n columns: blank over the figure,
// each heading right-aligned over its cells. It is as wide as a line with n
// cells, so in a right-aligned run it stands over them.
func ColumnHeads(figure int, columns []Column, n int) string {
	var b strings.Builder
	b.WriteString(strings.Repeat(" ", figure))
	for _, c := range columns[:min(n, len(columns))] {
		b.WriteString(strings.Repeat(" ", FigureGap) + padLeft(c.Heading, c.Width))
	}
	return b.String()
}
