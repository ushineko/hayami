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
}

// Trail is one series plotted under a section's rows.
//
// **Each trail is scaled to its own range**, never to a shared axis. The
// cooler is the case that settled it: the processor swings thirty-five degrees
// where the coolant moves under one, so a shared degrees-Celsius axis flattens
// the coolant to a couple of pixels and destroys the signal the plot exists
// for. The consequence is worth being explicit about -- heights are not
// comparable between trails -- and it is why the real numbers are in the rows
// above and the plot carries no axis at all.
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
)

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
	// under a rate. Empty means there is none.
	Detail string
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

	// Gone marks a section whose source was answering and has stopped. Its
	// rows keep their last values and are drawn dim, because the reader's
	// question is whether they are still true.
	Gone bool
}
