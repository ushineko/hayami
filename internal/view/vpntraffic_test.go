package view_test

import (
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/hayami/internal/view"
)

func tunnel(rx, tx float64) *view.BandwidthReading {
	return &view.BandwidthReading{
		Name: "tun0", RxRate: rx, TxRate: tx, RxTotal: 4 << 30, TxTotal: 3 << 20, HasRate: true, HasTotal: true,
		RxTrail: []float64{rx}, TxTrail: []float64{tx},
	}
}

// A tunnel's rates and totals are an interface's, in its columns and under the
// tunnel's own line, which is the same width: the card gets no wider (spec 086).
func TestATunnelsRatesAreInAnInterfacesColumns(t *testing.T) {
	iface := view.Bandwidth([]view.BandwidthReading{{
		Name: "a", RxRate: 20 << 10, TxRate: 52, RxTotal: 714 << 30, TxTotal: 3 << 20, HasRate: true, HasTotal: true,
	}}).Rows[0]
	rows := view.VPNRows([]view.VPNReading{{
		Name: "b", State: view.VPNUp, Latency: view.Some(16.0), Connected: view.Some(192 * time.Hour),
		Traffic: tunnel(20<<10, 52),
	}})
	require.Len(t, rows, 2)
	state, rates := rows[0], rows[1]

	assert.Equal(t, "b", state.Label)
	assert.Empty(t, rates.Label, "the line above names the tunnel")
	assert.Equal(t, iface.Value, rates.Value)
	assert.Equal(t, "Σ   4.0 GiB        3.0 MiB  ", rates.Detail)
	assert.Equal(t, utf8.RuneCountInString(iface.Value), utf8.RuneCountInString(state.Value))
	assert.Equal(t, utf8.RuneCountInString(iface.Detail), utf8.RuneCountInString(rates.Detail))
}

// A rate means the same colour on a tunnel's line as on an interface's.
func TestATunnelsRateTakesAnInterfacesBand(t *testing.T) {
	rows := view.VPNRows([]view.VPNReading{{Name: "b", State: view.VPNUp, Traffic: tunnel(20<<20, 52)}})
	require.Len(t, rows, 2)
	require.Len(t, rows[1].Parts, 4)
	assert.Equal(t, view.RateBand(20<<20, true), rows[1].Parts[1].Status)
	assert.NotEqual(t, view.Info, rows[1].Parts[1].Status, "a busy rate has a band")
	assert.Equal(t, view.Info, rows[1].Parts[3].Status)
}

// Only a tunnel that is up with an interface has rates, and the rest are one line.
func TestATunnelWithoutTrafficIsOneLine(t *testing.T) {
	for _, r := range []view.VPNReading{
		{Name: "down", State: view.VPNDown},
		{Name: "dialing", State: view.VPNConnecting},
		{Name: "unknown", State: view.VPNUnknown},
		{Name: "no interface", State: view.VPNUp},
	} {
		assert.Len(t, view.VPNRows([]view.VPNReading{r}), 1, r.Name)
	}
}

// The rate line is there before the first rate, blank, so the rate arriving moves
// nothing.
func TestATunnelsFirstRateMovesNothing(t *testing.T) {
	blank := *tunnel(0, 0)
	blank.HasRate = false
	before := view.VPNRows([]view.VPNReading{{Name: "b", State: view.VPNUp, Traffic: &blank}})
	after := view.VPNRows([]view.VPNReading{{Name: "b", State: view.VPNUp, Traffic: tunnel(1<<10, 1<<10)}})

	require.Len(t, before, 2)
	assert.Contains(t, before[1].Value, view.Blank)
	assert.Equal(t, utf8.RuneCountInString(after[1].Value), utf8.RuneCountInString(before[1].Value))
}

// A pane's lines are read alone, so its rate line names the tunnel. In a stack it
// leans on the line above, as an interface's totals do.
func TestAPaneNamesATunnelsRateLine(t *testing.T) {
	sec := view.Section{Title: "Network", Rows: view.VPNRows([]view.VPNReading{
		{Name: "long-connection", Label: "short", State: view.VPNUp, Traffic: tunnel(1, 1)},
	})}

	pane := view.Render([]view.Section{sec}, view.ArrangeRow, 60)
	require.Len(t, pane, 2)
	assert.True(t, strings.HasPrefix(pane[0], "short"), pane[0])
	assert.True(t, strings.HasPrefix(pane[1], "short"), pane[1])

	stack := view.Render([]view.Section{sec}, view.ArrangeStack, 60)
	require.Len(t, stack, 4, "the heading, the state, the rates and the totals")
	assert.True(t, strings.HasPrefix(stack[2], "   "), stack[2])
}

// A tunnel's trace follows the interfaces', as a pair of one colour, and a tunnel
// with no traffic keeps an empty trace, which is what empties it on the plot.
func TestATunnelsTraceFollowsTheInterfaces(t *testing.T) {
	trails := view.VPNTrails([]view.VPNReading{
		{Name: "a", Label: "short", State: view.VPNUp, Traffic: tunnel(5, 6)},
		{Name: "b", State: view.VPNDown},
	}, 2)

	require.Len(t, trails, 4)
	assert.Equal(t, "short ↓", trails[0].Name)
	assert.Equal(t, []float64{5}, trails[0].Samples)
	assert.Equal(t, 2, trails[0].Series)
	assert.True(t, trails[0].Coloured)
	assert.Equal(t, "short ↑", trails[1].Name)
	assert.Equal(t, 2, trails[1].Series)
	assert.True(t, trails[1].Secondary)
	assert.Equal(t, "b ↓", trails[2].Name)
	assert.Empty(t, trails[2].Samples)
	assert.Equal(t, 3, trails[2].Series)
}

// The drawer's plot is told how long a check takes to come round.
func TestTheVPNDetailSaysTheTimeBetweenSamples(t *testing.T) {
	d := view.VPNDetail("net", []view.VPNDetailReading{{
		VPNReading: view.VPNReading{Name: "a", State: view.VPNUp},
		Samples:    []float64{1, 2}, Failed: []bool{false, false}, CheckEvery: time.Minute,
	}})
	assert.Equal(t, time.Minute, d.Parts[0].TrailEvery)
}
