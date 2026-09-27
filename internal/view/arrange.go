package view

import (
	"fmt"
	"strings"
)

// Arrangement is how sections are laid out. It is a property of the view and
// not a mode of the program: the same sections appear in all three, and a
// terminal pane showing one section is this setting, not a separate widget.
type Arrangement int

const (
	// ArrangeStack puts one section above another, full width. The shape of the
	// desktop panel, and of a narrow terminal.
	ArrangeStack Arrangement = iota
	// ArrangeGrid reflows sections into columns that fit the width, in the manner of
	// btop. A width that fits one column is Stack.
	ArrangeGrid
	// ArrangeRow draws one line per reading, the label at the left, the value at the
	// right, and the space between them taking the slack. This is what a pane
	// in a session manager wants, and what replaces the usage widget's own
	// terminal modes.
	ArrangeRow
)

// String names the arrangement as the settings and the command line spell it.
func (a Arrangement) String() string {
	switch a {
	case ArrangeGrid:
		return "grid"
	case ArrangeRow:
		return "row"
	default:
		return "stack"
	}
}

// ParseArrangement reads the name back. An unknown name is an error rather
// than a silent fall back to Stack: a user who typed "colums" should be told.
func ParseArrangement(s string) (Arrangement, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "stack":
		return ArrangeStack, nil
	case "grid":
		return ArrangeGrid, nil
	case "row":
		return ArrangeRow, nil
	}
	return ArrangeStack, fmt.Errorf("no arrangement named %q; it is stack, grid or row", s)
}

// MinColumnWidth is the narrowest a grid column may be before the grid gives
// up and stacks. Below it a label and its value have no room between them and
// the reflow is worse than no reflow.
const MinColumnWidth = 28

// ColumnGap is the space between two grid columns.
const ColumnGap = 2

// Render lays sections out at a width and returns the lines.
//
// Width is in characters and is the pane's, not the content's: every
// arrangement fills it rather than measuring itself, because a panel that is
// narrower than its pane leaves a ragged edge a person reads as a bug.
func Render(sections []Section, a Arrangement, width int) []string {
	return RenderWith(sections, a, width, nil)
}

// RenderWith is Render with a painter, for a shell that colours.
//
// Nothing is painted without one, which is what keeps a pipe, a redirect and
// every test in this package seeing exactly the text the view built.
func RenderWith(sections []Section, a Arrangement, width int, p Painter) []string {
	if width < 1 {
		width = 1
	}
	switch a {
	case ArrangeGrid:
		return renderGrid(sections, width, p)
	case ArrangeRow:
		return renderRow(sections, width, p)
	default:
		return renderStack(sections, width, p)
	}
}

// renderStack is one section above another.
func renderStack(sections []Section, width int, p Painter) []string {
	var out []string
	for i, s := range sections {
		if i > 0 {
			out = append(out, "")
		}
		out = append(out, block(s, width, p)...)
	}
	return out
}

// block is one section as lines at a width: the title, then a line per row.
func block(s Section, width int, p Painter) []string {
	title := s.Title
	if s.Gone {
		title += " (unavailable)"
	}
	out := []string{p.paint(truncate(title, width), Dim)}
	for _, r := range s.Rows {
		out = append(out, line(r, width, p))
		if r.Detail != "" {
			out = append(out, p.paint(rightAlign(r.Detail, width), Dim))
		}
	}
	out = append(out, meters(s.Meters, width, p)...)
	if line := Sparkline(s.Trail, width, SparkMinSpan); line != "" {
		out = append(out, p.paint(line, Info))
	}
	return out
}

// SparkMinSpan is the narrowest range a sparkline's scale may have, in the
// units of whatever it is plotting.
//
// One degree. Below it a coolant that has not moved is amplified into eight
// heights of noise, and a reader would take a hundredth of a degree for a
// trend.
const SparkMinSpan = 1.0

// MeterLabelGap is the space between a meter's label and its caption.
const MeterLabelGap = 1

// meters draws a section's meters: a label and a caption on one line, the bar
// on the next.
//
// The labels are padded to the widest of them so the captions line up down the
// section, which is what makes two windows of the same quota comparable at a
// glance. The bar takes the whole width, because a bar that stopped short
// would invent a maximum that is not the one the caption states.
func meters(ms []Meter, width int, p Painter) []string {
	if len(ms) == 0 {
		return nil
	}
	labelWidth := 0
	for _, m := range ms {
		labelWidth = max(labelWidth, runeLen(m.Name()))
	}

	out := make([]string, 0, len(ms)*2)
	for _, m := range ms {
		caption := m.Caption
		if m.Reset != "" {
			caption += " · " + m.Reset
		}
		head := p.paint(m.Name(), Info) + strings.Repeat(" ",
			max(MeterLabelGap, labelWidth-runeLen(m.Name())+MeterLabelGap)) +
			p.paint(caption, m.Status)
		out = append(out, head, paintBar(m.Fraction, width, m.Status, p))
	}
	return out
}

// paintBar draws a bar with its filled part in the status colour and its track
// dim, so a glance at the colour says the same thing as a reading of the
// number.
func paintBar(fraction float64, width int, status Status, p Painter) string {
	if width < 1 {
		return ""
	}
	full := int(MeterFraction(fraction)*float64(width) + 0.5)
	return p.paint(strings.Repeat(string(BarFull), full), status) +
		p.paint(strings.Repeat(string(BarEmpty), width-full), Dim)
}

// line is a row at a width: the label left, the value and unit right, and the
// space between taking the change in width. The value and the unit keep their
// own widths, so a number that gains a digit does not move the label.
func line(r Row, width int, p Painter) string {
	right := r.Value
	if r.Unit != "" {
		right += " " + r.Unit
	}
	label := r.Label
	gap := width - runeLen(label) - runeLen(right)
	if gap < 1 {
		// No room for both. The value is the reading and the label is the
		// reminder, so the label gives way.
		label = truncate(label, max(0, width-runeLen(right)-1))
		gap = max(1, width-runeLen(label)-runeLen(right))
	}
	// The label is white, as a meter's name is. It says what the number beside
	// it is, and a reader who cannot tell two rows apart has no use for
	// either. What is dim is the furniture: a track, a heading, a reset.
	return p.paint(label, Info) + strings.Repeat(" ", gap) + p.paint(right, r.Status)
}

// rightAlign puts a detail line against the right edge, under the value it
// belongs to.
func rightAlign(s string, width int) string {
	s = truncate(s, width)
	if pad := width - runeLen(s); pad > 0 {
		return strings.Repeat(" ", pad) + s
	}
	return s
}

// renderRow is one line per reading, in columns.
//
// Four of them: the label, a bar that takes whatever is left, the figures, and
// the reset hard against the right edge. Columns rather than one right-aligned
// run, because the eye finds a number by where it is: a reset that lands in a
// different place on every line has to be read for rather than glanced at.
//
// The section's title is not repeated. In a pane of three lines, a heading on
// each one spends three columns saying what the labels already say; it is
// carried only for a row that has no label of its own.
func renderRow(sections []Section, width int, p Painter) []string {
	c := rowColumns(sections)

	var out []string
	for _, s := range sections {
		for _, r := range s.Rows {
			out = append(out, line(labelled(s, r), width, p))
		}
		for _, m := range s.Meters {
			out = append(out, meterRow(s, m, width, c, p))
		}
		if trail := trailRow(s, width, c, p); trail != "" {
			out = append(out, trail)
		}
	}
	return out
}

// trailRow is a section's series as one line: the section's name in the label
// column, then the plot taking the rest.
//
// Named, because in a pane a bare row of block characters is a row nobody can
// attribute. It is the one line in this arrangement that is not a reading, and
// it earns its place for the reason the archetype gives: a trend is what a
// reader takes from a panel they never touch.
func trailRow(s Section, width int, c columns, p Painter) string {
	label := padRight(s.Title, c.label)
	plot := Sparkline(s.Trail, width-runeLen(label)-1, SparkMinSpan)
	if plot == "" {
		return ""
	}
	return p.paint(label, Dim) + " " + p.paint(plot, Info)
}

// labelled gives a row the section's name only when it has none of its own.
func labelled(s Section, r Row) Row {
	if r.Label == "" {
		r.Label = s.Title
	}
	return r
}

// rowColumns measures the three fixed columns across every section, so the
// lines line up with each other rather than each with itself.
func rowColumns(sections []Section) (c columns) {
	for _, s := range sections {
		if len(s.Trail) > 0 {
			c.label = max(c.label, runeLen(s.Title))
		}
		for _, r := range s.Rows {
			c.label = max(c.label, runeLen(r.Label))
		}
		for _, m := range s.Meters {
			c.label = max(c.label, runeLen(m.Label))
			c.badge = max(c.badge, runeLen(m.Badge))
			c.window = max(c.window, runeLen(m.Window))
			c.figures = max(c.figures, runeLen(m.Caption))
			c.reset = max(c.reset, runeLen(m.Reset))
		}
	}
	return c
}

// columns are the fixed widths a pane's lines share.
type columns struct{ label, badge, window, figures, reset int }

// name lays a meter's three name parts out in their columns.
//
// Three columns rather than one, so the badges line up under each other and
// the window names do too. An eye scanning a pane for the plan letter should
// find it in the same place on every line rather than hunting for it after a
// name whose length it cannot predict.
func (c columns) name(m Meter) string {
	out := padRight(m.Label, c.label)
	if c.badge > 0 {
		out += " " + padRight(m.Badge, c.badge)
	}
	if c.window > 0 {
		out += " " + padRight(m.Window, c.window)
	}
	return out
}

// width is the room the fixed columns take, with a space between each.
func (c columns) width() int {
	out := c.label + c.figures + c.reset + 3
	if c.badge > 0 {
		out += c.badge + 1
	}
	if c.window > 0 {
		out += c.window + 1
	}
	return out
}

// meterRow is one meter as a line: name, bar, figures, reset.
//
// The name is white rather than dim. It is not decoration: it says which
// account and which window the bar beside it is about, and a reader who cannot
// tell two lines apart has no use for either. The track is what is dim, and
// the reset, which is the one thing a glance can skip.
//
// The bar takes what the fixed columns leave. Where that is too little to be
// worth drawing, the bar goes and the figures stay: the caption is the reading
// and the bar is the impression.
func meterRow(s Section, m Meter, width int, c columns, p Painter) string {
	name := c.name(m)
	if m.Label == "" && m.Badge == "" && m.Window == "" {
		name = padRight(s.Title, c.label)
	}
	// The figures are ranged left in their column so the first one begins at
	// the same place on every line. Ranging them right would line up their
	// ends, which is not where an eye looks for them.
	figures := padRight(m.Caption, c.figures)
	reset := padLeft(m.Reset, c.reset)

	barWidth := width - c.width()
	if barWidth < MinBarWidth {
		return line(Row{Label: m.Name(), Value: strings.TrimSpace(m.Caption + " " + m.Reset),
			Status: m.Status}, width, p)
	}

	return p.paint(name, Info) + " " +
		paintBar(m.Fraction, barWidth, m.Status, p) + " " +
		p.paint(figures, m.Status) + " " + p.paint(reset, Dim)
}

// MinBarWidth is the narrowest a bar may be before it is not worth the room.
const MinBarWidth = 8

// padRight ranges text left in a column.
func padRight(s string, width int) string {
	if n := width - runeLen(s); n > 0 {
		return s + strings.Repeat(" ", n)
	}
	return s
}

// padLeft ranges text right in a column.
func padLeft(s string, width int) string {
	if n := width - runeLen(s); n > 0 {
		return strings.Repeat(" ", n) + s
	}
	return s
}

// renderGrid reflows sections into columns.
//
// The column count comes from the width and MinColumnWidth, and one column is
// ArrangeStack: a grid that drew a single narrow column would be a stack with worse
// spacing. Sections fill column-major, so reading down a column follows the
// order the user set.
func renderGrid(sections []Section, width int, p Painter) []string {
	cols := (width + ColumnGap) / (MinColumnWidth + ColumnGap)
	if cols < 2 || len(sections) < 2 {
		return renderStack(sections, width, p)
	}
	if cols > len(sections) {
		cols = len(sections)
	}
	colWidth := (width - ColumnGap*(cols-1)) / cols

	// Column-major: the first column takes the first sections.
	perCol := (len(sections) + cols - 1) / cols
	columns := make([][]string, cols)
	height := 0
	for c := range cols {
		for i := c * perCol; i < (c+1)*perCol && i < len(sections); i++ {
			if len(columns[c]) > 0 {
				columns[c] = append(columns[c], "")
			}
			columns[c] = append(columns[c], block(sections[i], colWidth, p)...)
		}
		height = max(height, len(columns[c]))
	}

	out := make([]string, 0, height)
	for row := range height {
		var b strings.Builder
		for c := range cols {
			if c > 0 {
				b.WriteString(strings.Repeat(" ", ColumnGap))
			}
			cell := ""
			if row < len(columns[c]) {
				cell = columns[c][row]
			}
			b.WriteString(cell)
			if pad := colWidth - runeLen(cell); pad > 0 && c < cols-1 {
				b.WriteString(strings.Repeat(" ", pad))
			}
		}
		out = append(out, strings.TrimRight(b.String(), " "))
	}
	return out
}
