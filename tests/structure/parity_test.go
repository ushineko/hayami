/*
Package structure holds the tests that are about the shape of the program
rather than about what it computes.

The parity test is the one that matters. Two shells drawing the same sections
drift the moment one of them learns something the other does not, and the
drift is invisible until a user asks why the terminal is missing a reading.
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

// allowList names a section one shell draws and the other cannot, with the
// reason. It is empty, and a section added to it is a decision someone made on
// purpose rather than a difference that crept in.
var allowList = map[string]string{}

// Both shells hold the same sources, built by the same function from the same
// settings. This is the whole of parity: neither shell constructs a section of
// its own, so neither can have one the other lacks.
//
// Every section in the registry (spec 038) builds a source under its own key,
// and the section that source draws carries the registry's title and icon:
// the builder took them from the one list rather than spelling them again.
// That every icon has a glyph in the window is internal/gui's
// TestEverySectionHasAnIcon.
func TestFeatureParity(t *testing.T) {
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

	drawn := make([]string, 0, len(specs))
	for _, s := range panel.Sources(panel.Keys(), env) {
		drawn = append(drawn, s.Key())
	}
	assert.Equal(t, panel.Keys(), drawn,
		"a section this build knows was not built from its own key")
	assert.Empty(t, allowList,
		"a section is drawn by one shell and not the other; say why here or fix it")
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
