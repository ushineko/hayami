package view

import (
	"fmt"
	"time"
)

// The waiting line's names: Linux's IO pressure and Windows's disk queue,
// which it has instead (spec 097 D3).
const (
	IOPressure = "pressure"
	IOQueue    = "queue"
)

// IOFigures are the disks as the IO summary says them: rates in bytes per
// second, their recent history, each disk's busy share from 0 to 1 (negative
// for none), the pressure in percent or the queue, and the swap's rates.
type IOFigures struct {
	Read, Write           Opt[float64]
	ReadTrail, WriteTrail []float64
	TrailEvery            time.Duration
	Busy                  []float64
	HasBusy               bool
	Queues                bool
	Pressure, Queue       Opt[float64]
	SwapIn, SwapOut       float64
	HasSwap               bool
}

/*
IOSummary is the IO drawer's summary (spec 097 R4): the disks' read and write
rates with a plot of both, a cell per disk by how busy it is, the IO pressure
or the disks' queue, and the swap's rates. Every line is there whether or not
its figure is.
*/
func IOSummary(f IOFigures) (Summary, []Reason) {
	var reasons []Reason
	missing := func(label string) {
		reasons = append(reasons, Reason{Label: label, Text: "not read", Status: Info})
	}
	width := bandwidthUnitWidth()

	rateLine := func(name string, v Opt[float64]) DetailLine {
		n, u := NoRate()
		if v.OK {
			n, u = Rate(v.V)
		} else {
			missing(name)
		}
		return DetailLine{Name: name, Value: n, Unit: PadUnit(u, width)}
	}
	read, write := rateLine("read", f.Read), rateLine("write", f.Write)

	heat := Heat{Name: "disks", Values: append([]float64(nil), f.Busy...)}
	if !f.HasBusy {
		missing("disks")
	}

	waiting := DetailLine{Name: IOPressure, Value: NoQuantity(), Unit: "%"}
	switch {
	case f.Queues:
		waiting = DetailLine{Name: IOQueue, Value: NoQuantity()}
		if f.Queue.OK {
			waiting.Value = Quantity(f.Queue.V)
		} else {
			missing(IOQueue)
		}
	case f.Pressure.OK:
		waiting.Value = Quantity(f.Pressure.V)
	default:
		missing(IOPressure)
	}

	swap := DetailLine{Name: "swap", Value: fmt.Sprintf("↓ %s ↑ %s", rate(0, false, width), rate(0, false, width))}
	if f.HasSwap {
		swap.Value = fmt.Sprintf("↓ %s ↑ %s", rate(f.SwapIn, true, width), rate(f.SwapOut, true, width))
	} else {
		missing("swap")
	}

	return Summary{
		Trails: []Trail{
			{Name: "read", Samples: f.ReadTrail, Series: 0, Coloured: true},
			{Name: "write", Samples: f.WriteTrail, Series: 0, Coloured: true, Secondary: true},
		},
		TrailEvery: f.TrailEvery,
		Heat:       &heat,
		Lines:      []DetailLine{read, write, waiting, swap},
	}, reasons
}
