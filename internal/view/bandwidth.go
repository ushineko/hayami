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

	// RxTrail and TxTrail are the recent rates, oldest first. Empty until
	// two polls have given a rate, which is the honest state after a start.
	RxTrail []float64
	TxTrail []float64
}

// Bandwidth turns readings into a section.
//
// Each interface becomes two rows, down and up, because a single row carrying
// both would have two values in one column and neither would hold still. The
// cumulative totals go in the detail line under each rate, which is where the
// monitor puts them and why it can show four numbers per interface in 265 px.
func Bandwidth(readings []BandwidthReading) Section {
	s := Section{Key: "bandwidth", Title: "Bandwidth", Icon: IconBandwidth}
	unit := UnitWidth("B/s", "KiB/s", "MiB/s", "GiB/s", "TiB/s")

	for _, r := range readings {
		s.Rows = append(s.Rows, interfaceRow(r, unit))
	}

	// The trend: down then up for each interface, in the rows' order, all
	// against one scale (spec 021). A shared scale is what makes a 1 KiB/s
	// interface a flat line beside a 20 MiB/s one, which is the reading --
	// scaled each to its own range they would look equally busy.
	//
	// A trail is emitted for every interface drawn, even before it has a
	// sample: the window builds its plot from the first section it sees, and
	// the first poll has no rate to plot.
	s.TrailScale = ScaleShared
	for i, r := range readings {
		s.Trails = append(s.Trails,
			Trail{Name: r.Name + " ↓", Samples: r.RxTrail, Status: Info, Series: i, Coloured: true},
			Trail{Name: r.Name + " ↑", Samples: r.TxTrail, Status: Info, Series: i, Secondary: true, Coloured: true})
	}
	return s
}

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
	parts := rates(r, unitWidth)
	row := Row{Label: r.Name, Parts: parts}
	for _, p := range parts {
		row.Value += p.Text
	}
	if r.HasTotal {
		// The totals take no band: they are a record of what has passed,
		// not a rate, and a total that went amber at 10 MiB would be amber
		// for the rest of the day.
		row.Detail = totals(r)
	}
	return row
}

// rates is the two directions, padded so neither moves the other, each with
// the band of its own rate (spec 031). The arrows are the row's plain text: it
// is the figure that is emphasised, not the furniture beside it.
func rates(r BandwidthReading, unitWidth int) []Part {
	return []Part{
		{Text: "↓ ", Status: Info},
		{Text: rate(r.RxRate, r.HasRate, unitWidth), Status: RateBand(r.RxRate, r.HasRate)},
		{Text: "  ↑ ", Status: Info},
		{Text: rate(r.TxRate, r.HasRate, unitWidth), Status: RateBand(r.TxRate, r.HasRate)},
	}
}

/*
The rate bands, in bytes per second (spec 031).

**Binary, and the first one where the unit changes.** 1 MiB/s is the rate at
which the figure stops reading KiB/s and starts reading MiB/s, so the colour
and the unit agree about where "small" ends. Below it an interface is doing
what interfaces do all day -- a page, a sync, a chat -- and is drawn as it
always was.

**Then a decade each.** 10 MiB/s is a large download on a home line, or a
100-megabit link full; 100 MiB/s is most of a gigabit link (whose ceiling is
about 119 MiB/s), which is a copy across the room or a game being installed.
Three steps a reader can tell apart at a glance, and no fourth: past a gigabit
the only question left is which interface, and the row already says.
*/
const (
	RateNotable = 1 << 20
	RateBusy    = 10 << 20
	RateFlatOut = 100 << 20
)

// RateBand is the emphasis a rate is drawn with. A rate that has not arrived
// has none.
//
// No band is Bad, and none is Good either: a rate is not a verdict. Warn is the
// one verdict colour used, for the middle band, because it is the scheme's
// amber and amber reads as "look here" without reading as "something broke";
// the error colour would.
func RateBand(bytesPerSecond float64, has bool) Status {
	switch {
	case !has || bytesPerSecond < RateNotable:
		return Info
	case bytesPerSecond < RateBusy:
		return Accent
	case bytesPerSecond < RateFlatOut:
		return Warn
	default:
		return Strong
	}
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
