package panel_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/hayami/internal/config"
	"github.com/ushineko/hayami/internal/core"
	"github.com/ushineko/hayami/internal/panel"
	"github.com/ushineko/hayami/internal/view"
)

// A tunnel that is up gets its traffic on the card from the kernel's counters, with
// no interface chosen, and one that is down does not (spec 086).
func TestAnUpTunnelHasItsTrafficOnTheCard(t *testing.T) {
	tempCache(t)
	rx := uint64(1000)
	counters := func() (map[string]core.Counters, error) {
		rx += 2048
		return map[string]core.Counters{"tun0": {Rx: rx, Tx: 10}, "eno2": {Rx: 5}}, nil
	}
	state := core.VPNUp
	active := func(context.Context) ([]core.VPN, error) {
		return []core.VPN{{Name: "a", State: state, Interface: "tun0"}, {Name: "b", State: core.VPNDown}}, nil
	}
	b := panel.NewBandwidth([]string{"eno2"}, counters)
	b.SetVPNSystems([]core.VPNBackend{{Name: "fake", Active: active}}, nil,
		config.BandwidthSettings{VPNs: []config.VPNItem{{Name: "a"}, {Name: "b"}}})

	// The first read has no rate to give, and its line is there all the same.
	_, err := b.Poll(context.Background())
	require.NoError(t, err)
	rows := vpnRows(b.Section())
	require.Len(t, rows, 3, "a's state and rates, and b's state")
	assert.Contains(t, rows[1].Value, view.Blank)
	assert.Contains(t, rows[1].Detail, "Σ")

	require.Eventually(t, func() bool {
		_, err := b.Poll(context.Background())
		require.NoError(t, err)
		rows = vpnRows(b.Section())
		return !strings.Contains(rows[1].Value, view.Blank)
	}, 5*time.Second, 50*time.Millisecond)
	assert.Equal(t, "tun0", rows[1].Tip)

	// Series 0 is the interface's; the tunnels follow it.
	sec := b.Section()
	require.Len(t, sec.Trails, 6, "an interface and two tunnels, down and up each")
	assert.Equal(t, 1, sec.Trails[2].Series)
	assert.NotEmpty(t, sec.Trails[2].Samples)
	assert.Equal(t, 2, sec.Trails[4].Series)
	assert.Empty(t, sec.Trails[4].Samples)

	state = core.VPNDown
	b.ForgetVPNReadTime()
	_, err = b.Poll(context.Background())
	require.NoError(t, err)
	sec = b.Section()
	assert.Len(t, vpnRows(sec), 2, "two down tunnels are two lines")
	assert.Empty(t, sec.Trails[2].Samples, "a tunnel's trace empties when it goes down")
}

// Choosing only VPNs still reads a tunnel's counters.
func TestACardWithOnlyVPNsReadsTheirCounters(t *testing.T) {
	tempCache(t)
	counters := func() (map[string]core.Counters, error) {
		return map[string]core.Counters{"tun0": {Rx: 1 << 30, Tx: 1 << 20}}, nil
	}
	b := panel.NewBandwidth(nil, counters)
	b.SetVPNSystems([]core.VPNBackend{{Name: "fake", Active: answering(core.VPN{Name: "a", State: core.VPNUp, Interface: "tun0"})}},
		nil, config.BandwidthSettings{VPNs: []config.VPNItem{{Name: "a"}}})

	has, err := b.Poll(context.Background())
	require.NoError(t, err)
	assert.True(t, has)
	rows := vpnRows(b.Section())
	require.Len(t, rows, 2)
	assert.Contains(t, rows[1].Detail, "1.0 GiB")
}

// readings gives a tunnel's rates beside it, and no address (spec 086 R8).
func TestReadingsCarryATunnelsRates(t *testing.T) {
	tempCache(t)
	counters := func() (map[string]core.Counters, error) {
		return map[string]core.Counters{"tun0": {Rx: 77, Tx: 88}}, nil
	}
	b := panel.NewBandwidth(nil, counters)
	b.SetVPNSystems([]core.VPNBackend{{Name: "fake", Active: answering(
		core.VPN{Name: "a", State: core.VPNUp, Interface: "tun0"}, core.VPN{Name: "b", State: core.VPNDown},
	)}}, nil, config.BandwidthSettings{VPNs: []config.VPNItem{{Name: "a"}, {Name: "b"}}})
	_, err := b.Poll(context.Background())
	require.NoError(t, err)

	raw, err := json.Marshal(b.Data())
	require.NoError(t, err)
	var data struct {
		VPNs []struct {
			Name  string `json:"name"`
			Rates *struct {
				RxTotal uint64 `json:"rx_total"`
			} `json:"rates"`
		} `json:"vpns"`
	}
	require.NoError(t, json.Unmarshal(raw, &data), string(raw))
	require.Len(t, data.VPNs, 2)
	assert.Equal(t, "a", data.VPNs[0].Name, "the VPN's own fields are still at the top level")
	require.NotNil(t, data.VPNs[0].Rates)
	assert.Equal(t, uint64(77), data.VPNs[0].Rates.RxTotal)
	assert.Nil(t, data.VPNs[1].Rates)
	assert.NotContains(t, string(raw), "address")
}

// The plot holds the newest checks that ran, a full window, so a check that could
// not run does not shorten it and break the drift (spec 086 R11).
func TestTheDetailPlotsTheNewestChecksThatRan(t *testing.T) {
	b, path := healthy(t, func(context.Context, core.VPNCheck) error { return nil }, checked())
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
	at := time.Now().Add(-5 * time.Hour)
	var lines []string
	const total = 70
	for i := range total {
		checks := ""
		if i == total-3 {
			checks = `,"checks":[{"type":"ping","did_not_run":true}]`
		}
		lines = append(lines, fmt.Sprintf(`{"at":%q,"vpn":"a","latency_ms":%d,"ok":true%s}`,
			at.Add(time.Duration(i)*time.Minute).Format(time.RFC3339), i, checks))
	}
	require.NoError(t, os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600))
	_, err := b.Poll(context.Background())
	require.NoError(t, err)

	d, err := b.PollDetail(context.Background(), view.DetailVPN)
	require.NoError(t, err)
	samples := d.Parts[0].Trails[0].Samples
	require.GreaterOrEqual(t, len(samples), 60)
	assert.Len(t, samples[len(samples)-60:], 60)
	assert.NotContains(t, samples, float64(total-3), "the check that did not run is not plotted")
	assert.Equal(t, time.Hour, d.Parts[0].TrailEvery, "the settings' check interval")
	assert.Len(t, samples, 60, "a full window, however many did not run")
}
