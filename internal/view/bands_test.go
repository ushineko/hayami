package view

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// A step is reached at its From, or only above it with Above; below the first
// step is the Floor (spec 039).
func TestBandsReachAStepAtItsFromOrAboveIt(t *testing.T) {
	b := Bands[string]{Floor: "low", Steps: []Step[string]{
		{From: 10, To: "from ten"},
		{From: 20, Above: true, To: "past twenty"},
	}}

	assert.Equal(t, "low", b.Of(9.99))
	assert.Equal(t, "from ten", b.Of(10))
	assert.Equal(t, "from ten", b.Of(20))
	assert.Equal(t, "past twenty", b.Of(20.01))
	assert.Equal(t, "low", Bands[string]{Floor: "low"}.Of(1e9), "no steps is the floor")
}

// Every table in this package ascends, which is what Of's early stop relies
// on: a step out of order would be skipped for every value past it.
func TestEveryTableAscends(t *testing.T) {
	statuses := map[string]Bands[Status]{
		"CoolantBands":   CoolantBands,
		"QuotaBands":     QuotaBands,
		"BatteryBands":   BatteryBands,
		"SegmentBands":   SegmentBands,
		"RateBands":      RateBands,
		"SignalEmphasis": SignalEmphasis,
	}
	counts := map[string]Bands[int]{
		"SignalRSSIBands": SignalRSSIBands, "SignalPercentBands": SignalPercentBands,
	}
	for name, b := range statuses {
		assert.True(t, ascends(b.Steps), "%s does not ascend", name)
	}
	for name, b := range counts {
		assert.True(t, ascends(b.Steps), "%s does not ascend", name)
	}
}

func ascends[T any](steps []Step[T]) bool {
	for i := 1; i < len(steps); i++ {
		if steps[i].From < steps[i-1].From {
			return false
		}
	}
	return true
}
