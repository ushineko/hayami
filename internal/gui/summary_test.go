package gui_test

import (
	"math"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/hayami/internal/gui"
	"github.com/ushineko/hayami/internal/view"
)

// summed is a Memory list of n programs under a summary of the memory, used
// at used GiB, with the note the summary covers.
func summed(n int, used float64) view.Detail {
	d := processes(view.DetailMemory, n, 1<<30)
	s, _ := view.MemorySummary(view.MemoryFigures{
		Used: used * (1 << 30), Total: 32 * (1 << 30), Cache: view.Some(3.0 * (1 << 30)),
		Swap: view.Some(1.0 * (1 << 30)), SwapTotal: 8 * (1 << 30), SwapName: "swap",
	})
	d.Summary = &s
	d.Note = "in use"
	return d
}

func summarySource(d view.Detail) *detailSource {
	return newDetailSource("cooler", coolerSection(), map[string]view.Detail{view.DetailMemory: d})
}

// kept is how many lines a drawer shows under its summary: the list's, and
// its reasons' and note's.
func kept(p *gui.Panel) int { return len(gui.DrawerLines(p)) + len(gui.DrawerNoteLines(p)) }

// Spec 094 R1, R3. A drawer with room draws the whole summary over the list,
// and the list keeps at least SummaryKeepsLines lines; the note the summary
// says already is not drawn.
func TestASummaryStandsOverAListWithRoom(t *testing.T) {
	src := summarySource(summed(200, 12))
	p, await := drawerPanel(t, src)
	gui.Tap(p, "cooler", view.DetailMemory, -1)
	await("the summary", func() bool { lines, _ := gui.DrawerSummary(p); return len(lines) > 0 })

	lines, meters := gui.DrawerSummary(p)
	assert.Equal(t, 1, meters)
	require.Len(t, lines, 3)
	assert.True(t, strings.HasPrefix(lines[0], "available|"), lines[0])
	assert.True(t, strings.HasPrefix(lines[2], "swap|"), lines[2])
	assert.GreaterOrEqual(t, kept(p), gui.SummaryKeepsLines)
	assert.Empty(t, gui.DrawerNoteLines(p), "the note says what the summary says")
}

// R3. A drawer too short for the summary and ten lines draws none of it, and
// the list is the one it would be without a summary, note and all.
func TestAShortDrawerHasNoSummary(t *testing.T) {
	src := summarySource(summed(200, 12))
	p, await := drawerPanelOver(t, nil, src)
	gui.Tap(p, "cooler", view.DetailMemory, -1)
	await("the list", func() bool { return len(gui.DrawerLines(p)) > 0 })

	lines, meters := gui.DrawerSummary(p)
	assert.Empty(t, lines)
	assert.Zero(t, meters)
	with, notes := gui.DrawerLines(p), gui.DrawerNoteLines(p)
	assert.Equal(t, []string{"in use"}, notes)

	plain := summed(200, 12)
	plain.Summary = nil
	src.set(view.DetailMemory, plain)
	polls, _ := src.count(view.DetailMemory)
	await("two readings more", func() bool { n, _ := src.count(view.DetailMemory); return n >= polls+2 })
	assert.Equal(t, with, gui.DrawerLines(p))
	assert.Equal(t, notes, gui.DrawerNoteLines(p))
}

// R3. Whatever the height, the summary is drawn exactly where the list keeps
// its ten lines under it, and whole: no height draws part of it.
func TestTheSummaryIsDrawnWhereTenLinesStayUnderIt(t *testing.T) {
	seen := map[bool]bool{}
	for rows := 0; rows <= 14; rows++ {
		src := summarySource(summed(200, 12))
		p, await := drawerPanelOver(t, cardOf(rows), src)
		gui.Tap(p, "cooler", view.DetailMemory, -1)
		await("the list", func() bool { return len(gui.DrawerLines(p)) > 0 })

		lines, meters := gui.DrawerSummary(p)
		drawn := len(lines) > 0
		seen[drawn] = true
		if drawn {
			assert.Len(t, lines, 3, "rows %d: part of the summary", rows)
			assert.Equal(t, 1, meters, "rows %d", rows)
			assert.GreaterOrEqual(t, kept(p), gui.SummaryKeepsLines, "rows %d", rows)
			continue
		}
		assert.Zero(t, meters, "rows %d", rows)
		// Without it, the list would have kept fewer than ten lines under
		// it: the four lines it takes plus ten more do not fit.
		assert.Less(t, kept(p), gui.SummaryKeepsLines+4, "rows %d: room for the summary, and none drawn", rows)
	}
	assert.True(t, seen[true] && seen[false], "the heights tried did not cross the line: %v", seen)
}

// R2, R3. A reading with every figure changed, and a reason more, neither
// draws nor hides the summary nor changes its lines' count, in a drawer with
// room and in one without.
func TestAReadingNeitherDrawsNorHidesTheSummary(t *testing.T) {
	for _, card := range []bool{true, false} {
		src := summarySource(summed(200, 12))
		p, await := drawerPanelOver(t, nil, src)
		if card {
			p, await = drawerPanel(t, src)
		}
		gui.Tap(p, "cooler", view.DetailMemory, -1)
		await("the list", func() bool { return len(gui.DrawerLines(p)) > 0 })
		before, meters := gui.DrawerSummary(p)

		next := summed(200, 30)
		next.Reasons = []view.Reason{{Label: "cache", Text: "not read"}}
		next.Summary.Lines[1].Value = view.NoQuantity()
		src.set(view.DetailMemory, next)
		await("the new reading", func() bool {
			return strings.Contains(strings.Join(gui.DrawerNoteLines(p), " "), "cache: not read")
		})

		after, metersAfter := gui.DrawerSummary(p)
		assert.Len(t, after, len(before), "card %v", card)
		assert.Equal(t, meters, metersAfter, "card %v", card)
	}
}

// cpuSummed is a CPU list of n programs under the processor's summary, its
// load at load percent and its cores at cores (negative offline).
func cpuSummed(n int, load float64, cores ...float64) view.Detail {
	d := processes(view.DetailCPU, n, 1)
	s, _ := view.CPUSummary(view.CPUFigures{
		Load: view.Some(load), Trail: []float64{load / 2, load},
		Cores: cores, HasCores: true, Clock: view.Some(3000.0), MaxClock: 5700,
		LoadKind: view.LoadAverage, LoadAvg: view.Some([3]float64{1, 2, 3}),
		Processes: 10, Threads: 100, HasTasks: true,
	})
	d.Summary = &s
	return d
}

func cpuSource(d view.Detail) *detailSource {
	return newDetailSource("cooler", coolerSection(), map[string]view.Detail{view.DetailCPU: d})
}

// Spec 095 R1, R7, R8. With room, the CPU list's summary is its meter, the
// plot of the load, a cell per core with an offline one empty, and its
// lines, and the list keeps its ten lines under it.
func TestTheCPUSummaryDrawsItsPlotAndItsCores(t *testing.T) {
	p, await := drawerPanelOver(t, cardOf(30), cpuSource(cpuSummed(200, 40, 90, -1, 10)))
	gui.Tap(p, "cooler", view.DetailCPU, -1)
	await("the summary", func() bool { lines, _ := gui.DrawerSummary(p); return len(lines) > 0 })

	lines, meters := gui.DrawerSummary(p)
	assert.Equal(t, 1, meters)
	assert.Len(t, lines, 3)
	plotted, samples := gui.DrawerSummaryPlot(p)
	assert.True(t, plotted)
	assert.Equal(t, 2, samples)
	cells, shown := gui.DrawerSummaryHeat(p)
	assert.True(t, shown)
	require.Len(t, cells, 3)
	assert.InDelta(t, 0.9, cells[0], 1e-9)
	assert.True(t, math.IsNaN(cells[1]), "the offline core")
	assert.GreaterOrEqual(t, kept(p), gui.SummaryKeepsLines)
}

// R2, R3. A drawer too short for all of it draws none of it: no plot, no
// strip, no line.
func TestAShortDrawerHasNoCPUSummary(t *testing.T) {
	p, await := drawerPanelOver(t, cardOf(6), cpuSource(cpuSummed(200, 40, 90, 10)))
	gui.Tap(p, "cooler", view.DetailCPU, -1)
	await("the list", func() bool { return len(gui.DrawerLines(p)) > 0 })

	lines, meters := gui.DrawerSummary(p)
	assert.Empty(t, lines)
	assert.Zero(t, meters)
	plotted, _ := gui.DrawerSummaryPlot(p)
	assert.False(t, plotted)
	_, shown := gui.DrawerSummaryHeat(p)
	assert.False(t, shown)
}

// R2. A core going offline, and every figure changing, moves no line: the
// list keeps its length.
func TestACoreGoingOfflineMovesNothing(t *testing.T) {
	src := cpuSource(cpuSummed(200, 40, 90, 50, 10))
	p, await := drawerPanelOver(t, cardOf(30), src)
	gui.Tap(p, "cooler", view.DetailCPU, -1)
	await("the summary", func() bool { lines, _ := gui.DrawerSummary(p); return len(lines) > 0 })
	before := len(gui.DrawerLines(p))

	src.set(view.DetailCPU, cpuSummed(200, 95, 100, -1, 100))
	await("the new reading", func() bool {
		cells, _ := gui.DrawerSummaryHeat(p)
		return len(cells) == 3 && math.IsNaN(cells[1])
	})
	assert.Equal(t, before, len(gui.DrawerLines(p)))
	lines, _ := gui.DrawerSummary(p)
	assert.Len(t, lines, 3)
}
