package view

/*
Summary is what a part is doing as a whole, over the list of what each program
does with it (spec 094). It is meters and lines, a fixed number of each for its
kind: a figure that cannot be read keeps its line in the empty form, so no
reading changes how tall the summary is.

The detail says what is in it; whether it is drawn is the shell's, which has
the room to weigh it against.
*/
type Summary struct {
	Meters []Meter      `json:"meters,omitempty"`
	Lines  []DetailLine `json:"lines,omitempty"`

	// Covers marks a summary that says what its detail's note says, so a
	// shell that draws the summary leaves the note out: the memory in use.
	Covers bool `json:"-"`
}

// MemoryFigures are the machine's memory as the Memory summary says it, in
// bytes. A swap whose total is zero is a machine without one; SwapName is
// what the system calls it, "swap" or "page file".
type MemoryFigures struct {
	Used, Total float64
	Cache       Opt[float64]
	Swap        Opt[float64]
	SwapTotal   float64
	SwapName    string
}

// SwapNone is the swap line of a machine without swap.
const SwapNone = "none"

/*
MemorySummary is the Memory drawer's summary (spec 094 R5): the memory used of
the total with a meter of the share, what is available, the cache, and the swap
used of its size. A figure the reading lacks is the empty form on its line, and
a reason says which.
*/
func MemorySummary(m MemoryFigures) (Summary, []Reason) {
	unit := UnitWidth(byteUnits...)
	size := func(v float64) string {
		n, u := Size(v)
		return n + " " + PadUnit(u, unit)
	}
	used := Meter{Label: "used", Status: Info, Caption: size(m.Used) + " of " + size(m.Total)}
	if m.Total > 0 {
		used.Fraction = min(m.Used/m.Total, 1)
	}
	n, u := Size(m.Total - m.Used)
	available := DetailLine{Name: "available", Value: n, Unit: PadUnit(u, unit)}

	var reasons []Reason
	cache := DetailLine{Name: "cache", Value: NoQuantity()}
	if m.Cache.OK {
		n, u := Size(m.Cache.V)
		cache.Value, cache.Unit = n, PadUnit(u, unit)
	} else {
		reasons = append(reasons, Reason{Label: "cache", Text: "not read", Status: Info})
	}
	name := m.SwapName
	if name == "" {
		name = "swap"
	}
	swap := DetailLine{Name: name, Value: NoQuantity()}
	switch {
	case !m.Swap.OK:
		reasons = append(reasons, Reason{Label: name, Text: "not read", Status: Info})
	case m.SwapTotal == 0:
		swap.Value = pad(SwapNone)
	default:
		n, u := Size(m.SwapTotal)
		swap.Value, swap.Unit = size(m.Swap.V)+" of "+n, PadUnit(u, unit)
	}
	return Summary{Meters: []Meter{used}, Lines: []DetailLine{available, cache, swap}, Covers: true}, reasons
}
