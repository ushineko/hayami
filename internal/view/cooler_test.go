package view_test

import (
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/hayami/internal/view"
)

// processors is a machine whose processor and graphics card both answered,
// with a load for each, and a coolant.
func processors() view.CoolerReading {
	return view.CoolerReading{Probes: []view.Probe{
		{ID: "cpu", Role: view.RoleCPU, Load: view.Some(12.0), Temp: view.Some(78.0)},
		{ID: "gpu", Role: view.RoleGPU, Load: view.Some(4.0), Temp: view.Some(41.0)},
		{ID: "coolant", Role: view.RoleCoolant, Temp: view.Some(38.9)},
	}}
}

// with is r with change made to the probe named id.
func with(r view.CoolerReading, id string, change func(*view.Probe)) view.CoolerReading {
	out := view.CoolerReading{Probes: append([]view.Probe(nil), r.Probes...)}
	for i := range out.Probes {
		if out.Probes[i].ID == id {
			change(&out.Probes[i])
		}
	}
	return out
}

// without is r without the probes named ids.
func without(r view.CoolerReading, ids ...string) view.CoolerReading {
	var out view.CoolerReading
	for _, p := range r.Probes {
		keep := true
		for _, id := range ids {
			keep = keep && p.ID != id
		}
		if keep {
			out.Probes = append(out.Probes, p)
		}
	}
	return out
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
	noLoad := func(p *view.Probe) { p.Load = view.Opt[float64]{} }
	r := with(with(processors(), "cpu", noLoad), "gpu", noLoad)
	s := view.Cooler(r)

	assert.Equal(t, "       78.0", s.Rows[0].Value)
	assert.Equal(t, "       41.0", s.Rows[1].Value)
	assert.Len(t, s.Rows[0].Value, len(view.Cooler(processors()).Rows[0].Value))
}

// R2.2. A card with neither a load nor a temperature is no row.
func TestAGPUWithNothingIsNoRow(t *testing.T) {
	r := with(processors(), "gpu", func(p *view.Probe) { p.Temp, p.Load = view.Opt[float64]{}, view.Opt[float64]{} })
	s := view.Cooler(r)

	for _, row := range s.Rows {
		assert.NotEqual(t, "GPU", row.Label)
	}
}

// Spec 034. A processor with a load and no temperature -- every Windows
// machine -- is a row, its load in the column the card's is in and its
// temperature and unit as spaces: the row is as wide as the card's, so the
// value and unit columns line up, and a temperature that arrived later would
// move nothing.
func TestAProcessorWithALoadAndNoTemperatureIsARowOfTheSameWidth(t *testing.T) {
	r := without(with(processors(), "cpu", func(p *view.Probe) { p.Temp = view.Opt[float64]{} }), "coolant")
	s := view.Cooler(r)

	require.Len(t, s.Rows, 2)
	cpu, gpu := s.Rows[0], s.Rows[1]
	assert.Equal(t, "CPU", cpu.Label)
	assert.Equal(t, " 12 %      ", cpu.Value)
	assert.Equal(t, "   ", cpu.Unit, "no unit for no temperature, and the unit's width kept")
	assert.Equal(t, len(gpu.Value), len(cpu.Value))
	assert.Equal(t, utf8.RuneCountInString(gpu.Unit), utf8.RuneCountInString(cpu.Unit))
	assert.Equal(t, gpu.Value[:view.LoadWidth], "  4 %", "the loads share a column")
}

// Spec 034. A processor with neither a load nor a temperature is no row.
func TestAProcessorWithNothingIsNoRow(t *testing.T) {
	r := with(processors(), "cpu", func(p *view.Probe) { p.Temp, p.Load = view.Opt[float64]{}, view.Opt[float64]{} })
	s := view.Cooler(r)

	for _, row := range s.Rows {
		assert.NotEqual(t, "CPU", row.Label)
	}
}

// R2.3. Three trails: the coolant with its band colour, the two processors
// with a series colour each -- CPU series 0, GPU series 1 -- so the window can
// tell them apart.
func TestTheCoolerHasThreeTrails(t *testing.T) {
	r := with(processors(), "coolant", func(p *view.Probe) { p.Trail = []float64{38.9} })
	r = with(r, "cpu", func(p *view.Probe) { p.Trail = []float64{78} })
	r = with(r, "gpu", func(p *view.Probe) { p.Trail = []float64{41} })
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
	r := with(processors(), "coolant", func(p *view.Probe) { p.Trail = []float64{38.9} })
	r = with(r, "cpu", func(p *view.Probe) { p.Trail = []float64{78} })
	s := view.Cooler(r)

	require.Len(t, s.Trails, 2)
}

// A GPU reading kept from an earlier poll is its row alone drawn dim.
func TestAStaleGPUIsItsRowAlone(t *testing.T) {
	r := with(processors(), "gpu", func(p *view.Probe) { p.Stale = true })
	s := view.Cooler(r)

	assert.False(t, s.Rows[0].Stale, "the CPU is live")
	assert.True(t, s.Rows[1].Stale)
	assert.False(t, s.Rows[2].Stale, "the coolant is live")
}

// Spec 044. Each row carries its probe's ID, and the speeds their own, so a
// shell can match a poll's rows to the ones it drew.
func TestEveryCoolerRowCarriesAnID(t *testing.T) {
	r := processors()
	r.Probes = append(r.Probes,
		view.Probe{ID: "fan", Role: view.RoleFan, RPM: view.Some(1200)},
		view.Probe{ID: "pump", Role: view.RolePump, RPM: view.Some(2400)})
	s := view.Cooler(r)

	ids := make([]string, 0, len(s.Rows))
	for _, row := range s.Rows {
		ids = append(ids, row.ID)
	}
	assert.Equal(t, []string{"cpu", "gpu", "coolant", view.SpeedsID}, ids)
	assert.Equal(t, "fan 1200  pump 2400", s.Rows[3].Value, "the fan first, then the pump, on one row")
}

// Spec 044. The rows are drawn in the roles' order, whatever order the
// probes arrive in: the processor, the card, the coolant.
func TestTheRowsFollowTheRolesNotTheProbesOrder(t *testing.T) {
	p := processors().Probes
	s := view.Cooler(view.CoolerReading{Probes: []view.Probe{p[2], p[1], p[0]}})

	require.Len(t, s.Rows, 3)
	assert.Equal(t, "cpu", s.Rows[0].ID)
	assert.Equal(t, "gpu", s.Rows[1].ID)
	assert.Equal(t, "coolant", s.Rows[2].ID)
}

/*
Spec 044. A machine with a second graphics card is a second GPU row and a
second GPU trace, and nothing in the view had to learn about it: the reading is
a list, and the role table says how a GPU is drawn. The second is "GPU 2" where
the card has no name, takes the next series colour, and sits after the first.
*/
func TestTwoGraphicsCardsAreTwoGPURows(t *testing.T) {
	r := processors()
	r.Probes = append(r.Probes, view.Probe{
		ID: "gpu:1", Role: view.RoleGPU, Load: view.Some(30.0), Temp: view.Some(55.0), Trail: []float64{55},
	})
	r = with(r, "gpu", func(p *view.Probe) { p.Trail = []float64{41} })
	s := view.Cooler(r)

	require.Len(t, s.Rows, 4)
	assert.Equal(t, []string{"cpu", "gpu", "gpu:1", "coolant"},
		[]string{s.Rows[0].ID, s.Rows[1].ID, s.Rows[2].ID, s.Rows[3].ID})
	assert.Equal(t, "GPU 2", s.Rows[2].Label)
	assert.Equal(t, " 30 %  55.0", s.Rows[2].Value)
	assert.Equal(t, len(s.Rows[1].Value), len(s.Rows[2].Value), "the two cards share the columns")

	require.Len(t, s.Trails, 2)
	assert.Equal(t, "GPU", s.Trails[0].Name)
	assert.Equal(t, "GPU 2", s.Trails[1].Name)
	assert.Equal(t, s.Trails[0].Series+1, s.Trails[1].Series, "the second card takes the next colour")
}

// A probe of a role the view does not know is not drawn, rather than drawn
// as something it is not.
func TestAnUnknownRoleIsNotDrawn(t *testing.T) {
	r := processors()
	r.Probes = append(r.Probes, view.Probe{ID: "vrm", Role: view.Role("vrm"), Temp: view.Some(60.0)})
	s := view.Cooler(r)

	assert.Len(t, s.Rows, 3)
}
