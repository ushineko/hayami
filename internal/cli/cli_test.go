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
	noDevices(t)
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

// AC11. The window rule can be installed, reported and removed from a shell.
//
// This exists so a person who has made the panel frameless can undo it
// **without the panel**: a glance window has no titlebar and no controls of
// its own, so without this the way back is System Settings or editing
// kwinrulesrc by hand.
func TestTheWindowRuleCanBeDrivenFromTheCommandLine(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	// No compositor to tell, which is the ordinary case under test and must
	// not make any of these fail.
	t.Setenv("DBUS_SESSION_BUS_ADDRESS", "unix:path=/nonexistent/hayami-test")

	out, err := runGUI(t, "window", "status")
	require.NoError(t, err)
	assert.Contains(t, out, "No window rule")

	out, err = runGUI(t, "window", "install")
	require.NoError(t, err)
	assert.Contains(t, out, "no titlebar")

	body, err := os.ReadFile(filepath.Join(dir, "kwinrulesrc"))
	require.NoError(t, err)
	assert.Contains(t, string(body), "noborder=true")
	assert.NotContains(t, string(body), "opacityactive",
		"the rule should not carry an opacity: the panel fades its own cards")

	out, err = runGUI(t, "window", "status")
	require.NoError(t, err)
	assert.Contains(t, out, "Frameless and on top")

	out, err = runGUI(t, "window", "remove")
	require.NoError(t, err)

	// Not "the titlebar is back". KWin takes decoration away from a window
	// already on screen and will not give it back; that happens when the
	// window is next created, and saying otherwise sends the user looking for
	// a change that is not going to arrive.
	assert.Contains(t, out, "next starts")

	out, err = runGUI(t, "window", "status")
	require.NoError(t, err)
	assert.Contains(t, out, "No window rule")
}

// runGUI runs a subcommand of the window binary and returns what it printed.
func runGUI(t *testing.T, args ...string) (string, error) {
	t.Helper()

	cmd := cli.GUI("1.2.3", []string{"sections", "about"}, func(cli.Options) error {
		t.Fatal("a window subcommand started the panel")
		return nil
	})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(args)

	err := cmd.Execute()
	return out.String(), err
}

// startGUI runs the window's root command with arguments and returns the
// options it would have started the panel with.
func startGUI(t *testing.T, args ...string) (cli.Options, string, error) {
	t.Helper()
	var got cli.Options
	cmd := cli.GUI("1.2.3", []string{"sections", "window", "about"}, func(o cli.Options) error {
		got = o
		return nil
	})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(append([]string{"--settings", filepath.Join(t.TempDir(), "settings.yaml")}, args...))
	err := cmd.Execute()
	return got, out.String(), err
}

// --preferences opens the window on the page it names, and a bare
// --preferences on the first, as it did before it took a name. The screenshot
// harness opens every page this way.
func TestPreferencesOpensOnTheNamedPage(t *testing.T) {
	o, _, err := startGUI(t)
	require.NoError(t, err)
	assert.Empty(t, o.Preferences, "the window opened without being asked")

	o, _, err = startGUI(t, "--preferences")
	require.NoError(t, err)
	assert.Equal(t, "sections", o.Preferences)

	o, _, err = startGUI(t, "--preferences=About")
	require.NoError(t, err)
	assert.Equal(t, "about", o.Preferences)
}

// A page the window does not have is a usage error that names the ones it
// does, rather than a window opened somewhere else.
func TestPreferencesRefusesAPageThatIsNotThere(t *testing.T) {
	_, _, err := startGUI(t, "--preferences=bandwidth")
	var usage *cli.UsageError
	require.ErrorAs(t, err, &usage)
	assert.Contains(t, err.Error(), "sections, window, about")
}

// The help names the pages, because tools/screenshot.sh --all reads them from
// it to know which to photograph.
func TestTheHelpNamesThePreferencesPages(t *testing.T) {
	_, out, err := startGUI(t, "--help")
	require.NoError(t, err)
	assert.Contains(t, out, "on one of: sections, window, about")
}
