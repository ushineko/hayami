package panel_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/hayami/internal/core"
	"github.com/ushineko/hayami/internal/panel"
	"github.com/ushineko/hayami/internal/view"
)

// Windows withholding the Wi-Fi details is said, aside: the row shows what it
// could read, and the pointer and doctor say why the rest is blank (spec 037).
func TestBandwidthSaysWhenWindowsWithholdsWiFi(t *testing.T) {
	b := panel.NewBandwidth([]string{"eth0"}, fakeCounters)
	b.SetWirelessReader(func(context.Context) (map[string]core.Wireless, error) {
		return map[string]core.Wireless{"eth0": {Connected: true}}, core.ErrWirelessDenied
	})

	_, err := b.Poll(t.Context())

	require.NoError(t, err)
	sec := b.Section()
	r := find(t, sec, "details withheld")
	assert.True(t, r.Aside, "a privacy setting took a line on the card")
	assert.Contains(t, r.Detail, "ms-settings:privacy-location")
	require.Len(t, sec.Rows, 1)
	assert.Contains(t, sec.Rows[0].Value, string(view.SegmentEmpty), "the radio lost its bars")
}

// A described radio reaches the row: bars, the link line and the tip.
func TestBandwidthDrawsADescribedRadio(t *testing.T) {
	b := panel.NewBandwidth([]string{"eth0"}, fakeCounters)
	b.SetWirelessReader(func(context.Context) (map[string]core.Wireless, error) {
		return map[string]core.Wireless{"eth0": {
			Connected: true, RSSI: -60, HasRSSI: true, Frequency: 2437, HasFrequency: true,
			Channel: 6, HasChannel: true, RxRate: 144.4, HasRx: true, Generation: "Wi-Fi 4",
		}}, nil
	})

	_, err := b.Poll(t.Context())

	require.NoError(t, err)
	sec := b.Section()
	require.Len(t, sec.Rows, 1)
	assert.Contains(t, sec.Rows[0].Value, "▮▮▮▯")
	lines := sec.Rows[0].DetailLines()
	require.Len(t, lines, 2)
	assert.Contains(t, lines[1], "2.4 GHz ch   6")
	assert.Contains(t, lines[1], "144 Mb/s")
	assert.Contains(t, sec.Rows[0].Tip, "Wi-Fi 4")
	assert.Empty(t, sec.Reasons)
}
