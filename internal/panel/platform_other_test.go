//go:build !windows

package panel_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/ushineko/sanshoku/bluez"

	"github.com/ushineko/hayami/internal/view"
)

// bluetoothAbsent is the Bluetooth vendor's line on a desk with nothing on it.
var bluetoothAbsent = []string{"no Bluetooth device with a battery"}

// permissionWords are what a device that may not be opened is told to do here.
var permissionWords = []string{"udev rule", "60-sanshoku.rules"}

// R3.7. BlueZ not answering is a machine with no Bluetooth adapter: absence,
// not failure.
func TestNoBluezIsNoBluetoothAdapter(t *testing.T) {
	// As sanshoku.Scan returns it: prefixed with the driver's name, and
	// wrapping sanshoku.ErrUnavailable, which Scan does not drop.
	noBluez := fmt.Errorf("bluez: %w", bluez.ErrNoBlueZ)
	p := (&desk{failing: map[string]error{"bluez": noBluez, "apple": noBluez}}).section(nil)

	poll(t, p)

	_, err := p.Poll(t.Context())
	require.NoError(t, err, "no adapter is not a failure to log")
	r := find(t, p.Section(), "no Bluetooth adapter")
	assert.Equal(t, view.Info, r.Status)
	assert.Len(t, p.Section().Reasons, 5, "one line for the adapter, not one per driver")
	assert.NotContains(t, reasonTexts(p.Section()), "no Bluetooth device with a battery",
		"no adapter and nothing connected are two different answers")
}

// R3.4. A Bluetooth driver that failed to list is Bluetooth that would not
// answer, in the vendor's name and not the driver's.
func TestABluetoothDriverThatFailsToListIsBluetoothThatWouldNotAnswer(t *testing.T) {
	boom := fmt.Errorf("bluez: the bus went away")
	p := (&desk{failing: map[string]error{"bluez": boom}}).section(nil)

	_, err := p.Poll(t.Context())

	require.ErrorIs(t, err, boom)
	assert.Equal(t, view.Warn, find(t, p.Section(), "a Bluetooth device would not answer").Status)
}
