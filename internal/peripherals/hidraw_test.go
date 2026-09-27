package peripherals

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The three report descriptors a Logitech receiver presents, as read from the
// machine this was written on. Only the third speaks HID++; the other two are
// the mouse and the keyboard interfaces and never answer a request.
var (
	descriptorMouse    = []byte{0x05, 0x01, 0x09, 0x02, 0xa1, 0x01, 0x09, 0x01, 0xa1, 0x00, 0x95, 0x10}
	descriptorKeyboard = []byte{0x05, 0x01, 0x09, 0x06, 0xa1, 0x01, 0x85, 0x01, 0x05, 0x07, 0x19, 0xe0}
	descriptorHIDPP    = []byte{0x06, 0x00, 0xff, 0x09, 0x01, 0xa1, 0x01, 0x85, 0x10, 0x95, 0x06, 0x75}
)

// node writes one hidraw node into a fake /sys/class/hidraw.
func node(t *testing.T, root, name, vendor string, descriptor []byte) {
	t.Helper()
	dir := filepath.Join(root, name, "device")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	uevent := "DRIVER=hid-generic\nHID_ID=0003:0000" + vendor + ":0000C547\nHID_NAME=A Device\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "uevent"), []byte(uevent), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "report_descriptor"), descriptor, 0o600))
}

// withTree points the package at a hidraw tree the test wrote.
func withTree(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	sys, dev := SysHidraw, DevDir
	SysHidraw, DevDir = root, "/dev"
	t.Cleanup(func() { SysHidraw, DevDir = sys, dev })
	return root
}

// AC2. The HID++ node is the one whose descriptor says so, and the mouse and
// keyboard interfaces beside it are not.
func TestTheHidppNodeIsChosenByItsDescriptorAndNotByItsNumber(t *testing.T) {
	root := withTree(t)
	// Deliberately numbered so that the HID++ node is neither first nor last:
	// a selection that happened to work by position would pass on one
	// ordering and fail on the next reboot.
	node(t, root, "hidraw10", "046D", descriptorMouse)
	node(t, root, "hidraw12", "046D", descriptorHIDPP)
	node(t, root, "hidraw11", "046D", descriptorKeyboard)

	found, err := hidppNodes()
	require.NoError(t, err)
	assert.Equal(t, []string{"/dev/hidraw12"}, found)
}

// AC2. A machine with no Logitech hardware has no Logitech node, and that is
// not an error.
func TestAMachineWithNoLogitechHardwareHasNoHidppNode(t *testing.T) {
	root := withTree(t)
	node(t, root, "hidraw0", "1038", descriptorHIDPP) // a SteelSeries device
	node(t, root, "hidraw1", "046D", descriptorMouse) // a Logitech mouse, no HID++

	found, err := hidppNodes()
	require.NoError(t, err)
	assert.Empty(t, found)
}

// AC2. A machine with no hidraw tree at all — a container, a kernel without
// the driver — is a machine with no peripherals, not a machine with a fault.
func TestNoHidrawTreeAtAllIsNotAnError(t *testing.T) {
	sys := SysHidraw
	SysHidraw = filepath.Join(t.TempDir(), "absent")
	t.Cleanup(func() { SysHidraw = sys })

	found, err := hidppNodes()
	require.NoError(t, err)
	assert.Empty(t, found)
}

// AC2. The descriptor is walked as items, not searched as bytes.
//
// `06 00 ff` here is the *data* of a long item, not a usage page. A byte
// search would find it and write HID++ requests into a device that has never
// heard of them.
func TestAVendorPageInAnotherItemsDataIsNotAHidppNode(t *testing.T) {
	descriptor := []byte{
		0x05, 0x01, // usage page: generic desktop
		0x0b, 0x06, 0x00, 0xff, 0x00, // a four-byte item whose data contains 06 00 ff
		0x85, 0x10, // report ID 0x10, but under the desktop page
	}
	assert.False(t, speaksHIDPP(descriptor))
}

// AC2. A truncated descriptor is refused rather than walked off the end.
func TestATruncatedDescriptorIsNotAHidppNode(t *testing.T) {
	assert.False(t, speaksHIDPP([]byte{0x06, 0x00}))
}

// AC2. The vendor is matched as a field. A product ID containing 046D is not a
// Logitech device.
func TestTheVendorIsMatchedAsAFieldAndNotAsASubstring(t *testing.T) {
	root := withTree(t)
	dir := filepath.Join(root, "hidraw0", "device")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "uevent"),
		[]byte("HID_ID=0003:00001038:0000046D\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "report_descriptor"), descriptorHIDPP, 0o600))

	found, err := hidppNodes()
	require.NoError(t, err)
	assert.Empty(t, found)
}
