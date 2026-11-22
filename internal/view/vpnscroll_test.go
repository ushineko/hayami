package view_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/hayami/internal/view"
)

// upTunnels are n tunnels that are up, each with traffic: three lines each in a
// stack (its state, its rates, its totals).
func upTunnels(n int) []view.VPNReading {
	out := make([]view.VPNReading, n)
	for i := range out {
		out[i] = view.VPNReading{Name: fmt.Sprintf("wg%d", i), State: view.VPNUp, Traffic: tunnel(1<<20, 1<<10)}
	}
	return out
}

// network is a Network section: one interface, then the tunnels as the card
// draws them.
func network(tunnels []view.VPNReading) view.Section {
	return view.Section{
		Key: "bandwidth", Title: "Network",
		Rows:        append([]view.Row{{Label: "eno2", Value: "1.0 MiB/s"}}, view.VPNRows(tunnels)...),
		ScrollLines: view.VPNScrollLines,
	}
}

// Spec 105 R1. A VPN that is down has no row and no trace; one that is
// dialling, up or unknown has a row, in the run.
func TestOnlyAVPNThatIsNotDownHasARow(t *testing.T) {
	rows := view.VPNRows([]view.VPNReading{
		{Name: "down", State: view.VPNDown},
		{Name: "dialing", State: view.VPNConnecting},
		{Name: "up", State: view.VPNUp},
		{Name: "unknown", State: view.VPNUnknown},
	})
	require.Len(t, rows, 3)
	for _, r := range rows {
		assert.NotContains(t, r.ID, "down")
		assert.True(t, r.Scrolls, r.ID)
	}
	assert.Empty(t, view.VPNTrails([]view.VPNReading{{Name: "down", State: view.VPNDown}}, 0))
	assert.False(t, view.VPNShown([]view.VPNReading{{Name: "down", State: view.VPNDown}}))
	assert.True(t, view.VPNShown([]view.VPNReading{{Name: "down"}, {Name: "up", State: view.VPNUp}}))
}

// Spec 105 R4. Five tunnels up are fifteen lines; the stack shows nine of them,
// with a bar beside them, and every line of the section is the pane's width.
func TestARunLongerThanItsLinesShowsItsLinesAndABar(t *testing.T) {
	sec := network(upTunnels(5))
	assert.Equal(t, 15, sec.RunLines(view.ArrangeStack))
	lines, regions := view.RenderRegions([]view.Section{sec}, view.ArrangeStack, 60, nil)

	require.Len(t, regions, 1)
	r := regions[0]
	assert.Equal(t, 9, r.RunH)
	assert.Equal(t, 2, r.RunY, "the run is not under the title and the interface")
	for i, l := range lines[1:] {
		assert.Equal(t, 60, len([]rune(l)), "line %d: %q", i+1, l)
	}
	run := lines[r.RunY : r.RunY+r.RunH]
	assert.True(t, strings.HasPrefix(run[0], "wg0"), run[0])
	assert.True(t, strings.HasSuffix(run[0], "┃"), "the thumb is not at the top: %q", run[0])
	assert.True(t, strings.HasSuffix(run[8], "│"), "the track is not at the bottom: %q", run[8])
	assert.NotContains(t, strings.Join(lines, "\n"), "wg3", "a fourth tunnel was drawn without scrolling")
}

// Spec 105 R4. Scrolled, the run starts further down, and a scroll past the end
// stops at the last line.
func TestAScrolledRunStartsWhereItWasScrolledTo(t *testing.T) {
	sec := network(upTunnels(5))
	sec.ScrollFirst = 3
	lines, regions := view.RenderRegions([]view.Section{sec}, view.ArrangeStack, 60, nil)
	assert.True(t, strings.HasPrefix(lines[regions[0].RunY], "wg1"), lines[regions[0].RunY])

	sec.ScrollFirst = 99
	assert.Equal(t, 6, sec.MostFirst(view.ArrangeStack))
	lines, regions = view.RenderRegions([]view.Section{sec}, view.ArrangeStack, 60, nil)
	last := lines[regions[0].RunY+regions[0].RunH-1]
	assert.Contains(t, last, "Σ", "the last line shown is not the last tunnel's totals")
	assert.True(t, strings.HasSuffix(last, "┃"), "the thumb is not at the bottom: %q", last)
}

// A run that fits draws as rows always have: no bar, and the full width.
func TestARunThatFitsHasNoBar(t *testing.T) {
	lines, regions := view.RenderRegions([]view.Section{network(upTunnels(2))}, view.ArrangeStack, 60, nil)
	assert.Equal(t, 6, regions[0].RunH)
	text := strings.Join(lines, "\n")
	assert.NotContains(t, text, "┃")
	assert.NotContains(t, text, "│")
}

// Spec 105 R8. In row the tunnels are two lines each, the totals left out, and
// the run is still nine lines.
func TestARunInRowIsNineLinesWithoutTotals(t *testing.T) {
	sec := network(upTunnels(5))
	assert.Equal(t, 10, sec.RunLines(view.ArrangeRow))
	_, regions := view.RenderRegions([]view.Section{sec}, view.ArrangeRow, 80, nil)
	assert.Equal(t, 9, regions[0].RunH)
	assert.Equal(t, 1, sec.MostFirst(view.ArrangeRow))
}

// A pointer finds a section and its run from the regions, in a grid as in a
// stack: the second column's section is to the right of the first's.
func TestTheRegionsSayWhereEachSectionAndItsRunAre(t *testing.T) {
	other := view.Section{Key: "cooler", Title: "Hardware", Rows: []view.Row{{Label: "CPU", Value: "41 °C"}}}
	_, regions := view.RenderRegions([]view.Section{network(upTunnels(5)), other}, view.ArrangeGrid, 120, nil)
	require.Len(t, regions, 2)
	net, cool := regions[0], regions[1]
	assert.Equal(t, "bandwidth", net.Key)
	assert.Greater(t, cool.X, net.X)
	assert.True(t, net.InRun(net.X+1, net.RunY))
	assert.False(t, net.InRun(net.X+1, net.RunY-1), "the interface's line is in the run")
	assert.False(t, cool.InRun(cool.X+1, cool.Y+1), "a section without a run has one")
	assert.True(t, cool.Contains(cool.X, cool.Y))
}
