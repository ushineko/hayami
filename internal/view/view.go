/*
Package view describes a section. It does not draw one.

A section is the unit both shells render: a title and some rows, each row a
label, a value, a unit and a verdict. The window turns it into cards and the
terminal turns it into text, and neither decides what a section *says* — that
is here, once, which is what makes the two shells comparable.

Nothing in this package imports a toolkit. The terminal panel builds with cgo
off and must keep doing so.
*/
package view

import "strings"

/*
Cell is one device as a block rather than as a line: a name, a reading, and
what the thing is doing.

The shape the monitor draws and the one this program went without for a while.
A row puts the name at the left and the percentage at the right margin in the
same weight, which reads as a list of facts; a cell puts the percentage in the
middle and large, with the name over it and the state under it, and the
percentage is what the eye lands on. For a battery that is the right answer,
because the number *is* the reading and the name is only which one.

What the row form got right is kept: a cell per device, appearing and
disappearing with the hardware, rather than the monitor's two fixed slots with
a submenu each. That was an artefact of a panel 260 pixels wide and is not
something a reader ever asked for.
*/
type Cell struct {
	// Label is the device's own name, over the reading.
	Label string

	// Value and Unit are the reading, kept apart for the reason a Row keeps
	// them apart: they are aligned separately.
	Value string
	Unit  string

	Status Status

	// Note is the quiet line under the reading: "Discharging", "Charging",
	// "Wired", "Disconnected", or the separate batteries of a device that has
	// more than one.
	//
	// **Always said.** The row form said it only for a battery that was
	// charging, which left the ordinary case -- a battery discharging
	// normally -- looking the same as a device nobody had heard from. The
	// state is a third of what a cell is for.
	Note string

	// Stale marks a last-known value whose source has stopped answering. It
	// dims the cell; it does not blank it and it does not drop the verdict.
	//
	// This is the design system's own degradation model
	// (glance.Reading.Stale): a value that was read and whose source has gone
	// stays legible and stops shouting. The monitor does the same thing with
	// a second, darker palette and the word "(Offline)" under the number.
	Stale bool

	// Placeholder marks a cell with no reading behind it: a slot the section
	// always draws so the card keeps its shape, with no device in it (spec
	// 022). It is drawn like any other cell and reported by nothing -- it is
	// not a reading and not a reason.
	Placeholder bool

	// Bar is the level as a fraction of full, drawn as a bar under the
	// state, and HasBar says there is one (spec 025). A percentage is a
	// number the eye has to read; a bar is a length it compares across the
	// card. Its colour is the cell's Status, dimmed with the cell.
	//
	// Only a reading with a level has one. A band already draws its level as
	// segments, and a bar under four segments would say it twice; a
	// placeholder has no level to draw.
	Bar    float64
	HasBar bool
}

// Trail is one series plotted under a section's rows.
//
// **How trails are scaled is the section's TrailScale.** By default each is
// scaled to its own range. The cooler is the case that settled it: the
// processor swings thirty-five degrees where the coolant moves under one, so a
// shared degrees-Celsius axis flattens the coolant to a couple of pixels and
// destroys the signal the plot exists for. The consequence is worth being
// explicit about -- heights are not comparable between trails -- and it is
// why the real numbers are in the rows above and the plot carries no axis at
// all. Bandwidth is the other case (spec 021): its trails share one scale, so
// a quiet interface is a flat line beside a busy one.
type Trail struct {
	// Name identifies the series to a shell that keeps its plot between
	// polls rather than rebuilding it.
	Name string

	// Samples are the readings, oldest first.
	Samples []float64

	// Status is the trail's colour. A secondary trace takes Info, which is
	// the muted one: a plot with two traces of equal weight has no primary,
	// and the coolant is what the eye should land on.
	Status Status
	// Series is which of a section's things this trail belongs to -- the
	// interface's ordinal on the bandwidth card -- so a shell can colour one
	// thing's trails as a pair. Zero for a section that does not use it.
	Series int

	// Secondary marks the second trail of a pair: the up rate beside the
	// down. A shell draws it in a fainter form of the pair's colour.
	Secondary bool

	// Coloured marks a trail that takes a series colour rather than a status
	// colour: Series picks it, Secondary fades it. Every bandwidth trail, and
	// the cooler's CPU and GPU, whose traces share a plot with the coolant
	// and are told apart by nothing else. The coolant is not coloured: its
	// colour is its band, which is a verdict.
	Coloured bool
}

// TrailScale is how a section's trails are fitted to the height of the plot.
type TrailScale int

const (
	// ScaleEach draws each trail against its own range, widened to
	// SparkMinSpan. The zero value, and the cooler's.
	ScaleEach TrailScale = iota
	// ScaleShared draws every trail of the section against one range: zero
	// at the bottom and the greatest sample across the trails at the top.
	// Heights are comparable between trails, which is the point for rates.
	ScaleShared
)

/*
Reason is why a section has nothing, or less than everything, to draw.

**A section that cannot be drawn must say so.** An absent card and an absent
cooler look identical, and on the machine that prompted this (issue #54) three
of the four sections were missing with nothing anywhere saying why -- not on
the card, not in `readings`, not in a log. The half-hour that cost is the whole
argument for this type.

Status carries the distinction a reader actually needs. Info is hardware this
machine does not have: stated plainly, dim, no alarm. Warn is a source that
tried and failed, which is a thing somebody may want to fix.

Detail is never drawn on the card. A card is read at a glance and an exit
status is not a glance; it is the hover note in the window and a line under its
reason in `doctor`.
*/
type Reason struct {
	// Label is the reading that is missing -- "Coolant", "CPU" -- or empty
	// for a line that runs across the section.
	Label string

	// Text is what the section says: "no cooler", "the cooler would not answer".
	Text string

	// Detail is the underlying error, for the tooltip and for doctor. Empty
	// for a reason that has nothing more to it, which is most of the Info
	// ones: "no Bluetooth adapter" is the whole story.
	Detail string

	// Status is Info for hardware that is not there and Warn for a source
	// that failed. Nothing here is Bad: a section that cannot be read is not
	// an emergency, and a panel that cried Bad over a missing headset would
	// be teaching its reader to ignore the colour.
	Status Status

	// Aside marks a reason that is true and not worth a line: it stays off
	// the card and the pane and is kept for the hover note and for doctor.
	//
	// For a device that is present and unreadable on a card already full of
	// devices that are not (issue #77). With two batteries drawing, a
	// headset this build cannot read is a footnote, and a line the width of
	// its name and a sentence set the width of the whole panel.
	Aside bool
}

// Status is a verdict on a reading. It is the same vocabulary the design
// system uses, kept here rather than imported so this package and the terminal
// panel stay free of Fyne.
type Status int

const (
	// Info is neutral: a fact with no verdict attached.
	Info Status = iota
	// Good is a positive verdict.
	Good
	// Warn deserves attention but not alarm.
	Warn
	// Bad is a failure.
	Bad

	// Accent and Strong are emphasis, not verdicts (spec 031): a reading
	// that is large enough to be worth finding at a glance, and is not wrong
	// for being large. A bandwidth rate is the case: 80 MiB/s is a file
	// arriving, not a fault, and drawing it in a verdict's colour would teach
	// the reader that a download is an alarm.
	//
	// Accent is drawn in the scheme's info colour -- the colour Info names
	// and does not draw on a row, where Info is plain text. Strong is the
	// strongest emphasis a shell has that is not the error colour, in bold
	// where the shell has bold. They come after Bad so that every status a
	// cache already holds keeps its number.
	Accent
	// Strong is the strongest emphasis: see Accent.
	Strong
)

// Part is one piece of a row's value with a status of its own.
//
// For a value that carries two readings with two verdicts: an interface's
// down and up rates on one line (spec 031). The texts are the same padded
// strings the value is made of, so colouring them moves nothing.
type Part struct {
	Text   string
	Status Status
}

// Row is one label-and-value line.
//
// Value and Unit are separate because they are aligned separately: the value
// right-aligned in a fixed column and the unit left-aligned after it, which is
// what stops a column moving when a rate crosses from KiB/s to MiB/s. A view
// that carried "1.5 MiB/s" as one string could not do that.
type Row struct {
	Label  string
	Value  string
	Unit   string
	Status Status

	// Detail is a second, quieter line under the row — the cumulative totals
	// under a rate. Empty means there is none. More than one line is
	// separated by "\n", each drawn as the first is: a Wi-Fi interface's link
	// goes under its totals (spec 037).
	Detail string

	// Stale marks a row whose source missed this poll: the last value is
	// kept and drawn dim, and the rest of the section is not. The GPU row
	// when nvidia-smi does not answer in time (spec 026); a whole source that
	// has stopped is the section's Gone instead.
	Stale bool

	// Parts colour the value piece by piece, and are empty for a row whose
	// value takes Status whole, which is most of them. When there are parts
	// their texts joined are Value exactly: Value is still what every width
	// is measured from, and Parts only say how to colour it.
	Parts []Part `json:",omitempty"`

	// Tip is what the row says when the pointer rests on it: the full name a
	// label was shortened from (spec 031). A shell without a pointer prints
	// it where it has room for it -- doctor does -- or not at all.
	Tip string `json:",omitempty"`

	// LabelWidth is the width the label column keeps for this row whatever
	// its label says, for a label that can change while the panel is up: a
	// processor's name that arrives a poll after "CPU". Zero for a label that
	// is what it is.
	LabelWidth int `json:",omitempty"`
}

// DetailLines are the row's detail lines, in order, and none for a row
// without a detail.
func (r Row) DetailLines() []string {
	if r.Detail == "" {
		return nil
	}
	return strings.Split(r.Detail, "\n")
}

// Section is a titled group of rows and meters, which is what a card is in the
// window and a block is in the terminal.
//
// Rows and Meters are separate rather than one list of a common interface,
// because they are laid out differently in every arrangement: a row's value
// goes to the right edge and a meter's bar takes the width that is left. A
// section may hold either or both, and meters are drawn under the rows.
type Section struct {
	// Key names the section in the settings and on the command line. It is
	// stable; Title is not, and may be translated or may carry a device's
	// name.
	Key    string
	Title  string
	Rows   []Row
	Meters []Meter

	// Cells are readings drawn as blocks rather than as lines, laid out
	// across the width. A section has rows or cells; nothing so far has both,
	// and they are separate for the reason rows and meters are -- they are
	// laid out differently in every arrangement.
	Cells []Cell

	// Trails are the series to plot under the rows, oldest first within each.
	// Empty for a section with nothing to plot, which is most of them.
	Trails []Trail

	// TrailScale is how the trails share, or do not share, a vertical scale.
	TrailScale TrailScale

	// Note is what the section says when the pointer rests on it: detail
	// there is no room to draw. Empty for a section with nothing extra to
	// say, which is most of them.
	//
	// A shell that has no pointer -- the terminal panel -- ignores it, which
	// is the right answer rather than a gap: a note is for what did not fit,
	// and a terminal that cannot show it is no worse off than before.
	Note string

	// Icon names the glyph a shell draws before the title, from the set in
	// this package. Empty draws none.
	//
	// A name rather than an image, because this package describes a section
	// and does not know what a picture is: the window resolves it to a Fyne
	// resource and the pane ignores it, which is the same split every other
	// field here follows.
	Icon IconName

	// Restored marks a section drawn from the cache of what the panel last
	// knew, before this run has heard anything.
	//
	// Both shells draw it the way they draw Gone -- dim -- because the
	// meaning is the same: this is the last reading and not the current one.
	// It is a separate field rather than a reuse because the pane appends
	// "(unavailable)" to a Gone section, and that is the wrong thing to say
	// about a panel that has only just started. A restored section says
	// nothing extra; being dim is the whole of the message, and it stops
	// being dim as soon as a live poll lands.
	Restored bool

	// Gone marks a section whose source was answering and has stopped. Its
	// rows keep their last values and are drawn dim, because the reader's
	// question is whether they are still true.
	Gone bool

	// Reasons are what this section could not read, and why. They are drawn
	// dim, after the rows, by both shells, and a section that has reasons is
	// drawn even when it has no readings at all -- which is the point of
	// them.
	//
	// **Never cached.** A reason is a statement about this moment; restoring
	// "the cooler would not answer" from yesterday's file would be asserting a
	// failure nobody has observed. The tag is the enforcement, because the
	// cache is this struct encoded whole.
	Reasons []Reason `json:"-"`
}

/*
Lines are the section's rows followed by its reasons, which is what both shells
draw and the only form either of them should draw.

A reason is a line in the same column layout as a reading, because that is what
it stands in for: "Coolant — no cooler" sits where the coolant would have been
and is read the same way. What it is not is a reading, so an Info reason takes
Dim -- the status this package keeps for a thing that is not a measurement --
and a Warn one keeps its verdict, because a source that failed is the one case
here worth a colour.

Detail never reaches a line. It is the hover note in the window and a line
under its reason in doctor; a card is read at a glance and a subprocess's exit
status is not a glance.
*/
func (s Section) Lines() []Row {
	if len(s.Reasons) == 0 {
		return s.Rows
	}
	out := make([]Row, 0, len(s.Rows)+len(s.Reasons))
	out = append(out, s.Rows...)
	for _, r := range s.Reasons {
		if r.Aside {
			continue
		}
		status := r.Status
		if status != Warn && status != Bad {
			status = Dim
		}
		if r.Label == "" {
			// A reason that stands in for no particular reading is a
			// sentence, and a sentence reads from the left. The label column
			// is where a shell puts text; the value column is right-aligned
			// against the edge, which is correct for a number and wrong for
			// "no Bluetooth device with a battery".
			out = append(out, Row{Label: r.Text, Status: status})
			continue
		}
		out = append(out, Row{Label: r.Label, Value: r.Text, Status: status})
	}
	return out
}

// Hover is what a shell with a pointer shows on hover: the section's own note,
// then each reason that has a detail, one per line.
//
// The details live here rather than on the card because they are the answer to
// a question a reader only sometimes asks -- "why not?" -- and a panel that
// spent two lines on an exit status would be a panel about itself.
//
// The rows' tips come first, one per line: the full names of the parts the
// labels shortened (spec 031). The design system's card takes one tip for the
// whole card, so a row's tip is a line of the card's.
func (s Section) Hover() string {
	var parts []string
	for _, r := range s.Rows {
		if r.Tip != "" {
			parts = append(parts, r.Tip)
		}
	}
	if s.Note != "" {
		parts = append(parts, s.Note)
	}
	for _, r := range s.Reasons {
		if r.Detail == "" {
			continue
		}
		if r.Label != "" {
			parts = append(parts, r.Label+": "+r.Detail)
			continue
		}
		parts = append(parts, r.Detail)
	}
	return strings.Join(parts, "\n")
}

// Quiet reports whether a section has nothing to draw at all: no readings and
// no reason for having none. That is a section a shell leaves out, and it is
// the only one -- an unconfigured section is not a fault and says nothing.
func (s Section) Quiet() bool {
	return len(s.Rows) == 0 && len(s.Cells) == 0 && len(s.Meters) == 0 && len(s.Reasons) == 0
}

// Dimmed reports whether a section's readings are the last ones heard rather
// than current ones — a source that has stopped answering, or a reading
// restored from the cache before this run has heard anything.
//
// Both shells draw it the same way, because it means the same thing.
func (s Section) Dimmed() bool { return s.Gone || s.Restored }

/*
IconName is the glyph a section asks for.

A closed set, not a free string: a shell has to turn it into something it can
draw, and a name nothing recognises is a card with a hole where the icon
should be. Adding one here means adding it to both shells, which is the point.
*/
type IconName string

// The glyphs the sections use.
const (
	// IconNone is no glyph, and the zero value: a section that says nothing
	// about an icon gets none.
	IconNone IconName = ""
	// IconPeripherals is a battery: the reading the section is about.
	IconPeripherals IconName = "peripherals"
	// IconBandwidth is a network.
	IconBandwidth IconName = "bandwidth"
	// IconCooler is a temperature.
	IconCooler IconName = "cooler"
	// IconUsage is a quota being spent.
	IconUsage IconName = "usage"
)
