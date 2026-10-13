package installer

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/fynstall/extension"

	"github.com/ushineko/hayami/internal/nethelper"
)

// A machine in each state the network check can find. The installer in
// these tests carries hayami 0.0.0.
var (
	netNotWindows = Network{Version: "0.0.0"}
	netMissing    = Network{State: nethelper.State{Applies: true}, Version: "0.0.0"}
	netOlder      = Network{State: nethelper.State{Applies: true, Installed: true, Version: "0.0.0-older"}, Version: "0.0.0"}
	netCurrent    = Network{State: nethelper.State{Applies: true, Installed: true, Version: "0.0.0"}, Version: "0.0.0"}
)

// answerNet makes the network check find n for the length of a test.
func answerNet(t *testing.T, n Network) {
	t.Helper()
	was := probeNet
	probeNet = func(*extension.Context) Network { return n }
	t.Cleanup(func() { probeNet = was })
}

// helperCalls counts the installs and removals a test caused, and makes
// each answer err.
func helperCalls(t *testing.T, err error) (installs, removals *int) {
	t.Helper()
	wasSet, wasRemove := setUpNet, removeNet
	t.Cleanup(func() { setUpNet, removeNet = wasSet, wasRemove })
	installs, removals = new(int), new(int)
	setUpNet = func(context.Context, *extension.Context) error { *installs++; return err }
	removeNet = func(context.Context, *extension.Context) error { *removals++; return err }
	return installs, removals
}

// installNet is the context of an install, or an uninstall, with network at
// value.
func installNet(value string, upgrade bool, logged *[]string) *extension.Context {
	return extension.NewContext(extension.Host{
		App:      extension.App{ID: "io.ushineko.hayami", Name: "hayami", Version: "0.0.0"},
		Scope:    func() string { return "user" },
		Root:     func() string { return "" },
		Params:   map[string]string{ParamSensors: No, ParamNetwork: value},
		Declared: []string{ParamSensors, ParamNetwork},
		Upgrade:  upgrade,
		Log: func(_ extension.Level, text string) {
			if logged != nil {
				*logged = append(*logged, text)
			}
		},
	})
}

// Spec 061. The check's answer in words: a helper that is missing is
// offered, one of another version is offered as an update and said to go on
// working, and one that is this version is not offered again.
func TestTheNetworkCheckOffersOnlyWhatIsMissing(t *testing.T) {
	assert.Empty(t, netNotWindows.Offer(), "off Windows there is nothing to offer")
	assert.Contains(t, netNotWindows.Found(), "nothing to set up")

	assert.Contains(t, netMissing.Found(), "is not installed")
	assert.Equal(t, "Install the network helper", netMissing.Offer())

	assert.Contains(t, netOlder.Found(), "0.0.0-older")
	assert.Contains(t, netOlder.Found(), "goes on working")
	assert.Equal(t, "Update the network helper", netOlder.Offer())

	assert.True(t, netCurrent.Current())
	assert.Contains(t, netCurrent.Found(), "nothing to do")
	assert.Empty(t, netCurrent.Offer(), "what is installed is not offered again")
}

// What a person agrees to is said: where the program goes, who the service
// runs as, who may read it, that it runs only while the list is open, that
// Windows asks for an administrator, and that it stays until removed.
func TestTheNetworkStepsSayWhatIsBeingAgreedTo(t *testing.T) {
	steps := strings.Join(netMissing.Steps(), "\n")
	assert.Contains(t, steps, "Program Files")
	assert.Contains(t, steps, "Local Service")
	assert.Contains(t, steps, "no other")
	assert.Contains(t, steps, "stops when it is closed")
	assert.Contains(t, netMissing.Steps()[len(netMissing.Steps())-1], "asks for an administrator once")
	assert.Contains(t, NetworkCaution, "stays until it is removed")
	assert.Contains(t, NetworkCaution, "works without it")
	assert.Contains(t, NetworkWhy, "nothing else")
}

// On the command line the page asks once and takes the answer, and where
// there is nothing to offer it asks nothing and the parameter is no.
func TestTheNetworkPageAsksOnceAndTakesTheAnswer(t *testing.T) {
	answerNet(t, netMissing)
	for _, yes := range []bool{true, false} {
		p := &prompter{yes: yes}
		c := installNet(No, false, nil)
		require.NoError(t, networkCLI(context.Background(), c, p))
		require.Equal(t, []string{"Install the network helper?"}, p.asked)
		assert.Contains(t, strings.Join(p.said, "\n"), "Local Service")
		want := No
		if yes {
			want = Yes
		}
		assert.Equal(t, want, c.Param(ParamNetwork))
	}

	for _, n := range []Network{netCurrent, netNotWindows} {
		answerNet(t, n)
		p := &prompter{yes: true}
		c := installNet(Yes, false, nil)
		require.NoError(t, networkCLI(context.Background(), c, p))
		assert.Empty(t, p.asked)
		assert.Equal(t, No, c.Param(ParamNetwork))
	}
}

// The hook installs the helper only when the page was told to, and one that
// fails is a warning: hayami is installed either way.
func TestTheHelperIsInstalledOnlyWhenAsked(t *testing.T) {
	answerNet(t, netCurrent)
	installs, _ := helperCalls(t, nil)

	require.NoError(t, afterInstall(context.Background(), installNet(No, false, nil)))
	require.Zero(t, *installs, "the helper was installed unasked")

	var logged []string
	require.NoError(t, afterInstall(context.Background(), installNet(Yes, false, &logged)))
	require.Equal(t, 1, *installs)
	assert.Contains(t, strings.Join(logged, "\n"), "The network helper is installed (0.0.0)")

	helperCalls(t, errors.New("Windows asked for an administrator and was told no"))
	logged = nil
	require.NoError(t, afterInstall(context.Background(), installNet(Yes, false, &logged)), "a refused prompt failed the install")
	assert.Contains(t, strings.Join(logged, "\n"), "was told no")
	assert.Contains(t, strings.Join(logged, "\n"), "hayami is installed without it")
}

// An uninstall takes the helper with it. An upgrade, whose first half is an
// uninstall, leaves it: it would be removed and the person asked to install
// it again, with two prompts for an administrator. A removal that fails is
// a warning that says how to do it by hand.
func TestAnUninstallRemovesTheHelperAndAnUpgradeLeavesIt(t *testing.T) {
	answerNet(t, netCurrent)
	_, removals := helperCalls(t, nil)

	require.NoError(t, beforeUninstall(context.Background(), installNet(No, true, nil)))
	require.Zero(t, *removals, "an upgrade removed the helper")

	require.NoError(t, beforeUninstall(context.Background(), installNet(No, false, nil)))
	require.Equal(t, 1, *removals)

	answerNet(t, netMissing)
	require.NoError(t, beforeUninstall(context.Background(), installNet(No, false, nil)))
	require.Equal(t, 1, *removals, "a helper that is not there was removed")

	answerNet(t, netCurrent)
	helperCalls(t, errors.New("Windows asked for an administrator and was told no"))
	var logged []string
	require.NoError(t, beforeUninstall(context.Background(), installNet(No, false, &logged)), "a refused prompt failed the uninstall")
	assert.Contains(t, strings.Join(logged, "\n"), "hayami-tui network-helper remove")
}
