package view

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// rowWidth is a row's value and unit as one string's width.
func rowWidth(r Row) int { return runeLen(r.Value) + runeLen(r.Unit) }

// Spec 097 R1. The card's IO row is the read and write rates, and no reading
// changes its width: not a rate's size, nor its absence.
func TestTheIORowKeepsItsWidth(t *testing.T) {
	io := func(read, write Opt[float64]) Row {
		s := Cooler(CoolerReading{Probes: []Probe{{ID: "io", Role: RoleIO, Read: read, Write: write}}})
		require.Len(t, s.Rows, 1)
		return s.Rows[0]
	}
	small := io(Some(0.0), Some(12.0))
	assert.Equal(t, "IO", small.Label)
	assert.Equal(t, "io", small.ID)
	assert.Contains(t, small.Value, "↓")
	assert.Contains(t, small.Value, "↑")
	for _, r := range []Row{io(Some(999.9*(1<<30)), Some(1023.0)), io(Some(5e6), Opt[float64]{})} {
		assert.Equal(t, rowWidth(small), rowWidth(r), r.Value)
	}

	none := Cooler(CoolerReading{Probes: []Probe{{ID: "io", Role: RoleIO}}})
	assert.Empty(t, none.Rows, "no rates read is no row")
}

// R2. The IO list is offered as IO, key i, and its figure and count column
// want no more of the drawer than the other three lists, so offering it
// widens nothing.
func TestTheIOListFitsTheDrawerTheOthersWant(t *testing.T) {
	info := ProcessInfo(DetailIO)
	assert.Equal(t, "IO", info.Label)
	assert.Equal(t, "i", info.Key)
	assert.LessOrEqual(t, ColumnsWant(DetailIO), ColumnsWant(DetailCPU))

	d := Processes("cooler", DetailIO, []ProcessUse{{Name: "copy", Value: 1 << 20, Second: 2 << 20}, {Name: "idle", Second: 1}})
	require.Len(t, d.Lines, 2)
	assert.Equal(t, "copy", d.Lines[0].Name)
	assert.Equal(t, ioWidth, runeLen(d.Lines[0].Value))
	assert.Equal(t, ioWidth, runeLen(d.Lines[1].Value))
}

func someIO() IOFigures {
	return IOFigures{
		Read: Some(1024.0), Write: Some(2048.0), ReadTrail: []float64{1, 2}, WriteTrail: []float64{3, 4},
		Busy: []float64{0.25, -1}, HasBusy: true,
		Pressure: Some(1.5), SwapIn: 4096, HasSwap: true,
	}
}

// R4. The summary is the read and write rates with a plot of each, a cell per
// disk, the pressure and the swap.
func TestTheIOSummarySaysTheDisks(t *testing.T) {
	s, reasons := IOSummary(someIO())

	assert.Empty(t, reasons)
	assert.Empty(t, s.Meters)
	require.Len(t, s.Trails, 2)
	assert.Equal(t, []float64{1, 2}, s.Trails[0].Samples)
	assert.Equal(t, []float64{3, 4}, s.Trails[1].Samples)
	require.NotNil(t, s.Heat)
	assert.Equal(t, []float64{0.25, -1}, s.Heat.Values)
	assert.Equal(t, []string{"read", "write", IOPressure, "swap"}, names(s))
	assert.Equal(t, "  1.0 KiB/s", s.Lines[0].figure())
	assert.Equal(t, "  1.5 %", s.Lines[2].figure())
	assert.Contains(t, s.Lines[3].Value, "4.0 KiB/s")
}

// R4, D3. Windows has a queue where Linux has the pressure, and the line keeps
// its name unread; nothing read is the same shape with a reason a line.
func TestAnIOSummaryKeepsItsShape(t *testing.T) {
	f := someIO()
	f.Queues, f.Queue = true, Some(0.5)
	s, _ := IOSummary(f)
	assert.Equal(t, IOQueue, s.Lines[2].Name)
	assert.Equal(t, "  0.5", s.Lines[2].Value)
	f.Queue = Opt[float64]{}
	s, _ = IOSummary(f)
	assert.Equal(t, IOQueue, s.Lines[2].Name, "an unread queue keeps its line's name")

	full, _ := IOSummary(someIO())
	empty, reasons := IOSummary(IOFigures{})
	assert.Equal(t, names(full), names(empty))
	assert.Len(t, empty.Trails, 2)
	assert.Len(t, reasons, 5, "read, write, disks, pressure, swap")
	assert.Equal(t, runeLen(full.Lines[3].Value), runeLen(empty.Lines[3].Value), "the swap line's width")
}
