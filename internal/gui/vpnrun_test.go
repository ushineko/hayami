package gui_test

import (
	"fmt"
	"testing"

	"fyne.io/fyne/v2/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/hayami/internal/gui"
	"github.com/ushineko/hayami/internal/panel"
	"github.com/ushineko/hayami/internal/view"
)

// networkWith is a Network section with one interface and n tunnels up, each
// with traffic, and one tunnel down.
func networkWith(n int) view.Section {
	sec := view.Bandwidth([]view.BandwidthReading{{Name: "eno2", HasRate: true, HasTotal: true, RxRate: 1 << 20, TxRate: 1 << 10}})
	vpns := []view.VPNReading{{Name: "down", State: view.VPNDown}}
	for i := range n {
		vpns = append(vpns, view.VPNReading{
			Name: fmt.Sprintf("wg%d", i), State: view.VPNUp, Interface: fmt.Sprintf("wg%d", i),
			Traffic: &view.BandwidthReading{Name: fmt.Sprintf("wg%d", i), HasRate: true, HasTotal: true, RxRate: 1 << 10},
		})
	}
	sec.Rows = append(sec.Rows, view.VPNRows(vpns)...)
	sec.ScrollLines = view.VPNScrollLines
	return sec
}

// Spec 105 R1, R4. The tunnels that are up are in the card's run, the one that
// is down is nowhere, and the run shows nine lines: the card is as tall with
// four or five tunnels up as with three, and shorter with two.
func TestTheTunnelsAreARunHeldToNineLines(t *testing.T) {
	a := test.NewTempApp(t)
	net := &fixed{networkWith(2)}
	p := gui.New(a, gui.Options{Sources: []panel.Source{net}, Title: "hayami"})

	p.Draw("bandwidth", networkWith(2), true)
	ids, visible, shown := gui.RunIDs(p, "bandwidth")
	require.True(t, shown)
	assert.Equal(t, view.VPNScrollLines, visible)
	assert.Len(t, ids, 6, "two tunnels: state, rates and totals each")
	assert.NotContains(t, ids, "vpn:down")
	two := gui.CardHeight(p, "bandwidth")

	p.Draw("bandwidth", networkWith(3), true)
	three := gui.CardHeight(p, "bandwidth")
	assert.Greater(t, three, two)

	for _, n := range []int{4, 5} {
		p.Draw("bandwidth", networkWith(n), true)
		ids, _, _ = gui.RunIDs(p, "bandwidth")
		assert.Len(t, ids, 3*n)
		assert.InDelta(t, three, gui.CardHeight(p, "bandwidth"), 0.01, "%d tunnels grew the card", n)
	}

	for _, r := range ids {
		assert.NotContains(t, rowIDs(gui.LaidOut(p, "bandwidth")), r, "a tunnel's row is also among the card's own")
	}

	p.Draw("bandwidth", networkWith(0), true)
	ids, _, shown = gui.RunIDs(p, "bandwidth")
	assert.Empty(t, ids)
	assert.False(t, shown, "an empty run is drawn")
}

// rowIDs are the IDs of the card's own rows.
func rowIDs(rows []gui.RowPlace, _ float32) []string { return ids(rows) }
