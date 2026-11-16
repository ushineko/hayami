package core

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Spec 097 R6. This machine's disks, read for real: the rates, a busy share
// per physical disk, the queue and the swap; and this process's own IO in the
// process table.
func TestWindowsReadsTheDisks(t *testing.T) {
	d := windowsDisks().Read(t.Context())

	require.True(t, d.HasRates)
	assert.True(t, d.Queues)
	assert.True(t, d.AllIO)
	assert.True(t, d.HasQueue)
	assert.True(t, d.HasSwap)
	require.True(t, d.HasBusy)
	assert.NotEmpty(t, d.Names)
	for i, b := range d.Busy {
		assert.True(t, b >= 0 && b <= 1, "%s: %v", d.Names[i], b)
	}
	t.Logf("read %.0f B/s, write %.0f B/s, disks %v busy %v, queue %.2f, swap %.0f/%.0f",
		d.Read, d.Write, d.Names, d.Busy, d.Queue, d.SwapIn, d.SwapOut)

	samples, err := systemProcesses(t.Context())
	require.NoError(t, err)
	for _, s := range samples {
		if s.PID == os.Getpid() {
			assert.True(t, s.HasIO)
			assert.Positive(t, s.IORead, "this test read its own binary")
			return
		}
	}
	t.Fatal("this process is not in the table")
}
