package view

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// someCPU is a Linux machine of four cores, the third offline.
func someCPU() CPUFigures {
	return CPUFigures{
		Load: Some(25.0), Trail: []float64{10, 20, 25}, TrailEvery: 5 * time.Second,
		Cores: []float64{90, 0, -1, 37.5}, HasCores: true,
		Clock: Some(3000.0), MaxClock: 5700,
		LoadKind: LoadAverage, LoadAvg: Some([3]float64{3.88, 4.38, 3.74}),
		Processes: 812, Threads: 10878, HasTasks: true,
	}
}

// Spec 095 R1. The load with its meter and its plot, a cell per core, then
// the clock, the load average and the tasks.
func TestTheCPUSummarySaysTheProcessor(t *testing.T) {
	s, reasons := CPUSummary(someCPU())

	assert.Empty(t, reasons)
	require.Len(t, s.Meters, 1)
	assert.Equal(t, "load", s.Meters[0].Label)
	assert.InDelta(t, 0.25, s.Meters[0].Fraction, 1e-9)
	require.Len(t, s.Trails, 1)
	assert.Equal(t, []float64{10, 20, 25}, s.Trails[0].Samples)
	require.NotNil(t, s.Heat)
	assert.Equal(t, []float64{0.9, 0, -1, 0.375}, s.Heat.Values)
	assert.Equal(t, []string{"clock", LoadAverage, "tasks"}, []string{s.Lines[0].Name, s.Lines[1].Name, s.Lines[2].Name})
	assert.Equal(t, "  3.0 of   5.7 GHz", s.Lines[0].figure())
	assert.Equal(t, "  3.9   4.4   3.7", s.Lines[1].figure())
	assert.Equal(t, "   812 procs   10878 threads", s.Lines[2].figure())
}

// R1 D1. Windows has a queue where Linux has a load average, and a clock
// without a maximum; the line keeps its name when the queue is not read.
func TestWindowsHasAQueue(t *testing.T) {
	f := someCPU()
	f.LoadKind, f.LoadAvg, f.Queue, f.MaxClock = LoadQueue, Opt[[3]float64]{}, Some(2.0), 0
	s, _ := CPUSummary(f)
	assert.Equal(t, LoadQueue, s.Lines[1].Name)
	assert.Equal(t, "    2", s.Lines[1].Value)
	assert.Equal(t, "  3.0 GHz", s.Lines[0].figure())

	f.Queue = Opt[float64]{}
	s, reasons := CPUSummary(f)
	assert.Equal(t, LoadQueue, s.Lines[1].Name)
	assert.Equal(t, NoQuantity(), s.Lines[1].Value)
	require.Len(t, reasons, 1)
	assert.Equal(t, LoadQueue, reasons[0].Label)
}

// R2, R6. Nothing read: the same shape, every figure in its empty form, and a
// reason for each.
func TestACPUSummaryOfNothingKeepsItsShape(t *testing.T) {
	full, _ := CPUSummary(someCPU())
	empty, reasons := CPUSummary(CPUFigures{Cores: []float64{-1, -1, -1, -1}, LoadKind: LoadAverage})

	assert.Len(t, empty.Meters, len(full.Meters))
	assert.Len(t, empty.Trails, len(full.Trails))
	assert.Len(t, empty.Heat.Values, len(full.Heat.Values))
	assert.Len(t, empty.Lines, len(full.Lines))
	for i := range full.Lines {
		assert.Equal(t, full.Lines[i].Name, empty.Lines[i].Name)
	}
	assert.Len(t, reasons, 4, "cores, clock, load avg, tasks")
}

// R9. A terminal draws the cores one character each, a dot for an offline
// one, under the plot; the JSON carries the cells.
func TestTheCoresPrintAsACharacterEach(t *testing.T) {
	assert.Equal(t, "▇▁·▅", Heat{Values: []float64{0.9, 0, -1, 0.5}}.Text())

	s, _ := CPUSummary(someCPU())
	text := Detail{Title: "CPU by process", Summary: &s}.Text()
	lines := strings.Split(text, "\n")
	require.GreaterOrEqual(t, len(lines), 7, text)
	assert.Contains(t, lines[1], "load")
	assert.Contains(t, lines[3], "cores")
	assert.Contains(t, lines[3], "▇▁·▄")
	assert.Contains(t, lines[4], "clock")

	raw, err := json.Marshal(Detail{Summary: &s})
	require.NoError(t, err)
	assert.Contains(t, string(raw), `"heat":{"name":"cores","values":[0.9,0,-1,0.375]}`)
}
