package installer

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/fynstall/extension"
)

// answerAPI makes the check find a for the length of a test, and counts the
// installs and removals the test caused, each answering err. The marker is
// the test's: a decision noted makes the next check find Asked, and an
// uninstall's removal of it makes it find it not.
func answerAPI(t *testing.T, a API, err error) (installs, removals *int) {
	t.Helper()
	wasProbe, wasSet, wasRemove, wasMark, wasUnmark := probeAPI, setUpAPI, removeAPI, markAsked, unmarkAsked
	t.Cleanup(func() {
		probeAPI, setUpAPI, removeAPI, markAsked, unmarkAsked = wasProbe, wasSet, wasRemove, wasMark, wasUnmark
	})
	installs, removals = new(int), new(int)
	probeAPI = func(*extension.Context) API { return a }
	setUpAPI = func(context.Context, *extension.Context) error { *installs++; return err }
	removeAPI = func(context.Context, *extension.Context) error { *removals++; return err }
	markAsked = func(*extension.Context) error { a.Asked = true; return nil }
	unmarkAsked = func(*extension.Context) { a.Asked = false }
	return installs, removals
}

// The marker is a file in the install's share directory, found once written.
func TestTheMarkerIsAFileInTheInstall(t *testing.T) {
	root := t.TempDir()
	c := extension.NewContext(extension.Host{Root: func() string { return root }, Params: map[string]string{}})
	path := askedMarker(c)
	assert.Equal(t, filepath.Join(root, "share", "api-asked"), path)
	assert.False(t, markerThere(path))
	require.NoError(t, writeMarker(path))
	assert.True(t, markerThere(path))
}

// asked is what the check finds once the installer has decided before.
func asked(a API) API { a.Asked = true; return a }

func apiContext(value string, upgrade bool, logged *[]string) *extension.Context {
	return extension.NewContext(extension.Host{
		App:      extension.App{ID: "io.ushineko.hayami", Name: "hayami", Version: "0.0.0"},
		Scope:    func() string { return "user" },
		Root:     func() string { return "" },
		Params:   map[string]string{ParamAPI: value},
		Declared: []string{ParamAPI},
		Upgrade:  upgrade,
		Log: func(_ extension.Level, text string) {
			if logged != nil {
				*logged = append(*logged, text)
			}
		},
	})
}

var (
	apiThere   = API{Offered: true, Installed: true}
	apiMissing = API{Offered: true}
	apiNotHere = API{}
)

/*
Spec 113 R25. What the installer does with the service, by what it found and
what it was told: auto, which --yes leaves, sets it up the first time the
installer decides, keeps one that is there on the new program, and never
gives back one someone removed; yes and no are the person's and win.
*/
func TestTheServiceIsPlacedAsTheInstallWasTold(t *testing.T) {
	cases := []struct {
		name               string
		found              API
		value              string
		installs, removals int
	}{
		{"auto, first time: set up", apiMissing, Auto, 1, 0},
		{"auto, decided before, there: kept on the new program", asked(apiThere), Auto, 1, 0},
		{"auto, decided before, removed: left off", asked(apiMissing), Auto, 0, 0},
		{"yes, removed before: set up", asked(apiMissing), Yes, 1, 0},
		{"yes, there: kept", asked(apiThere), Yes, 1, 0},
		{"no, first time: not set up", apiMissing, No, 0, 0},
		{"no, there: removed", asked(apiThere), No, 0, 1},
		{"yes, no service manager: nothing", apiNotHere, Yes, 0, 0},
		{"auto, no service manager: nothing", apiNotHere, Auto, 0, 0},
		{"an installer that does not know the parameter: nothing", asked(apiThere), "", 0, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			installs, removals := answerAPI(t, c.found, nil)
			placeAPI(context.Background(), apiContext(c.value, false, nil))
			assert.Equal(t, c.installs, *installs, "installs")
			assert.Equal(t, c.removals, *removals, "removals")
		})
	}
}

/*
The sequence the marker is for (found on the desk: fynstall's install half
cannot tell a first install from an upgrade): a first install under --yes
sets the service up; the person removes it; every --yes upgrade after leaves
it removed; an uninstall forgets, so a later install is a first install
again.
*/
func TestARemovedServiceIsNotGivenBackByUpgrades(t *testing.T) {
	installs, removals := answerAPI(t, apiMissing, nil)
	placeAPI(context.Background(), apiContext(Auto, false, nil))
	require.Equal(t, 1, *installs, "a first install sets it up")

	// The person clears the box: the service goes.
	probeAPI = func(*extension.Context) API { return API{Offered: true, Installed: true, Asked: true} }
	placeAPI(context.Background(), apiContext(No, false, nil))
	require.Equal(t, 1, *removals)

	// Every upgrade under --yes after that finds it removed and decided.
	marked := true
	probeAPI = func(*extension.Context) API { return API{Offered: true, Asked: marked} }
	unmarkAsked = func(*extension.Context) { marked = false }
	for range 3 {
		placeAPI(context.Background(), apiContext(Auto, false, nil))
	}
	assert.Equal(t, 1, *installs, "an upgrade never gives it back")

	// An uninstall forgets the decision with the install.
	dropAPI(context.Background(), apiContext("", false, nil))
	assert.False(t, marked, "the uninstall took the note")
	placeAPI(context.Background(), apiContext(Auto, false, nil))
	assert.Equal(t, 2, *installs, "a first install again")
}

// A step that fails is a warning that says how to do it by hand, and the
// install goes on (R25): hayami works without the service.
func TestAFailedServiceStepIsAWarning(t *testing.T) {
	var logged []string
	answerAPI(t, apiMissing, errors.New("no manager"))
	placeAPI(context.Background(), apiContext(Yes, false, &logged))
	assert.Contains(t, strings.Join(logged, "\n"), "installed without it")

	logged = nil
	answerAPI(t, apiThere, errors.New("no manager"))
	placeAPI(context.Background(), apiContext(No, true, &logged))
	assert.Contains(t, strings.Join(logged, "\n"), "systemctl --user disable --now hayami-api.socket")

	logged = nil
	answerAPI(t, apiNotHere, nil)
	placeAPI(context.Background(), apiContext(Yes, false, &logged))
	assert.Contains(t, strings.Join(logged, "\n"), "hayami-tui serve runs it by hand", "asked for where it cannot be, it says how to run it")
}

// An uninstall removes the service; an upgrade's uninstall half does not,
// since its install half keeps it on the new program.
func TestTheServiceGoesWithAnUninstallAndNotAnUpgrade(t *testing.T) {
	_, removals := answerAPI(t, apiThere, nil)
	dropAPI(context.Background(), apiContext("", true, nil))
	assert.Equal(t, 0, *removals, "an upgrade keeps it")
	dropAPI(context.Background(), apiContext("", false, nil))
	assert.Equal(t, 1, *removals, "an uninstall removes it")

	_, removals = answerAPI(t, apiMissing, nil)
	dropAPI(context.Background(), apiContext("", false, nil))
	assert.Equal(t, 0, *removals, "nothing to remove")
}

/*
The page proposes what auto would do, and the person's answer is the
parameter: the first time the default is yes, after that without the service
it is no, and with the service it is keeping it. A flag's yes or no is what
the page proposes in its place.
*/
func TestThePageProposesWhatAutoWouldDo(t *testing.T) {
	cases := []struct {
		name  string
		found API
		value string
		want  string
	}{
		{"first time", apiMissing, Auto, Yes},
		{"decided before, without it", asked(apiMissing), Auto, No},
		{"decided before, with it", asked(apiThere), Auto, Yes},
		{"a flag said no", asked(apiThere), No, No},
		{"a flag said yes", asked(apiMissing), Yes, Yes},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			answerAPI(t, c.found, nil)
			ctx := apiContext(c.value, false, nil)
			p := &linePrompter{}
			require.NoError(t, apiCLI(context.Background(), ctx, p))
			assert.Equal(t, c.want, p.defaults[0], "the question's default")
			assert.Equal(t, c.want, ctx.Param(ParamAPI), "an empty answer takes it")
		})
	}

	// An answer is the parameter, whatever was proposed.
	answerAPI(t, apiThere, nil)
	ctx := apiContext(Auto, true, nil)
	require.NoError(t, apiCLI(context.Background(), ctx, &linePrompter{answers: []string{"maybe", "no"}}))
	assert.Equal(t, No, ctx.Param(ParamAPI), "an answer that is not yes or no is asked again")

	// Where there is nothing to set up, the page says so and asks nothing.
	answerAPI(t, apiNotHere, nil)
	ctx = apiContext(Yes, false, nil)
	p := &linePrompter{}
	require.NoError(t, apiCLI(context.Background(), ctx, p))
	assert.Empty(t, p.defaults)
	assert.Equal(t, No, ctx.Param(ParamAPI))
	assert.Contains(t, strings.Join(p.said, "\n"), "hayami-tui serve")
}

// linePrompter answers Ask with its answers in turn, then with the default,
// and keeps the defaults it was offered.
type linePrompter struct {
	answers  []string
	defaults []string
	said     []string
}

func (p *linePrompter) Ask(_, def string) (string, error) {
	p.defaults = append(p.defaults, def)
	if len(p.answers) == 0 {
		return def, nil
	}
	a := p.answers[0]
	p.answers = p.answers[1:]
	return a, nil
}
func (p *linePrompter) AskSecret(string) (string, error) { return "", errors.New("no secret") }
func (p *linePrompter) Confirm(string) (bool, error) {
	return false, errors.New("the page asks with a default")
}
func (p *linePrompter) Say(text string) { p.said = append(p.said, text) }
