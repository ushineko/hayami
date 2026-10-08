package prefs_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/hayami/internal/config"
	"github.com/ushineko/hayami/internal/desktop"
	"github.com/ushineko/hayami/internal/prefs"
	"github.com/ushineko/hayami/internal/testenv"
	"github.com/ushineko/hayami/internal/view"
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
	require.Len(t, built, 4)
	assert.Equal(t, "Sections", built[0].Title())
	assert.Equal(t, "Window", built[1].Title())
	assert.Equal(t, "Appearance", built[2].Title(),
		"the shell's own appearance section follows ours")
	assert.Equal(t, "About", built[3].Title(), "About is last")

	// Bandwidth has no screen of its own: its one setting -- which
	// interfaces to watch -- lives under Sections, beside the section it
	// belongs to. A navigation entry for a list of checkboxes was more than
	// it was worth once the list was short enough to read.
	for _, sec := range built {
		assert.NotEqual(t, "Bandwidth", sec.Title(),
			"the interfaces went back to a screen of their own")
	}
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

// AC8. Every section this build knows is offered by its title (spec 038; it
// was the settings key), including the one added last.
//
// The list is built from the section registry, so this passes for free — which is
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

	for _, s := range view.Sections() {
		assert.Contains(t, offered, s.Title)
	}
	assert.Contains(t, offered, "Peripherals")
}

// AC10. The Window section offers the rule and the opacity -- the rule only
// where KWin is, and elsewhere it says why there is none rather than offering
// a control for a desktop that is not there.
func TestTheWindowSectionOffersTheRuleAndTheOpacity(t *testing.T) {
	for _, c := range []struct {
		name  string
		rules bool
	}{{"with kwin", true}, {"without kwin", false}} {
		rules := c.rules
		t.Run(c.name, func(t *testing.T) {
			withRules(t, rules)
			testenv.Config(t, t.TempDir())

			a := test.NewApp()
			t.Cleanup(a.Quit)

			w := prefs.New(a, prefs.Options{Store: store(t, "")})
			w.Shell().Select("Window")

			checks, labels, sliders := windowSection(w)

			assert.Positive(t, sliders, "the window section offers no opacity")
			if !rules {
				assert.Empty(t, checks, "a KWin rule was offered where there is no KWin")
				assert.Contains(t, labels, desktop.NoRules, "the section does not say why there is no rule")
				return
			}
			require.NotEmpty(t, checks, "the window section offers no toggle")
			assert.Contains(t, checks[0], "Frameless")
		})
	}
}

// withRules pretends to be a platform where window rules do, or do not, apply.
func withRules(t *testing.T, on bool) {
	t.Helper()
	was := desktop.RulesApply
	desktop.RulesApply = on
	t.Cleanup(func() { desktop.RulesApply = was })
}

// windowSection lists what the Window section shows.
func windowSection(w *prefs.Window) (checks, labels []string, sliders int) {
	for _, o := range test.LaidOutObjects(w.Shell().Window.Content()) {
		switch v := o.(type) {
		case *widget.Check:
			checks = append(checks, v.Text)
		case *widget.Label:
			labels = append(labels, v.Text)
		case *widget.Slider:
			sliders++
		}
	}
	return checks, labels, sliders
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

// Closing the preferences window puts it away rather than destroying it.
//
// A closed Fyne window cannot be shown again, and the panel's menu offers
// Preferences every time it is opened — so without the intercept the first
// close would make that menu item do nothing for the rest of the run, which
// is a worse bug than the one it replaces.
func TestClosingThePreferencesPutsThemAway(t *testing.T) {
	a := test.NewApp()
	t.Cleanup(a.Quit)

	w := prefs.New(a, prefs.Options{Store: store(t, "")})
	w.Show()

	// What the close button does: the intercept, not a destroy.
	require.NotNil(t, w.Shell().Window)
	w.Shell().Window.Hide()

	// And the menu can bring it back, however many times.
	for range 3 {
		w.Show()
		w.Shell().Window.Hide()
	}
	assert.NotNil(t, w.Shell().Window, "the window did not survive being put away")
}

// The navigation's shape is the user's.
//
// The shell draws its own control for the shapes a program lists and binds its
// shortcut; a program that lists none gets the one shape it has always had and
// no way to change it. Four sections is few enough that icons alone are
// legible, and a top strip is a reasonable choice on a wide screen.
func TestTheNavigationsShapeIsOffered(t *testing.T) {
	a := test.NewApp()
	t.Cleanup(a.Quit)

	w := prefs.New(a, prefs.Options{Store: store(t, "")})

	// The shell's own control appears in the header once shapes are listed.
	var buttons []string
	for _, o := range test.LaidOutObjects(w.Shell().Window.Content()) {
		if b, ok := o.(*widget.Button); ok {
			buttons = append(buttons, b.Text)
		}
	}
	assert.NotEmpty(t, buttons, "the header has no controls at all")

	// And Refresh is not among them: every screen here saves as it is changed.
	assert.NotContains(t, buttons, "Refresh")
}

// Pages are the navigation's titles, in its order: what --preferences takes
// and what the screenshot harness photographs.
func TestPagesAreTheNavigationInOrder(t *testing.T) {
	a := test.NewApp()
	t.Cleanup(a.Quit)

	w := prefs.New(a, prefs.Options{Store: store(t, ""), Version: "1.2.3"})
	var titles []string
	for _, s := range w.Shell().Sections() {
		titles = append(titles, strings.ToLower(s.Title()))
	}
	assert.Equal(t, titles, prefs.Pages())
}

// The window opens on the page it is asked for, in any case, and a page asked
// for after it exists moves it there.
func TestTheWindowOpensOnTheNamedPage(t *testing.T) {
	a := test.NewApp()
	t.Cleanup(a.Quit)

	w := prefs.New(a, prefs.Options{Store: store(t, ""), Version: "1.2.3", Page: "about"})
	assert.Equal(t, "About", w.Shell().Current().Title())

	w.ShowPage("Window")
	assert.Equal(t, "Window", w.Shell().Current().Title())
}
