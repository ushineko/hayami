package view

import (
	"strconv"
	"strings"
)

// The battery bands, in percent.
//
// The reference's, and round numbers because a battery has no alarm to measure
// one from — unlike the coolant, whose bands in spec 006 are the pump's own.
// They are here rather than inline so it is plain they are a convention and
// not a measurement.
const (
	BatteryLow      = 50
	BatteryCritical = 20
)

// Discharging is what a battery that is merely running says.
//
// Named because it is the one state that is also the default, and the one
// place that needs to recognise it -- a device already listing its separate
// batteries has no width left for a word that says nothing had happened.
const Discharging = "Discharging"

// Charge is what a peripheral's battery is doing, in the view's own words.
//
// The view does not import the reader, so the state crosses as this rather
// than as the reader's type. It is three cases and not a string, so a spelling
// cannot arrive here that the panel does not draw.
type Charge int

const (
	// Draining is the ordinary case and is not said.
	Draining Charge = iota
	// Filling is on the cable.
	Filling
	// Charged is done filling and still on the cable.
	Charged
)

// PeripheralReading is one device, measured and not yet formatted.
type PeripheralReading struct {
	// Name is the device's own name, which is the row's label.
	Name string

	// Level is a percentage. HasLevel is false for a device that is there and
	// has not said how full it is — a headset on its cradle — which is not
	// the same as a device that is flat.
	Level    int
	HasLevel bool

	Charge Charge

	// Stale marks a device that answered before and did not this time. Its
	// last level is kept and drawn dim, because the reader's question about a
	// headset that has gone quiet is whether what it said last is still true,
	// not what it says now.
	Stale bool

	// Cells are the separate batteries inside a device that has more than
	// one, already named and in the order to draw them. Empty for the
	// ordinary device with a single battery.
	//
	// Level is the row's number and these are the line beneath it: a pair of
	// earbuds is one device on the desk and should be one row on the panel,
	// but which ear is low is exactly what the wearer wants to know.
	Cells []PeripheralCell
}

// PeripheralCell is one battery inside a device: an earbud, a case.
type PeripheralCell struct {
	// Name is short because these sit several to a line: "L", "R", "case".
	Name  string
	Level int
}

// PeripheralsReading is every device the poll knows about.
type PeripheralsReading struct {
	Devices []PeripheralReading
}

/*
Peripherals turns a reading into a section of cells.

A cell per device: the name over the level, the level large and in the middle,
and what the battery is doing under it. The monitor's shape, and the right one
for a battery -- the number *is* the reading and the name is only which one.

This replaces a row per device, which was argued for here and was wrong. What
that argument got right is kept and is worth restating: a cell per device,
appearing and disappearing with the hardware, rather than the monitor's two
fixed slots each pointed at a device type through a submenu. The slots were an
artefact of a panel 260 pixels wide and are not something a reader ever asked
for. The *row* was the part that did not survive being looked at beside the
program it replaces.

Charging is **said and not coloured**. A device on its cable is not a warning
however empty it is -- it is being dealt with -- and colouring it would put a
red cell on the panel for the one battery nobody needs to think about.
*/
func Peripherals(r PeripheralsReading) Section {
	s := Section{Key: "peripherals", Title: "Peripherals"}
	for _, d := range r.Devices {
		s.Cells = append(s.Cells, peripheral(d))
	}
	return s
}

// peripheral is one device's cell.
func peripheral(d PeripheralReading) Cell {
	cell := Cell{Label: d.Name, Unit: "%", Note: note(d)}

	if !d.HasLevel {
		// No level, so no verdict, and no unit either: a cell centres its
		// reading rather than aligning it in a column, so there is nothing
		// for a lone percent sign to hold a place in.
		cell.Value, cell.Unit = strings.TrimSpace(NoQuantity()), ""
		cell.Status = Dim
		return cell
	}

	cell.Value = strings.TrimSpace(Count(d.Level))
	switch {
	case d.Stale:
		// The number is kept and the verdict is dropped. A red cell for a
		// battery nobody has heard from in ten minutes asserts something the
		// panel does not know.
		cell.Status = Dim
	case d.Charge != Draining:
		cell.Status = Info
	case d.Level <= BatteryCritical:
		cell.Status = Bad
	case d.Level <= BatteryLow:
		cell.Status = Warn
	default:
		cell.Status = Good
	}
	return cell
}

/*
note is the line under a cell's reading, and there is always one.

The row form said this only for a battery that was charging, which left the
ordinary case -- a battery discharging normally -- looking exactly like a
device nobody had heard from. The monitor says "Discharging", "Wired",
"Charging" or "Disconnected" under every cell and it is right to: the state is
a third of what a cell is for, and a blank third reads as a cell that has not
finished loading.

The separate batteries of a device that has several come first when there are
any: for a pair of earbuds they *are* the detail, and the state is said beside
them rather than instead of them -- except when the state is the ordinary one.
"L 80  R 90  case 50  Discharging" does not fit the width of a cell and the
last word of it is the one worth least: a battery that is going down is what a
battery does, and for this device the three numbers are the reading.
*/
func note(d PeripheralReading) string {
	state := chargeNote(d)

	if cells := cellNote(d.Cells); cells != "" {
		if state != "" && state != Discharging {
			return cells + "  " + state
		}
		return cells
	}
	return state
}

// chargeNote is what the cell says about what the battery is doing.
func chargeNote(d PeripheralReading) string {
	switch {
	case d.Stale:
		return "Not answering"
	case d.Charge == Filling:
		return "Charging"
	case d.Charge == Charged:
		return "Charged"
	case !d.HasLevel:
		// A device that is there and has not said how full it is. The monitor
		// calls this "Wired" for a keyboard on its cable and "Disconnected"
		// for a headset on its cradle; neither is knowable from here, so it
		// says the one thing that is true of both -- and says it in two
		// words, because a cell is as wide as a device name and a state that
		// has to be truncated is a state nobody reads.
		return "No reading"
	default:
		return Discharging
	}
}

// cellNote lists the separate batteries: "L 100  R 100  case 80".
func cellNote(cells []PeripheralCell) string {
	var parts []string
	for _, c := range cells {
		parts = append(parts, c.Name+" "+strconv.Itoa(c.Level))
	}
	return strings.Join(parts, "  ")
}
