package view

import (
	"fmt"
	"strings"
)

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

	// Radio says the interface is Wi-Fi, and Link is what it said about its
	// link (spec 037). A radio with no link is still a radio: its row keeps
	// the bars' place, empty, rather than shrinking by them.
	Radio bool
	Link  LinkReading
}

// LinkReading is a Wi-Fi link, mirroring core.Wireless as BandwidthReading
// mirrors core.Rates. Each figure has its flag; the network's name is not one
// of them.
type LinkReading struct {
	Connected  bool
	RSSI       int
	HasRSSI    bool
	Signal     int
	HasSignal  bool
	Band       string
	Channel    int
	HasChannel bool
	Generation string
	RxRate     float64
	TxRate     float64
	HasRx      bool
	HasTx      bool
}

// Bandwidth turns readings into a section.
//
// Each interface becomes two rows, down and up, because a single row carrying
// both would have two values in one column and neither would hold still. The
// cumulative totals go in the detail line under each rate, which is where the
// monitor puts them and why it can show four numbers per interface in 265 px.
func Bandwidth(readings []BandwidthReading) Section {
	s := BandwidthInfo.section()
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
	if r.Radio {
		// The signal leads the rates (spec 037): it is about the same link,
		// and the eye reads it before the figures it explains.
		parts = append([]Part{{Text: SignalBars(r.Link) + " ", Status: signalStatus(r.Link)}}, parts...)
	}
	row := Row{Label: r.Name, Parts: parts}
	for _, p := range parts {
		row.Value += p.Text
	}
	var details []string
	if r.HasTotal {
		// The totals take no band: they are a record of what has passed,
		// not a rate, and a total that went amber at 10 MiB would be amber
		// for the rest of the day.
		details = append(details, totals(r))
	}
	if r.Radio {
		details = append(details, linkLine(r.Link))
		row.Tip = linkTip(r.Name, r.Link)
	}
	row.Detail = strings.Join(details, "\n")
	return row
}

/*
The signal's bands, in dBm, for the four bars (spec 037).

**From the RSSI, by the thresholds the trade uses.** -55 dBm and better is as
good as Wi-Fi gets in a home; -67 is the figure voice and video are planned
to; -75 is where a link starts dropping packets; below that it is holding on.
Windows' own percentage is used only where there is no RSSI, in quarters, and
is the weaker of the two: it is a vendor's mapping of the same figure.
*/
const (
	SignalExcellent = -55
	SignalGood      = -67
	SignalFair      = -75
)

// SignalLevel is how many of BandSegments bars a link earns: none for a radio
// with no link or nothing said about its signal.
func SignalLevel(l LinkReading) int {
	switch {
	case !l.Connected:
		return 0
	case l.HasRSSI && l.RSSI >= SignalExcellent:
		return 4
	case l.HasRSSI && l.RSSI >= SignalGood:
		return 3
	case l.HasRSSI && l.RSSI >= SignalFair:
		return 2
	case l.HasRSSI:
		return 1
	case l.HasSignal && l.Signal >= 75:
		return 4
	case l.HasSignal && l.Signal >= 50:
		return 3
	case l.HasSignal && l.Signal >= 25:
		return 2
	case l.HasSignal:
		return 1
	default:
		return 0
	}
}

// SignalBars draws a link's level in the battery's four segments (spec 018):
// the same glyphs, the same width, filled from the left.
func SignalBars(l LinkReading) string { return segments(SignalLevel(l)) }

// signalStatus is the bars' emphasis. One bar is the only verdict: a link that
// weak is the likely reason the rates beside it are low. More than that is
// drawn as the rates are, because a good signal is not news.
func signalStatus(l LinkReading) Status {
	if SignalLevel(l) == 1 {
		return Warn
	}
	return Info
}

// linkLine is the link under the totals: strength, band and channel, and the
// rate the radio negotiated, each padded to the widest it can be so the line
// holds still as they change (glance rule). A radio with no link says so in
// the line's place, so the row is the same height either way.
func linkLine(l LinkReading) string {
	if !l.Connected {
		// As wide as a link's line, so connecting does not widen the card.
		return fmt.Sprintf("%*s", linkLineWidth, "not connected")
	}
	rssi := strings.Repeat(" ", 8)
	if l.HasRSSI {
		rssi = fmt.Sprintf("%4d dBm", l.RSSI)
	}
	band := fmt.Sprintf("%7s", l.Band)
	channel := strings.Repeat(" ", 6)
	if l.HasChannel {
		channel = fmt.Sprintf("ch %3d", l.Channel)
	}
	rate := strings.Repeat(" ", 9)
	if l.HasRx {
		rate = fmt.Sprintf("%4.0f Mb/s", l.RxRate)
	}
	return rssi + " · " + band + " " + channel + " · " + rate
}

// linkLineWidth is how wide linkLine always is: "-100 dBm · 2.4 GHz ch 165 · 2402 Mb/s".
const linkLineWidth = 8 + 3 + 7 + 1 + 6 + 3 + 9

// linkTip is the rest of what is known about the link, for the pointer: the
// generation, Windows' percentage and both rates, which the line under the row
// has no room for.
func linkTip(name string, l LinkReading) string {
	if !l.Connected {
		return name + ": Wi-Fi, not connected"
	}
	parts := []string{}
	if l.Generation != "" {
		parts = append(parts, l.Generation)
	}
	if l.HasSignal {
		parts = append(parts, fmt.Sprintf("signal %d %%", l.Signal))
	}
	if l.HasRx || l.HasTx {
		parts = append(parts, fmt.Sprintf("link %s down, %s up Mb/s", mbit(l.RxRate, l.HasRx), mbit(l.TxRate, l.HasTx)))
	}
	if len(parts) == 0 {
		return name + ": Wi-Fi"
	}
	return name + ": " + strings.Join(parts, ", ")
}

// mbit is a link rate for the tip, or a dash where there is none.
func mbit(v float64, has bool) string {
	if !has {
		return "–"
	}
	return fmt.Sprintf("%.0f", v)
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
