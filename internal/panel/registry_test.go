package panel_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/ushineko/sanshoku"

	"github.com/ushineko/hayami/internal/config"
	"github.com/ushineko/hayami/internal/panel"
	"github.com/ushineko/hayami/internal/view"
)

// Spec 046. A source reads its own section's settings from the configuration
// the environment carries: the bandwidth source watches the interfaces under
// the bandwidth section's entry, not a field on Env.
func TestASourceReadsItsOwnSectionsSettings(t *testing.T) {
	settings := config.Bandwidth.Set(config.Config{}, config.BandwidthSettings{Interfaces: []string{"eth0", "wlan9"}})
	settings.Interfaces = nil // only the section's own entry says it

	got := panel.Sources([]string{"bandwidth"}, panel.Env{Settings: settings, Scan: noScan, Counters: fakeCounters})
	require.Len(t, got, 1)
	_, err := got[0].Poll(t.Context())
	require.NoError(t, err)

	b, ok := got[0].(*panel.Bandwidth)
	require.True(t, ok)
	names := make([]string, 0, 2)
	for _, r := range b.Readings() {
		names = append(names, r.Name)
	}
	assert.Equal(t, []string{"eth0", "wlan9"}, names)
}

// noScan finds no devices, so building a device section here opens none.
func noScan(context.Context, ...sanshoku.Driver) ([]sanshoku.Candidate, error) { return nil, nil }

// Spec 038. Every section the view lists has a builder, and the source it
// builds answers to the section's key: a section listed without one would be
// one the settings could name and nothing would draw.
func TestEverySectionBuildsASourceUnderItsOwnKey(t *testing.T) {
	specs := panel.Specs()
	require.Len(t, specs, len(view.Sections()))
	for _, s := range specs {
		t.Run(s.Key, func(t *testing.T) {
			src := s.New(panel.Env{Scan: noScan, Counters: fakeCounters})
			require.NotNil(t, src)
			assert.Equal(t, s.Key, src.Key())
		})
	}
}

// Spec 038. Keys, the defaults and Sources all come from the one list, in its
// order, and a key no section has is skipped rather than refused.
func TestKeysDefaultsAndSourcesFollowTheOneList(t *testing.T) {
	var keys []string
	for _, s := range view.Sections() {
		keys = append(keys, s.Key)
	}
	assert.Equal(t, keys, panel.Keys())
	assert.Equal(t, keys, view.DefaultSections(), "every section is on by default today")

	got := panel.Sources([]string{"cooler", "no-such-section", "bandwidth"}, panel.Env{Scan: noScan, Counters: fakeCounters})
	require.Len(t, got, 2)
	assert.Equal(t, "cooler", got[0].Key())
	assert.Equal(t, "bandwidth", got[1].Key())
}
