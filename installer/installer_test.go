package installer

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/fynstall/extension"

	"github.com/ushineko/hayami/internal/core"
)

// A machine in each state the check can find, with invented readings.
var (
	notWindows   = Sensors{}
	notInstalled = Sensors{Applies: true, URL: core.LHMURL, Problem: "It is not running"}
	installed    = Sensors{Applies: true, URL: core.LHMURL, Exe: `C:\Packages\LibreHardwareMonitor\LibreHardwareMonitor.exe`,
		Host: core.LHMHost{Running: true}, Problem: "It is running, and its web server does not answer at " + core.LHMURL}
	byHand = Sensors{Applies: true, URL: core.LHMURL, Host: core.LHMHost{Task: true},
		Problem: "It is not running, though it is registered to start with Windows"}
	ready = Sensors{Applies: true, URL: core.LHMURL, Exe: `C:\Packages\LibreHardwareMonitor\LibreHardwareMonitor.exe`,
		Host: core.LHMHost{Running: true, Task: true, PawnIO: true}, Reading: "Core (Tctl/Tdie) 48.1 °C"}
)

// answer makes the check find s for the length of a test.
func answer(t *testing.T, s Sensors) {
	t.Helper()
	was := probe
	probe = func(context.Context) Sensors { return s }
	t.Cleanup(func() { probe = was })
}

// prompter is a terminal that answers every question with yes or no, and
// keeps what it was shown.
type prompter struct {
	yes   bool
	said  []string
	asked []string
}

func (p *prompter) Ask(string, string) (string, error) {
	return "", errors.New("the page asks no line of text")
}
func (p *prompter) AskSecret(string) (string, error) {
	return "", errors.New("the page asks no secret")
}
func (p *prompter) Say(text string) { p.said = append(p.said, text) }
func (p *prompter) Confirm(question string) (bool, error) {
	p.asked = append(p.asked, question)
	return p.yes, nil
}

// install is the context of an install with sensors at value.
func install(value string, logged *[]string) *extension.Context {
	return extension.NewContext(extension.Host{
		App:      extension.App{ID: "io.ushineko.hayami", Name: "hayami", Version: "0.0.0"},
		Scope:    func() string { return "user" },
		Root:     func() string { return "" },
		Params:   map[string]string{ParamSensors: value, ParamNetwork: No},
		Declared: []string{ParamSensors, ParamNetwork},
		Log: func(_ extension.Level, text string) {
			if logged != nil {
				*logged = append(*logged, text)
			}
		},
	})
}

// Spec 059. The check's answer in words: each state says what was found, and
// only a machine with something to set up is offered anything.
func TestTheCheckSaysWhatItFoundAndOffersOnlyWhatIsMissing(t *testing.T) {
	assert.Empty(t, notWindows.Offer(), "off Windows there is nothing to offer")
	assert.Contains(t, notWindows.Found(), "nothing to set up")

	assert.Contains(t, notInstalled.Found(), "is not installed")
	assert.Equal(t, "Install LibreHardwareMonitor and configure it for hayami", notInstalled.Offer())
	assert.Contains(t, notInstalled.Steps()[0], "winget")

	assert.Contains(t, installed.Found(), "is installed, and hayami cannot read")
	assert.Contains(t, installed.Found(), "its web server does not answer")
	assert.Equal(t, "Configure LibreHardwareMonitor for hayami", installed.Offer())
	assert.NotContains(t, strings.Join(installed.Steps(), "\n"), "winget", "what is installed is not installed again")

	assert.True(t, byHand.Installed(), "one installed another way is still installed")
	assert.Contains(t, byHand.Found(), "is installed")

	assert.True(t, ready.Ready())
	assert.Contains(t, ready.Found(), "Core (Tctl/Tdie) 48.1 °C")
	assert.Empty(t, ready.Offer(), "what works is not offered again")
}

// Every list of steps ends by saying Windows asks for an administrator, and
// the caution names what a person is agreeing to.
func TestTheStepsAndTheCautionSayWhatIsBeingAgreedTo(t *testing.T) {
	for _, s := range []Sensors{notInstalled, installed} {
		steps := s.Steps()
		assert.Contains(t, steps[len(steps)-1], "asks for an administrator once")
	}
	assert.Contains(t, Caution, "administrator")
	assert.Contains(t, Caution, "kernel driver")
	assert.Contains(t, Caution, "works without it")
}

// On the command line the page says what it found and what setting up does,
// asks once, and sets the parameter from the answer. The default is no.
func TestThePageAsksOnceAndTakesTheAnswer(t *testing.T) {
	answer(t, notInstalled)
	for _, yes := range []bool{true, false} {
		p := &prompter{yes: yes}
		c := install(No, nil)
		require.NoError(t, sensorsCLI(context.Background(), c, p))
		require.Equal(t, []string{"Install LibreHardwareMonitor and configure it for hayami?"}, p.asked)
		said := strings.Join(p.said, "\n")
		assert.Contains(t, said, "is not installed")
		assert.Contains(t, said, "winget")
		assert.Contains(t, said, "kernel driver")
		want := No
		if yes {
			want = Yes
		}
		assert.Equal(t, want, c.Param(ParamSensors))
	}
}

// Where there is nothing to offer the page asks nothing, and sets no even
// when a flag said yes: nothing is set up twice.
func TestThePageAsksNothingWhenThereIsNothingToOffer(t *testing.T) {
	for _, s := range []Sensors{ready, notWindows} {
		answer(t, s)
		p := &prompter{yes: true}
		c := install(Yes, nil)
		require.NoError(t, sensorsCLI(context.Background(), c, p))
		assert.Empty(t, p.asked)
		assert.Equal(t, No, c.Param(ParamSensors))
	}
}

// The hook sets LibreHardwareMonitor up only when the page was told to, and
// a set-up that fails is a warning: hayami is installed either way.
func TestTheHookSetsUpOnlyWhenAskedAndNeverFailsTheInstall(t *testing.T) {
	ran := 0
	was := setUp
	t.Cleanup(func() { setUp = was })
	setUp = func(context.Context, *extension.Context) error { ran++; return nil }

	require.NoError(t, afterInstall(context.Background(), install(No, nil)))
	require.Zero(t, ran, "LibreHardwareMonitor was set up unasked")

	require.NoError(t, afterInstall(context.Background(), install(Yes, nil)))
	require.Equal(t, 1, ran)

	setUp = func(context.Context, *extension.Context) error { return errors.New("winget is not here") }
	var logged []string
	require.NoError(t, afterInstall(context.Background(), install(Yes, &logged)), "a failed set-up failed the install")
	assert.Contains(t, strings.Join(logged, "\n"), "winget is not here")
	assert.Contains(t, strings.Join(logged, "\n"), "hayami is installed without it")
}

// The extension keeps fynstall's rules against the parameters fynstall.yaml
// declares: the page sets a declared parameter and has a command-line path.
func TestTheExtensionKeepsFynstallsRules(t *testing.T) {
	config := readRoot(t, "fynstall.yaml")
	require.Regexp(t, `(?m)^  - name: `+ParamSensors+`$`, config, "fynstall.yaml does not declare the page's parameter")
	require.Regexp(t, `(?m)^  - name: `+ParamNetwork+`$`, config, "fynstall.yaml does not declare the network page's parameter")
	require.Regexp(t, `(?m)^extension: \./installer$`, config)
	require.NoError(t, Extension.Check([]string{ParamSensors, ParamNetwork}))
	require.NotNil(t, Extension.AfterInstall)
	require.NotNil(t, Extension.BeforeUninstall)
}

// The installer says the version VERSION says, or it installs one version
// and calls it another.
func TestTheInstallersVersionIsVERSIONs(t *testing.T) {
	version := strings.TrimSpace(readRoot(t, "VERSION"))
	m := regexp.MustCompile(`(?m)^  version: (\S+)$`).FindStringSubmatch(readRoot(t, "fynstall.yaml"))
	require.NotNil(t, m, "fynstall.yaml has no app.version")
	require.Equal(t, version, m[1], "fynstall.yaml's app.version is not VERSION's")
}

// readRoot reads a file at the repository's root.
func readRoot(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", name))
	require.NoError(t, err)
	return strings.ReplaceAll(string(b), "\r\n", "\n")
}
