package panel_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/ushineko/sanshoku/bluez"

	"github.com/ushineko/hayami/internal/core"
	"github.com/ushineko/hayami/internal/panel"
	"github.com/ushineko/hayami/internal/view"
)

// fakeHost is a platform table with nothing of the machine's in it: its
// processor chain is the providers given, its card says nothing, and its
// accounts say which they are (spec 043).
func fakeHost(cpu ...core.Provider[float64]) *core.Host {
	return &core.Host{
		Platform:       "windows",
		CPUTemperature: core.Chain[float64]{Providers: cpu},
		CPUMissing: func(tried []string, err error) *core.Absence {
			return &core.Absence{Code: core.AbsenceCPUSensor, Detail: "tried " + joinNames(tried), Err: err}
		},
		Graphics: &core.GraphicsReader{
			Native:     func(context.Context) core.Graphics { return core.Graphics{} },
			NativeName: "the test card",
		},
		GPUMissing: func(tried []string) string { return "card: " + joinNames(tried) },
		CPULoad:    func() *core.CPULoad { return core.NewCPULoad("/nonexistent") },
		CPUName:    func() string { return "" },
		Counters:   fakeCounters,
		Wireless:   func(context.Context) (map[string]core.Wireless, error) { return nil, nil },
		Permission: func(error) (*core.Absence, bool) { return nil, false },
	}
}

func joinNames(names []string) string {
	out := ""
	for i, n := range names {
		if i > 0 {
			out += ", "
		}
		out += n
	}
	return out
}

// noSensor is a processor provider that has nothing.
func noSensor(name string) core.Provider[float64] {
	return core.ProviderFunc[float64]{N: name, F: func(context.Context) (float64, error) {
		return 0, errors.New("no such sensor")
	}}
}

/*
Spec 043. The cooler's reasons are its host's: a processor with no temperature
names every provider the host's chain tried, and a card with none every route,
from the chain's own record -- not a list the panel keeps.
*/
func TestTheCoolersReasonsNameWhatTheHostsChainsTried(t *testing.T) {
	h := fakeHost(noSensor("first sensor"), noSensor("second sensor"))
	c := panel.NewCoolerOn(h, (&desk{}).scan)

	_, _ = c.Poll(t.Context())

	cpu := find(t, c.Section(), "no sensor")
	assert.Equal(t, "tried first sensor, second sensor", cpu.Detail)
	assert.Equal(t, "card: the test card", findAside(t, c.Section(), "no GPU sensor").Detail)
}

// findAside is the section's reason with text, aside or not.
func findAside(t *testing.T, s view.Section, text string) view.Reason {
	t.Helper()
	for _, r := range s.Reasons {
		if r.Text == text {
			return r
		}
	}
	require.Failf(t, "no reason", "no reason %q in %v", text, s.Reasons)
	return view.Reason{}
}

// R3.7. BlueZ not answering is a machine with no Bluetooth adapter: absence,
// not failure -- one line for the adapter, not one per driver, and not the
// "nothing connected" line, which is a different answer.
func TestNoBluezIsNoBluetoothAdapter(t *testing.T) {
	// As sanshoku.Scan returns it: prefixed with the driver's name, and
	// wrapping sanshoku.ErrUnavailable, which Scan does not drop.
	noBluez := fmt.Errorf("bluez: %w", bluez.ErrNoBlueZ)
	h := fakeHost()
	h.Platform = "linux" // where the Bluetooth drivers say they read
	k := &desk{failing: map[string]error{"bluez": noBluez, "apple": noBluez}}
	p := panel.NewPeripheralsOn(h, k.scan, time.Now)

	_, err := p.Poll(t.Context())

	require.NoError(t, err, "no adapter is not a failure to log")
	r := find(t, p.Section(), "no Bluetooth adapter")
	assert.Equal(t, view.Info, r.Status)
	assert.Len(t, p.Section().Reasons, 6, "one line for the adapter, not one per driver")
	assert.NotContains(t, reasonTexts(p.Section()), "no Bluetooth device with a battery")
}

// Spec 043. A device the system would not open is told the host's advice, as
// the one line a reader can act on (spec 040).
func TestADeviceThatMayNotBeOpenedIsToldTheHostsAdvice(t *testing.T) {
	refused := errors.New("refused")
	h := fakeHost()
	h.Permission = func(err error) (*core.Absence, bool) {
		if !errors.Is(err, refused) {
			return nil, false
		}
		return &core.Absence{Code: core.AbsencePermission, Detail: "the host's advice", Actionable: true, Err: err}, true
	}
	dock := &peripheral{driver: "razer", name: "Razer Mouse Dock Pro", path: "/dev/hidraw4", openErr: refused}
	p := panel.NewPeripheralsOn(h, (&desk{devices: []*peripheral{dock}}).scan, time.Now)

	_, err := p.Poll(t.Context())

	require.NoError(t, err)
	r := find(t, p.Section(), "Razer Mouse Dock Pro is not permitted")
	assert.Equal(t, "the host's advice", r.Detail)
	assert.True(t, r.Actionable)
}

/*
Spec 048 (was spec 043's host field). Whether Bluetooth is asked is the
drivers' to say, by the systems they read on, asked about the host's
platform. Linux asks BlueZ and Apple's accessory protocol as one vendor;
Windows asks the bluez driver, which reads the Bluetooth stack's device
properties there since sanshoku 0.1.11 (sanshoku spec 016). Windows asked
none before that (spec 035). A system no Bluetooth driver reads on asks none
and has no Bluetooth line.
*/
func TestThePlatformSaysWhetherBluetoothIsAsked(t *testing.T) {
	noBluez := errors.New("bluez: the bus went away")
	k := &desk{failing: map[string]error{"bluez": noBluez}}

	h := fakeHost()
	h.Platform = "darwin"
	without := panel.NewPeripheralsOn(h, k.scan, time.Now)
	_, err := without.Poll(t.Context())
	require.NoError(t, err, "a host no Bluetooth driver reads on asked one")
	for _, text := range reasonTexts(without.Section()) {
		assert.NotContains(t, text, "Bluetooth")
	}

	for _, platform := range []string{"linux", "windows"} {
		h := fakeHost()
		h.Platform = platform
		with := panel.NewPeripheralsOn(h, k.scan, time.Now)
		_, err = with.Poll(t.Context())
		require.ErrorIs(t, err, noBluez, "%s: the host's Bluetooth driver was not asked", platform)
		assert.Equal(t, view.Warn, find(t, with.Section(), "a Bluetooth device would not answer").Status, platform)
	}
}
