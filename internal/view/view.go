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

// Section is a titled group of rows, which is what a card is in the window and
// a block is in the terminal.
type Section struct {
	// Key names the section in the settings and on the command line. It is
	// stable; Title is not, and may be translated or may carry a device's
	// name.
	Key   string
	Title string
	Rows  []Row

	// Gone marks a section whose source was answering and has stopped. Its
	// rows keep their last values and are drawn dim, because the reader's
	// question is whether they are still true.
	Gone bool
}
