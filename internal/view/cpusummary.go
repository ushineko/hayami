package view

import (
	"fmt"
	"strings"
	"time"
)

// Heat is many proportions of one kind on one line, each from 0 to 1; a
// negative value is one that is not there, an offline core, and keeps its
// place.
type Heat struct {
	Name   string    `json:"name"`
	Values []float64 `json:"values"`
}

// heatBlocks are the characters a terminal draws a heat cell in, emptiest
// first.
var heatBlocks = []rune("▁▂▃▄▅▆▇█")

// Text is the row as a terminal draws it: one character per cell, its height
// its proportion from ▁ for an idle one, so every cell is seen, and a dot
// for a cell that is not there.
func (h Heat) Text() string {
	var b strings.Builder
	for _, v := range h.Values {
		if v < 0 {
			b.WriteRune('·')
			continue
		}
		b.WriteRune(heatBlocks[int(min(v, 1)*float64(len(heatBlocks)-1)+0.5)])
	}
	return b.String()
}

// The waiting line's names: Linux's load average, and Windows's processor
// queue, which it has instead (spec 095 D1).
const (
	LoadAverage = "load avg"
	LoadQueue   = "queue"
)

// CPUFigures are the processor as the CPU summary says it. Load is the whole
// processor's percent and Trail its recent history; Cores each core's
// percent, negative for none; clocks in MHz, MaxClock zero where not known.
type CPUFigures struct {
	Load       Opt[float64]
	Trail      []float64
	TrailEvery time.Duration

	Cores    []float64
	HasCores bool

	Clock    Opt[float64]
	MaxClock float64

	// LoadKind is which of the two the machine has, LoadAverage or
	// LoadQueue; LoadAvg is the average's three, Queue the queue.
	LoadKind string
	LoadAvg  Opt[[3]float64]
	Queue    Opt[float64]

	Processes, Threads int
	HasTasks           bool
}

/*
CPUSummary is the CPU drawer's summary (spec 095 R1): the load with its meter
and a plot of it, a cell per core, the clock, the load average or the queue,
and the processes and threads. Its shape is the machine's: a cell for every
core whether or not it is read, and every line whether or not its figure is.
*/
func CPUSummary(f CPUFigures) (Summary, []Reason) {
	var reasons []Reason
	missing := func(label string) {
		reasons = append(reasons, Reason{Label: label, Text: "not read", Status: Info})
	}

	load := Meter{Label: "load", Status: Info, Caption: NoQuantity() + " %"}
	if f.Load.OK {
		load.Caption, load.Fraction = Quantity(f.Load.V)+" %", min(max(f.Load.V/100, 0), 1)
	}
	s := Summary{Meters: []Meter{load}, TrailEvery: f.TrailEvery,
		Trails: []Trail{{Name: "load", Samples: f.Trail}}}

	heat := Heat{Name: "cores", Values: make([]float64, len(f.Cores))}
	for i, c := range f.Cores {
		heat.Values[i] = -1
		if c >= 0 {
			heat.Values[i] = min(c/100, 1)
		}
	}
	if !f.HasCores {
		missing("cores")
	}
	s.Heat = &heat

	clock := DetailLine{Name: "clock", Value: NoQuantity(), Unit: "GHz"}
	switch {
	case !f.Clock.OK:
		missing("clock")
	case f.MaxClock > 0:
		clock.Value = Quantity(f.Clock.V/1000) + " of " + Quantity(f.MaxClock/1000)
	default:
		clock.Value = Quantity(f.Clock.V / 1000)
	}

	waiting := DetailLine{Name: LoadAverage, Value: NoQuantity()}
	switch {
	case f.LoadKind == LoadQueue:
		waiting.Name = LoadQueue
		if f.Queue.OK {
			waiting.Value = Count(int(f.Queue.V))
		} else {
			missing(LoadQueue)
		}
	case f.LoadAvg.OK:
		a := f.LoadAvg.V
		waiting.Value = fmt.Sprintf("%s %s %s", Quantity(a[0]), Quantity(a[1]), Quantity(a[2]))
	default:
		missing(LoadAverage)
	}

	tasks := DetailLine{Name: "tasks", Value: NoQuantity()}
	if f.HasTasks {
		tasks.Value = fmt.Sprintf("%6d procs %7d threads", f.Processes, f.Threads)
	} else {
		missing("tasks")
	}
	s.Lines = []DetailLine{clock, waiting, tasks}
	return s, reasons
}
