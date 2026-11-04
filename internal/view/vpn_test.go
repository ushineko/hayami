package view_test

import (
	"testing"
	"time"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/hayami/internal/view"
)

// A VPN's value keeps one width whatever it says: connecting and disconnecting
// must not move the line, and every part's text joined is the value.
func TestAVPNRowKeepsItsWidthInEveryState(t *testing.T) {
	readings := []view.VPNReading{
		{Name: "a", State: view.VPNDown},
		{Name: "a", State: view.VPNConnecting},
		{Name: "a", State: view.VPNUnknown},
		{Name: "a", State: view.VPNUp, Connected: view.Some(2 * time.Minute)},
		{Name: "a", State: view.VPNUp, Connected: view.Some(11*time.Hour + 3*time.Minute)},
		{Name: "a", State: view.VPNUp, Connected: view.Some(99*24*time.Hour + 23*time.Hour)},
		{Name: "a", State: view.VPNUp, Connected: view.Some(1200 * 24 * time.Hour)},
	}
	rows := view.VPNRows(readings)
	width := utf8.RuneCountInString(rows[0].Value)

	for i, r := range rows {
		assert.Equal(t, width, utf8.RuneCountInString(r.Value), "reading %d: %q", i, r.Value)
		joined := ""
		for _, p := range r.Parts {
			joined += p.Text
		}
		assert.Equal(t, r.Value, joined)
	}
}

func TestAVPNRowSaysWhatStateItIsIn(t *testing.T) {
	cases := []struct {
		state  view.VPNState
		word   string
		status view.Status
	}{
		{view.VPNUp, "up", view.Good},
		{view.VPNDown, "down", view.Warn},
		{view.VPNConnecting, "dialing", view.Info},
		{view.VPNUnknown, view.Blank, view.Info},
	}
	for _, c := range cases {
		row := view.VPNRows([]view.VPNReading{{Name: "a", State: c.state}})[0]
		assert.Contains(t, row.Value, c.word)
		assert.Equal(t, c.status, row.Parts[0].Status, c.word)
	}
}

// A row keeps its identity while a tunnel changes state, or the window would
// rebuild it.
func TestAVPNRowHasOneIDThroughItsStates(t *testing.T) {
	up := view.VPNRows([]view.VPNReading{{Name: "a", State: view.VPNUp}})[0]
	down := view.VPNRows([]view.VPNReading{{Name: "a", State: view.VPNDown}})[0]
	assert.Equal(t, up.ID, down.ID)
	assert.NotEmpty(t, up.ID)
}

func TestALabelTakesTheCardAndTheNameTheTip(t *testing.T) {
	row := view.VPNRows([]view.VPNReading{{
		Name: "invented-long-provider-name-udp", Label: "Gateway", State: view.VPNUp, Interface: "tun0",
	}})[0]

	assert.Equal(t, "Gateway", row.Label)
	assert.Contains(t, row.Tip, "invented-long-provider-name-udp")
	assert.Contains(t, row.Tip, "tun0")
}

func TestAStaleReadingDrawsDim(t *testing.T) {
	require.True(t, view.VPNRows([]view.VPNReading{{Name: "a", Stale: true}})[0].Stale)
}

// A tunnel can push dozens of routes. They must not push out the health, or the
// next VPN.
func TestManyRoutesDoNotCrowdOutTheHealth(t *testing.T) {
	routes := make([]string, 50)
	for i := range routes {
		routes[i] = "10.0.0.0/32"
	}
	d := view.VPNDetail("bandwidth", []view.VPNDetailReading{
		{VPNReading: view.VPNReading{Name: "a", State: view.VPNUp}, Routes: routes,
			Stats: view.VPNStats{Checks: 4, Failures: 1, AverageMS: 12}},
		{VPNReading: view.VPNReading{Name: "b", State: view.VPNDown}},
	})

	require.Len(t, d.Parts, 2, "each VPN is its own part, which the window rules off")
	assert.Equal(t, "b", d.Parts[1].Title)
	names := make([]string, 0, len(d.Parts[0].Lines))
	for _, l := range d.Parts[0].Lines {
		names = append(names, l.Name)
	}
	assert.Less(t, len(names), 15)
	assert.Contains(t, names, "Uptime")
	assert.Contains(t, d.Text(), "and 46 more")
}

// Every width-sensitive latency fits its column.
func TestALatencyFitsItsColumnAtAnySize(t *testing.T) {
	want := utf8.RuneCountInString(view.VPNRows([]view.VPNReading{{Name: "a"}})[0].Value)
	for _, ms := range []float64{0, 0.4, 12, 999, 999.6, 1500, 99_949, 99_951, 1e9} {
		row := view.VPNRows([]view.VPNReading{{Name: "a", State: view.VPNUp, Latency: view.Some(ms)}})[0]
		assert.Equal(t, want, utf8.RuneCountInString(row.Value), "%v ms: %q", ms, row.Value)
	}
	failing := view.VPNRows([]view.VPNReading{{Name: "a", State: view.VPNUp, Failing: true}})[0]
	assert.Equal(t, want, utf8.RuneCountInString(failing.Value))
	assert.Contains(t, failing.Value, "fail")
}

// One long value makes a scrolling drawer as wide as itself, which pushes every
// row's value past the edge: seen on a real window, with a provider's generated
// name on a real connection.
func TestALongValueIsClippedFromItsMiddle(t *testing.T) {
	d := view.VPNDetail("bandwidth", []view.VPNDetailReading{{
		VPNReading: view.VPNReading{Name: "invented-provider-gateway-aes-128-cbc-udp-dns", Label: "a"},
		Addresses:  []string{"2001:db8:1234:5678:9abc:def0:1234:5678/64"},
	}})

	for _, l := range d.Parts[0].Lines {
		assert.LessOrEqual(t, utf8.RuneCountInString(l.Value), 28, l.Name)
		if l.Name == "Connection" {
			assert.Contains(t, l.Value, "…")
			assert.Contains(t, l.Value, "invented")
			assert.Contains(t, l.Value, "udp-dns", "both ends stay")
		}
		if l.Name == "Address" {
			assert.Contains(t, l.Value, "/64", "the prefix length stays")
		}
	}
}
