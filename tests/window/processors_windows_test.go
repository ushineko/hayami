package window_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/hayami/internal/core"
	"github.com/ushineko/hayami/internal/view"
)

// coolerSettle is how long the panel is given after its window appears: the
// cooler polls at once and its first poll takes two processor samples apart.
const coolerSettle = 4 * time.Second

// processorRows are the labels of this machine's processor and graphics card
// rows, as the cooler section names them, or a skip when the card does not
// report to D3DKMT and there is no row to line up with.
func processorRows(t *testing.T) (cpu, gpu string) {
	t.Helper()
	g := core.NewGraphicsReader().Native(t.Context())
	if !g.HasTemperature {
		t.Skip("no card here reports to D3DKMT, so there is no row to line up with")
	}
	return view.NameLabel("CPU", core.HostCPUName()), view.NameLabel("GPU", g.Name)
}

/*
Spec 034. On Windows the processor has a load and no temperature, and its row
is drawn: the load in the column the graphics card's load is in, and nothing
where the temperature would be.

Read off the picture. The processor's and the card's lines are found by their
labels (the harness's card). The processor's line ends where the card's load
ends -- the right edge of its "%" -- to the pixel, and the card's line carries
more after that point (its temperature), which the processor's does not. A
processor row that is missing is a picture with a line fewer than the card;
a load drawn in the temperature's column ends where the card's line ends.
*/
func TestTheProcessorsLoadIsDrawnInTheCardsColumnWithoutATemperature(t *testing.T) {
	cpuLabel, gpuLabel := processorRows(t)
	// Nothing listens on the discard port: a LibreHardwareMonitor running on
	// this desk (spec 036) would otherwise give the processor a temperature.
	s := panelSettings{Sections: []string{"cooler"}, LHM: "http://127.0.0.1:9/data.json", Settle: coolerSettle}
	hwnd := start(t, s)
	c := cardOf(t, "cooler", s)

	p := shoot(t, hwnd, "processors.png")
	t.Logf("picture: %s", p.path)
	cpuLine, gpuLine := p.row(t, c, cpuLabel), p.row(t, c, gpuLabel)
	cpu, gpu := p.words(cpuLine), p.words(gpuLine)
	t.Logf("processor %v: %v", cpuLine, cpu)
	t.Logf("card      %v: %v", gpuLine, gpu)
	require.GreaterOrEqual(t, len(cpu), 2, "the processor's line is a label and a load")

	end := cpu[len(cpu)-1].To
	matched := -1
	for i, w := range gpu {
		if abs(w.To-end) <= 1 {
			matched = i
		}
	}
	require.NotEqual(t, -1, matched,
		"the processor's line ends at x=%d, where no part of the card's line ends", end)
	assert.Less(t, matched, len(gpu)-1,
		"the processor's line ends where the card's temperature does: its load has moved into the temperature's column")
}

/*
Spec 036. With LibreHardwareMonitor running, the processor has its
temperature from it, and the row is the card's shape: the temperature ends
where the card's does.

Read off the picture as above. The processor's line now ends at its degrees
sign, which is the card's line's last word too, to the pixel; and the
processor's load still ends where the card's load does. Pointed at an address
nothing serves, the processor's line ends at its load, and the first check
fails.
*/
func TestTheProcessorsTemperatureFromLibreHardwareMonitorIsInTheCardsColumn(t *testing.T) {
	cpuLabel, gpuLabel := processorRows(t)
	if _, err := core.NewLHM("").CPUTemperature(t.Context()); err != nil {
		t.Skipf("LibreHardwareMonitor gives no CPU temperature here: %v", err)
	}
	s := panelSettings{Sections: []string{"cooler"}, LHM: core.LHMURL, Settle: coolerSettle}
	hwnd := start(t, s)
	c := cardOf(t, "cooler", s)

	p := shoot(t, hwnd, "processors-lhm.png")
	t.Logf("picture: %s", p.path)
	cpuLine, gpuLine := p.row(t, c, cpuLabel), p.row(t, c, gpuLabel)
	cpu, gpu := p.words(cpuLine), p.words(gpuLine)
	t.Logf("processor %v: %v", cpuLine, cpu)
	t.Logf("card      %v: %v", gpuLine, gpu)
	require.GreaterOrEqual(t, len(cpu), 3, "the processor's line is a label, a load and a temperature")
	require.GreaterOrEqual(t, len(gpu), 3)

	assert.LessOrEqual(t, abs(cpu[len(cpu)-1].To-gpu[len(gpu)-1].To), 1,
		"the processor's line ends at x=%d and the card's at x=%d: the temperatures are not one column",
		cpu[len(cpu)-1].To, gpu[len(gpu)-1].To)
	loads := 0
	for _, c := range cpu {
		for _, g := range gpu[:len(gpu)-1] {
			if abs(c.To-g.To) <= 1 {
				loads++
			}
		}
	}
	assert.Positive(t, loads, "nothing on the processor's line but its end lines up with the card's: the load has moved")
}
