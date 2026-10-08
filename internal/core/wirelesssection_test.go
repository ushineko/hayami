package core_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/hayami/internal/core"
)

// radio is a Wi-Fi reader that counts its calls and says what it is given.
type radio struct {
	calls int
	say   map[string]core.Wireless
	err   error
}

func (r *radio) read(context.Context) (map[string]core.Wireless, error) {
	r.calls++
	return r.say, r.err
}

// A desk of wired interfaces asks the WLAN once, on the first poll, and never
// again: the glance rule's "a reading nobody shows costs nothing" (spec 037).
func TestAWiredDeskAsksAboutWiFiOnce(t *testing.T) {
	src := &counting{present: map[string]bool{"eno2": true}}
	b := section([]string{"eno2"}, src)
	r := &radio{say: map[string]core.Wireless{"wlan0": {Connected: true}}}
	b.SetWirelessReader(r.read)

	poll(t, b, 5)

	assert.Equal(t, 1, r.calls, "a desk with no watched radio kept asking")
	_, isRadio := b.Wireless("eno2")
	assert.False(t, isRadio)
}

// A watched radio is asked about on every poll, and its description reaches
// the readings and the JSON.
func TestAWatchedRadioIsDescribedEveryPoll(t *testing.T) {
	src := &counting{present: map[string]bool{"wlan0": true, "eno2": true}}
	b := section([]string{"wlan0", "eno2"}, src)
	r := &radio{say: map[string]core.Wireless{"wlan0": {Connected: true, RSSI: -50, HasRSSI: true}}}
	b.SetWirelessReader(r.read)

	poll(t, b, 3)

	assert.Equal(t, 3, r.calls)
	w, isRadio := b.Wireless("wlan0")
	require.True(t, isRadio)
	assert.Equal(t, -50, w.RSSI)
	for _, rd := range b.Readings() {
		if rd.Name == "wlan0" {
			require.NotNil(t, rd.Wireless)
			assert.Equal(t, -50, rd.Wireless.RSSI)
		} else {
			assert.Nil(t, rd.Wireless, "a wired interface carried a link")
		}
	}
}

// An interface that was Wi-Fi and is missing from a later answer is still a
// radio -- its row keeps the bars' place -- with nothing to say.
func TestARadioThatGoesQuietIsStillARadio(t *testing.T) {
	src := &counting{present: map[string]bool{"wlan0": true}}
	b := section([]string{"wlan0"}, src)
	r := &radio{say: map[string]core.Wireless{"wlan0": {Connected: true}}}
	b.SetWirelessReader(r.read)
	poll(t, b, 1)

	r.say = nil
	poll(t, b, 1)

	w, isRadio := b.Wireless("wlan0")
	assert.True(t, isRadio)
	assert.False(t, w.Connected)
}

// Watching a new set of names asks again, once: one of them may be a radio.
func TestChoosingInterfacesAsksAgain(t *testing.T) {
	src := &counting{present: map[string]bool{"eno2": true, "wlan0": true}}
	b := section([]string{"eno2"}, src)
	r := &radio{say: map[string]core.Wireless{"wlan0": {Connected: true}}}
	b.SetWirelessReader(r.read)
	poll(t, b, 2)
	require.Equal(t, 1, r.calls)

	b.SetInterfaces([]string{"eno2", "wlan0"})
	poll(t, b, 2)

	assert.Equal(t, 3, r.calls, "the new radio was not asked about on every poll")
}

// Windows' refusal is kept for the section's reason, beside what was read.
func TestALocationRefusalIsKept(t *testing.T) {
	src := &counting{present: map[string]bool{"wlan0": true}}
	b := section([]string{"wlan0"}, src)
	r := &radio{say: map[string]core.Wireless{"wlan0": {Connected: true}}, err: core.ErrWirelessDenied}
	b.SetWirelessReader(r.read)
	poll(t, b, 1)

	assert.ErrorIs(t, b.WirelessErr(), core.ErrWirelessDenied)
	_, isRadio := b.Wireless("wlan0")
	assert.True(t, isRadio)
}
