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
	if width < 1 {
		width = 1
	}
	switch a {
	case ArrangeGrid:
		return renderGrid(sections, width)
	case ArrangeRow:
		return renderRow(sections, width)
	default:
		return renderStack(sections, width)
	}
}

// renderStack is one section above another.
func renderStack(sections []Section, width int) []string {
	var out []string
	for i, s := range sections {
		if i > 0 {
			out = append(out, "")
		}
		out = append(out, block(s, width)...)
	}
	return out
}

// block is one section as lines at a width: the title, then a line per row.
func block(s Section, width int) []string {
	title := s.Title
	if s.Gone {
		title += " (unavailable)"
	}
	out := []string{truncate(title, width)}
	for _, r := range s.Rows {
		out = append(out, line(r, width))
		if r.Detail != "" {
			out = append(out, rightAlign(r.Detail, width))
		}
	}
	return out
}

// line is a row at a width: the label left, the value and unit right, and the
// space between taking the change in width. The value and the unit keep their
// own widths, so a number that gains a digit does not move the label.
func line(r Row, width int) string {
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
	return label + strings.Repeat(" ", gap) + right
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

// renderRow is one line per reading with no titles: the section's name joins
// its row's label, because a pane of three lines has no room for a heading
// above each one.
func renderRow(sections []Section, width int) []string {
	var out []string
	for _, s := range sections {
		for _, r := range s.Rows {
			labelled := r
			if r.Label == "" {
				labelled.Label = s.Title
			} else {
				labelled.Label = s.Title + " " + r.Label
			}
			out = append(out, line(labelled, width))
		}
	}
	return out
}

// renderGrid reflows sections into columns.
//
// The column count comes from the width and MinColumnWidth, and one column is
// ArrangeStack: a grid that drew a single narrow column would be a stack with worse
// spacing. Sections fill column-major, so reading down a column follows the
// order the user set.
func renderGrid(sections []Section, width int) []string {
	cols := (width + ColumnGap) / (MinColumnWidth + ColumnGap)
	if cols < 2 || len(sections) < 2 {
		return renderStack(sections, width)
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
			columns[c] = append(columns[c], block(sections[i], colWidth)...)
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
