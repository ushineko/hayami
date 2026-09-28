package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/hayami/internal/config"
	"github.com/ushineko/hayami/internal/desktop"
	"github.com/ushineko/hayami/internal/view"
)

func open(t *testing.T, body string) (*config.Store, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "settings.yaml")
	if body != "" {
		require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
	}
	s, err := config.Open(path)
	require.NoError(t, err)
	return s, path
}

func TestAMachineWithNoSettingsFileGetsTheDefaults(t *testing.T) {
	s, _ := open(t, "")

	c := s.Config()

	assert.Equal(t, []string{"bandwidth", "usage", "cooler", "peripherals"}, c.Sections)
	assert.Equal(t, "stack", c.Arrangement)
	assert.Empty(t, c.Interfaces,
		"guessing an interface would be this program deciding what is interesting about someone's network")
}

// The file is YAML because a person opens it. If this ever writes JSON, the
// codec registration has been lost and nobody will notice from the program.
func TestTheSettingsFileIsYamlAPersonCanRead(t *testing.T) {
	s, path := open(t, "")

	require.NoError(t, s.SetConfig(config.Config{
		Sections:    []string{"bandwidth"},
		Arrangement: "row",
		Interfaces:  []string{"eth0"},
	}))
	require.NoError(t, s.Flush())

	body, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(body), "arrangement: row")
	assert.NotContains(t, string(body), `"arrangement"`, "this is JSON, not YAML")
}

// Two binaries at different versions read this file. A save that dropped the
// key the other one had set would lose a user's setting silently, which is
// the failure the library's store exists to prevent.
func TestASectionThisBuildDoesNotKnowSurvivesASave(t *testing.T) {
	s, path := open(t, "hayami:\n    arrangement: grid\nsomethingelse:\n    kept: yes\n")

	require.NoError(t, s.SetConfig(config.Config{Arrangement: "row"}))
	require.NoError(t, s.Flush())

	body, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(body), "somethingelse")
	assert.Contains(t, string(body), "kept")
}

// A stray tab in a hand-edited file must not stop the panel. It draws on
// defaults and says so, because a panel that refused to start could not be
// fixed from the pane it failed in.
func TestAnUnreadableFileYieldsDefaultsAndAnError(t *testing.T) {
	s, _ := open(t, "hayami: [this is not a mapping\n")

	assert.Error(t, s.Unreadable())
	assert.Equal(t, config.Default().Arrangement, s.Config().Arrangement)
}

func TestTheArrangementIsReadFromTheFile(t *testing.T) {
	s, _ := open(t, "hayami:\n    arrangement: grid\n")

	a, err := s.Config().ParseArrangement()

	require.NoError(t, err)
	assert.Equal(t, view.ArrangeGrid, a)
}

func TestAHiddenSectionIsOneTheSettingsDoNotName(t *testing.T) {
	c := config.Config{Sections: []string{"bandwidth"}}

	assert.True(t, c.Shows("bandwidth"))
	assert.False(t, c.Shows("cooler"))
}

// AC8. The opacity round-trips through the settings file.
func TestTheOpacityRoundTrips(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.yaml")

	s, err := config.Open(path)
	require.NoError(t, err)

	c := s.Config()
	c.Opacity = 70
	require.NoError(t, s.SetConfig(c))
	require.NoError(t, s.Flush())

	again, err := config.Open(path)
	require.NoError(t, err)
	assert.Equal(t, 70, again.Config().Opacity)
	assert.Equal(t, 70, again.Config().OpacityOrDefault())
}

// AC8. A settings file written before this field existed gets the default
// rather than an invisible panel.
//
// Zero is not a legal opacity, so it is the marker for "not set" — which is
// what every file written before spec 010 carries.
func TestASettingsFileWithNoOpacityGetsTheDefault(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.yaml")
	require.NoError(t, os.WriteFile(path,
		[]byte("hayami:\n    sections:\n        - usage\n"), 0o600))

	s, err := config.Open(path)
	require.NoError(t, err)

	// The store merges the file over Default(), so an old file comes back
	// carrying the default rather than a zero. That is the better behaviour
	// and is what this asserts; OpacityOrDefault is the guard for the case
	// the merge cannot catch, which is a file that names 0 outright.
	assert.Equal(t, desktop.DefaultOpacity, s.Config().Opacity)
	assert.Equal(t, desktop.DefaultOpacity, s.Config().OpacityOrDefault())
}

// An opacity outside the range is not honoured: a panel nobody can see is not
// a setting anyone chose.
func TestAnImpossibleOpacityFallsBackToTheDefault(t *testing.T) {
	for _, v := range []int{-10, 0, 101, 1000} {
		c := config.Config{Opacity: v}
		assert.Equal(t, desktop.DefaultOpacity, c.OpacityOrDefault(), "at %d", v)
	}
}

// The panel keeps a text size of its own, separate from the appearance's.
//
// A Fyne theme is application-wide, so without this the panel and the
// preferences window would share a size — and they are read at different
// distances.
func TestThePanelKeepsItsOwnFontSize(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.yaml")

	s, err := config.Open(path)
	require.NoError(t, err)

	c := s.Config()
	c.FontSize = 9
	require.NoError(t, s.SetConfig(c))
	require.NoError(t, s.Flush())

	again, err := config.Open(path)
	require.NoError(t, err)
	assert.InDelta(t, 9, again.Config().FontSize, 0.01)
}

// Unset means "whatever the appearance says", which is what every settings
// file written before the field existed carries.
func TestAnUnsetFontSizeFollowsTheAppearance(t *testing.T) {
	assert.InDelta(t, 14, config.Config{}.FontSizeOr(14), 0.01)
	assert.InDelta(t, 14, config.Config{FontSize: 0}.FontSizeOr(14), 0.01)
	assert.InDelta(t, 14, config.Config{FontSize: -3}.FontSizeOr(14), 0.01)
	assert.InDelta(t, 9, config.Config{FontSize: 9}.FontSizeOr(14), 0.01)
}
