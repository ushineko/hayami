package config_test

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/hayami/internal/config"
	"github.com/ushineko/hayami/internal/view"
)

// Spec 046. A file from before section settings carries the interfaces and
// the LibreHardwareMonitor address on the root, and each section still reads
// its own from there.
func TestAnOldFileGivesEachSectionItsSettings(t *testing.T) {
	s, _ := open(t, "hayami:\n    interfaces: [eth0, wlan0]\n    lhm: http://127.0.0.1:9000/data.json\n")

	c := s.Config()

	assert.Equal(t, []string{"eth0", "wlan0"}, config.Bandwidth.Get(c).Interfaces)
	assert.Equal(t, "http://127.0.0.1:9000/data.json", config.Cooler.Get(c).LHM)
}

// A file with each section's own entry is read from there, ahead of whatever
// the root fields say.
func TestASectionsOwnEntryIsReadAheadOfTheRoot(t *testing.T) {
	s, _ := open(t, "hayami:\n"+
		"    interfaces: [old0]\n"+
		"    sectionSettings:\n"+
		"        bandwidth:\n"+
		"            interfaces: [eth0]\n"+
		"        cooler:\n"+
		"            lhm: http://127.0.0.1:9001/data.json\n")

	c := s.Config()

	assert.Equal(t, []string{"eth0"}, config.Bandwidth.Get(c).Interfaces)
	assert.Equal(t, "http://127.0.0.1:9001/data.json", config.Cooler.Get(c).LHM)
}

// A save writes the section's own entry and the root fields both, so a build
// from before spec 046 reading the same file still finds the setting.
func TestASaveWritesTheSectionsEntryAndTheRootBoth(t *testing.T) {
	s, path := open(t, "")

	c := config.Bandwidth.Set(s.Config(), config.BandwidthSettings{Interfaces: []string{"eth0"}})
	c = config.Cooler.Set(c, config.CoolerSettings{LHM: "http://127.0.0.1:9002/data.json"})
	require.NoError(t, s.SetConfig(c))
	require.NoError(t, s.Flush())

	reopened, err := config.Open(path)
	require.NoError(t, err)
	got := reopened.Config()
	assert.Equal(t, []string{"eth0"}, got.Interfaces, "the root field an older build reads")
	assert.Equal(t, "http://127.0.0.1:9002/data.json", got.LHM, "the root field an older build reads")
	assert.Equal(t, []string{"eth0"}, config.Bandwidth.Get(got).Interfaces)
	assert.Equal(t, "http://127.0.0.1:9002/data.json", config.Cooler.Get(got).LHM)

	body, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(body), "sectionSettings:", "the section's own entry is in the file")
}

// An entry that does not decode is treated as absent: the section falls back
// to the root fields rather than the whole file failing to load.
func TestAnEntryThatDoesNotDecodeFallsBackToTheRoot(t *testing.T) {
	c := config.Config{
		Interfaces:      []string{"eth0"},
		SectionSettings: map[string]json.RawMessage{"bandwidth": json.RawMessage(`{"interfaces": 7}`)},
	}

	assert.Equal(t, []string{"eth0"}, config.Bandwidth.Get(c).Interfaces)
}

// Nothing anywhere is the zero settings: no interfaces, the default address.
func TestNoSettingsAtAllIsTheZeroSettings(t *testing.T) {
	assert.Empty(t, config.Bandwidth.Get(config.Config{}).Interfaces)
	assert.Empty(t, config.Cooler.Get(config.Config{}).LHM)
}

// Set leaves the configuration it was given alone: a Config is passed by
// value, and its map must not be shared with the copy Set returns.
func TestSetDoesNotChangeTheConfigItWasGiven(t *testing.T) {
	before := config.Bandwidth.Set(config.Config{}, config.BandwidthSettings{Interfaces: []string{"eth0"}})

	_ = config.Bandwidth.Set(before, config.BandwidthSettings{Interfaces: []string{"wlan0"}})

	assert.Equal(t, []string{"eth0"}, config.Bandwidth.Get(before).Interfaces)
}

// Each setting belongs to a section this build has.
func TestEverySettingBelongsToASection(t *testing.T) {
	for _, key := range config.SettingKeys() {
		_, ok := view.SectionByKey(key)
		assert.True(t, ok, "a setting for %q, which is no section", key)
	}
}
