package view

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
}

// PeripheralsReading is every device the poll knows about.
type PeripheralsReading struct {
	Devices []PeripheralReading
}

// Peripherals turns a reading into a section.
//
// A row per device, appearing and disappearing as the hardware does. The
// program this replaces draws two fixed cells, each configured to a device
// type through a submenu, which is an artefact of a panel 260 pixels wide and
// not something a reader ever asked for.
//
// Charging is **said and not coloured**. A device on its cable is not a
// warning however empty it is — it is being dealt with — and colouring it
// would put a red row on the panel for the one battery nobody needs to think
// about.
func Peripherals(r PeripheralsReading) Section {
	s := Section{Key: "peripherals", Title: "Peripherals"}
	unit := UnitWidth("%")

	for _, d := range r.Devices {
		s.Rows = append(s.Rows, peripheral(d, unit))
	}
	return s
}

// peripheral is one device's row.
func peripheral(d PeripheralReading, unit int) Row {
	row := Row{Label: d.Name, Unit: PadUnit("%", unit), Detail: note(d)}

	if !d.HasLevel {
		// No level, so no verdict. The unit stays for the column's sake: the
		// one device that is quiet should not move the ones that are not.
		row.Value = NoQuantity()
		row.Status = Dim
		return row
	}

	row.Value = Count(d.Level)
	switch {
	case d.Stale:
		// The number is kept and the verdict is dropped. A red row for a
		// battery nobody has heard from in ten minutes asserts something the
		// panel does not know.
		row.Status = Dim
	case d.Charge != Draining:
		row.Status = Info
	case d.Level <= BatteryCritical:
		row.Status = Bad
	case d.Level <= BatteryLow:
		row.Status = Warn
	default:
		row.Status = Good
	}
	return row
}

// note is the quiet line under a row, where there is something to say.
func note(d PeripheralReading) string {
	switch {
	case d.Stale:
		return "not answering"
	case d.Charge == Filling:
		return "charging"
	case d.Charge == Charged:
		return "charged"
	default:
		return ""
	}
}
