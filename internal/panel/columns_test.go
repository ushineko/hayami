package panel_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/hayami/internal/core"
	"github.com/ushineko/hayami/internal/panel"
	"github.com/ushineko/hayami/internal/view"
)

// columnsHost is a machine of thirty-two gibibytes with two browsers and an
// editor, read twice so each has a share of the processors, and a card whose
// reader knows a browser's PID and one the process table does not.
func columnsHost() (*core.Host, *gpuReader) {
	first := []core.ProcessSample{
		{PID: 1, Name: "browser", CPUSeconds: 10}, {PID: 2, Name: "browser", CPUSeconds: 1}, {PID: 3, Name: "editor", CPUSeconds: 4},
	}
	second := []core.ProcessSample{
		{PID: 1, Name: "browser", CPUSeconds: 10.2, Memory: 2 << 30},
		{PID: 2, Name: "browser", CPUSeconds: 1.1, Memory: 1 << 30},
		{PID: 3, Name: "editor", CPUSeconds: 4.5, Memory: 4 << 30},
	}
	gpu := &gpuReader{found: []core.GPUProcess{{PID: 1, Name: "browser", Load: 30}, {PID: 99, Name: "game", Load: 60}}}
	return detailHost(stepping(first, second), gpu, ""), gpu
}

func keys(cols []view.Column) []string {
	out := make([]string, 0, len(cols))
	for _, c := range cols {
		out = append(out, c.Key)
	}
	return out
}

// cell is the trimmed cell of the line named name under the column key.
func cell(t *testing.T, d view.Detail, name, key string) string {
	t.Helper()
	col := -1
	for i, c := range d.Columns {
		if c.Key == key {
			col = i
		}
	}
	require.GreaterOrEqual(t, col, 0, "no column %q in %v", key, keys(d.Columns))
	for _, l := range d.Lines {
		if l.Name == name {
			require.Len(t, l.Cells, len(d.Columns))
			return strings.TrimSpace(l.Cells[col])
		}
	}
	require.Fail(t, "no line "+name)
	return ""
}

func polledCooler(t *testing.T, h *core.Host) *panel.Cooler {
	t.Helper()
	c := panel.NewCoolerOn(h, noScan)
	panel.SetProcessors(c, func() (float64, bool) { return 12, true }, func(context.Context) core.Graphics { return core.Graphics{} })
	_, _ = c.Poll(t.Context())
	return c
}

// Spec 092 R2, R3. Each list's columns, in its order, with the figures of
// the reading the list is ordered by: two browsers are a count of two and
// the sum of their memory, three of thirty-two gibibytes 9.4 % of it.
func TestEachListHasItsColumns(t *testing.T) {
	h, _ := columnsHost()
	c := polledCooler(t, h)

	cpu, err := c.PollDetail(t.Context(), view.DetailCPU)
	require.NoError(t, err)
	assert.Equal(t, []string{view.ColumnCount, view.ColumnMemory, view.ColumnShare}, keys(cpu.Columns))
	assert.Equal(t, "×2", cell(t, cpu, "browser", view.ColumnCount))
	assert.Equal(t, "3.0 GiB", cell(t, cpu, "browser", view.ColumnMemory))
	assert.Equal(t, "9.4 %", cell(t, cpu, "browser", view.ColumnShare))
	assert.Equal(t, "×1", cell(t, cpu, "editor", view.ColumnCount))

	// The memory's list reads the processes again. Read in the same tick of
	// Windows's clock, which can be 16 ms, the two readings are no time apart
	// and no process has a share of the processors, so no CPU column: the
	// lists are read seconds apart in a panel, and here a tick apart.
	time.Sleep(50 * time.Millisecond)
	memory, err := c.PollDetail(t.Context(), view.DetailMemory)
	require.NoError(t, err)
	assert.Equal(t, []string{view.ColumnCount, view.ColumnCPU, view.ColumnShare}, keys(memory.Columns))
	assert.Equal(t, "12.5 %", cell(t, memory, "editor", view.ColumnShare))
	assert.NotEqual(t, view.Blank, strings.Fields(cell(t, memory, "editor", view.ColumnCPU))[0],
		"a process read twice has a share of the processors")
}

// R3. The card's list joins its processes to the process table by PID; a
// process the table lacks keeps its load and shows the empty form, not a
// zero.
func TestTheCardsListJoinsByPID(t *testing.T) {
	h, _ := columnsHost()
	c := polledCooler(t, h)

	gpu, err := c.PollDetail(t.Context(), view.DetailGPU)
	require.NoError(t, err)
	assert.Equal(t, []string{"game", "browser"}, lineNames(gpu), "still ordered by the card's load")
	assert.Equal(t, []string{view.ColumnCount, view.ColumnCPU, view.ColumnMemory}, keys(gpu.Columns))
	assert.Equal(t, "2.0 GiB", cell(t, gpu, "browser", view.ColumnMemory), "the browser the card knows is PID 1 alone")
	assert.Equal(t, view.Blank, cell(t, gpu, "game", view.ColumnMemory))
	assert.Equal(t, view.Blank, cell(t, gpu, "game", view.ColumnCPU))
}

// R3. With no memory total there is nothing to be a share of, and the list
// does not offer the column.
func TestNoTotalNoShare(t *testing.T) {
	h, _ := columnsHost()
	c := panel.NewCoolerOn(h, noScan) // never polled: no total read

	cpu, err := c.PollDetail(t.Context(), view.DetailCPU)
	require.NoError(t, err)
	assert.Equal(t, []string{view.ColumnCount, view.ColumnMemory}, keys(cpu.Columns))
}

// R4. The columns are read, never sorted by: the processor's list is in its
// own order though the memory's would differ.
func TestTheColumnsDoNotOrderTheList(t *testing.T) {
	h, _ := columnsHost()
	c := polledCooler(t, h)

	cpu, err := c.PollDetail(t.Context(), view.DetailCPU)
	require.NoError(t, err)
	assert.Equal(t, []string{"editor", "browser"}, lineNames(cpu), "0.5 s against 0.3 s")
	assert.Equal(t, "×1", cell(t, cpu, "editor", view.ColumnCount))
}

// R2. A drill page of one name has the list's columns but the count; a page
// of one process has none.
func TestADrillPageHasTheColumnsButTheCount(t *testing.T) {
	h, _ := columnsHost()
	c := polledCooler(t, h)

	page, err := c.PollDetail(t.Context(), "memory/browser")
	require.NoError(t, err)
	part := page.Parts[0]
	assert.Equal(t, []string{view.ColumnCPU, view.ColumnShare}, keys(part.Columns))
	assert.Equal(t, "9.4 %", cell(t, part, "all", view.ColumnShare))
	for _, l := range part.Lines {
		if !l.Heading {
			assert.Len(t, l.Cells, 2, "line %q", l.Name)
		}
	}

	one, err := c.PollDetail(t.Context(), "memory/browser/1")
	require.NoError(t, err)
	assert.Empty(t, one.Parts[0].Columns)
}
