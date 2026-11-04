package prefs_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/hayami/internal/config"
	"github.com/ushineko/hayami/internal/core"
	"github.com/ushineko/hayami/internal/prefs"
)

func systems(names ...string) func() []core.VPNBackend {
	return func() []core.VPNBackend {
		return []core.VPNBackend{{
			Name:       "fake",
			Active:     func(context.Context) ([]core.VPN, error) { return nil, nil },
			Configured: func(context.Context) ([]string, error) { return names, nil },
		}}
	}
}

// vpnWindow is the preferences on their Sections page, with the list read.
func vpnWindow(t *testing.T, s *config.Store, importFrom string, names ...string) *prefs.Window {
	t.Helper()
	a := test.NewApp()
	t.Cleanup(a.Quit)
	if importFrom == "" {
		importFrom = filepath.Join(t.TempDir(), "absent.json")
	}
	w := prefs.New(a, prefs.Options{
		Store: s, Version: "1.2.3", VPNSystems: systems(names...), VPNToggleConfig: importFrom,
		Background: func(f func()) { f() },
	})
	w.ShowPage("sections")
	return w
}

func tick(w *prefs.Window, text string) *widget.Check {
	for _, o := range test.LaidOutObjects(w.Shell().Window.Content()) {
		if c, ok := o.(*widget.Check); ok && c.Text == text {
			return c
		}
	}
	return nil
}

func button(w *prefs.Window, text string) *widget.Button {
	for _, o := range test.LaidOutObjects(w.Shell().Window.Content()) {
		if b, ok := o.(*widget.Button); ok && b.Text == text {
			return b
		}
	}
	return nil
}

func TestAConfiguredVPNIsOfferedAndChoosingItIsSaved(t *testing.T) {
	s := store(t, "")
	w := vpnWindow(t, s, "", "invented")

	c := tick(w, "invented")
	require.NotNil(t, c, "a connection that is down is still offered")
	require.False(t, c.Checked, "none until asked")

	test.Tap(c)

	got := config.Bandwidth.Get(s.Config()).VPNs
	require.Len(t, got, 1)
	assert.Equal(t, "invented", got[0].Name)
}

// A setting nobody can see is worse than a list with a stale name on it.
func TestAChosenVPNNoSystemHasStaysListed(t *testing.T) {
	s := store(t, "")
	require.NoError(t, s.SetConfig(config.Bandwidth.Set(s.Config(), config.BandwidthSettings{
		VPNs: []config.VPNItem{{Name: "gone"}},
	})))

	w := vpnWindow(t, s, "", "invented")

	c := tick(w, "gone (not found on this machine)")
	require.NotNil(t, c)
	assert.True(t, c.Checked)
}

func TestChoosingKeepsOrderAndUnchoosingForgets(t *testing.T) {
	b := prefs.ChooseVPN(config.BandwidthSettings{}, "b", true)
	b = prefs.ChooseVPN(b, "a", true)
	b = prefs.ChooseVPN(b, "a", true)
	assert.Equal(t, []string{"b", "a"}, []string{b.VPNs[0].Name, b.VPNs[1].Name}, "the order chosen, once each")

	assert.Equal(t, []string{"b", "a", "c"}, prefs.OrderVPNs([]string{"a", "c"}, b.VPNs),
		"chosen first in the card's order, the rest after")

	b.VPNs[0].Label = "kept nowhere"
	b = prefs.ChooseVPN(b, "b", false)
	require.Len(t, b.VPNs, 1)
	assert.Equal(t, "a", b.VPNs[0].Name)
}

// Interfaces and VPNs share a setting. A change to one must not lose the other.
func TestChangingAnInterfaceKeepsTheVPNs(t *testing.T) {
	s := store(t, "")
	require.NoError(t, s.SetConfig(config.Bandwidth.Set(s.Config(), config.BandwidthSettings{
		Interfaces: []string{"lo"}, VPNs: []config.VPNItem{{Name: "invented", Label: "Kept"}},
		VPNCheckSeconds: 60,
	})))
	w := vpnWindow(t, s, "", "invented")

	if c := tick(w, "lo"); c != nil {
		test.Tap(c)
	}

	got := config.Bandwidth.Get(s.Config())
	require.Len(t, got.VPNs, 1)
	assert.Equal(t, "Kept", got.VPNs[0].Label)
	assert.Equal(t, 60, got.VPNCheckSeconds)
}

func TestACheckCanBeAddedAndRemoved(t *testing.T) {
	s := store(t, "")
	require.NoError(t, s.SetConfig(config.Bandwidth.Set(s.Config(), config.BandwidthSettings{
		VPNs: []config.VPNItem{{Name: "invented"}},
	})))
	w := vpnWindow(t, s, "", "invented")

	add := button(w, "Add a check")
	require.NotNil(t, add)
	test.Tap(add)

	got := config.Bandwidth.Get(s.Config()).VPNs[0].Checks
	require.Len(t, got, 1)
	assert.Equal(t, "ping", got[0].Type)

	remove := button(w, "Remove")
	require.NotNil(t, remove, "the new check's row is drawn")
	test.Tap(remove)
	assert.Empty(t, config.Bandwidth.Get(s.Config()).VPNs[0].Checks)
}

// vpnToggle writes a vpn-toggle settings file with invented names.
func vpnToggle(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.json")
	require.NoError(t, os.WriteFile(path, []byte(`{
	  "monitor": {"check_interval_seconds": 30, "grace_period_seconds": 20},
	  "vpns": [
	    {"name": "office", "asserts": [{"type": "ping", "host": "10.0.0.1"}]},
	    {"name": "exit", "asserts": [{"type": "geolocation", "field": "city"}]}
	  ]}`), 0o600))
	return path
}

func TestTheImportIsOfferedOnce(t *testing.T) {
	s := store(t, "")
	path := vpnToggle(t)
	before, err := os.ReadFile(path) //nolint:gosec // the test's own temporary file
	require.NoError(t, err)

	w := vpnWindow(t, s, path, "office")
	yes := button(w, "Import 2 VPNs")
	require.NotNil(t, yes)
	test.Tap(yes)

	got := config.Bandwidth.Get(s.Config())
	require.Len(t, got.VPNs, 2)
	assert.Equal(t, 30, got.VPNCheckSeconds)
	assert.Equal(t, 20, got.VPNGraceSeconds)
	assert.True(t, got.VPNImportOffered)
	assert.Equal(t, "ping", got.VPNs[0].Checks[0].Type)
	assert.Empty(t, got.VPNs[1].Checks, "the geolocation check did not come")

	after, err := os.ReadFile(path) //nolint:gosec // the test's own temporary file
	require.NoError(t, err)
	assert.Equal(t, before, after, "vpn-toggle's file is only read")

	again := vpnWindow(t, s, path, "office")
	assert.Nil(t, button(again, "Import 2 VPNs"), "answered is answered")
}

func TestTheImportCanBeDeclinedForGood(t *testing.T) {
	s := store(t, "")
	path := vpnToggle(t)
	w := vpnWindow(t, s, path)

	no := button(w, "Not now")
	require.NotNil(t, no)
	test.Tap(no)

	got := config.Bandwidth.Get(s.Config())
	assert.Empty(t, got.VPNs)
	assert.True(t, got.VPNImportOffered)
	assert.Nil(t, button(vpnWindow(t, s, path), "Not now"))
}

// Somebody who chose VPNs by hand has answered another way.
func TestNoImportIsOfferedOnceAVPNIsChosen(t *testing.T) {
	s := store(t, "")
	require.NoError(t, s.SetConfig(config.Bandwidth.Set(s.Config(), config.BandwidthSettings{
		VPNs: []config.VPNItem{{Name: "chosen"}},
	})))

	w := vpnWindow(t, s, vpnToggle(t))

	assert.Nil(t, button(w, "Not now"))
}

func TestWhatAnImportLeftOutIsSaid(t *testing.T) {
	text := prefs.ImportOutcome(config.VPNImport{
		VPNs: []config.VPNItem{{Name: "a"}}, LeftOut: []string{"exit: geolocation check"},
	})
	assert.Contains(t, text, "exit: geolocation check")
	assert.Contains(t, text, "third party")
}

// A system that will not say is left out; the rest are offered, each name once.
func TestASystemThatWillNotListIsLeftOut(t *testing.T) {
	got := prefs.ListVPNs(context.Background(), []core.VPNBackend{
		{Configured: func(context.Context) ([]string, error) { return nil, errors.New("refused") }},
		{Configured: func(context.Context) ([]string, error) { return []string{"b", "a"}, nil }},
		{Configured: func(context.Context) ([]string, error) { return []string{"a"}, nil }},
		{},
	})
	assert.True(t, slices.Equal([]string{"a", "b"}, got), "%v", got)
}

// A platform with no VPN systems offers none of this (spec 085 R3).
func TestAPlatformWithNoSystemsHasNoVPNGroup(t *testing.T) {
	a := test.NewApp()
	t.Cleanup(a.Quit)
	w := prefs.New(a, prefs.Options{
		Store: store(t, ""), Version: "1.2.3",
		VPNSystems: func() []core.VPNBackend { return nil }, VPNToggleConfig: vpnToggle(t),
	})
	w.ShowPage("sections")

	assert.Nil(t, button(w, "Import 2 VPNs"))
}
