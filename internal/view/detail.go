package view

import (
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/ushineko/hayami/internal/safetext"
)

/*
Detail is what a section says when it is asked for more: the processes behind
a rate, the limits behind a meter (spec 060). It is data, as a Section is, and
neither shell decides what it says: the window draws it in a drawer beside
the cards and the command line prints it.

A detail has lines or meters or both. Lines are a list, greatest first, each a
name and a value formatted to a fixed width, so a new reading moves nothing;
meters are quotas, drawn as a card draws them. A shell shows as many lines as
it has room for and never scrolls, so the order is the content.
*/
type Detail struct {
	// Section is the key of the section the detail belongs to, and ID names
	// it within that section: "cpu", "memory", "processes", an account.
	Section string `json:"section"`
	ID      string `json:"id"`

	// Title is the heading: "CPU by process".
	Title string `json:"title"`

	Meters []Meter      `json:"meters,omitempty"`
	Lines  []DetailLine `json:"lines,omitempty"`

	// Summary is the part as a whole, over the list (spec 094); nil for a
	// detail without one, which is every list but the Memory drawer's
	// and every drill page.
	Summary *Summary `json:"summary,omitempty"`

	// Columns are the extra columns the lines carry cells for, in the order
	// a narrowing shell drops them, last first (spec 092). A shell shows as
	// many as its width fits; the command line prints them all.
	Columns []Column `json:"columns,omitempty"`

	// Trails are a plot over the lines, against one scale, for a detail that is
	// a part of another: each VPN's own.
	Trails []Trail `json:"trails,omitempty"`

	// TrailEvery is the time expected between the trails' samples, over which a
	// new one drifts into place (spec 086). Zero draws it in one step.
	TrailEvery time.Duration `json:"-"`

	// TitleParts are a value on the heading's line, for a part that says the
	// one thing a reader looks for next to its name: a VPN's state.
	TitleParts []Part `json:"title_parts,omitempty"`

	// Filterable marks a list a person narrows by name: the process lists.
	// A breakdown of a quota is short and is not.
	Filterable bool `json:"filterable,omitempty"`

	// Note is one quiet line under the list: what the list does not count,
	// or the total it is a share of.
	Note string `json:"note,omitempty"`

	// Reasons are what the detail could not read, and why, as a section's
	// are.
	Reasons []Reason `json:"reasons,omitempty"`

	// Parts are the details a detail is made of, each under its own Title,
	// for a section that shows everything it has at once: the usage of every
	// account (spec 062). A detail with parts has nothing else of its own
	// but its heading, and a shell that cannot fit them scrolls.
	Parts []Detail `json:"parts,omitempty"`

	// Open is what a click on the heading asks for, as a line's Open: an
	// account's own page, a forge read again (spec 102). Empty for a
	// heading that is only read.
	Open string `json:"-"`

	// Days is a row of cells under the heading, each a choice: a forge's
	// fortnight, a day each (spec 102). Nil for every other detail.
	Days *DayStrip `json:"days,omitempty"`

	// Controls are lines a shell that can act may offer, each with what a press
	// on it asks for in Open. They are in no text and no JSON, and only the
	// desktop panel reads them (spec 085 R20).
	Controls []DetailLine `json:"-"`
}

// DetailLine is one line of a detail: a name at the left and a value at the
// right, as a Row is, or a heading over the lines that follow it.
type DetailLine struct {
	// Name is what the line is about, cut to DetailNameWidth. For a process
	// it is the program's name and nothing else about it (spec 060 R6).
	Name string `json:"name"`

	// Value and Unit are the reading at a fixed width, apart for the reason
	// a Row keeps them apart.
	Value string `json:"value,omitempty"`
	Unit  string `json:"unit,omitempty"`

	// Parts colour the value piece by piece, as a Row's do; joined they are
	// Value.
	Parts []Part `json:"parts,omitempty"`

	Status Status `json:"status,omitempty"`

	// Heading marks a line that names the lines under it and has no value.
	Heading bool `json:"heading,omitempty"`

	// DayOpens are a strip's cells' details, on the line a terminal marks
	// for the strip (Selectable); nil on every other line.
	DayOpens []string `json:"-"`

	// Wide marks a line whose name is a title and may take the room its
	// short value leaves, rather than DetailNameWidth: a pull request on a
	// day's page (spec 102). Its name is cut to DevWideNameWidth.
	Wide bool `json:"wide,omitempty"`

	// Cells are the line's extra figures, one under each of its detail's
	// Columns and at its width (spec 092).
	Cells []string `json:"cells,omitempty"`

	// Open is what a click on the line asks for: the ID of the detail it
	// leads to, or one of the Act names below. Empty for a line that is
	// only to be read, which is most of them.
	Open string `json:"open,omitempty"`
}

// The actions a detail line can carry in Open, as against a detail to open.
// They are named and not a second field because a line does one thing when
// it is clicked, and a line that could both navigate and act would have to
// say which it meant.
const (
	ActPause   = "!pause"
	ActBack    = "!back"
	ActKill    = "!kill"
	ActConfirm = "!kill-yes"
	ActCancel  = "!kill-no"
)

// DetailNameWidth is the width a detail line's name is cut to. A drawer is a
// fixed number of ems wide and the widest value, two rates, takes most of it.
const DetailNameWidth = 20

// DetailInfo is a detail a source offers: its ID within the section and the
// heading it would carry.
type DetailInfo struct {
	ID    string `json:"id"`
	Title string `json:"title"`

	// Label and Key are how a person chooses this detail among a section's
	// others once it is shown (spec 062): a word for a switch, "CPU", and the
	// one character that selects it. Empty for a detail that is not one of a
	// choice: a section's only detail, or one a shell shows as part of
	// another.
	Label string `json:"label,omitempty"`
	Key   string `json:"key,omitempty"`

	// OnRequest keeps a detail out of `readings` and `doctor`, which are what
	// gets pasted into a bug report: it is read only when a person opens it, or
	// names it to `detail`. A VPN's addresses (spec 085 R9).
	OnRequest bool `json:"-"`
}

// JoinedRow is parts as the value of a row.
func JoinedRow(parts []Part) Row {
	row := Row{Parts: parts}
	for _, p := range parts {
		row.Value += p.Text
	}
	return row
}

// Matching is the detail with only the lines keep passes, headings kept. It
// is what a filter leaves: the list is narrowed before it is cut to a height,
// so a name far down a long list can be found.
func (d Detail) Matching(keep func(name string) bool) Detail {
	out := d
	out.Lines = nil
	for _, l := range d.Lines {
		if l.Heading || keep(l.Name) {
			out.Lines = append(out.Lines, l)
		}
	}
	return out
}

// Top is the detail with at most n lines: the list as long as a height fits.
func (d Detail) Top(n int) Detail {
	if n < 0 {
		n = 0
	}
	if len(d.Lines) > n {
		d.Lines = d.Lines[:n]
	}
	return d
}

// Quiet reports whether a detail has nothing at all to draw.
func (d Detail) Quiet() bool {
	return len(d.Trails) == 0 && len(d.Lines) == 0 && len(d.Meters) == 0 && len(d.Reasons) == 0 && d.Note == "" && len(d.Parts) == 0
}

// printable is the detail with every string a terminal would act on made
// safe: the names in a detail are processes', peers' and providers', which
// hayami does not choose (spec 076).
func (d Detail) printable() Detail {
	p := func(s string) string { return safetext.Printable(s, 0) }
	d.Title, d.Note = p(d.Title), p(d.Note)
	d.Trails = slices.Clone(d.Trails)
	for i := range d.Trails {
		d.Trails[i].Name = p(d.Trails[i].Name)
	}
	meters := make([]Meter, len(d.Meters))
	for i, m := range d.Meters {
		m.Label, m.Caption, m.Reset = p(m.Label), p(m.Caption), p(m.Reset)
		meters[i] = m
	}
	d.Meters = meters
	d.Lines = printableLines(d.Lines, p)
	if d.Summary != nil {
		s := *d.Summary
		s.Meters, s.Lines = make([]Meter, len(d.Summary.Meters)), printableLines(d.Summary.Lines, p)
		for i, m := range d.Summary.Meters {
			m.Label, m.Caption = p(m.Label), p(m.Caption)
			s.Meters[i] = m
		}
		d.Summary = &s
	}
	reasons := make([]Reason, len(d.Reasons))
	for i, r := range d.Reasons {
		r.Label, r.Text, r.Detail = p(r.Label), p(r.Text), p(r.Detail)
		reasons[i] = r
	}
	d.Reasons = reasons
	titleParts := make([]Part, len(d.TitleParts))
	for i, tp := range d.TitleParts {
		tp.Text = p(tp.Text)
		titleParts[i] = tp
	}
	d.TitleParts = titleParts
	// A detail's parts are made safe as it is: a forge's day puts titles
	// other people wrote into them (spec 102 review, F1), and a shell that
	// draws the parts must not be handed what a terminal would obey.
	if len(d.Parts) > 0 {
		parts := make([]Detail, len(d.Parts))
		for i, part := range d.Parts {
			parts[i] = part.printable()
		}
		d.Parts = parts
	}
	return d
}

// printableLines is a copy of lines with every string made safe by p.
func printableLines(in []DetailLine, p func(string) string) []DetailLine {
	lines := make([]DetailLine, len(in))
	for i, l := range in {
		l.Name, l.Value, l.Unit = p(l.Name), p(l.Value), p(l.Unit)
		l.Cells = slices.Clone(l.Cells)
		for j := range l.Cells {
			l.Cells[j] = p(l.Cells[j])
		}
		lines[i] = l
	}
	return lines
}

// Text is the detail as plain lines, for the command line: the heading, the
// meters, the lines, the note, and each reason with its detail, then each
// part the same way under a blank line.
func (d Detail) Text() string {
	d = d.printable()
	var b strings.Builder
	b.WriteString(d.Title)
	if row := JoinedRow(d.TitleParts); row.Value != "" {
		b.WriteString("  " + strings.TrimRight(row.Value, " "))
	}
	b.WriteString("\n")
	for _, m := range d.Meters {
		b.WriteString("  " + strings.TrimRight(padRight(truncate(m.Name(), MeterNameWidth), MeterNameWidth)+" "+m.Caption+"  "+m.Reset, " ") + "\n")
	}
	for _, t := range d.Trails {
		if len(t.Samples) > 0 {
			b.WriteString("  " + padRight(truncate(t.Name, DetailNameWidth), DetailNameWidth) + "  " +
				Sparkline(t.Samples, min(len(t.Samples), detailTrailWidth), SparkMinSpan) + "\n")
		}
	}
	// The summary is printed whole, above the list's heading line (spec 094
	// R9): a terminal has the room a drawer may not.
	if s := d.Summary; s != nil {
		for _, m := range s.Meters {
			b.WriteString("  " + strings.TrimRight(padRight(truncate(m.Name(), DetailNameWidth), DetailNameWidth)+"  "+m.Caption, " ") + "\n")
		}
		for _, t := range s.Trails {
			if len(t.Samples) > 0 {
				b.WriteString("  " + padRight("", DetailNameWidth) + "  " +
					Sparkline(t.Samples, min(len(t.Samples), detailTrailWidth), SparkMinSpan) + "\n")
			}
		}
		if s.Heat != nil {
			b.WriteString("  " + padRight(s.Heat.Name, DetailNameWidth) + "  " + s.Heat.Text() + "\n")
		}
		for _, l := range s.Lines {
			b.WriteString("  " + strings.TrimRight(padRight(l.Name, DetailNameWidth)+"  "+l.figure(), " ") + "\n")
		}
	}
	// A strip of days is printed with its initials under it (spec 102 R14).
	if d.Days != nil {
		cells, initials := dayCells(*d.Days, -1)
		b.WriteString("  " + cells + "\n  " + strings.TrimRight(initials, " ") + "\n")
	}
	// Every column is printed: a terminal's line is as long as it needs
	// (spec 092 R9), and the heading over them names them.
	figure := FigureWidth(d.Lines)
	if len(d.Columns) > 0 && figure > 0 {
		b.WriteString("  " + strings.TrimRight(padRight("", DetailNameWidth)+"  "+ColumnHeads(figure, d.Columns, len(d.Columns)), " ") + "\n")
	}
	for _, l := range d.Lines {
		if l.Heading {
			b.WriteString("  " + l.Name + "\n")
			continue
		}
		value := l.figure()
		if len(l.Cells) > 0 {
			value = padRight(value, figure)
			for _, c := range l.Cells {
				value += strings.Repeat(" ", FigureGap) + c
			}
		}
		b.WriteString("  " + strings.TrimRight(padRight(l.Name, DetailNameWidth)+"  "+value, " ") + "\n")
	}
	if d.Note != "" {
		b.WriteString("  " + d.Note + "\n")
	}
	for _, r := range d.Reasons {
		line := r.Text
		if r.Label != "" {
			line = r.Label + ": " + line
		}
		b.WriteString("  " + line + "\n")
		if r.Detail != "" {
			b.WriteString("    " + r.Detail + "\n")
		}
	}
	for _, part := range d.Parts {
		b.WriteString("\n" + part.Text())
	}
	return b.String()
}

// MeterNameWidth is the width a meter's name takes in a detail's text.
const MeterNameWidth = 16

// ProcessUse is one process's share of something, measured and not yet
// formatted. It carries a name and figures and nothing else: no command
// line, no path, no owner (spec 060 R6).
type ProcessUse struct {
	Name string

	// Value is what the list is ordered by. Second is the other figure of a
	// list with two, the up rate beside the down.
	Value  float64
	Second float64

	// Count is how many processes the use is, once uses of one name are
	// summed; zero for a use read from one process, which is one.
	Count int

	// CPU, Memory and Share are the use's other figures, for a list's
	// columns (spec 092): the share of the processors, the bytes held, and
	// those bytes as a percentage of all memory. Each is absent where it
	// was not read.
	CPU, Memory, Share Opt[float64]
}

// processKind is how one kind of process list is drawn.
type processKind struct {
	title string
	// label and key are how the list is chosen in a switch; empty for a
	// list that is its section's only one.
	label, key string
	// format is a summed process as a line's value.
	format func(ProcessUse) DetailLine
	// order is what the list is sorted by, greatest first.
	order func(ProcessUse) float64
	// columns are the list's extra columns, in the order a narrowing
	// drawer drops them, last first (spec 092).
	columns []string
	// figure is the width of a line's figure, value and unit, for a list
	// with columns (spec 093).
	figure int
}

// The process lists, by the detail's ID (rule 2 of docs/architecture.md: a
// table, not a switch).
const (
	DetailCPU       = "cpu"
	DetailGPU       = "gpu"
	DetailMemory    = "memory"
	DetailIO        = "io"
	DetailProcesses = "processes"
	DetailPeers     = "peers"
)

var processKinds = map[string]processKind{
	DetailCPU: {title: "CPU by process", label: "CPU", key: "c", format: percentLine, order: first,
		columns: []string{ColumnCount, ColumnMemory, ColumnShare}, figure: percentWidth},
	DetailGPU: {title: "GPU by process", label: "GPU", key: "g", format: percentLine, order: first,
		columns: []string{ColumnCount, ColumnCPU, ColumnMemory}, figure: percentWidth},
	DetailMemory: {title: "Memory by process", label: "Mem", key: "m", format: sizeLine, order: first,
		columns: []string{ColumnCount, ColumnCPU, ColumnShare}, figure: sizeWidth},
	// The IO list's figure is two rates, wider than the others' one, so it
	// carries the count alone and the drawer is the width the other three
	// want (spec 097 R2).
	DetailIO: {title: "IO by process", label: "IO", key: "i", format: ioLine,
		order:   func(p ProcessUse) float64 { return p.Value + p.Second },
		columns: []string{ColumnCount}, figure: ioWidth},
	// Both of the bandwidth lists carry a label, because a switch is drawn
	// only from the choices that have one and only when the open detail is
	// among them (gui.choices). With one of the two unlabelled there was no
	// switch at all, and the peer breakdown could not be reached from the
	// panel: it existed, and nothing led to it.
	DetailProcesses: {title: "Bandwidth by process", label: "Procs", key: "p", format: ratesLine,
		order: func(p ProcessUse) float64 { return p.Value + p.Second }},
	DetailPeers: {title: "Bandwidth by peer", label: "Peers", key: "e", format: ratesLine,
		order: func(p ProcessUse) float64 { return p.Value + p.Second }},
}

func first(p ProcessUse) float64 { return p.Value }

// ProcessTitle is the heading of a process list.
func ProcessTitle(id string) string { return processKinds[id].title }

// ProcessInfo is a process list as a source offers it: its heading, and the
// label and key it is chosen by where its section has more than one.
func ProcessInfo(id string) DetailInfo {
	kind := processKinds[id]
	return DetailInfo{ID: id, Title: kind.title, Label: kind.label, Key: kind.key}
}

// OtherProcesses is the name of the line for what was measured and could not
// be put to a process.
const OtherProcesses = "other"

/*
Processes turns processes into a list: one line per name, the figures of
processes that share a name summed, greatest first (spec 060 R5, R6).

A browser is forty processes and a person asks about the browser. The name is
all that is kept of a process; ties are broken by name so two idle programs do
not swap places between polls.
*/
func Processes(section, id string, uses []ProcessUse) Detail {
	kind := processKinds[id]
	d := Detail{Section: section, ID: id, Title: kind.title, Filterable: true}

	sums := map[string]*ProcessUse{}
	var names []string
	for _, u := range uses {
		name := u.Name
		if name == "" {
			name = OtherProcesses
		}
		u.Name = name
		s, ok := sums[name]
		if !ok {
			sums[name] = &u
			names = append(names, name)
			continue
		}
		*s = s.plus(u)
	}
	sort.Slice(names, func(i, j int) bool {
		a, b := kind.order(*sums[names[i]]), kind.order(*sums[names[j]])
		if a != b {
			return a > b
		}
		return names[i] < names[j]
	})
	d.Columns = OfferColumns(id, uses, true)
	for _, name := range names {
		line := kind.format(*sums[name])
		line.Name = truncate(name, DetailNameWidth)
		line.Cells = Cells(d.Columns, *sums[name])
		d.Lines = append(d.Lines, line)
	}
	return d
}

// percentLine is a share of the whole machine, or of a card: " 12.3 %".
func percentLine(p ProcessUse) DetailLine {
	return DetailLine{Value: Quantity(min(max(p.Value, 0), 999.9)), Unit: "%"}
}

// sizeLine is a byte count: " 1.2 GiB".
func sizeLine(p ProcessUse) DetailLine {
	n, u := Size(p.Value)
	return DetailLine{Value: n, Unit: PadUnit(u, UnitWidth(byteUnits...))}
}

// ioLine is a read and a write rate on one line, as ratesLine draws a
// network's, in the plain colour of a fact: a disk has no band a rate is a
// verdict on.
func ioLine(p ProcessUse) DetailLine {
	width := bandwidthUnitWidth()
	parts := []Part{
		{Text: "↓ ", Status: Info},
		{Text: rate(p.Value, true, width), Status: Info},
		{Text: " ↑ ", Status: Info},
		{Text: rate(p.Second, true, width), Status: Info},
	}
	line := DetailLine{Parts: parts}
	for _, part := range parts {
		line.Value += part.Text
	}
	return line
}

// ioWidth is the width of an IO line's figure.
var ioWidth = runeLen(ioLine(ProcessUse{}).Value)

// ratesLine is a down and an up rate on one line, each in its own band, as an
// interface's row draws them and closer together: a drawer is narrower than
// the two would like.
func ratesLine(p ProcessUse) DetailLine {
	width := UnitWidth("B/s", "KiB/s", "MiB/s", "GiB/s", "TiB/s")
	parts := []Part{
		{Text: "↓", Status: Info},
		{Text: rate(p.Value, true, width), Status: RateBand(p.Value, true)},
		{Text: " ↑", Status: Info},
		{Text: rate(p.Second, true, width), Status: RateBand(p.Second, true)},
	}
	line := DetailLine{Parts: parts}
	for _, part := range parts {
		line.Value += part.Text
	}
	return line
}

/*
DrillLine is one line of a drill-down, drawn in the units of the list it
was reached through.

The formatter is the list's own (processKinds), so a process's share of the
processors is a percentage on every page it appears on and a byte count is
a byte count. A drill that formatted its own figures would be a second
place for the units to be decided, and the two would drift.

Indent shows depth in a model whose lines are flat, by padding the name. A
later form would carry it on DetailLine so a shell could lay it out.
*/
func DrillLine(detail, name string, indent int, u ProcessUse) DetailLine {
	kind, ok := processKinds[detail]
	if !ok || kind.format == nil {
		return DetailLine{Name: strings.Repeat(" ", indent) + name}
	}
	line := kind.format(u)
	line.Name = strings.Repeat(" ", indent) + name
	return line
}

// PeerLine is one line of the peer breakdown: a process, or a peer indented
// under it, or the remainder. Indent shows depth by padding the name.
func PeerLine(name string, indent int, rx, tx float64) DetailLine {
	line := ratesLine(ProcessUse{Value: rx, Second: tx})
	line.Name = strings.Repeat(" ", indent) + name
	return line
}
