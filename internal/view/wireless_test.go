package view_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/hayami/internal/view"
)

// wifi is a connected link with every figure, invented.
func wifi() view.LinkReading {
	return view.LinkReading{
		Connected: true, RSSI: -47, HasRSSI: true, Signal: 87, HasSignal: true,
		Band: "5 GHz", Channel: 149, HasChannel: true, Generation: "Wi-Fi 5",
		RxRate: 780, TxRate: 866.7, HasRx: true, HasTx: true,
	}
}

// A Wi-Fi interface's row leads with its signal in the battery's four
// segments, ahead of the rates; a wired one has none (spec 037).
func TestAWiFiRowLeadsWithItsBars(t *testing.T) {
	s := view.Bandwidth([]view.BandwidthReading{
		{Name: "Wi-Fi 2", HasRate: true, HasTotal: true, Radio: true, Link: wifi()},
		{Name: "Ethernet 2", HasRate: true, HasTotal: true},
	})
	require.Len(t, s.Rows, 2)

	assert.True(t, strings.HasPrefix(s.Rows[0].Value, "▮▮▮▮ ↓"), "the Wi-Fi row: %q", s.Rows[0].Value)
	assert.True(t, strings.HasPrefix(s.Rows[1].Value, "↓"), "the wired row: %q", s.Rows[1].Value)
	assert.NotContains(t, s.Rows[1].Value, string(view.SegmentFull))

	// The parts still make up the value exactly, so colouring moves nothing.
	var joined string
	for _, p := range s.Rows[0].Parts {
		joined += p.Text
	}
	assert.Equal(t, s.Rows[0].Value, joined)
}

func TestTheBarsFollowTheRSSI(t *testing.T) {
	cases := []struct {
		rssi int
		bars string
	}{
		{-40, "▮▮▮▮"}, {-55, "▮▮▮▮"}, {-60, "▮▮▮▯"}, {-67, "▮▮▮▯"}, {-70, "▮▮▯▯"}, {-80, "▮▯▯▯"},
	}
	for _, c := range cases {
		l := view.LinkReading{Connected: true, RSSI: c.rssi, HasRSSI: true}
		assert.Equal(t, c.bars, view.SignalBars(l), "%d dBm", c.rssi)
	}
	// Windows' percentage where there is no RSSI.
	assert.Equal(t, "▮▮▮▯", view.SignalBars(view.LinkReading{Connected: true, Signal: 60, HasSignal: true}))
	// A radio with no link is four outlines, not a blank and not a guess.
	assert.Equal(t, "▯▯▯▯", view.SignalBars(view.LinkReading{}))
}

// The link goes under the totals, on a line of its own, and says nothing of
// the network's name: there is no field for one.
func TestTheLinkIsTheLineUnderTheTotals(t *testing.T) {
	s := view.Bandwidth([]view.BandwidthReading{
		{Name: "Wi-Fi 2", HasRate: true, HasTotal: true, Radio: true, Link: wifi()},
	})
	lines := s.Rows[0].DetailLines()
	require.Len(t, lines, 2)
	assert.Contains(t, lines[0], "Σ")
	assert.Equal(t, " -47 dBm ·   5 GHz ch 149 ·  780 Mb/s", lines[1])
	assert.Contains(t, s.Rows[0].Tip, "Wi-Fi 5")
	assert.Contains(t, s.Rows[0].Tip, "signal 87 %")
}

// Neither the row nor the link line moves as the link changes, connects or
// drops (glance rule): a value that arrives must not resize the panel.
func TestTheWiFiRowHoldsStill(t *testing.T) {
	links := []view.LinkReading{
		{},
		{Connected: true},
		wifi(),
		{Connected: true, RSSI: -100, HasRSSI: true, Band: "2.4 GHz", Channel: 1, HasChannel: true, RxRate: 1, HasRx: true},
		{Connected: true, RSSI: -9, HasRSSI: true, Band: "6 GHz", Channel: 233, HasChannel: true, RxRate: 5764, HasRx: true},
	}
	values, links2 := map[int]bool{}, map[int]bool{}
	for _, l := range links {
		s := view.Bandwidth([]view.BandwidthReading{{Name: "Wi-Fi 2", HasTotal: true, Radio: true, Link: l}})
		values[len([]rune(s.Rows[0].Value))] = true
		lines := s.Rows[0].DetailLines()
		require.Len(t, lines, 2, "%+v", l)
		links2[len([]rune(lines[1]))] = true
	}
	assert.Len(t, values, 1, "the row changed width with the link: %v", values)
	assert.Len(t, links2, 1, "the link line changed width: %v", links2)
}

// One bar is the only verdict; a good signal is drawn as the rates are.
func TestOnlyOneBarIsMarked(t *testing.T) {
	weak := view.Bandwidth([]view.BandwidthReading{{Name: "w", Radio: true,
		Link: view.LinkReading{Connected: true, RSSI: -85, HasRSSI: true}}})
	strong := view.Bandwidth([]view.BandwidthReading{{Name: "w", Radio: true, Link: wifi()}})
	assert.Equal(t, view.Warn, weak.Rows[0].Parts[0].Status)
	assert.Equal(t, view.Info, strong.Rows[0].Parts[0].Status)
}
