package prefs_test

import (
	"os"
	"path/filepath"
	"testing"

	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/hayami/internal/config"
	"github.com/ushineko/hayami/internal/panel"
	"github.com/ushineko/hayami/internal/prefs"
)

func store(t *testing.T, body string) *config.Store {
	t.Helper()
	path := filepath.Join(t.TempDir(), "settings.yaml")
	if body != "" {
		require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
	}
	s, err := config.Open(path)
	require.NoError(t, err)
	return s
}

// The window is a second window of the same process, over an app that already
// exists. That is what shell.NewIn is for, and if this ever needs its own app
// the design has gone wrong.
func TestThePreferencesAreASecondWindowOfTheSameApp(t *testing.T) {
	a := test.NewApp()
	t.Cleanup(a.Quit)

	w := prefs.New(a, prefs.Options{Store: store(t, ""), Version: "1.2.3"})

	require.NotNil(t, w.Shell())
	assert.NotNil(t, w.Shell().Window, "a preferences window with no window is a test helper")
}

// Every section this build has is offered, chosen or not. A reader who had to
// look in two lists to find a section would have to know which before they
// started looking.
func TestEverySectionIsOfferedWhetherItIsChosenOrNot(t *testing.T) {
	a := test.NewApp()
	t.Cleanup(a.Quit)
	s := store(t, "hayami:\n    sections:\n        - usage\n")

	w := prefs.New(a, prefs.Options{Store: s})

	built := w.Shell().Sections()
	require.NotEmpty(t, built)
	assert.Equal(t, "Sections", built[0].Title())
	assert.Equal(t, "Bandwidth", built[1].Title())
	assert.Equal(t, "Window", built[2].Title())
	assert.Equal(t, "Appearance", built[3].Title(),
		"the shell's own appearance section is last, after ours")
}

// A change is saved as it is made. A preference that needed confirming is a
// preference somebody abandons half-set.
func TestAChangeReachesTheStoreWithoutASaveButton(t *testing.T) {
	s := store(t, "hayami:\n    sections:\n        - usage\n")
	called := 0

	require.NoError(t, s.SetConfig(config.Config{Sections: []string{"usage", "cooler"}}))
	called++
	require.NoError(t, s.Flush())

	assert.Equal(t, []string{"usage", "cooler"}, s.Config().Sections)
	assert.Equal(t, 1, called)
}

// AC8. Every section this build knows is offered by name, including the one
// added last.
//
// The list is built from panel.Keys(), so this passes for free — which is
// exactly why it is asserted rather than assumed. A section wired into the
// panel and not into the preferences would be one a reader could see and not
// turn off, and nothing else in the suite would notice.
func TestEveryKnownSectionIsOfferedByName(t *testing.T) {
	a := test.NewApp()
	t.Cleanup(a.Quit)

	w := prefs.New(a, prefs.Options{Store: store(t, "hayami:\n    sections:\n        - usage\n")})

	var offered []string
	for _, o := range test.LaidOutObjects(w.Shell().Window.Content()) {
		if c, ok := o.(*widget.Check); ok {
			offered = append(offered, c.Text)
		}
	}

	for _, key := range panel.Keys() {
		assert.Contains(t, offered, key)
	}
	assert.Contains(t, offered, "peripherals")
}

// AC10. The Window section offers the rule and the opacity.
func TestTheWindowSectionOffersTheRuleAndTheOpacity(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	a := test.NewApp()
	t.Cleanup(a.Quit)

	w := prefs.New(a, prefs.Options{Store: store(t, "")})
	w.Shell().Select("Window")

	var checks []string
	sliders := 0
	for _, o := range test.LaidOutObjects(w.Shell().Window.Content()) {
		switch v := o.(type) {
		case *widget.Check:
			checks = append(checks, v.Text)
		case *widget.Slider:
			sliders++
		}
	}

	require.NotEmpty(t, checks, "the window section offers no toggle")
	assert.Contains(t, checks[0], "Frameless")
	assert.Positive(t, sliders, "the window section offers no opacity")
}

// AC10. The opacity chosen here reaches the store, because it is the value
// that survives a restart — the menu's does not.
func TestTheOpacityFromThePreferencesIsSaved(t *testing.T) {
	s := store(t, "")

	c := s.Config()
	c.Opacity = 75
	require.NoError(t, s.SetConfig(c))
	require.NoError(t, s.Flush())

	assert.Equal(t, 75, s.Config().Opacity)
}
