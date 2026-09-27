package cli_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/hayami/internal/cli"
)

// run executes the terminal command tree with arguments and returns what it
// wrote and what it returned, the way a person at a prompt would see it.
func run(t *testing.T, args ...string) (string, error) {
	t.Helper()
	var out bytes.Buffer
	cmd := cli.TUI("1.2.3")
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(args)
	// Execute first: Go evaluates return values left to right, so reading the
	// buffer in the return statement reads it before anything has run.
	err := cmd.Execute()
	return out.String(), err
}

// settings writes a settings file for a test and returns its path.
func settings(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "settings.yaml")
	require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
	return path
}

func TestTheArrangementsAreListed(t *testing.T) {
	out, err := run(t, "arrangements")

	require.NoError(t, err)
	for _, name := range []string{"stack", "grid", "row"} {
		assert.Contains(t, out, name)
	}
}

// A misuse of the command line is exit 2, distinct from a command that ran and
// failed, so a script can tell them apart. UsageError is what carries that.
func TestASectionThisBuildDoesNotHaveIsAMisuse(t *testing.T) {
	_, err := run(t, "--sections", "nosuch")

	require.Error(t, err)
	var usage *cli.UsageError
	assert.ErrorAs(t, err, &usage, "a bad flag value should be exit 2, not exit 1")
	assert.Contains(t, err.Error(), "bandwidth", "the message should say what there is")
}

func TestAnArrangementThisBuildDoesNotHaveIsAMisuse(t *testing.T) {
	_, err := run(t, "--arrangement", "colums")

	require.Error(t, err)
	var usage *cli.UsageError
	assert.ErrorAs(t, err, &usage)
}

// An argument the tree does not take is a misuse too, rather than being
// ignored: a pane started with a typo should say so.
func TestAStrayArgumentIsRefused(t *testing.T) {
	_, err := run(t, "usage")

	require.Error(t, err)
}

// The readings are on the terminal binary and not the window's, because a
// windowed binary has no console to print to on Windows -- which is the same
// reason there are two binaries at all.
func TestTheReadingsArePrintedAsJson(t *testing.T) {
	path := settings(t, "hayami:\n    sections:\n        - bandwidth\n    interfaces: []\n")

	out, err := run(t, "readings", "--settings", path)

	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(strings.TrimSpace(out), "{"), "got %q", out)
	assert.Contains(t, out, "bandwidth")
}

// A settings file that does not parse is said out loud and then worked around.
// A panel that refused to start could not be fixed from the pane it failed in.
func TestAnUnreadableSettingsFileIsReportedAndSurvived(t *testing.T) {
	path := settings(t, "hayami: [this is not a mapping\n")

	out, err := run(t, "readings", "--settings", path)

	require.NoError(t, err)
	assert.Contains(t, out, "could not be read")
}
