package cli_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/hayami/internal/cli"
)

func TestTheHelperFollowsItsSettingsAndSurvivesABadSave(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.yaml")
	version := time.Now().Add(-time.Hour)
	write := func(body string) {
		t.Helper()
		require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
		version = version.Add(time.Minute)
		require.NoError(t, os.Chtimes(path, version, version))
	}
	named := func(name string) string {
		return "hayami:\n  sectionSettings:\n    bandwidth:\n      vpns:\n        - name: " + name + "\n"
	}
	var said []string
	settings := cli.FollowBandwidth(path, func(text string) { said = append(said, text) })

	assert.Empty(t, settings().VPNs, "no file yet is nothing chosen")

	write(named("first"))
	require.Len(t, settings().VPNs, 1)
	assert.Equal(t, "first", settings().VPNs[0].Name)

	write(named("second"))
	assert.Equal(t, "second", settings().VPNs[0].Name, "without a restart")

	write("hayami: [not what a save leaves\n")
	for range 3 {
		require.Len(t, settings().VPNs, 1, "a bad save does not forget the VPNs")
		assert.Equal(t, "second", settings().VPNs[0].Name)
	}
	assert.Len(t, said, 1, "said once, and not at every look")

	require.NoError(t, os.Remove(path))
	assert.Equal(t, "second", settings().VPNs[0].Name, "nor does a file that has gone")

	write(named("third"))
	assert.Equal(t, "third", settings().VPNs[0].Name, "and the next good save is read")
}
