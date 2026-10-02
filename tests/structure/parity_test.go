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
	"github.com/ushineko/sanshoku/hidraw"

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
func TestFeatureParity(t *testing.T) {
	keys := panel.Keys()
	require.NotEmpty(t, keys)

	sources := panel.Sources(keys, []string{"eth0"}, fakeCounters)

	drawn := make([]string, 0, len(sources))
	for _, s := range sources {
		drawn = append(drawn, s.Key())
	}
	assert.Equal(t, keys, drawn,
		"a section this build knows was not built from its own key")
	assert.Empty(t, allowList,
		"a section is drawn by one shell and not the other; say why here or fix it")
}

// The terminal panel renders whatever it is given, and what it is given is
// panel.Source. If this ever needs a type switch on the source, parity is
// already lost.
func TestTheTerminalPanelDrawsEverySection(t *testing.T) {
	noMachine(t)
	sources := panel.Sources(panel.Keys(), []string{"eth0"}, fakeCounters)
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
	sys, dev := hidraw.SysRoot, hidraw.DevRoot
	hidraw.SysRoot, hidraw.DevRoot = t.TempDir(), t.TempDir()
	t.Cleanup(func() { hidraw.SysRoot, hidraw.DevRoot = sys, dev })
}

// polledAll is every source reporting that it has something to say. The
// message type is unexported, so the model is driven the way the program
// drives it: through Update with a real message.
func polledAll(sources []panel.Source) any { return tui.Drawn(sources) }

func fakeCounters() (map[string]core.Counters, error) {
	return map[string]core.Counters{"eth0": {Rx: 1000, Tx: 500}}, nil
}
