package view

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Spec 096 R1. The memory used of the total with its meter, the two clocks,
// and the power of its limit.
func TestTheGPUSummarySaysTheCard(t *testing.T) {
	s, reasons := GPUSummary(GPUFigures{
		VRAM: 11 * gib, VRAMTotal: 24 * gib, HasVRAM: true,
		Clock: Some(780.0), MemClock: Some(810.4), Power: Some(57.52), PowerLimit: 400,
	})

	assert.Empty(t, reasons)
	require.Len(t, s.Meters, 1)
	assert.Equal(t, "VRAM", s.Meters[0].Label)
	assert.InDelta(t, 11.0/24, s.Meters[0].Fraction, 1e-9)
	assert.Equal(t, " 11.0 GiB of  24.0 GiB", s.Meters[0].Caption)
	assert.Equal(t, []string{"VRAM", "clock", "mem clock", "power"}, names(s))
	assert.Equal(t, "  780 MHz", s.Lines[0].figure())
	assert.Equal(t, "  810 MHz", s.Lines[1].figure())
	assert.Equal(t, " 57.5 of 400.0 W", s.Lines[2].figure())
}

// R2. A card that gives nothing has the same lines, each empty, and a reason
// for each; a power without a limit is the draw alone.
func TestAGPUSummaryOfNothingKeepsItsShape(t *testing.T) {
	full, _ := GPUSummary(GPUFigures{HasVRAM: true, VRAMTotal: 1, Clock: Some(1.0), MemClock: Some(1.0), Power: Some(1.0)})
	empty, reasons := GPUSummary(GPUFigures{})

	assert.Equal(t, names(full), names(empty))
	assert.Len(t, reasons, 4)
	for _, l := range empty.Lines {
		assert.Equal(t, NoQuantity(), l.Value, l.Name)
	}
	assert.Equal(t, "  1.0 W", full.Lines[2].figure())
}
