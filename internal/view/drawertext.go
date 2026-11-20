package view

import (
	"regexp"
	"strings"
)

/*
The drawer as a terminal draws it (spec 098): the same Detail the window's
drawer draws, laid out as lines of text at a width and a height. Like
RenderWith for the cards, it is the terminal's arrangement of data the view
already describes, and decides nothing about what a detail says.
*/

// SummaryKeepsLines is how many lines a list, its reasons and its note keep
// under a summary, or the summary is not drawn (spec 094 R3). Both shells
// draw by it.
const SummaryKeepsLines = 10

// DrawerShape is the state a terminal's drawer is drawn in: the lists a
// person can switch between and the one shown, the filter and whether it is
// being typed, the marked line (an index into Selectable, -1 for none),
// whether the reading is paused, and how far a detail of parts is scrolled.
type DrawerShape struct {
	Choices   []DetailInfo
	Shown     string
	Filter    string
	Filtering bool
	Marked    int
	Paused    bool
	Scroll    int

	// Day is the cell marked on a strip of days while the strip's line is
	// marked (spec 102 R13); -1 is the last, today.
	Day int
}

// FilterMatcher is how a typed filter matches a name, as the window's does:
// a regular expression, ignoring case, or the text itself where it is not
// one.
func FilterMatcher(text string) func(string) bool {
	if text == "" {
		return func(string) bool { return true }
	}
	if re, err := regexp.Compile("(?i)" + text); err == nil {
		return re.MatchString
	}
	lower := strings.ToLower(text)
	return func(name string) bool { return strings.Contains(strings.ToLower(name), lower) }
}

/*
PaneDetail is the detail as a read-only drawer shows it: narrowed by the
filter where it is a list a person filters, and without the lines that act
(ending processes, a VPN's switch): the terminal's drawer reads and does not
act (spec 098 R10).
*/
func PaneDetail(d Detail, filter string) Detail {
	if d.Filterable && filter != "" {
		d = d.Matching(FilterMatcher(filter))
	}
	d.Lines = withoutActions(d.Lines)
	if len(d.Parts) > 0 {
		parts := make([]Detail, len(d.Parts))
		for i, p := range d.Parts {
			p.Lines = withoutActions(p.Lines)
			parts[i] = p
		}
		d.Parts = parts
	}
	return d
}

func withoutActions(lines []DetailLine) []DetailLine {
	out := make([]DetailLine, 0, len(lines))
	for _, l := range lines {
		if !strings.HasPrefix(l.Open, "!") {
			out = append(out, l)
		}
	}
	return out
}

// Selectable are the lines a person can mark, in the order they are drawn:
// every line that is not a heading, the parts' after one another. A part's
// heading that opens something is one (spec 102), and so is its strip of
// days, as a line whose DayOpens are its cells'.
func Selectable(d Detail) []DetailLine {
	var out []DetailLine
	add := func(lines []DetailLine) {
		for _, l := range lines {
			if !l.Heading {
				out = append(out, l)
			}
		}
	}
	add(d.Lines)
	for _, p := range d.Parts {
		if p.Open != "" {
			out = append(out, DetailLine{Name: p.Title, Open: p.Open})
		}
		if p.Days != nil {
			out = append(out, DetailLine{Name: p.Title, DayOpens: p.Days.Opens})
		}
		add(p.Lines)
	}
	return out
}

// MarkedDay is the cell a strip's Day picks among n: -1, or past the end, is
// the last.
func MarkedDay(day, n int) int {
	if day < 0 || day >= n {
		return n - 1
	}
	return day
}

// dayCells are a strip as a terminal draws it: three characters a day, the
// glyph between spaces, the day marked between brackets and the day whose
// page is open between parentheses; and under it the days' initials.
func dayCells(s DayStrip, marked int) (cells, initials string) {
	glyphs := []rune(s.Text())
	var c, i strings.Builder
	for n, g := range glyphs {
		open, shut := " ", " "
		switch n {
		case marked:
			open, shut = "[", "]"
		case s.Selected:
			open, shut = "(", ")"
		}
		c.WriteString(open + string(g) + shut)
		label := " "
		if n < len(s.Labels) {
			label = s.Labels[n]
		}
		i.WriteString(" " + label + " ")
	}
	return c.String(), i.String()
}

/*
RenderDetail is a detail as a terminal's drawer, at most height lines of at
most width characters: its heading, the switch, the filter, its meters, the
summary where the list keeps SummaryKeepsLines lines under it (spec 094), the
heading line over as many columns as the width fits whole (spec 092), the
list cut to the height, and its reasons and note. A detail of parts is its
parts one after another, scrolled.
*/
func RenderDetail(d Detail, width, height int, s DrawerShape) []string {
	if width < 1 || height < 1 {
		return nil
	}
	d = d.printable()
	head := d.Title
	if row := JoinedRow(d.TitleParts); row.Value != "" {
		head += "  " + strings.TrimRight(row.Value, " ")
	}
	if s.Paused {
		head = padRight(head, width-len("paused")) + "paused"
	}
	out := []string{head}
	if sw := switchLine(s); sw != "" {
		out = append(out, sw)
	}
	if d.Filterable {
		out = append(out, filterLine(s))
	}
	for _, m := range d.Meters {
		out = append(out, meterLine(m, width))
	}
	if len(d.Parts) > 0 {
		out = append(out, partsLines(d, width, height-len(out), s)...)
		return clip(out, width, height)
	}

	tail := tailText(d)
	summary := summaryText(d.Summary, width)
	figure := FigureWidth(d.Lines)
	cells := columnsFit(d, figure, width)
	heads := cells > 0 && len(d.Lines) > 0
	room := func() int {
		n := height - len(out) - len(tail)
		if heads {
			n--
		}
		return n
	}
	// All of the summary or none of it, where the list, its reasons and its
	// note keep their lines under it: the window's rule, by the height.
	if len(summary) > 0 && height-len(out)-len(summary)-boolInt(heads) >= SummaryKeepsLines {
		out = append(out, summary...)
		if d.Summary.Covers {
			d.Note = ""
			tail = tailText(d)
		}
	}
	// A heading line over no line of the list names columns nobody sees.
	if heads && room() < 1 {
		cells, heads = 0, false
	}
	if heads {
		out = append(out, "  "+padRight("", DetailNameWidth)+"  "+ColumnHeads(figure, d.Columns, cells))
	}
	// The heading line is drawn now, and the list has what is left.
	fits := height - len(out) - len(tail)
	marked := 0
	for _, l := range d.Lines[:min(len(d.Lines), max(fits, 0))] {
		out = append(out, lineText(l, figure, cells, !l.Heading && marked == s.Marked))
		if !l.Heading {
			marked++
		}
	}
	out = append(out, tail...)
	return clip(out, width, height)
}

// switchLine is the lists a person can switch between, the shown one in
// brackets; none where there is nothing to choose.
func switchLine(s DrawerShape) string {
	if len(s.Choices) < 2 {
		return ""
	}
	parts := make([]string, len(s.Choices))
	for i, c := range s.Choices {
		parts[i] = " " + c.Label + " "
		if c.ID == s.Shown {
			parts[i] = "[" + c.Label + "]"
		}
	}
	return strings.Join(parts, " ")
}

// filterLine is the filter as typed, a cursor while it is being typed, or
// the key that starts it.
func filterLine(s DrawerShape) string {
	switch {
	case s.Filtering:
		return "/" + s.Filter + "▏"
	case s.Filter != "":
		return "/" + s.Filter
	default:
		return "f filter"
	}
}

// meterLine is a meter as its name, a bar of block characters and its
// caption.
func meterLine(m Meter, width int) string {
	caption := strings.TrimSpace(m.Caption)
	bar := max(width-2-DetailNameWidth-2-runeLen(caption)-2, 4)
	full := int(min(max(m.Fraction, 0), 1)*float64(bar) + 0.5)
	return "  " + padRight(truncate(m.Name(), DetailNameWidth), DetailNameWidth) + "  " +
		strings.Repeat("█", full) + strings.Repeat("░", bar-full) + "  " + caption
}

// summaryText is a summary's lines: its meters, its plot a line per trace,
// its row of cells, its lines.
func summaryText(s *Summary, width int) []string {
	if s == nil {
		return nil
	}
	var out []string
	for _, m := range s.Meters {
		out = append(out, meterLine(m, width))
	}
	plot := max(width-2-DetailNameWidth-2, 1)
	for _, t := range s.Trails {
		// One trace is the meter's above it, and is not named again; two,
		// the disks' read and write, are each named.
		name := ""
		if len(s.Trails) > 1 {
			name = t.Name
		}
		out = append(out, "  "+padRight(truncate(name, DetailNameWidth), DetailNameWidth)+"  "+
			Sparkline(t.Samples, min(len(t.Samples), plot), SparkMinSpan))
	}
	if s.Heat != nil {
		out = append(out, "  "+padRight(s.Heat.Name, DetailNameWidth)+"  "+s.Heat.Text())
	}
	for _, l := range s.Lines {
		out = append(out, lineText(l, 0, 0, false))
	}
	return out
}

// columnsFit is how many of the detail's columns fit the width whole, left
// to right (spec 092).
func columnsFit(d Detail, figure, width int) int {
	if len(d.Columns) == 0 || figure == 0 {
		return 0
	}
	room := width - 2 - DetailNameWidth - 2
	n := 0
	for n < len(d.Columns) && ColumnsWidth(figure, d.Columns, n+1) <= room {
		n++
	}
	return n
}

// lineText is one line of a list: a mark where it is the marked one, its
// name, and its figure with its first cells cells.
func lineText(l DetailLine, figure, cells int, marked bool) string {
	lead := "  "
	if marked {
		lead = "› "
	}
	if l.Heading {
		return lead + l.Name
	}
	value := l.figure()
	if cells > 0 && len(l.Cells) > 0 {
		value = padRight(value, figure)
		for _, c := range l.Cells[:min(cells, len(l.Cells))] {
			value += strings.Repeat(" ", FigureGap) + c
		}
	}
	return lead + padRight(truncate(l.Name, DetailNameWidth), DetailNameWidth) + "  " + value
}

// tailText is a detail's reasons, then its note.
func tailText(d Detail) []string {
	var out []string
	for _, r := range d.Reasons {
		line := r.Text
		if r.Label != "" {
			line = r.Label + ": " + line
		}
		out = append(out, "  "+line)
	}
	if d.Note != "" {
		out = append(out, "  "+d.Note)
	}
	return out
}

// partsLines are a detail's parts one after another, each under its title,
// scrolled by s.Scroll and cut to height, the marked line counted across them.
func partsLines(d Detail, width, height int, s DrawerShape) []string {
	var all []string
	marked := 0
	for i, p := range d.Parts {
		if i > 0 {
			all = append(all, "")
		}
		if p.Title != "" {
			head := p.Title
			if row := JoinedRow(p.TitleParts); row.Value != "" {
				head = padRight(head, max(width-runeLen(row.Value), runeLen(head)+2)) + row.Value
			}
			if p.Open != "" {
				head = markText(head, marked == s.Marked)
				marked++
			}
			all = append(all, head)
		}
		if p.Days != nil {
			day := -1
			if marked == s.Marked {
				day = MarkedDay(s.Day, len(p.Days.Values))
			}
			cells, initials := dayCells(*p.Days, day)
			all = append(all, markText(cells, marked == s.Marked), "  "+initials)
			marked++
		}
		for _, m := range p.Meters {
			all = append(all, meterLine(m, width))
		}
		for _, t := range p.Trails {
			all = append(all, "  "+padRight(truncate(t.Name, DetailNameWidth), DetailNameWidth)+"  "+
				Sparkline(t.Samples, min(len(t.Samples), detailTrailWidth), SparkMinSpan))
		}
		figure := FigureWidth(p.Lines)
		cells := columnsFit(p, figure, width)
		if cells > 0 && len(p.Lines) > 0 {
			all = append(all, "  "+padRight("", DetailNameWidth)+"  "+ColumnHeads(figure, p.Columns, cells))
		}
		for _, l := range p.Lines {
			if l.Wide {
				all = append(all, wideText(l, width, !l.Heading && marked == s.Marked))
				if !l.Heading {
					marked++
				}
				continue
			}
			all = append(all, lineText(l, figure, cells, !l.Heading && marked == s.Marked))
			if !l.Heading {
				marked++
			}
		}
		all = append(all, tailText(p)...)
	}
	start := min(max(s.Scroll, 0), max(len(all)-height, 0))
	return all[start:min(len(all), start+max(height, 0))]
}

// clip cuts each line to width and the lines to height.
func clip(lines []string, width, height int) []string {
	lines = lines[:min(len(lines), height)]
	for i, l := range lines {
		lines[i] = truncate(strings.TrimRight(l, " "), width)
	}
	return lines
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// markText is a part's own line, marked as a list's line is.
func markText(text string, marked bool) string {
	if marked {
		return "› " + text
	}
	return "  " + text
}

// wideText is a wide line (spec 102): its name takes what its value leaves.
func wideText(l DetailLine, width int, marked bool) string {
	value := l.Value
	room := max(width-2-runeLen(value)-2, 1)
	return markText(padRight(truncate(l.Name, room), room)+"  "+value, marked)
}
