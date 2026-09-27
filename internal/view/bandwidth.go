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
		s.Rows = append(s.Rows,
			rateRow(r.Name+" down", r.RxRate, r.RxTotal, r.HasRate, r.HasTotal, unit),
			rateRow(r.Name+" up", r.TxRate, r.TxTotal, r.HasRate, r.HasTotal, unit),
		)
	}
	return s
}

// rateRow is one direction: the rate as the value, the total as the detail.
func rateRow(label string, rate float64, total uint64, hasRate, hasTotal bool, unitWidth int) Row {
	value, unit := NoRate()
	if hasRate {
		value, unit = Rate(rate)
	}
	row := Row{Label: label, Value: value, Unit: PadUnit(unit, unitWidth)}
	if hasTotal {
		n, u := Size(float64(total))
		row.Detail = "Σ " + n + " " + u
	}
	return row
}
