package gui_test

import (
	"path/filepath"
	"testing"

	"fyne.io/fyne/v2/test"
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
