package view

import "strings"

// SparkRunes are the eight heights a sparkline is drawn with, lowest first.
//
// Eight, because that is what the block characters give and a pane has one
// line to spend. A trend is what this says: whether a number has been climbing
// for five minutes, which is the one thing a reader takes from a panel they
// never touch.
var SparkRunes = []rune("▁▂▃▄▅▆▇█")

// Sparkline draws a series as one line.
//
// The scale is the series' own: its lowest sample is the floor and its highest
// the ceiling, so a coolant that moved between 45.8 and 46.6 degrees fills the
// line the way a processor that moved between 65 and 98 does. That is
// deliberate — the question is "is this going up", not "how does this compare
// with that" — and it is why a panel never draws two series against one axis.
//
// minSpan is the narrowest range the scale may have. Without it, a series that
// has not moved at all is amplified into noise: eight heights across a
// hundredth of a degree.
func Sparkline(series []float64, width int, minSpan float64) string {
	if width < 1 || len(series) == 0 {
		return ""
	}
	if len(series) > width {
		series = series[len(series)-width:]
	}

	low, high := series[0], series[0]
	for _, v := range series {
		low = min(low, v)
		high = max(high, v)
	}
	span := high - low
	if span < minSpan {
		// Centre a flat series rather than pinning it to the floor: a line
		// along the bottom reads as "nothing here" and a line through the
		// middle reads as "steady", which is what it is.
		low -= (minSpan - span) / 2
		span = minSpan
	}

	var b strings.Builder
	for _, v := range series {
		at := int((v - low) / span * float64(len(SparkRunes)-1))
		b.WriteRune(SparkRunes[clampIndex(at, len(SparkRunes))])
	}
	return b.String()
}

// clampIndex keeps an index inside a slice, for the sample that lands exactly
// on the ceiling and rounds past it.
func clampIndex(i, n int) int {
	if i < 0 {
		return 0
	}
	if i >= n {
		return n - 1
	}
	return i
}

// Series is a fixed number of samples with the newest at the end.
//
// A ring in the plain sense: it holds its capacity and drops the oldest. It is
// what this process has seen since it started, so a restart starts it again —
// a panel that drew a line through one point would be inventing a trend.
type Series struct {
	samples  []float64
	capacity int
}

// NewSeries builds a series that holds capacity samples.
func NewSeries(capacity int) *Series {
	if capacity < 1 {
		capacity = 1
	}
	return &Series{capacity: capacity}
}

// Add records a sample.
func (s *Series) Add(v float64) {
	s.samples = append(s.samples, v)
	if len(s.samples) > s.capacity {
		s.samples = s.samples[len(s.samples)-s.capacity:]
	}
}

// Samples are what the series holds, oldest first.
func (s *Series) Samples() []float64 {
	if s == nil {
		return nil
	}
	return append([]float64(nil), s.samples...)
}

// Len is how many samples there are.
func (s *Series) Len() int {
	if s == nil {
		return 0
	}
	return len(s.samples)
}
