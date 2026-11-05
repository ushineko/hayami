package cli_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
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

// Doctor says what is there and what is missing for what was asked (spec 085 R27).
func TestDoctorSaysWhereTheHelperStands(t *testing.T) {
	say := func(installed, running bool, on int, toggle bool, err error) string {
		return strings.Join(cli.KeepaliveLines(installed, running, on, toggle, err), "\n")
	}

	quiet := say(false, false, 0, false, nil)
	assert.Contains(t, quiet, "not installed")
	assert.NotContains(t, quiet, "warning", "not having it is no fault")

	assert.Contains(t, say(false, false, 2, false, nil), "nothing is keeping those VPNs working",
		"switched on with nothing to do it")
	assert.Contains(t, say(true, false, 1, false, nil), "installed, not running")
	assert.Contains(t, say(true, false, 1, false, nil), "nothing is keeping those VPNs working")

	running := say(true, true, 1, false, nil)
	assert.Contains(t, running, "running, switched on for 1 VPN")
	assert.NotContains(t, running, "warning")
}

func TestDoctorWarnsOfVPNToggleOnlyAlongsideTheHelper(t *testing.T) {
	alongside := strings.Join(cli.KeepaliveLines(true, true, 1, true, nil), "\n")
	assert.Contains(t, alongside, "vpn-toggle's service is running too")
	assert.Contains(t, alongside, "bounce each other's tunnel")

	byHand := strings.Join(cli.KeepaliveLines(false, true, 1, true, nil), "\n")
	assert.Contains(t, byHand, "vpn-toggle's service is running too", "however the helper was started")

	assert.NotContains(t, strings.Join(cli.KeepaliveLines(false, false, 0, true, nil), "\n"), "vpn-toggle",
		"vpn-toggle on its own is no fault of this program's")

	refused := errors.New("no session bus") //nolint:err113 // a test's failure
	assert.Contains(t, strings.Join(cli.KeepaliveLines(true, true, 1, false, refused), "\n"), "no session bus",
		"not knowing is said, and is not an all clear")
}
