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
