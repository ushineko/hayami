package gui_test

import (
	"path/filepath"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	fynetheme "fyne.io/fyne/v2/theme"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	fdtheme "github.com/ushineko/fynedesygn/theme"

	"github.com/ushineko/hayami/internal/config"
	"github.com/ushineko/hayami/internal/gui"
)

func store(t *testing.T) *config.Store {
	t.Helper()
	s, err := config.Open(filepath.Join(t.TempDir(), "settings.yaml"))
	require.NoError(t, err)
	// Closed before the directory goes, so no write the menu scheduled lands
	// after the test (#125).
	t.Cleanup(func() { require.NoError(t, s.Close()) })
	return s
}

// AC9. The menu has the three items its own documentation claims.
//
// It claimed them for nine specs while having two, which is the whole of
// issue #26: a doc comment describing a feature nobody had built.
func TestTheMenuHasItsThreeItems(t *testing.T) {
	a := test.NewApp()
	t.Cleanup(a.Quit)

	m := gui.Menu(a, store(t), "1.2.3", nil)()

	require.Len(t, m.Items, 3)
	assert.Equal(t, "Preferences…", m.Items[0].Label)
	assert.Equal(t, "Opacity", m.Items[1].Label)
	assert.Equal(t, "Quit", m.Items[2].Label)
}

// AC9. The opacity submenu offers the steps and ticks the one in the settings.
func TestTheOpacitySubmenuTicksTheCurrentValue(t *testing.T) {
	a := test.NewApp()
	t.Cleanup(a.Quit)

	s := store(t)
	c := s.Config()
	c.Opacity = 85
	require.NoError(t, s.SetConfig(c))

	m := gui.Menu(a, s, "1.2.3", nil)()
	sub := m.Items[1].ChildMenu
	require.NotNil(t, sub, "the opacity item has no submenu")
	require.Len(t, sub.Items, len(gui.OpacitySteps))

	ticked := 0
	for i, item := range sub.Items {
		if item.Checked {
			ticked++
			assert.Equal(t, 85, gui.OpacitySteps[i])
		}
	}
	assert.Equal(t, 1, ticked, "exactly one opacity should be ticked")
}

// A settings file with no opacity ticks the default rather than nothing.
func TestTheSubmenuTicksTheDefaultWhenNothingIsSet(t *testing.T) {
	a := test.NewApp()
	t.Cleanup(a.Quit)

	m := gui.Menu(a, store(t), "1.2.3", nil)()
	sub := m.Items[1].ChildMenu
	require.NotNil(t, sub)

	ticked := false
	for _, item := range sub.Items {
		if item.Checked {
			ticked = true
		}
	}
	assert.True(t, ticked, "no opacity was ticked at all")
}

// The panel takes a resize from the window manager.
//
// A fixed-size window tells the window manager it will not, so a frameless
// panel's own Resize menu item is greyed out and there is no way to ask for a
// wider one. It cannot be made narrower than its content whatever this says —
// Fyne clamps to the minimum size — which is why the content had to be
// narrowed separately.
func TestThePanelTakesAResize(t *testing.T) {
	a := test.NewApp()
	t.Cleanup(a.Quit)

	p := gui.New(a, gui.Options{Title: "hayami"})
	require.NotNil(t, p.Window())

	assert.False(t, p.Window().Window().FixedSize(),
		"a fixed-size panel cannot be resized by the window manager at all")
}

// The panel draws in the appearance the user chose.
//
// It used to hard-code the scheme and the default face, so the Appearance
// screen in the preferences changed the preferences window and nothing else —
// a font chooser with no effect on the panel sitting next to it.
func TestThePanelFollowsTheSavedAppearance(t *testing.T) {
	a := test.NewApp()
	t.Cleanup(a.Quit)

	s := store(t)

	saved := fdtheme.DefaultAppearance()
	saved.Scheme = "Nord"
	saved.TextSize = 18
	saved.SaveTo(s.Settings())

	got := gui.Appearance(a, s)
	assert.Equal(t, "Nord", got.Scheme)
	assert.InDelta(t, 18, got.TextSize, 0.01)
}

// A panel with no settings yet takes the design system's own defaults rather
// than nothing at all.
func TestAPanelWithNoSettingsTakesTheDefaults(t *testing.T) {
	a := test.NewApp()
	t.Cleanup(a.Quit)

	got := gui.Appearance(a, store(t))
	assert.Equal(t, fdtheme.DefaultAppearance().Scheme, got.Scheme)
}

// The shell asks for a theme every time it lays itself out, and the panel
// answers by setting the application's theme -- which rebuilds every window
// and takes with it the subtree override that keeps the preferences window in
// its own appearance.
//
// Notifying on every layout therefore undid the separation the moment the
// window was built: the whole preferences window drew in the panel's text
// size, 8 pt against the 12 pt on its own Appearance screen. It also makes a
// loop, because putting the override back is a layout.
func TestThePanelIsToldOnlyWhenTheAppearanceHasChanged(t *testing.T) {
	var told int
	hook := gui.ThemeWith(store(t), func(config.Config) { told++ }, func(f func()) { f() })

	a := fdtheme.DefaultAppearance()
	require.NotNil(t, hook(a))
	settled := told

	hook(a)
	hook(a)
	assert.Equal(t, settled, told, "laying the window out again is not a change of appearance")

	bigger := a
	bigger.TextSize = a.TextSize + 4
	hook(bigger)
	assert.Equal(t, settled+1, told, "a real change still reaches the panel")

	hook(bigger)
	assert.Equal(t, settled+1, told)
}

// The first ask is a change: the panel starts in whatever theme it was built
// with and has to be told once even if nothing the user did caused it.
func TestTheFirstAskAlwaysTellsThePanel(t *testing.T) {
	var told int
	hook := gui.ThemeWith(store(t), func(config.Config) { told++ }, func(f func()) { f() })

	hook(fdtheme.DefaultAppearance())

	assert.Equal(t, 1, told)
}

/*
The theme the preferences window wraps itself in carries none of the panel's
fade.

It used to. The reasoning was sound while that window owned the application's
theme -- setting a theme replaces whatever was wrapped around the last one, so
the fade had to be re-applied -- and it stopped being sound the moment the
window took OwnAppearance and started theming only its own subtree instead.

What it produced was a settings window whose every button and separator was
drawn at ninety-five per cent, over a framebuffer the panel had already asked
GLFW to make transparent. The desktop showed through the controls, and it was
reported as "prefs is partially transparent, seems to be incorrectly using the
style from the panel" -- which is exactly what it was.
*/
func TestThePreferencesThemeCarriesNoneOfThePanelsFade(t *testing.T) {
	st := store(t)
	c := st.Config()
	c.Opacity = 50 // a fade nobody could miss
	require.NoError(t, st.SetConfig(c))

	hook := gui.ThemeWith(st, nil, func(f func()) { f() })
	th := hook(fdtheme.DefaultAppearance())

	for _, name := range []fyne.ThemeColorName{
		fynetheme.ColorNameButton, fynetheme.ColorNameSeparator,
	} {
		_, _, _, a := th.Color(name, fynetheme.VariantDark).RGBA()
		assert.Equal(t, uint32(0xffff), a, "%s is faded in the preferences window", name)
	}
}

// And the panel's own theme still carries it, because that is where the cards
// are and the fade is theirs.
func TestThePanelsThemeStillFadesItsCards(t *testing.T) {
	faded := gui.WithCardOpacity(fdtheme.DefaultAppearance().Theme(), 50)

	_, _, _, a := faded.Color(fynetheme.ColorNameButton, fynetheme.VariantDark).RGBA()
	assert.Less(t, a, uint32(0xffff))
}
