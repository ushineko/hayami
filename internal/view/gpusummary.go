package view

// GPUFigures are the card as the GPU summary says it: its memory in bytes,
// its clocks in MHz, its power in watts and the limit, zero where not known.
type GPUFigures struct {
	VRAM, VRAMTotal float64
	HasVRAM         bool

	Clock, MemClock Opt[float64]

	Power      Opt[float64]
	PowerLimit float64
}

/*
GPUSummary is the GPU drawer's summary (spec 096 R1): the card's memory used
of its total with a meter, its graphics clock, its memory clock, and the power
it draws of its limit. Every line is there whether or not its figure is, and a
figure the card's route does not give is the empty form and a reason.
*/
func GPUSummary(f GPUFigures) (Summary, []Reason) {
	var reasons []Reason
	missing := func(label string) {
		reasons = append(reasons, Reason{Label: label, Text: "not read", Status: Info})
	}
	unit := UnitWidth(byteUnits...)
	size := func(v float64) string {
		n, u := Size(v)
		return n + " " + PadUnit(u, unit)
	}

	vram := Meter{Label: "VRAM", Status: Info, Caption: NoQuantity()}
	if f.HasVRAM && f.VRAMTotal > 0 {
		vram.Caption = size(f.VRAM) + " of " + size(f.VRAMTotal)
		vram.Fraction = min(f.VRAM/f.VRAMTotal, 1)
	} else {
		missing("VRAM")
	}

	clock := func(name string, v Opt[float64]) DetailLine {
		l := DetailLine{Name: name, Value: NoQuantity(), Unit: "MHz"}
		if v.OK {
			l.Value = Count(int(v.V + 0.5))
		} else {
			missing(name)
		}
		return l
	}
	gr, mem := clock("clock", f.Clock), clock("mem clock", f.MemClock)

	power := DetailLine{Name: "power", Value: NoQuantity(), Unit: "W"}
	switch {
	case !f.Power.OK:
		missing("power")
	case f.PowerLimit > 0:
		power.Value = Quantity(f.Power.V) + " of " + Quantity(f.PowerLimit)
	default:
		power.Value = Quantity(f.Power.V)
	}
	return Summary{Meters: []Meter{vram}, Lines: []DetailLine{gr, mem, power}}, reasons
}
