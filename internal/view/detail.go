package view

import (
	"sort"
	"strings"
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
	return len(d.Lines) == 0 && len(d.Meters) == 0 && len(d.Reasons) == 0 && d.Note == "" && len(d.Parts) == 0
}

// Text is the detail as plain lines, for the command line: the heading, the
// meters, the lines, the note, and each reason with its detail, then each
// part the same way under a blank line.
func (d Detail) Text() string {
	var b strings.Builder
	b.WriteString(d.Title + "\n")
	for _, m := range d.Meters {
		b.WriteString("  " + strings.TrimRight(padRight(truncate(m.Name(), MeterNameWidth), MeterNameWidth)+" "+m.Caption+"  "+m.Reset, " ") + "\n")
	}
	for _, l := range d.Lines {
		if l.Heading {
			b.WriteString("  " + l.Name + "\n")
			continue
		}
		value := l.Value
		if l.Unit != "" {
			value += " " + l.Unit
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
}

// The process lists, by the detail's ID (rule 2 of docs/architecture.md: a
// table, not a switch).
const (
	DetailCPU       = "cpu"
	DetailGPU       = "gpu"
	DetailMemory    = "memory"
	DetailProcesses = "processes"
	DetailPeers     = "peers"
)

var processKinds = map[string]processKind{
	DetailCPU:    {title: "CPU by process", label: "CPU", key: "c", format: percentLine, order: first},
	DetailGPU:    {title: "GPU by process", label: "GPU", key: "g", format: percentLine, order: first},
	DetailMemory: {title: "Memory by process", label: "Mem", key: "m", format: sizeLine, order: first},
	DetailProcesses: {title: "Bandwidth by process", format: ratesLine,
		order: func(p ProcessUse) float64 { return p.Value + p.Second }},
	DetailPeers: {title: "Bandwidth by peer", label: "Peers", key: "p", format: ratesLine,
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
		s, ok := sums[name]
		if !ok {
			s = &ProcessUse{Name: name}
			sums[name] = s
			names = append(names, name)
		}
		s.Value += u.Value
		s.Second += u.Second
	}
	sort.Slice(names, func(i, j int) bool {
		a, b := kind.order(*sums[names[i]]), kind.order(*sums[names[j]])
		if a != b {
			return a > b
		}
		return names[i] < names[j]
	})
	for _, name := range names {
		line := kind.format(*sums[name])
		line.Name = truncate(name, DetailNameWidth)
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
