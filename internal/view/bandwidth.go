package view

// BandwidthReading is one interface's numbers, already measured and not yet
// formatted. It mirrors core.Rates without importing it: the view describes
// what is drawn and takes plain numbers, and a test here builds one by hand.
type BandwidthReading struct {
	Name     string
	RxRate   float64
	TxRate   float64
	RxTotal  uint64
	TxTotal  uint64
	HasRate  bool
	HasTotal bool
}

// Bandwidth turns readings into a section.
//
// Each interface becomes two rows, down and up, because a single row carrying
// both would have two values in one column and neither would hold still. The
// cumulative totals go in the detail line under each rate, which is where the
// monitor puts them and why it can show four numbers per interface in 265 px.
func Bandwidth(readings []BandwidthReading) Section {
	s := Section{Key: "bandwidth", Title: "Bandwidth"}
	unit := UnitWidth("B/s", "KiB/s", "MiB/s", "GiB/s", "TiB/s")

	for _, r := range readings {
		s.Rows = append(s.Rows, interfaceRow(r, unit))
	}
	return s
}

// rateRow is one direction: the rate as the value, the total as the detail.
/*
interfaceRow is one interface on one line, with its totals on a second.

**Both directions on one line, as the monitor draws them.** An interface used
to take four lines here — a row for each direction and a totals line under
each — and a card is a card per interface on a panel meant to be glanced at.
The monitor puts the name at the left, both rates at the right, and both
totals under them, which is half the height and reads no worse: down and up
are a pair and are read as one.

Every part is padded to a fixed width, so the line does not move as a rate
crosses from KiB/s to MiB/s. That is the same rule a single rate was already
held to and it matters more here, not less: two changing values on one line
have two chances to drag it about.
*/
func interfaceRow(r BandwidthReading, unitWidth int) Row {
	row := Row{Label: r.Name, Value: rates(r, unitWidth)}
	if r.HasTotal {
		row.Detail = totals(r)
	}
	return row
}

// rates is the two directions, padded so neither moves the other.
func rates(r BandwidthReading, unitWidth int) string {
	return "↓ " + rate(r.RxRate, r.HasRate, unitWidth) +
		"  ↑ " + rate(r.TxRate, r.HasRate, unitWidth)
}

// rate is one direction's figure and unit, at a width that does not change.
func rate(v float64, has bool, unitWidth int) string {
	number, unit := NoRate()
	if has {
		number, unit = Rate(v)
	}
	return number + " " + PadUnit(unit, unitWidth)
}

// totals is the pair of cumulative figures, under the rates they belong to.
func totals(r BandwidthReading) string {
	unit := UnitWidth("B", "KiB", "MiB", "GiB", "TiB")

	rx, rxUnit := Size(float64(r.RxTotal))
	tx, txUnit := Size(float64(r.TxTotal))
	return "Σ ↓ " + rx + " " + PadUnit(rxUnit, unit) +
		"  ↑ " + tx + " " + PadUnit(txUnit, unit)
}
