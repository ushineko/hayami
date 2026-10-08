/*
Package structure holds the tests that are about the shape of the program
rather than about what it computes.

Two shells drawing the same sections drift the moment one of them learns
something the other does not, and the drift is invisible until a user asks why
the terminal is missing a reading. What each shell draws is compared in
internal/gui (TestTheWindowAndTheTerminalDrawTheSameFacts, spec 047); these
tests hold the structure that makes the comparison mean something: one
registry, one kind of source, and a terminal that draws every one of them.
*/
package structure_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/hayami/internal/config"
	"github.com/ushineko/hayami/internal/core"
	"github.com/ushineko/hayami/internal/panel"
	"github.com/ushineko/hayami/internal/testenv"
	"github.com/ushineko/hayami/internal/tui"
	"github.com/ushineko/hayami/internal/view"
)

// Both shells hold the same sources, built by the same function from the same
// settings: neither shell constructs a section of its own, so neither can have
// one the other lacks. Whether they then draw the same thing is spec 047's test.
//
// Every section in the registry (spec 038) builds a source under its own key,
// and the section that source draws carries the registry's title and icon:
// the builder took them from the one list rather than spelling them again.
// That every icon has a glyph in the window is internal/gui's
// TestEverySectionHasAnIcon.
func TestEveryRegistryEntryBuildsItsSource(t *testing.T) {
	noMachine(t)
	specs := panel.Specs()
	require.NotEmpty(t, specs)

	env := panel.Env{Settings: config.Bandwidth.Set(config.Config{}, config.BandwidthSettings{Interfaces: []string{"eth0"}}), Counters: fakeCounters}
	for _, spec := range specs {
		t.Run(spec.Key, func(t *testing.T) {
			src := spec.New(env)
			require.NotNil(t, src)
			assert.Equal(t, spec.Key, src.Key(), "a section was not built under its own key")
			sec := src.Section()
			assert.Equal(t, spec.Title, sec.Title)
			assert.Equal(t, spec.Icon, sec.Icon)
		})
	}

}

// The terminal panel renders whatever it is given, and what it is given is
// panel.Source. If this ever needs a type switch on the source, parity is
// already lost.
func TestTheTerminalPanelDrawsEverySection(t *testing.T) {
	noMachine(t)
	sources := panel.Sources(panel.Keys(), panel.Env{Settings: config.Bandwidth.Set(config.Config{}, config.BandwidthSettings{Interfaces: []string{"eth0"}}), Counters: fakeCounters})
	m := tui.New(tui.Options{Sources: sources, Arrangement: view.ArrangeStack})

	for _, s := range sources {
		_, err := s.Poll(t.Context())
		require.NoError(t, err)
	}
	m2, _ := m.Update(polledAll(sources))

	assert.Len(t, m2.(tui.Model).Sections(), len(sources))
}

// noMachine takes away what the real sources would read, so polling them
// opens no device and fetches no usage: an empty hidraw tree, a system bus
// that does not resolve, and no credential store or cache.
func noMachine(t *testing.T) {
	t.Helper()
	testenv.Home(t, t.TempDir())
	testenv.Cache(t, t.TempDir())
	t.Setenv("PATH", t.TempDir())
	t.Setenv("DBUS_SYSTEM_BUS_ADDRESS", "unix:path=/nonexistent/hayami-test")
	testenv.NoDevices(t)
}

// polledAll is every source reporting that it has something to say. The
// message type is unexported, so the model is driven the way the program
// drives it: through Update with a real message.
func polledAll(sources []panel.Source) any { return tui.Drawn(sources) }

func fakeCounters() (map[string]core.Counters, error) {
	return map[string]core.Counters{"eth0": {Rx: 1000, Tx: 500}}, nil
}
