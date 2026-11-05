package installer

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/fynstall/extension"
)

// A machine in each state the keepalive check can find.
var (
	keepNotHere = Keepalive{}
	keepMissing = Keepalive{Offered: true}
	keepThere   = Keepalive{Offered: true, Installed: true}
)

// answerKeep makes the check find k for the length of a test, and counts the installs
// and removals the test caused, each answering err.
func answerKeep(t *testing.T, k Keepalive, err error) (installs, removals *int) {
	t.Helper()
	wasProbe, wasSet, wasRemove := probeKeep, setUpKeep, removeKeep
	t.Cleanup(func() { probeKeep, setUpKeep, removeKeep = wasProbe, wasSet, wasRemove })
	installs, removals = new(int), new(int)
	probeKeep = func() Keepalive { return k }
	setUpKeep = func(context.Context, *extension.Context) error { *installs++; return err }
	removeKeep = func(context.Context, *extension.Context) error { *removals++; return err }
	return installs, removals
}

func keepContext(value string, upgrade bool, logged *[]string) *extension.Context {
	return extension.NewContext(extension.Host{
		App:      extension.App{ID: "io.ushineko.hayami", Name: "hayami", Version: "0.0.0"},
		Scope:    func() string { return "user" },
		Root:     func() string { return "" },
		Params:   map[string]string{ParamKeepalive: value},
		Declared: []string{ParamKeepalive},
		Upgrade:  upgrade,
		Log: func(_ extension.Level, text string) {
			if logged != nil {
				*logged = append(*logged, text)
			}
		},
	})
}

func TestTheKeepaliveCheckOffersOnlyWhatIsMissing(t *testing.T) {
	assert.False(t, keepNotHere.applies())
	assert.Empty(t, keepNotHere.Offer())

	assert.Equal(t, "Install the VPN keepalive helper", keepMissing.Offer())
	assert.Contains(t, keepMissing.Found(), "is not installed")

	assert.Empty(t, keepThere.Offer(), "what is installed is not offered again")
	assert.Contains(t, keepThere.Found(), "restarts it")
}

// Installing it is only ever asked for; an install with no answer leaves it off.
func TestAnInstallThatDidNotAskInstallsNoHelper(t *testing.T) {
	installs, _ := answerKeep(t, keepMissing, nil)
	placeKeepalive(context.Background(), keepContext(No, false, nil))
	assert.Zero(t, *installs)
}

func TestAnInstallThatAskedInstallsTheHelper(t *testing.T) {
	installs, _ := answerKeep(t, keepMissing, nil)
	placeKeepalive(context.Background(), keepContext(Yes, false, nil))
	assert.Equal(t, 1, *installs)
}

// A running helper is the old program until something restarts it, and an upgrade is
// the only thing that knows (the parameter itself is not carried).
func TestAnUpgradeRestartsAnInstalledHelperWithoutBeingAsked(t *testing.T) {
	installs, _ := answerKeep(t, keepThere, nil)
	var logged []string
	placeKeepalive(context.Background(), keepContext(No, true, &logged))
	assert.Equal(t, 1, *installs)
	assert.Contains(t, strings.Join(logged, "\n"), "restarted on this version")
}

func TestAPlatformWithNoServiceInstallsNoHelper(t *testing.T) {
	installs, _ := answerKeep(t, keepNotHere, nil)
	placeKeepalive(context.Background(), keepContext(Yes, false, nil))
	assert.Zero(t, *installs)
}

// hayami is installed by then and works without it.
func TestAHelperThatWouldNotInstallIsAWarning(t *testing.T) {
	answerKeep(t, keepMissing, errors.New("no manager here")) //nolint:err113 // a test's failure
	var logged []string
	placeKeepalive(context.Background(), keepContext(Yes, false, &logged))
	assert.Contains(t, strings.Join(logged, "\n"), "no manager here")
}

func TestAnUninstallRemovesTheHelperAndAnUpgradeDoesNot(t *testing.T) {
	_, removals := answerKeep(t, keepThere, nil)
	dropKeepalive(context.Background(), keepContext(No, true, nil))
	assert.Zero(t, *removals, "the first half of an upgrade")

	dropKeepalive(context.Background(), keepContext(No, false, nil))
	assert.Equal(t, 1, *removals)

	_, removals = answerKeep(t, keepMissing, nil)
	dropKeepalive(context.Background(), keepContext(No, false, nil))
	assert.Zero(t, *removals, "nothing there to remove")
}

func TestAHelperThatWouldNotGoSaysHow(t *testing.T) {
	answerKeep(t, keepThere, errors.New("no manager here")) //nolint:err113 // a test's failure
	var logged []string
	dropKeepalive(context.Background(), keepContext(No, false, &logged))
	joined := strings.Join(logged, "\n")
	assert.Contains(t, joined, "no manager here")
	assert.Contains(t, joined, "hayami-tui vpn-keepalive remove")
}

func TestTheKeepalivePageOnTheCommandLine(t *testing.T) {
	answerKeep(t, keepMissing, nil)
	p := &prompter{yes: true}
	c := keepContext(No, false, nil)
	require.NoError(t, keepaliveCLI(context.Background(), c, p))
	assert.Equal(t, []string{"Install the VPN keepalive helper?"}, p.asked)
	assert.Equal(t, Yes, c.Param(ParamKeepalive))

	// Installed, a flag asks for nothing and nothing is asked.
	answerKeep(t, keepThere, nil)
	p = &prompter{yes: true}
	c = keepContext(Yes, false, nil)
	require.NoError(t, keepaliveCLI(context.Background(), c, p))
	assert.Empty(t, p.asked)
	assert.Equal(t, No, c.Param(ParamKeepalive))
}
