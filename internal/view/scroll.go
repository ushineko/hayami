package view

/*
A section's scrolling run (spec 105): the rows marked Scrolls, held to
ScrollLines lines, the rest a turn of the wheel or a key away.

The view draws the run, cut to its lines from ScrollFirst, and a bar in the
column beside it; a shell keeps where it was scrolled to and says so in
ScrollFirst before drawing. A run that fits draws as rows always have. One
that does not gives the section's rows the last column for the bar, so every
figure in the section still ends in one column.
*/

// Region is where one section was drawn, in lines and characters from the
// top left of what Render returned, and where its run is within that: RunY is
// the run's first line drawn and RunH how many, none for a section without a
// run. A shell with a pointer finds the run under it from these.
type Region struct {
	Key        string
	X, Y, W, H int
	RunY, RunH int
}

// Contains reports whether a point is in the region.
func (r Region) Contains(x, y int) bool {
	return x >= r.X && x < r.X+r.W && y >= r.Y && y < r.Y+r.H
}

// InRun reports whether a point is on the region's run.
func (r Region) InRun(x, y int) bool {
	return r.RunH > 0 && x >= r.X && x < r.X+r.W && y >= r.RunY && y < r.RunY+r.RunH
}

// details reports whether an arrangement draws a row's detail lines under it:
// every one but row, which is a line per reading.
func details(a Arrangement) bool { return a != ArrangeRow }

// RunLines is how many lines the section's run has in an arrangement,
// shown or not.
func (s Section) RunLines(a Arrangement) int {
	n := 0
	for _, r := range s.Lines() {
		if !r.Scrolls {
			continue
		}
		n++
		if details(a) {
			n += len(r.DetailLines())
		}
	}
	return n
}

// MostFirst is the furthest the run can be scrolled in an arrangement: the
// first line drawn when its last line is the last one shown.
func (s Section) MostFirst(a Arrangement) int {
	if s.ScrollLines <= 0 {
		return 0
	}
	return max(0, s.RunLines(a)-s.ScrollLines)
}

// overflows reports whether the run has more lines than it shows.
func (s Section) overflows(a Arrangement) bool {
	return s.ScrollLines > 0 && s.RunLines(a) > s.ScrollLines
}

// runParts splits a section's lines, as drawn by draw, into those above the
// run, the run, and those below it.
func runParts(s Section, draw func(Row) []string) (above, run, below []string) {
	for _, r := range s.Lines() {
		lines := draw(r)
		switch {
		case r.Scrolls:
			run = append(run, lines...)
		case len(run) == 0:
			above = append(above, lines...)
		default:
			below = append(below, lines...)
		}
	}
	return above, run, below
}

// window cuts a run to the lines it shows from first, each with its bar
// beside it. A run that fits is returned as it is.
func window(run []string, s Section, a Arrangement, p Painter) []string {
	if !s.overflows(a) {
		return run
	}
	first := max(0, min(s.ScrollFirst, s.MostFirst(a)))
	shown := run[first : first+s.ScrollLines]
	out := make([]string, len(shown))
	for i, l := range shown {
		out[i] = l + bar(i, s.ScrollLines, len(run), first, p)
	}
	return out
}

// bar is the scroll bar's glyph on line i of a run that shows shown of total
// lines from first: the thumb where the lines shown are, the track elsewhere.
func bar(i, shown, total, first int, p Painter) string {
	thumb := max(1, (shown*shown+total/2)/total)
	at := 0
	if most := total - shown; most > 0 {
		at = (first*(shown-thumb) + most/2) / most
	}
	if i >= at && i < at+thumb {
		return p.paint("┃", Info)
	}
	return p.paint("│", Dim)
}

// padded gives each line a blank last column, for the lines beside a run's
// bar.
func padded(lines []string) []string {
	out := make([]string, len(lines))
	for i, l := range lines {
		out[i] = l + " "
	}
	return out
}

// RenderRegions is RenderWith, and where each section was drawn.
func RenderRegions(sections []Section, a Arrangement, width int, p Painter) ([]string, []Region) {
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
