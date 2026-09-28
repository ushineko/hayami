package desktop_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/hayami/internal/desktop"
)

const appID = "io.example.test"

// rules points the package at a kwinrulesrc the test owns.
//
// Never the real one. It holds every window rule the user has, including two
// belonging to the program this replaces, and a test that wrote to it would be
// a test that could lose them.
func rules(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	path := filepath.Join(dir, "kwinrulesrc")
	if body != "" {
		require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
	}
	return path
}

func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	require.NoError(t, err)
	return string(b)
}

// AC1. The rule carries what a glance window needs.
func TestTheRuleIsFramelessOnTopAndTranslucent(t *testing.T) {
	path := rules(t, "")

	// The reconfigure afterwards needs KWin and there is none under test; the
	// rule is still written, which is what this asserts.
	_ = desktop.Install(appID, 88)

	body := read(t, path)
	assert.Contains(t, body, appID)
	assert.Contains(t, body, "noborder=true")
	assert.Contains(t, body, "above=true")
	assert.Contains(t, body, "opacityactive=88")
	assert.Contains(t, body, "opacityinactive=88")
	assert.Contains(t, body, desktop.Description,
		"a rule the user cannot identify is one they cannot remove")
}

// AC1. Everything in the rule is forced.
//
// Frameless and on top should be: a glance window the user can accidentally
// push behind something is not one.
//
// The opacity is forced too, and that is **not** what this program asked for
// or what the reference does. fynedesygn's Rule.Opacity says it is "applied
// initially rather than forced, so the user can still override it from the
// window menu", and the rule it writes says `opacityactiverule=2`, which is
// forced. The reference's own rule uses 4. The practical difference is that
// KWin's window menu cannot override hayami's opacity, so the panel's menu is
// the only way to try a value.
//
// This asserts what the library does rather than what it says, so that the day
// it is fixed this test fails and says why. See "Gaps found" in the spec.
func TestEverythingInTheRuleIsForced(t *testing.T) {
	path := rules(t, "")
	_ = desktop.Install(appID, 90)

	body := read(t, path)
	assert.Contains(t, body, "noborderrule=2", "frameless should be forced")
	assert.Contains(t, body, "aboverule=2", "on top should be forced")
	assert.Contains(t, body, "opacityactiverule=2",
		"if this fails, fynedesygn now writes the applied-initially rule its doc promises")
}

// AC2. Installing leaves every rule the user already had, and their numbering.
//
// This is the failure nobody notices until they wonder where their other rules
// went, and it is the reason this test exists rather than being assumed from
// the library.
func TestInstallingLeavesTheUsersOtherRulesAlone(t *testing.T) {
	existing := `[General]
count=2
rules=aaa,bbb

[aaa]
Description=Something the user set up
above=true
wmclass=somebody-elses-program

[bbb]
Description=Another one
noborder=true
wmclass=a-second-program
`
	path := rules(t, existing)
	_ = desktop.Install(appID, 95)

	body := read(t, path)
	assert.Contains(t, body, "Something the user set up")
	assert.Contains(t, body, "somebody-elses-program")
	assert.Contains(t, body, "Another one")
	assert.Contains(t, body, "a-second-program")
	assert.Contains(t, body, appID)
}

// AC3. Installing twice updates the rule rather than adding a second.
func TestInstallingTwiceUpdatesTheRule(t *testing.T) {
	path := rules(t, "")

	_ = desktop.Install(appID, 95)
	_ = desktop.Install(appID, 70)

	body := read(t, path)
	assert.Equal(t, 1, countOf(body, "opacityactive=70"))
	assert.Zero(t, countOf(body, "opacityactive=95"), "the first opacity survived")

	current, err := desktop.Current(appID)
	require.NoError(t, err)
	assert.True(t, current.Installed)
	assert.Equal(t, 70, current.Opacity)
}

func countOf(body, sub string) int {
	n := 0
	for i := 0; i+len(sub) <= len(body); i++ {
		if body[i:i+len(sub)] == sub {
			n++
		}
	}
	return n
}

// AC4. Removing takes this rule out and leaves the others.
func TestRemovingLeavesTheUsersOtherRulesAlone(t *testing.T) {
	existing := `[General]
count=1
rules=aaa

[aaa]
Description=Something the user set up
above=true
wmclass=somebody-elses-program
`
	path := rules(t, existing)

	_ = desktop.Install(appID, 95)
	_ = desktop.Remove(appID)

	body := read(t, path)
	assert.Contains(t, body, "somebody-elses-program")
	assert.NotContains(t, body, appID)

	current, err := desktop.Current(appID)
	require.NoError(t, err)
	assert.False(t, current.Installed)
}

// AC4. Removing a rule that is not there is not an error: the user's intent is
// that there be no rule, and there is none.
func TestRemovingARuleThatIsNotThereIsNotAnError(t *testing.T) {
	rules(t, "")

	err := desktop.Remove(appID)
	assert.NotErrorIs(t, err, os.ErrNotExist)

	current, err := desktop.Current(appID)
	require.NoError(t, err)
	assert.False(t, current.Installed)
}

// AC5. A lookup on a machine with no rules file at all answers "no rule"
// rather than failing.
func TestNoRulesFileAtAllIsNoRule(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(t.TempDir(), "absent"))

	current, err := desktop.Current(appID)
	require.NoError(t, err)
	assert.False(t, current.Installed)
}

// An opacity that is not a percentage is refused before anything is written.
func TestAnOpacityThatIsNotAPercentageIsRefused(t *testing.T) {
	path := rules(t, "")

	require.Error(t, desktop.Install(appID, 101))
	require.Error(t, desktop.Install(appID, -1))
	require.Error(t, desktop.SetOpacity(appID, 101))

	_, err := os.Stat(path)
	assert.True(t, os.IsNotExist(err), "a refused opacity wrote a rules file anyway")
}

// AC7. With no session bus, the live call says so plainly rather than
// panicking or hanging.
func TestWithNoSessionBusTheLiveCallSaysSo(t *testing.T) {
	t.Setenv("DBUS_SESSION_BUS_ADDRESS", "unix:path=/nonexistent/hayami-test")

	err := desktop.SetOpacity(appID, 80)
	require.Error(t, err)
	assert.ErrorIs(t, err, desktop.ErrNoKWin)
}

// AC7. And the rule is still written when the compositor cannot be told, so it
// applies the next time one starts.
func TestARuleIsWrittenEvenWhenKWinCannotBeTold(t *testing.T) {
	path := rules(t, "")
	t.Setenv("DBUS_SESSION_BUS_ADDRESS", "unix:path=/nonexistent/hayami-test")

	err := desktop.Install(appID, 95)
	assert.ErrorIs(t, err, desktop.ErrNoKWin)
	assert.Contains(t, read(t, path), appID,
		"the rule should be on disk even when kwin is not there to reload it")
}
