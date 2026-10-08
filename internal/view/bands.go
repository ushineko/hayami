package view

/*
Bands is a reading's verdict as a table: thresholds in ascending order, each
with what a value that reaches it gets (spec 039).

A value takes the To of the last step it reaches, and Floor below the first.
A step is reached at From itself, or only above it when Above is set. One
ascending form says both "from half" (>= 0.50) and "past four fifths"
(> 0.80), and a reading where low is bad is written the same way, with the bad
verdict as the Floor: a battery is Bad, then Warn above 20, then Good above
50.

T is the verdict: a Status for a colour, an int for a number of bars. The
steps must ascend; TestEveryTableAscends holds each table here to it.
*/
type Bands[T any] struct {
	Floor T
	Steps []Step[T]
}

// Step is one threshold of a Bands table.
type Step[T any] struct {
	From  float64
	Above bool
	To    T
}

// Of is the verdict on v.
func (b Bands[T]) Of(v float64) T {
	out := b.Floor
	for _, s := range b.Steps {
		if v < s.From || (s.Above && v == s.From) {
			break
		}
		out = s.To
	}
	return out
}

// CoolantBands is the verdict on a liquid temperature, from the pump's own
// alarm (CoolantWarm, CoolantHot).
var CoolantBands = Bands[Status]{Floor: Good, Steps: []Step[Status]{
	{From: CoolantWarm, To: Warn},
	{From: CoolantHot, To: Bad},
}}

// QuotaBands is the verdict on a proportion of a limit: amber from half, red
// past four fifths, as the usage widget draws it (issue #75).
var QuotaBands = Bands[Status]{Floor: Good, Steps: []Step[Status]{
	{From: 0.50, To: Warn},
	{From: 0.80, Above: true, To: Bad},
}}

// BatteryBands is the verdict on a draining battery's level, in percent: Bad
// at BatteryCritical and below, Warn to BatteryLow, Good above.
var BatteryBands = Bands[Status]{Floor: Bad, Steps: []Step[Status]{
	{From: BatteryCritical, Above: true, To: Warn},
	{From: BatteryLow, Above: true, To: Good},
}}

// SegmentBands is the verdict on a battery that reports a band of
// BandSegments rather than a percentage: the thresholds a percentage cell uses,
// applied to what exists, so the lowest step is critical and the one above it
// low (spec 018).
var SegmentBands = Bands[Status]{Floor: Bad, Steps: []Step[Status]{
	{From: 2, To: Warn},
	{From: 3, To: Good},
}}

// RateBands is the emphasis a rate is drawn with, in bytes a second.
var RateBands = Bands[Status]{Floor: Info, Steps: []Step[Status]{
	{From: RateNotable, To: Accent},
	{From: RateBusy, To: Warn},
	{From: RateFlatOut, To: Strong},
}}

// SignalRSSIBands is a link's bars from its RSSI, in dBm.
var SignalRSSIBands = Bands[int]{Floor: 1, Steps: []Step[int]{
	{From: SignalFair, To: 2},
	{From: SignalGood, To: 3},
	{From: SignalExcellent, To: 4},
}}

// SignalPercentBands is a link's bars from Windows' percentage, in quarters,
// where there is no RSSI.
var SignalPercentBands = Bands[int]{Floor: 1, Steps: []Step[int]{
	{From: 25, To: 2},
	{From: 50, To: 3},
	{From: 75, To: 4},
}}

// SignalEmphasis is the bars' colour by how many there are: one bar is the
// only verdict, and none (no link) is none.
var SignalEmphasis = Bands[Status]{Floor: Info, Steps: []Step[Status]{
	{From: 1, To: Warn},
	{From: 2, To: Info},
}}
