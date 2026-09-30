package view_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/hayami/internal/view"
)

// processors is a machine whose processor and graphics card both answered,
// with a load for each.
func processors() view.CoolerReading {
	return view.CoolerReading{
		CPU: 78, HasCPU: true, CPULoad: 12, HasCPULoad: true,
		GPU: 41, HasGPU: true, GPULoad: 4, HasGPULoad: true,
		Coolant: 38.9, HasLiquid: true,
	}
}

// R2.2. Each processor is one line: load, then temperature, the temperature
// in the column the coolant's is in.
func TestEachProcessorIsItsLoadAndTemperatureOnOneLine(t *testing.T) {
	s := view.Cooler(processors())

	require.Len(t, s.Rows, 3)
	cpu, gpu, coolant := s.Rows[0], s.Rows[1], s.Rows[2]
	assert.Equal(t, "CPU", cpu.Label)
	assert.Equal(t, " 12 %  78.0", cpu.Value)
	assert.Equal(t, "GPU", gpu.Label)
	assert.Equal(t, "  4 %  41.0", gpu.Value)
	assert.Equal(t, cpu.Unit, coolant.Unit, "the units share a column")
	assert.Equal(t, len(cpu.Value), len(gpu.Value))
}

// R2.2. The first poll has no load. The row is the same width without it, so
// the panel does not move when the second poll brings one.
func TestAProcessorWithoutALoadYetKeepsItsWidth(t *testing.T) {
	r := processors()
	r.HasCPULoad, r.HasGPULoad = false, false
	s := view.Cooler(r)

	assert.Equal(t, "       78.0", s.Rows[0].Value)
	assert.Equal(t, "       41.0", s.Rows[1].Value)
	assert.Len(t, s.Rows[0].Value, len(view.Cooler(processors()).Rows[0].Value))
}

// R2.2. No temperature is no GPU row, whatever the load says.
func TestTheGPURowIsNotDrawnWithoutATemperature(t *testing.T) {
	r := processors()
	r.HasGPU = false
	s := view.Cooler(r)

	for _, row := range s.Rows {
		assert.NotEqual(t, "GPU", row.Label)
	}
}

// R2.3. Three trails: the coolant with its band colour, the two processors
// with a series colour each -- CPU series 0, GPU series 1 -- so the window can
// tell them apart.
func TestTheCoolerHasThreeTrails(t *testing.T) {
	r := processors()
	r.Trail, r.CPUTrail, r.GPUTrail = []float64{38.9}, []float64{78}, []float64{41}
	s := view.Cooler(r)

	require.Len(t, s.Trails, 3)
	assert.Equal(t, "Coolant", s.Trails[0].Name)
	assert.False(t, s.Trails[0].Coloured, "the coolant's colour is its band")
	assert.Equal(t, view.Good, s.Trails[0].Status)
	assert.Equal(t, "CPU", s.Trails[1].Name)
	assert.True(t, s.Trails[1].Coloured)
	assert.Equal(t, 0, s.Trails[1].Series)
	assert.Equal(t, "GPU", s.Trails[2].Name)
	assert.True(t, s.Trails[2].Coloured)
	assert.Equal(t, 1, s.Trails[2].Series)
	assert.Equal(t, view.Info, s.Trails[2].Status)
	assert.Equal(t, view.ScaleEach, s.TrailScale)
}

// A card with no samples yet has no trail, as the others do not.
func TestNoGPUSamplesIsNoGPUTrail(t *testing.T) {
	r := processors()
	r.Trail, r.CPUTrail = []float64{38.9}, []float64{78}
	s := view.Cooler(r)

	require.Len(t, s.Trails, 2)
}

// A GPU reading kept from an earlier poll is its row alone drawn dim.
func TestAStaleGPUIsItsRowAlone(t *testing.T) {
	r := processors()
	r.GPUStale = true
	s := view.Cooler(r)

	assert.False(t, s.Rows[0].Stale, "the CPU is live")
	assert.True(t, s.Rows[1].Stale)
	assert.False(t, s.Rows[2].Stale, "the coolant is live")
}
