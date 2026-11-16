package core

import (
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Spec 095 R5. This machine's processor, read for real: a load for every
// core, a clock, a run queue and the tasks.
func TestWindowsReadsTheProcessor(t *testing.T) {
	p := windowsProcessor().Read(t.Context())

	require.True(t, p.HasCores)
	assert.Len(t, p.Cores, runtime.NumCPU())
	for i, c := range p.Cores {
		assert.True(t, c >= 0 && c <= 100, "core %d: %v", i, c)
	}
	assert.True(t, p.HasClock)
	assert.Positive(t, p.Clock)
	assert.True(t, p.HasQueue)
	assert.True(t, p.HasTasks)
	assert.Positive(t, p.Processes)
	assert.Greater(t, p.Threads, p.Processes)
	t.Logf("cores %.0f, clock %.0f MHz, queue %.0f, %d processes, %d threads", p.Cores, p.Clock, p.Queue, p.Processes, p.Threads)
}
