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

// answerAPITCP makes the Port page's check find a, the service installed or
// not, for the length of a test, and counts the set-ups and removals it
// caused, each answering err.
func answerAPITCP(t *testing.T, a *APITCP, service bool, err error) (setups, removals *int) {
	t.Helper()
	was := []any{probeAPITCP, setUpAPITCP, removeAPITCP, markTCPAsked, unmarkTCPAsked, apiTCPInstalled}
	t.Cleanup(func() {
		probeAPITCP = was[0].(func(*extension.Context) APITCP)
		setUpAPITCP = was[1].(func(context.Context, *extension.Context, int) error)
		removeAPITCP = was[2].(func(context.Context, *extension.Context) error)
		markTCPAsked = was[3].(func(*extension.Context) error)
		unmarkTCPAsked = was[4].(func(*extension.Context))
		apiTCPInstalled = was[5].(func() bool)
	})
	setups, removals = new(int), new(int)
	probeAPITCP = func(*extension.Context) APITCP { return *a }
	setUpAPITCP = func(context.Context, *extension.Context, int) error {
		*setups++
		if err == nil {
			a.Set = true
		}
		return err
	}
	removeAPITCP = func(context.Context, *extension.Context) error {
		*removals++
		if err == nil {
			a.Set = false
		}
		return err
	}
	markTCPAsked = func(*extension.Context) error { a.Asked = true; return nil }
	unmarkTCPAsked = func(*extension.Context) { a.Asked = false }
	apiTCPInstalled = func() bool { return service }
	return setups, removals
}

func tcpContext(value string, upgrade bool, logged *[]string) *extension.Context {
	return extension.NewContext(extension.Host{
		App:      extension.App{ID: "io.ushineko.hayami", Name: "hayami", Version: "0.0.0"},
		Scope:    func() string { return "user" },
		Root:     func() string { return "" },
		Params:   map[string]string{ParamAPITCP: value},
		Declared: []string{ParamAPITCP},
		Upgrade:  upgrade,
		Log: func(_ extension.Level, text string) {
			if logged != nil {
				*logged = append(*logged, text)
			}
		},
	})
}

/*
Spec 115 R12. What the installer does with the port, by what it found and
what it was told: auto sets it up the first time and keeps one that is there;
yes and no win; and without the service there is no port, whatever was said.
*/
func TestThePortIsPlacedAsTheInstallWasTold(t *testing.T) {
	cases := []struct {
		name            string
		found           APITCP
		service         bool
		param           string
		setups, removes int
	}{
		{"first install, auto", APITCP{Service: true}, true, Auto, 1, 0},
		{"upgrade with a port, auto", APITCP{Service: true, Set: true, Asked: true}, true, Auto, 1, 0},
		{"a port removed before, auto", APITCP{Service: true, Asked: true}, true, Auto, 0, 0},
		{"a port removed before, yes", APITCP{Service: true, Asked: true}, true, Yes, 1, 0},
		{"a port there, no", APITCP{Service: true, Set: true, Asked: true}, true, No, 0, 1},
		{"no port, no", APITCP{Service: true}, true, No, 0, 0},
		{"no service, yes", APITCP{}, false, Yes, 0, 0},
		{"no service, a key left", APITCP{Set: true}, false, Auto, 0, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a := tc.found
			setups, removes := answerAPITCP(t, &a, tc.service, nil)
			placeAPITCP(context.Background(), tcpContext(tc.param, false, nil))
			assert.Equal(t, tc.setups, *setups, "set-ups")
			assert.Equal(t, tc.removes, *removes, "removals")
		})
	}
}

// Spec 115 R12. A port someone removed stays removed through --yes upgrades,
// which leave the parameter auto.
func TestARemovedPortIsNotGivenBackByUpgrades(t *testing.T) {
	a := APITCP{Service: true}
	setups, removes := answerAPITCP(t, &a, true, nil)
	placeAPITCP(context.Background(), tcpContext(Auto, false, nil))
	require.Equal(t, 1, *setups)
	placeAPITCP(context.Background(), tcpContext(No, true, nil))
	require.Equal(t, 1, *removes)
	for range 3 {
		placeAPITCP(context.Background(), tcpContext(Auto, true, nil))
	}
	assert.Equal(t, 1, *setups, "an upgrade gave the port back")
}

// Spec 115 R12. A failed step is a warning that says the socket still
// serves, and the decision is not noted, so the next install tries again.
func TestAFailedPortStepIsAWarning(t *testing.T) {
	a := APITCP{Service: true}
	_, _ = answerAPITCP(t, &a, true, errors.New("the manager said no"))
	var logged []string
	placeAPITCP(context.Background(), tcpContext(Yes, false, &logged))
	require.Len(t, logged, 1)
	assert.True(t, strings.Contains(logged[0], "the socket serves without it"), logged[0])
	assert.False(t, a.Asked, "a failure was noted as decided")
}

// Spec 115 R12. An uninstall takes the key and the note; an upgrade keeps
// both.
func TestThePortGoesWithAnUninstallAndNotAnUpgrade(t *testing.T) {
	a := APITCP{Service: true, Set: true, Asked: true}
	_, removes := answerAPITCP(t, &a, true, nil)
	dropAPITCP(context.Background(), tcpContext(Auto, true, nil))
	assert.Equal(t, 0, *removes)
	assert.True(t, a.Asked)
	dropAPITCP(context.Background(), tcpContext(Auto, false, nil))
	assert.Equal(t, 1, *removes)
	assert.False(t, a.Asked)
}

// Spec 115 R12. The page says what was found and proposes what auto would
// do; with no service it is passed over.
func TestThePortPageSaysWhatItFound(t *testing.T) {
	assert.False(t, APITCP{}.applies(), "a page with no service to offer a port for")
	assert.Contains(t, APITCP{Service: true, Set: true, Port: 47931}.Found(), "127.0.0.1:47931")
	assert.Equal(t, Yes, APITCP{Service: true}.proposed())
	assert.Equal(t, Yes, APITCP{Service: true, Set: true, Asked: true}.proposed())
	assert.Equal(t, No, APITCP{Service: true, Asked: true}.proposed())
	for _, step := range (APITCP{Port: 47930}).Steps() {
		assert.NotEmpty(t, step)
	}
}
