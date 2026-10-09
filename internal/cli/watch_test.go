package cli_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/hayami/internal/cli"
	"github.com/ushineko/hayami/internal/config"
	"github.com/ushineko/hayami/internal/view"
)

// rewrite replaces the settings file and moves its modification time on, so
// a file system with a coarse clock still shows the change.
func rewrite(t *testing.T, path, body string, at time.Time) {
	t.Helper()
	require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
	require.NoError(t, os.Chtimes(path, at, at))
}

const twoSections = "hayami:\n    sections:\n        - bandwidth\n        - cooler\n"

// Spec 052. The terminal panel sees a change the window's preferences made:
// the sections, their order and the arrangement, once each, and reading the
// file does not write it.
func TestTheTerminalSeesTheSettingsChange(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.yaml")
	rewrite(t, path, twoSections, time.Now().Add(-time.Minute))
	o := cli.Options{Config: config.Config{Sections: []string{"bandwidth", "cooler"}}, Arrangement: view.ArrangeStack}
	look := cli.WatchSettings(path, o, false, false)

	_, _, changed := look()
	assert.False(t, changed, "an untouched file is a change")

	body := "hayami:\n    sections:\n        - usage\n        - bandwidth\n    arrangement: grid\n"
	rewrite(t, path, body, time.Now())
	shown, arr, changed := look()
	require.True(t, changed, "a moved section was not seen")
	assert.Equal(t, []string{"usage", "bandwidth"}, shown)
	assert.Equal(t, view.ArrangeGrid, arr)

	_, _, changed = look()
	assert.False(t, changed, "the same change was reported twice")
	got, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, body, string(got), "looking at the settings wrote them")
}

// A pane started with --sections keeps them whatever the file says; the
// arrangement, not given, still follows.
func TestAPanesOwnSectionsStayAsGiven(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.yaml")
	rewrite(t, path, twoSections, time.Now().Add(-time.Minute))
	o := cli.Options{Config: config.Config{Sections: []string{"usage"}}, Arrangement: view.ArrangeRow}
	look := cli.WatchSettings(path, o, true, false)

	rewrite(t, path, "hayami:\n    sections:\n        - cooler\n    arrangement: row\n", time.Now())
	_, _, changed := look()
	assert.False(t, changed, "a pane's own sections followed the file")

	rewrite(t, path, "hayami:\n    sections:\n        - cooler\n    arrangement: grid\n", time.Now().Add(time.Second))
	shown, arr, changed := look()
	require.True(t, changed)
	assert.Equal(t, []string{"usage"}, shown)
	assert.Equal(t, view.ArrangeGrid, arr)
}
