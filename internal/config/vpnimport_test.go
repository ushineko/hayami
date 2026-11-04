package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/hayami/internal/config"
)

// The shape of vpn-toggle's own file, with invented names.
const vpnToggleFixture = `{
  "version": "2.0.0",
  "monitor": {"enabled": true, "check_interval_seconds": 30, "grace_period_seconds": 20},
  "vpns": [
    {"name": "office", "display_name": "office", "enabled": false,
     "asserts": [{"type": "dns_lookup", "hostname": "intranet.example", "expected_prefix": "10."}]},
    {"name": "provider-gateway-udp", "display_name": "Exit", "enabled": true,
     "asserts": [{"type": "geolocation", "field": "city", "expected_value": "Invented"}]},
    {"name": "lab", "enabled": true,
     "asserts": [{"type": "ping", "host": "10.0.0.1", "timeout_seconds": 3}]},
    {"display_name": "no name", "asserts": []}
  ],
  "startup": {"restore_vpns": ["office"]}
}`

func TestVPNTogglesSettingsBecomeHayamis(t *testing.T) {
	got, err := config.ParseVPNToggle([]byte(vpnToggleFixture))

	require.NoError(t, err)
	assert.Equal(t, 30, got.CheckSeconds)
	assert.Equal(t, 20, got.GraceSeconds)
	assert.Equal(t, []config.VPNItem{
		{Name: "office", Checks: []config.VPNCheckItem{{Type: "dns", Target: "intranet.example", Prefix: "10."}}},
		{Name: "provider-gateway-udp", Label: "Exit"},
		{Name: "lab", Checks: []config.VPNCheckItem{{Type: "ping", Target: "10.0.0.1", TimeoutSeconds: 3}}},
	}, got.VPNs)
}

// A geolocation check told a third party the address. It is not carried over, and
// the person is told it was not (spec 085).
func TestAGeolocationCheckIsLeftOutAndNamed(t *testing.T) {
	got, err := config.ParseVPNToggle([]byte(vpnToggleFixture))

	require.NoError(t, err)
	require.Len(t, got.LeftOut, 1)
	assert.Contains(t, got.LeftOut[0], "provider-gateway-udp")
	assert.Contains(t, got.LeftOut[0], "geolocation")
}

func TestAFileThatCannotBeImportedSaysSo(t *testing.T) {
	_, err := config.ParseVPNToggle([]byte(`not json`))
	require.Error(t, err)

	_, err = config.ParseVPNToggle([]byte(`{"vpns": []}`))
	require.Error(t, err)

	_, err = config.ReadVPNToggle(filepath.Join(t.TempDir(), "absent.json"))
	require.Error(t, err)
}

// The file is another program's, read to a limit. One far past any real size is
// cut, which then does not parse, and is not read whole.
func TestAnOversizedFileIsNotReadWhole(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"vpns":[`+strings.Repeat(" ", 2<<20)+`]}`), 0o600))

	_, err := config.ReadVPNToggle(path)

	require.Error(t, err)
}

func FuzzParseVPNToggle(f *testing.F) {
	f.Add([]byte(vpnToggleFixture))
	f.Add([]byte(`{"vpns":[{"name":"a","asserts":[{"type":"ping"}]}]}`))
	f.Add([]byte(`{`))
	f.Fuzz(func(_ *testing.T, raw []byte) {
		_, _ = config.ParseVPNToggle(raw)
	})
}
