package panel_test

import (
	"io/fs"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/ushineko/sanshoku/battery"
	"golang.org/x/sys/windows"

	"github.com/ushineko/hayami/internal/panel"
	"github.com/ushineko/hayami/internal/view"
)

// bluetoothAbsent is nothing: Windows has no Bluetooth vendor (spec 035).
var bluetoothAbsent []string

// permissionWords are what a device that may not be opened is told to do here.
var permissionWords = []string{"another program"}

/*
R1. On Windows the Bluetooth drivers are not asked at all.

They read BlueZ and an L2CAP socket, neither of which Windows has, and asked
anyway they put a line about a Linux service into every doctor run. A desk
whose BlueZ and Apple drivers would fail loudly is a desk that says nothing
about them.
*/
func TestWindowsAsksNoBluetoothDriver(t *testing.T) {
	boom := fs.ErrInvalid
	p := (&desk{failing: map[string]error{"bluez": boom, "apple": boom}}).section(nil)

	_, err := p.Poll(t.Context())

	require.NoError(t, err, "a driver that was asked would have failed")
	for _, text := range reasonTexts(p.Section()) {
		assert.NotContains(t, text, "Bluetooth")
	}
}

/*
R2. A device Windows will not open is a device that may not be opened, named
as such, rather than one that "would not answer".

sanshoku.IsPermission asks for Go's EACCES and EPERM, which are not what
Windows returns: ERROR_ACCESS_DENIED is. The detail says what is worth looking
for here, which is not a udev rule.
*/
func TestADeviceWindowsWillNotOpenIsNotPermitted(t *testing.T) {
	mouse := &peripheral{driver: "razer", name: "Razer Basilisk Ultimate Dongle", path: `\\?\HID#VID_1532&PID_0088&MI_00`,
		openErr: &fs.PathError{Op: "open", Path: "hid", Err: windows.ERROR_ACCESS_DENIED}}
	kb := &peripheral{driver: "aula", name: "AULA F75", path: `\\?\HID#VID_3554&PID_FA09&MI_01`,
		says: []battery.Battery{{Name: "F75", Level: 100, HasLevel: true, Kind: battery.KindKeyboard}}}
	p := (&desk{devices: []*peripheral{kb, mouse}}).section(nil)

	drawn, err := p.Poll(t.Context())

	require.NoError(t, err, "nothing will change until the other program lets go; a reason, not a log line")
	require.True(t, drawn)
	r := find(t, p.Section(), "Razer Basilisk Ultimate Dongle is not permitted")
	assert.Equal(t, view.Warn, r.Status)
	assert.Equal(t, panel.PermissionDetail, r.Detail)
	assert.NotContains(t, r.Detail, "udev")
}
