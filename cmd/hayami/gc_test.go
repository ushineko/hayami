package main

import (
	"runtime/debug"
	"testing"

	"github.com/stretchr/testify/assert"
)

// The panel collects at GCPercent when nothing says otherwise.
func TestThePanelCollectsAtItsOwnPercentage(t *testing.T) {
	previous := debug.SetGCPercent(100)
	t.Cleanup(func() { debug.SetGCPercent(previous) })

	assert.True(t, tuneGC(func(string) string { return "" }))
	assert.Equal(t, GCPercent, debug.SetGCPercent(100), "the percentage in force")
}

// GOGC wins, as it does for any Go program: the runtime has already read it,
// and overriding it would make the default impossible to measure against.
func TestGOGCIsLeftAlone(t *testing.T) {
	previous := debug.SetGCPercent(100)
	t.Cleanup(func() { debug.SetGCPercent(previous) })

	assert.False(t, tuneGC(func(k string) string {
		if k == "GOGC" {
			return "200"
		}
		return ""
	}))
	assert.Equal(t, 100, debug.SetGCPercent(100), "the percentage was changed under GOGC")
}
