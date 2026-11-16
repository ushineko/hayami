package view

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// summedList is a Memory list of n programs under the memory's summary.
func summedList(n int) Detail {
	var uses []ProcessUse
	for i := range n {
		uses = append(uses, ProcessUse{Name: fmt.Sprintf("p%02d", i), Value: float64(n-i) * (1 << 20),
			Count: 1, CPU: Some(1.0), Share: Some(0.5)})
	}
	d := Processes("cooler", DetailMemory, uses)
	s, _ := MemorySummary(someMemory())
	d.Summary, d.Note = &s, "in use"
	return d
}

// Spec 098 R7. The summary is drawn where the list keeps SummaryKeepsLines
// lines under it, and then all of it, its note left out; otherwise none of
// it, the note kept. No height draws part of it.
func TestATerminalDrawerDrawsTheSummaryByHeight(t *testing.T) {
	seen := map[bool]bool{}
	for height := 4; height <= 30; height++ {
		lines := RenderDetail(summedList(100), 80, height, DrawerShape{Marked: -1})
		require.LessOrEqual(t, len(lines), height, "height %d", height)
		text := strings.Join(lines, "\n")
		drawn := strings.Contains(text, "available")
		seen[drawn] = true
		for _, part := range []string{"used", "available", "cache", "swap"} {
			assert.Equal(t, drawn, strings.Contains(text, "  "+part), "height %d: %s", height, part)
		}
		kept := 0
		for _, l := range lines {
			if strings.HasPrefix(l, "  p") {
				kept++
			}
		}
		if drawn {
			assert.GreaterOrEqual(t, kept, SummaryKeepsLines, "height %d", height)
			assert.NotContains(t, text, "in use", "the summary says it")
		}
	}
	assert.True(t, seen[true] && seen[false], "the heights did not cross the line")
}

// R7. As many columns as the width fits, whole and in order, under a
// heading line; a list too short for a line under the heading has none.
func TestATerminalDrawerFitsItsColumnsToTheWidth(t *testing.T) {
	d := summedList(5)
	d.Summary = nil
	last := -1
	// From the narrowest a line of the figure alone fits: name, gaps, figure.
	for width := 33; width <= 90; width += 4 {
		lines := RenderDetail(d, width, 20, DrawerShape{Marked: -1})
		n := columnsFit(d, FigureWidth(d.Lines), width)
		assert.GreaterOrEqual(t, n, last, "more room, fewer columns at %d", width)
		last = n
		for _, l := range lines {
			assert.LessOrEqual(t, runeLen(l), width, "a line wider than %d: %q", width, l)
			assert.NotContains(t, l, "…", "a column cut at %d: %q", width, l)
		}
		if n > 0 {
			assert.Contains(t, strings.Join(lines, "\n"), d.Columns[0].Heading)
		}
	}
	assert.Equal(t, len(d.Columns), last, "the widest drawer shows every column")

	tight := RenderDetail(d, 90, 3, DrawerShape{Marked: -1})
	assert.NotContains(t, strings.Join(tight, "\n"), "procs", "a heading over no line")
}

// R7. A detail of parts is its parts one after another, scrolled; the
// marked line is counted across them.
func TestATerminalDrawerScrollsItsParts(t *testing.T) {
	var parts []Detail
	for p := range 3 {
		part := Detail{Title: fmt.Sprintf("account %d", p)}
		for i := range 5 {
			part.Lines = append(part.Lines, DetailLine{Name: fmt.Sprintf("a%d-%d", p, i), Value: "1"})
		}
		parts = append(parts, part)
	}
	d := Detail{Title: "Usage", Parts: parts}
	assert.Len(t, Selectable(d), 15)

	top := strings.Join(RenderDetail(d, 60, 8, DrawerShape{Marked: 6}), "\n")
	assert.Contains(t, top, "account 0")
	assert.NotContains(t, top, "account 2")

	down := strings.Join(RenderDetail(d, 60, 8, DrawerShape{Marked: 6, Scroll: 9}), "\n")
	assert.NotContains(t, down, "account 0")
	assert.Contains(t, down, "› a1-1", "the seventh selectable line is marked")
}

// R10. The pane's detail has no line that acts.
func TestThePanesDetailDoesNotAct(t *testing.T) {
	d := Detail{Lines: []DetailLine{{Name: "a", Value: "1"}, {Name: "end all", Open: ActKill}, {Name: "pause", Open: ActPause}}}
	assert.Equal(t, []string{"a"}, names(Summary{Lines: PaneDetail(d, "").Lines}))
}
