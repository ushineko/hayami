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
func TestTheRuleIsFramelessAndOnTopAndNothingElse(t *testing.T) {
	path := rules(t, "")

	// The reconfigure afterwards needs KWin and there is none under test; the
	// rule is still written, which is what this asserts.
	_ = desktop.Install(appID)

	body := read(t, path)
	assert.Contains(t, body, appID)
	assert.Contains(t, body, "noborder=true")
	assert.Contains(t, body, "above=true")
	assert.NotContains(t, body, "opacityactive",
		"the rule should not carry an opacity: the panel fades its own cards")
	assert.Contains(t, body, desktop.Description,
		"a rule the user cannot identify is one they cannot remove")
}

// AC1. What the rule carries is forced.
//
// A glance window the user can accidentally push behind something is not one,
// and a titlebar that comes back when KWin feels like it is not frameless.
func TestWhatTheRuleCarriesIsForced(t *testing.T) {
	path := rules(t, "")
	_ = desktop.Install(appID)

	body := read(t, path)
	assert.Contains(t, body, "noborderrule=2", "frameless should be forced")
	assert.Contains(t, body, "aboverule=2", "on top should be forced")
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
	_ = desktop.Install(appID)

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

	_ = desktop.Install(appID)
	_ = desktop.Install(appID)

	body := read(t, path)
	assert.Equal(t, 1, countOf(body, appID), "installing twice left two rules")

	current, err := desktop.Current(appID)
	require.NoError(t, err)
	assert.True(t, current.Installed)
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

	_ = desktop.Install(appID)
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

// AC7. And the rule is still written when the compositor cannot be told, so it
// applies the next time one starts.
func TestARuleIsWrittenEvenWhenKWinCannotBeTold(t *testing.T) {
	path := rules(t, "")
	t.Setenv("DBUS_SESSION_BUS_ADDRESS", "unix:path=/nonexistent/hayami-test")

	err := desktop.Install(appID)
	assert.ErrorIs(t, err, desktop.ErrNoKWin)
	assert.Contains(t, read(t, path), appID,
		"the rule should be on disk even when kwin is not there to reload it")
}

// AC. A rule written before the title match existed is cleaned up.
//
// The rule used to match on the app ID alone, which stripped the titlebar off
// every window in the program. Adding the title fixed that and created a worse
// problem: a remove keyed on the new match cannot see a rule written under the
// old one, so the old rule stayed behind, kept stripping both windows, and
// made the preferences checkbox look like it did nothing.
func TestARuleFromBeforeTheTitleMatchIsRemoved(t *testing.T) {
	legacy := `[General]
count=1
rules=old

[old]
Description=hayami — frameless, on top, translucent
wmclass=io.example.test
wmclassmatch=1
above=true
noborder=true
`
	path := rules(t, legacy)

	require.NoError(t, desktop.Remove(appID))

	body := read(t, path)
	assert.NotContains(t, body, appID,
		"a rule written before the title match survived the remove")
}

// And installing does not leave one beside the new rule.
func TestInstallingDoesNotLeaveTheOlderRuleBeside(t *testing.T) {
	legacy := `[General]
count=1
rules=old

[old]
Description=hayami — frameless, on top, translucent
wmclass=io.example.test
wmclassmatch=1
noborder=true
`
	path := rules(t, legacy)

	_ = desktop.Install(appID)

	body := read(t, path)
	assert.Equal(t, 1, countOf(body, appID),
		"two rules for one app: the older one still matches every window")
	assert.Contains(t, body, "title=hayami",
		"the surviving rule is the one that matches only the panel")
}

// A rule from either version reads as installed. A panel held frameless by an
// older version's rule is a panel that is frameless, and a checkbox saying
// otherwise would be lying about what is on screen.
func TestAnOlderRuleStillReadsAsInstalled(t *testing.T) {
	rules(t, `[General]
count=1
rules=old

[old]
Description=hayami — frameless, on top, translucent
wmclass=io.example.test
wmclassmatch=1
noborder=true
`)

	current, err := desktop.Current(appID)
	require.NoError(t, err)
	assert.True(t, current.Installed)
}

// The rule matches the panel's title as well as the app ID, so it leaves every
// other window in the program alone.
func TestTheRuleMatchesThePanelAndNotTheWholeProgram(t *testing.T) {
	path := rules(t, "")
	_ = desktop.Install(appID)

	body := read(t, path)
	assert.Contains(t, body, "title="+desktop.PanelTitle)
	assert.Contains(t, body, "titlematch=1", "the title should match exactly")
}
