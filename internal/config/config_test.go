package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/hayami/internal/config"
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
