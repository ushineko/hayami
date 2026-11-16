package view

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const gib = 1 << 30

// A machine's memory as the summary says it: 12 of 32 GiB used, 3 GiB of
// cache, 2 of 8 GiB of swap.
func someMemory() MemoryFigures {
	return MemoryFigures{
		Used: 12 * gib, Total: 32 * gib, Cache: Some(3.0 * gib),
		Swap: Some(2.0 * gib), SwapTotal: 8 * gib, SwapName: "swap",
	}
}

// names are a summary's line names, the meters' first.
func names(s Summary) []string {
	var out []string
	for _, m := range s.Meters {
		out = append(out, m.Label)
	}
	for _, l := range s.Lines {
		out = append(out, l.Name)
	}
	return out
}

// The summary is used with its meter, available, cache and swap, in that
// order (spec 094 R5).
func TestTheMemorySummarySaysTheMachine(t *testing.T) {
	s, reasons := MemorySummary(someMemory())

	assert.Empty(t, reasons)
	assert.Equal(t, []string{"used", "available", "cache", "swap"}, names(s))
	require.Len(t, s.Meters, 1)
	assert.InDelta(t, 12.0/32, s.Meters[0].Fraction, 1e-9)
	assert.Equal(t, " 12.0 GiB of  32.0 GiB", s.Meters[0].Caption)
	assert.Equal(t, " 20.0 GiB", s.Lines[0].figure())
	assert.Equal(t, "  3.0 GiB", s.Lines[1].figure())
	assert.Equal(t, "  2.0 GiB of   8.0 GiB", s.Lines[2].figure())
}

// A figure that was not read keeps its line in the empty form and a reason
// says which; no swap is "none" and no reason (R2).
func TestAMissingFigureKeepsItsLine(t *testing.T) {
	m := someMemory()
	m.Cache, m.Swap, m.SwapName = Opt[float64]{}, Opt[float64]{}, "page file"
	s, reasons := MemorySummary(m)

	assert.Equal(t, []string{"used", "available", "cache", "page file"}, names(s))
	assert.Equal(t, NoQuantity(), s.Lines[1].Value)
	assert.Equal(t, NoQuantity(), s.Lines[2].Value)
	require.Len(t, reasons, 2)
	assert.Equal(t, "cache", reasons[0].Label)
	assert.Equal(t, "page file", reasons[1].Label)

	m = someMemory()
	m.SwapTotal, m.Swap = 0, Some(0.0)
	s, reasons = MemorySummary(m)
	assert.Empty(t, reasons)
	assert.Equal(t, SwapNone, strings.TrimSpace(s.Lines[2].Value))
}

// Every value changed, and the summary is as many lines as it was: no reading
// changes its height (R2).
func TestAReadingDoesNotChangeTheSummarysHeight(t *testing.T) {
	a, _ := MemorySummary(someMemory())
	b, _ := MemorySummary(MemoryFigures{Used: 1023, Total: 900 * gib, Cache: Some(5e12), Swap: Some(0.0), SwapTotal: 0})
	c, _ := MemorySummary(MemoryFigures{})

	assert.Equal(t, names(a), names(b))
	assert.Equal(t, names(a), names(c))
}

// The command line prints the summary over the list's heading, and the JSON
// carries it (R9).
func TestASummaryPrintsAboveTheList(t *testing.T) {
	s, _ := MemorySummary(someMemory())
	d := Detail{Title: "Memory by process", Summary: &s,
		Lines:   []DetailLine{{Name: "editor", Value: "  1.0", Unit: "GiB", Cells: []string{"  ×1"}}},
		Columns: []Column{{Key: ColumnCount, Heading: "procs", Width: 4}}}

	text := d.Text()
	used, avail, swap := strings.Index(text, "used"), strings.Index(text, "available"), strings.Index(text, "2.0 GiB of   8.0 GiB")
	heads, editor := strings.Index(text, "procs"), strings.Index(text, "editor")
	for _, i := range []int{used, avail, swap, heads, editor} {
		require.GreaterOrEqual(t, i, 0, text)
	}
	assert.Less(t, used, avail)
	assert.Less(t, swap, heads)
	assert.Less(t, heads, editor)
	assert.Contains(t, text, "12.0 GiB of  32.0 GiB")

	raw, err := json.Marshal(d)
	require.NoError(t, err)
	var back struct {
		Summary struct {
			Lines []DetailLine `json:"lines"`
		} `json:"summary"`
	}
	require.NoError(t, json.Unmarshal(raw, &back))
	assert.Len(t, back.Summary.Lines, 3)

	// A detail without one has no summary in its JSON.
	raw, err = json.Marshal(Detail{Title: "CPU by process"})
	require.NoError(t, err)
	assert.NotContains(t, string(raw), "summary")
}
