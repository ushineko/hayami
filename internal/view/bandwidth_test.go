package view_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/hayami/internal/view"
)

// An interface is one line, with its totals under it.
//
// It used to be four: a row per direction and a totals line under each. A card
// per interface, on a panel meant to be glanced at.
func TestAnInterfaceIsOneRow(t *testing.T) {
	s := view.Bandwidth([]view.BandwidthReading{
		{Name: "eno2", RxRate: 940_000, TxRate: 920_000, RxTotal: 1 << 38, TxTotal: 1 << 37,
			HasRate: true, HasTotal: true},
	})

	require.Len(t, s.Rows, 1, "an interface took more than one row")
	assert.Equal(t, "eno2", s.Rows[0].Label)
}

// Both directions are on that line, and both totals on the one under it.
func TestBothDirectionsAreDrawn(t *testing.T) {
	s := view.Bandwidth([]view.BandwidthReading{
		{Name: "eno2", RxRate: 940_000, TxRate: 920_000, RxTotal: 1 << 38, TxTotal: 1 << 37,
			HasRate: true, HasTotal: true},
	})
	require.Len(t, s.Rows, 1)

	assert.Contains(t, s.Rows[0].Value, "↓")
	assert.Contains(t, s.Rows[0].Value, "↑")
	assert.Contains(t, s.Rows[0].Detail, "Σ")
	assert.Contains(t, s.Rows[0].Detail, "↓")
	assert.Contains(t, s.Rows[0].Detail, "↑")
}

/*
The line does not move as the rates change.

This is the width rule, and it matters more with two figures on a line than
with one: two changing values have two chances to drag it about. A rate
crossing from KiB/s to MiB/s is the case that does it, because the unit is a
different number of characters.
*/
func TestTheLineDoesNotMoveAsTheRatesChange(t *testing.T) {
	widths := map[int]bool{}
	for _, pair := range [][2]float64{
		{0, 0},
		{1, 9},
		{940, 1},
		{940_000, 920_000},
		{9_400_000_000, 1},
	} {
		s := view.Bandwidth([]view.BandwidthReading{
			{Name: "eno2", RxRate: pair[0], TxRate: pair[1], HasRate: true},
		})
		require.Len(t, s.Rows, 1)
		widths[len([]rune(s.Rows[0].Value))] = true
	}

	assert.Len(t, widths, 1, "the value column changed width with the reading: %v", widths)
}

// The totals do not move it either.
func TestTheTotalsDoNotMoveTheLine(t *testing.T) {
	widths := map[int]bool{}
	for _, total := range []uint64{0, 1, 1 << 20, 1 << 38, 1 << 45} {
		s := view.Bandwidth([]view.BandwidthReading{
			{Name: "eno2", RxTotal: total, TxTotal: total, HasTotal: true},
		})
		require.Len(t, s.Rows, 1)
		widths[len([]rune(s.Rows[0].Detail))] = true
	}

	assert.Len(t, widths, 1, "the totals line changed width: %v", widths)
}

// An interface that has not been read yet is drawn at its full width, so the
// card does not jump when the first reading arrives.
func TestAnInterfaceWithNoReadingKeepsItsWidth(t *testing.T) {
	blank := view.Bandwidth([]view.BandwidthReading{{Name: "eno2"}})
	read := view.Bandwidth([]view.BandwidthReading{
		{Name: "eno2", RxRate: 940_000, TxRate: 920_000, HasRate: true},
	})

	require.Len(t, blank.Rows, 1)
	require.Len(t, read.Rows, 1)
	assert.Equal(t,
		len([]rune(read.Rows[0].Value)),
		len([]rune(blank.Rows[0].Value)),
		"the row changed width when its first reading arrived")
}

// A reading with no totals has no second line at all, rather than an empty one.
func TestNoTotalsIsNoSecondLine(t *testing.T) {
	s := view.Bandwidth([]view.BandwidthReading{
		{Name: "eno2", RxRate: 1, TxRate: 1, HasRate: true},
	})
	require.Len(t, s.Rows, 1)
	assert.Empty(t, s.Rows[0].Detail)
}

// Several interfaces are several rows, in the order they were given.
func TestSeveralInterfacesKeepTheirOrder(t *testing.T) {
	s := view.Bandwidth([]view.BandwidthReading{
		{Name: "eno2"}, {Name: "wg0"}, {Name: "tailscale0"},
	})

	require.Len(t, s.Rows, 3)
	var names []string
	for _, r := range s.Rows {
		names = append(names, r.Label)
	}
	assert.Equal(t, []string{"eno2", "wg0", "tailscale0"}, names)
}

// No interfaces is no rows: the section is dark until one is named, which is
// this program refusing to guess what is interesting about somebody's network.
func TestNoInterfacesIsNoRows(t *testing.T) {
	s := view.Bandwidth(nil)
	assert.Empty(t, s.Rows)
	assert.Equal(t, "bandwidth", s.Key)
}

// The card is half the height it was, which is the whole point.
func TestTheCardIsHalfTheHeightItWas(t *testing.T) {
	s := view.Bandwidth([]view.BandwidthReading{
		{Name: "eno2", HasRate: true, HasTotal: true},
		{Name: "wg0", HasRate: true, HasTotal: true},
	})

	// Two interfaces: two rows and two detail lines, where it was four and
	// four. The pane draws a detail line under its row, so this counts what
	// the pane will draw.
	lines := 0
	for _, r := range s.Rows {
		lines++
		if r.Detail != "" {
			lines++
		}
	}
	assert.Equal(t, 4, lines, "two interfaces should be four lines, not eight")
}

// The arrows are the monitor's, and they are what makes one line legible as
// two readings.
func TestTheDirectionsAreMarkedTheWayTheMonitorMarksThem(t *testing.T) {
	s := view.Bandwidth([]view.BandwidthReading{
		{Name: "eno2", RxRate: 1, TxRate: 2, HasRate: true},
	})
	require.Len(t, s.Rows, 1)

	down := strings.Index(s.Rows[0].Value, "↓")
	up := strings.Index(s.Rows[0].Value, "↑")
	require.NotEqual(t, -1, down)
	require.NotEqual(t, -1, up)
	assert.Less(t, down, up, "down is read first")
}

// Each interface drawn gets two trails, down then up, in the rows' order, and
// the section asks for one scale across them (spec 021). Coloured, Series and
// Secondary are what lets the window colour an interface's two lines as a pair.
func TestTwoInterfacesAreFourTrailsUnderOneScale(t *testing.T) {
	s := view.Bandwidth([]view.BandwidthReading{
		{Name: "eno2", HasRate: true, HasTotal: true, RxTrail: []float64{1, 2}, TxTrail: []float64{3, 4}},
		{Name: "wlan0", HasRate: true, HasTotal: true, RxTrail: []float64{5}, TxTrail: []float64{6}},
	})

	assert.Equal(t, view.ScaleShared, s.TrailScale)
	assert.Equal(t, []view.Trail{
		{Name: "eno2 ↓", Samples: []float64{1, 2}, Status: view.Info, Series: 0, Coloured: true},
		{Name: "eno2 ↑", Samples: []float64{3, 4}, Status: view.Info, Series: 0, Secondary: true, Coloured: true},
		{Name: "wlan0 ↓", Samples: []float64{5}, Status: view.Info, Series: 1, Coloured: true},
		{Name: "wlan0 ↑", Samples: []float64{6}, Status: view.Info, Series: 1, Secondary: true, Coloured: true},
	}, s.Trails)
}

// The cooler keeps its own-range plot: the zero value of TrailScale.
func TestTheCoolerKeepsEachTrailOnItsOwnScale(t *testing.T) {
	s := view.Cooler(view.CoolerReading{Probes: []view.Probe{{ID: "coolant", Role: view.RoleCoolant, Temp: view.Some(46.0), Trail: []float64{46}}}})

	assert.Equal(t, view.ScaleEach, s.TrailScale)
}

/*
Spec 031, R1.2. Each rate is drawn in its band, the down and the up apart: a
rate in each band is given that band's status, whichever direction it is in.
*/
func TestEachRateIsInItsOwnBand(t *testing.T) {
	bands := []struct {
		rate float64
		want view.Status
	}{
		{0, view.Info},
		{12 << 10, view.Info},
		{view.RateNotable - 1, view.Info},
		{view.RateNotable, view.Accent},
		{4 << 20, view.Accent},
		{view.RateBusy, view.Warn},
		{80 << 20, view.Warn},
		{view.RateFlatOut, view.Strong},
		{3 << 30, view.Strong},
	}
	for _, b := range bands {
		s := view.Bandwidth([]view.BandwidthReading{
			{Name: "eno2", RxRate: b.rate, TxRate: 1, HasRate: true},
			{Name: "wlan0", RxRate: 1, TxRate: b.rate, HasRate: true},
		})
		require.Len(t, s.Rows, 2)
		down, up := downUp(t, s.Rows[0]), downUp(t, s.Rows[1])

		assert.Equal(t, b.want, down[0], "down at %.0f B/s", b.rate)
		assert.Equal(t, view.Info, down[1], "the up rate took the down's band")
		assert.Equal(t, view.Info, up[0], "the down rate took the up's band")
		assert.Equal(t, b.want, up[1], "up at %.0f B/s", b.rate)
	}
}

// downUp is the statuses of a bandwidth row's two rates, down then up.
func downUp(t *testing.T, r view.Row) [2]view.Status {
	t.Helper()
	require.Len(t, r.Parts, 4, "a row is ↓, the down rate, ↑ and the up rate")
	assert.Contains(t, r.Parts[0].Text, "↓")
	assert.Contains(t, r.Parts[2].Text, "↑")
	return [2]view.Status{r.Parts[1].Status, r.Parts[3].Status}
}

// R1.2. No band is the error colour: a fast download is not a failure. And a
// rate that has not arrived has none.
func TestNoRateIsAFailure(t *testing.T) {
	for _, rate := range []float64{0, 1 << 20, 50 << 20, 900 << 20, 1 << 40} {
		assert.NotEqual(t, view.Bad, view.RateBand(rate, true))
		assert.NotEqual(t, view.Good, view.RateBand(rate, true))
	}
	assert.Equal(t, view.Info, view.RateBand(500<<20, false))
}

// R1.4. The parts are the value, exactly: colouring them changes no width.
// The padded strings are the ones the row drew before it had parts.
func TestThePartsAreTheValue(t *testing.T) {
	for _, rate := range []float64{0, 940, 2 << 20, 40 << 20, 400 << 20} {
		s := view.Bandwidth([]view.BandwidthReading{
			{Name: "eno2", RxRate: rate, TxRate: rate / 3, HasRate: true},
		})
		row := s.Rows[0]
		joined := ""
		for _, p := range row.Parts {
			joined += p.Text
		}
		assert.Equal(t, row.Value, joined)

		rx, rxUnit := view.Rate(rate)
		tx, txUnit := view.Rate(rate / 3)
		unit := view.UnitWidth("B/s", "KiB/s", "MiB/s", "GiB/s", "TiB/s")
		assert.Equal(t, "↓ "+rx+" "+view.PadUnit(rxUnit, unit)+"  ↑ "+tx+" "+view.PadUnit(txUnit, unit), row.Value,
			"the value is not the string it was before it had parts")
	}
}

// R1.3. The totals are a record, not a rate: the detail line takes no band,
// however large.
func TestTheTotalsTakeNoBand(t *testing.T) {
	s := view.Bandwidth([]view.BandwidthReading{
		{Name: "eno2", RxRate: 400 << 20, TxRate: 400 << 20, RxTotal: 1 << 45, TxTotal: 1 << 44,
			HasRate: true, HasTotal: true},
	})

	flat := view.Section{Rows: s.Rows}.Lines()
	require.Len(t, flat, 1)
	assert.NotEmpty(t, flat[0].Detail)
	assert.Equal(t, view.Info, flat[0].Status, "the bands are in the parts, not on the row")

	var painted []string
	view.RenderWith([]view.Section{s}, view.ArrangeStack, 60, func(text string, st view.Status) string {
		if strings.Contains(text, "Σ") {
			painted = append(painted, text)
			assert.Equal(t, view.Dim, st, "the totals took a colour")
		}
		return text
	})
	assert.NotEmpty(t, painted, "the totals were not drawn")
}

// R3.1. The pane colours each rate on its own, and keeps the arrows plain.
// A row that has gone is dimmed whole: a band is a statement about now.
func TestThePaneColoursEachRate(t *testing.T) {
	s := view.Bandwidth([]view.BandwidthReading{
		{Name: "eno2", RxRate: 40 << 20, TxRate: 2 << 10, HasRate: true},
	})
	seen := map[view.Status]string{}
	paint := func(text string, st view.Status) string { seen[st] += text; return text }

	view.RenderWith([]view.Section{s}, view.ArrangeStack, 60, paint)

	assert.Contains(t, seen[view.Warn], "MiB/s")
	assert.NotContains(t, seen[view.Warn], "KiB/s", "the up rate took the down's colour")
	assert.Contains(t, seen[view.Info], "KiB/s")
	assert.Contains(t, seen[view.Info], "↓")

	s.Gone = true
	seen = map[view.Status]string{}
	view.RenderWith([]view.Section{s}, view.ArrangeStack, 60, paint)
	assert.Empty(t, seen[view.Warn], "a rate from a source that has gone kept its band")
}
