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
