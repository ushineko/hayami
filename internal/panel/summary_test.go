package panel_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/hayami/internal/core"
	"github.com/ushineko/hayami/internal/panel"
	"github.com/ushineko/hayami/internal/view"
)

// summaryCooler is a cooler over a memory that reads read in turn, polled
// once.
func summaryCooler(t *testing.T, read ...core.Memory) *panel.Cooler {
	t.Helper()
	h := detailHost(stepping([]core.ProcessSample{{PID: 1, Name: "editor", Memory: 1 << 30}}), &gpuReader{}, "")
	call := 0
	h.Memory = core.Chain[core.Memory]{Providers: []core.Provider[core.Memory]{
		core.ProviderFunc[core.Memory]{N: "the test memory", F: func(context.Context) (core.Memory, error) {
			m := read[min(call, len(read)-1)]
			call++
			return m, nil
		}},
	}}
	c := panel.NewCoolerOn(h, noScan)
	panel.SetProcessors(c, func() (float64, bool) { return 12, true }, func(context.Context) core.Graphics { return core.Graphics{} })
	_, _ = c.Poll(t.Context())
	return c
}

// Spec 094 R5, R8. The Memory list carries the summary, from the reading the
// card's line is from: the memory is read once per poll, and a list read
// after it says what that poll read.
func TestTheMemoryListSumsUpThePollsReading(t *testing.T) {
	c := summaryCooler(t,
		core.Memory{Used: 16 << 30, Total: 32 << 30, Cache: 4 << 30, HasCache: true, SwapUsed: 1 << 30, SwapTotal: 8 << 30, HasSwap: true, SwapName: "swap"},
		core.Memory{Used: 30 << 30, Total: 32 << 30},
	)

	d, err := c.PollDetail(t.Context(), view.DetailMemory)
	require.NoError(t, err)
	require.NotNil(t, d.Summary)
	assert.Equal(t, " 16.0 GiB of  32.0 GiB", d.Summary.Meters[0].Caption, "the poll's reading, not a second one")
	assert.Equal(t, "  4.0", d.Summary.Lines[1].Value)
	assert.Empty(t, d.Reasons)

	// The next poll's reading lacks the cache and the swap: their lines stay,
	// empty, and the reasons say so.
	_, _ = c.Poll(t.Context())
	d, err = c.PollDetail(t.Context(), view.DetailMemory)
	require.NoError(t, err)
	assert.Len(t, d.Summary.Lines, 3)
	assert.Equal(t, view.NoQuantity(), d.Summary.Lines[1].Value)
	assert.Len(t, d.Reasons, 2)
}

// R4. The other lists and a drill page have no summary: a page is about a
// program, not the machine.
func TestOnlyTheMemoryListHasASummary(t *testing.T) {
	c := summaryCooler(t, core.Memory{Used: 16 << 30, Total: 32 << 30, HasCache: true, HasSwap: true})
	_, _ = c.Poll(t.Context())

	cpu, err := c.PollDetail(t.Context(), view.DetailCPU)
	require.NoError(t, err)
	assert.Nil(t, cpu.Summary)

	page, err := c.PollDetail(t.Context(), "memory/editor")
	require.NoError(t, err)
	assert.Nil(t, page.Summary)
	for _, part := range page.Parts {
		assert.Nil(t, part.Summary)
	}
}

// processorCooler is a cooler whose processor reads two online cores and an
// offline one between them, and rest; its load is read as loads in turn,
// polled once per load.
func processorCooler(t *testing.T, rest core.Processor, loads ...float64) *panel.Cooler {
	t.Helper()
	h := detailHost(stepping([]core.ProcessSample{{PID: 1, Name: "editor", CPUSeconds: 1}}), &gpuReader{}, "")
	sample := 0.0
	h.Processor = func() *core.ProcessorReader {
		return core.NewProcessorReader(func() ([]core.ProcessorTimes, error) {
			sample++
			return []core.ProcessorTimes{
				{Busy: 50 * sample, Idle: 50 * sample, Online: true}, {},
				{Busy: 10 * sample, Idle: 90 * sample, Online: true},
			}, nil
		}, func(context.Context) core.Processor { return rest })
	}
	c := panel.NewCoolerOn(h, noScan)
	call := 0
	panel.SetProcessors(c, func() (float64, bool) {
		v := loads[min(call, len(loads)-1)]
		call++
		return v, true
	}, func(context.Context) core.Graphics { return core.Graphics{} })
	for range loads {
		_, _ = c.Poll(t.Context())
	}
	return c
}

// Spec 095 R1, R3. The CPU list carries the summary: the load and its plot
// from the cooler's polls, so the plot has its history when the drawer
// opens; each core's load, an offline one in its place; the rest read now.
// A drill page under it has none.
func TestTheCPUListSumsUpTheProcessor(t *testing.T) {
	c := processorCooler(t, core.Processor{
		Clock: 3000, MaxClock: 5700, HasClock: true,
		LoadAvg: [3]float64{1, 2, 3}, HasLoadAvg: true,
		Processes: 10, Threads: 100, HasTasks: true,
	}, 10, 20, 30)

	d, err := c.PollDetail(t.Context(), view.DetailCPU)
	require.NoError(t, err)
	require.NotNil(t, d.Summary)
	assert.InDelta(t, 0.3, d.Summary.Meters[0].Fraction, 1e-9, "the last poll's load")
	assert.Equal(t, []float64{10, 20, 30}, d.Summary.Trails[0].Samples, "every poll's")
	require.NotNil(t, d.Summary.Heat)
	assert.Equal(t, []float64{0.5, -1, 0.1}, d.Summary.Heat.Values)
	assert.Equal(t, view.LoadAverage, d.Summary.Lines[1].Name)
	assert.Empty(t, d.Reasons)

	page, err := c.PollDetail(t.Context(), "cpu/editor")
	require.NoError(t, err)
	assert.Nil(t, page.Summary)
}

// D1. A machine with a queue names the line for it even when it was not
// read: the shape is the machine's, not the reading's.
func TestAQueueMachineKeepsItsLine(t *testing.T) {
	c := processorCooler(t, core.Processor{Queues: true}, 10)

	d, err := c.PollDetail(t.Context(), view.DetailCPU)
	require.NoError(t, err)
	assert.Equal(t, view.LoadQueue, d.Summary.Lines[1].Name)
	assert.Equal(t, view.NoQuantity(), d.Summary.Lines[1].Value)
}
