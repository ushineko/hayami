package panel_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/hayami/internal/config"
	"github.com/ushineko/hayami/internal/core"
	"github.com/ushineko/hayami/internal/panel"
	"github.com/ushineko/hayami/internal/view"
)

func noCounters() (map[string]core.Counters, error) { return map[string]core.Counters{}, nil }

// tempCache points the readings' cache, which the health store lives in, at a
// directory of the test's own.
func tempCache(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", dir)
	t.Setenv("LOCALAPPDATA", dir)
	t.Setenv("XDG_STATE_HOME", dir)
	return dir
}

func network(t *testing.T, interfaces []string, active func(context.Context) ([]core.VPN, error), chosen ...config.VPNItem) *panel.Bandwidth {
	t.Helper()
	tempCache(t)
	b := panel.NewBandwidth(interfaces, noCounters)
	b.SetVPNSystems([]core.VPNBackend{{Name: "fake", Active: active}}, nil, config.BandwidthSettings{VPNs: chosen})
	return b
}

func answering(vpns ...core.VPN) func(context.Context) ([]core.VPN, error) {
	return func(context.Context) ([]core.VPN, error) { return vpns, nil }
}

func vpnRows(s view.Section) []view.Row {
	var out []view.Row
	for _, r := range s.Rows {
		if len(r.ID) > 4 && r.ID[:4] == "vpn:" {
			out = append(out, r)
		}
	}
	return out
}

// A machine with no VPN chosen sees the card, and the JSON, it always did
// (spec 085 R7).
func TestWithNoVPNChosenTheCardIsUnchanged(t *testing.T) {
	asked := false
	b := network(t, []string{"eth0"}, func(context.Context) ([]core.VPN, error) {
		asked = true
		return nil, nil
	})
	plain := panel.NewBandwidth([]string{"eth0"}, noCounters)
	for _, s := range []*panel.Bandwidth{b, plain} {
		_, err := s.Poll(context.Background())
		require.NoError(t, err)
	}

	assert.False(t, asked, "no VPN system is asked about nothing")
	assert.Equal(t, plain.Section(), b.Section())
	raw, err := json.Marshal(b.Data())
	require.NoError(t, err)
	assert.Equal(t, byte('['), raw[0], "the JSON is still a list of interfaces")
}

// Nothing but VPNs chosen is still a card with something to say, and not one
// saying no interfaces were picked.
func TestAVPNAloneDrawsTheCard(t *testing.T) {
	b := network(t, nil, answering(), config.VPNItem{Name: "a"})

	has, err := b.Poll(context.Background())

	require.NoError(t, err)
	assert.True(t, has)
	s := b.Section()
	assert.Len(t, vpnRows(s), 1)
	assert.Empty(t, s.Reasons)
}

func TestAnUpVPNDrawsWithItsLabelAndConnectedTime(t *testing.T) {
	since := time.Now().Add(-90 * time.Minute)
	b := network(t, nil, answering(core.VPN{Name: "long-name", State: core.VPNUp, Interface: "tun0", Since: since}),
		config.VPNItem{Name: "long-name", Label: "Short"})

	_, err := b.Poll(context.Background())
	require.NoError(t, err)

	rows := vpnRows(b.Section())
	require.Len(t, rows, 1)
	assert.Equal(t, "Short", rows[0].Label)
	assert.Contains(t, rows[0].Value, "up")
	assert.Contains(t, rows[0].Value, "1h 30m")
	assert.Contains(t, rows[0].Tip, "tun0")
}

// A probe that failed is reported as that, and the row does not read as down
// (spec 085 R4).
func TestAFailedProbeIsNotATunnelDown(t *testing.T) {
	b := network(t, nil, func(context.Context) ([]core.VPN, error) {
		return nil, errors.New("the bus refused")
	}, config.VPNItem{Name: "a"})

	_, err := b.Poll(context.Background())
	require.NoError(t, err)

	s := b.Section()
	rows := vpnRows(s)
	require.Len(t, rows, 1)
	assert.NotContains(t, rows[0].Value, "down")
	require.Len(t, s.Reasons, 1)
	assert.Equal(t, view.Warn, s.Reasons[0].Status)
	assert.Contains(t, s.Reasons[0].Detail, "the bus refused")
}

func TestNoSystemRunningIsSaid(t *testing.T) {
	b := network(t, nil, func(context.Context) ([]core.VPN, error) {
		return nil, core.ErrVPNBackendAbsent
	}, config.VPNItem{Name: "a"})

	_, err := b.Poll(context.Background())
	require.NoError(t, err)

	s := b.Section()
	require.Len(t, s.Reasons, 1)
	assert.Equal(t, view.Info, s.Reasons[0].Status)
}

// A platform with no systems draws no row, even from settings copied from a
// machine that has them (spec 085 R3).
func TestAPlatformWithNoSystemsDrawsNoRow(t *testing.T) {
	b := panel.NewBandwidth(nil, noCounters)
	b.SetVPNSystems(nil, nil, config.BandwidthSettings{VPNs: []config.VPNItem{{Name: "a"}}})

	has, err := b.Poll(context.Background())

	require.NoError(t, err)
	assert.False(t, has)
	assert.Empty(t, vpnRows(b.Section()))
}

// The JSON says what the card says and carries no address (spec 085 R9).
func TestTheJSONNamesTheVPNs(t *testing.T) {
	b := network(t, nil, answering(core.VPN{Name: "a", State: core.VPNUp, Interface: "tun0"}), config.VPNItem{Name: "a"})
	_, err := b.Poll(context.Background())
	require.NoError(t, err)

	raw, err := json.Marshal(b.Data())
	require.NoError(t, err)

	var got struct {
		VPNs []map[string]any `json:"vpns"`
	}
	require.NoError(t, json.Unmarshal(raw, &got))
	require.Len(t, got.VPNs, 1)
	assert.Equal(t, "up", got.VPNs[0]["state"])
	for key := range got.VPNs[0] {
		assert.Contains(t, []string{"name", "backend", "state", "interface", "since", "stale"}, key)
	}
}

// A chosen list changes while the panel runs, from the preferences window.
func TestChangingTheChosenVPNsTakesEffect(t *testing.T) {
	b := network(t, nil, answering(), config.VPNItem{Name: "a"})
	b.SetVPNs(config.BandwidthSettings{VPNs: []config.VPNItem{{Name: "b"}, {Name: "c"}}})

	_, err := b.Poll(context.Background())
	require.NoError(t, err)

	assert.Len(t, vpnRows(b.Section()), 2)
}

func TestOnlyAChosenVPNCanBePressed(t *testing.T) {
	tempCache(t)
	var calls []string
	b := panel.NewBandwidth(nil, noCounters)
	b.SetVPNSystems([]core.VPNBackend{{
		Name:       "fake",
		Active:     answering(),
		Configured: func(context.Context) ([]string, error) { return []string{"chosen", "not chosen"}, nil },
		Connect: func(_ context.Context, name string, _ core.SignIn) error {
			calls = append(calls, name)
			return nil
		},
	}}, nil, config.BandwidthSettings{VPNs: []config.VPNItem{{Name: "chosen"}}})

	require.NoError(t, b.ControlVPN(context.Background(), view.VPNConnect, "chosen", nil))
	require.ErrorIs(t, b.ControlVPN(context.Background(), view.VPNConnect, "not chosen", nil), core.ErrVPNNotConfigured)
	require.Error(t, b.ControlVPN(context.Background(), "explode", "chosen", nil))

	assert.Equal(t, []string{"chosen"}, calls)
}
