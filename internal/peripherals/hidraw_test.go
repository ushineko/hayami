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

/*
The vendor descriptors spec 016 reads, copied off the devices themselves.

apexControl is the Apex Pro TKL Wireless Gen 3's control endpoint: usage page
0xFFC0 with a 64-byte input and output and a big feature report for the OLED.
apexInput is the interface beside it, which declares only 0xFFC1 and has no
output at all -- it is the discriminator, because a reader that matched "any
vendor page" would pick the one that cannot be written to.

razerDock is the Mouse Dock Pro's only node. Its vendor collection sits behind
a mouse, a keyboard and a consumer one, which is why a byte search for the page
would not do and the descriptor is walked as items.
*/
var (
	apexControl = []byte{
		0x06, 0xc0, 0xff, 0x09, 0x01, 0xa1, 0x01, 0x06, 0xc1, 0xff, 0x15, 0x00,
		0x26, 0xff, 0x00, 0x75, 0x08, 0x09, 0xf0, 0x95, 0x40, 0x81, 0x02, 0x09,
		0xf1, 0x95, 0x40, 0x91, 0x02, 0x09, 0xf2, 0x96, 0x81, 0x02, 0xb1, 0x02, 0xc0,
	}
	apexInput = []byte{
		0x06, 0xc1, 0xff, 0x09, 0x01, 0xa1, 0x01, 0x09, 0xf0, 0x15, 0x00, 0x26,
		0xff, 0x00, 0x75, 0x08, 0x95, 0x40, 0x81, 0x02, 0xc0,
	}
	razerDock = []byte{
		0x05, 0x01, 0x09, 0x02, 0xa1, 0x01, 0x09, 0x01, 0xa1, 0x00, 0xc0, 0xc0,
		0x05, 0x0c, 0x09, 0x01, 0xa1, 0x01, 0xc0,
		0x06, 0x00, 0xff, 0x09, 0x02, 0xa1, 0x01, 0x85, 0x00, 0xc0,
	}
)

// node writes one hidraw node into a fake /sys/class/hidraw.
func node(t *testing.T, root, name, vendor string, descriptor []byte) {
	t.Helper()
	namedNode(t, root, name, vendor, "C547", "A Device", descriptor)
}

// namedNode writes one hidraw node with a product ID and a name of its own,
// for a test about either.
func namedNode(t *testing.T, root, name, vendor, product, hid string, descriptor []byte) {
	t.Helper()
	physNode(t, root, name, vendor, product, hid, "usb-0000:00:14.0-1/input0", descriptor)
}

// physNode writes one hidraw node with a HID_PHYS of its own, for a test about
// which USB device an interface belongs to.
func physNode(t *testing.T, root, name, vendor, product, hid, phys string, descriptor []byte) {
	t.Helper()
	dir := filepath.Join(root, name, "device")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	uevent := "DRIVER=hid-generic\nHID_ID=0003:0000" + vendor + ":0000" + product +
		"\nHID_NAME=" + hid + "\nHID_PHYS=" + phys + "\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "uevent"), []byte(uevent), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "report_descriptor"), descriptor, 0o600))
}

/*
AC2. The same reader finds the keyboard under both of its product IDs.

The Apex enumerates as 0x1644 with its keyboard on 2.4 GHz and 0x1646 with the
same keyboard on its cable, on the same USB port, with the hidraw numbers
landing on the same indices both times. A reader keyed to the product reads
whichever one it was told about; that invalidated a round of measurements
during this spec's investigation before anyone noticed the number had moved.
*/
func TestADeviceIsFoundUnderEitherOfItsProductIDs(t *testing.T) {
	for _, product := range []string{"1644", "1646"} {
		root := withTree(t)
		namedNode(t, root, "hidraw5", "1038", product, "SteelSeries Apex Pro TKL", apexControl)

		found, err := nodes(steelseriesVendor, usagePage(SteelSeriesPage))

		require.NoError(t, err)
		require.Len(t, found, 1, "product %s was not found", product)
		assert.Equal(t, "/dev/hidraw5", found[0].Path)
		assert.Equal(t, "SteelSeries Apex Pro TKL", found[0].Name)
	}
}

// AC2. The control endpoint is the one declaring the page asked for, and the
// interface beside it declaring a different vendor page is not.
func TestTheControlEndpointIsChosenByItsUsagePage(t *testing.T) {
	root := withTree(t)
	namedNode(t, root, "hidraw5", "1038", "1644", "Apex", apexControl)
	namedNode(t, root, "hidraw6", "1038", "1644", "Apex", apexInput)

	found, err := nodes(steelseriesVendor, usagePage(SteelSeriesPage))

	require.NoError(t, err)
	require.Len(t, found, 1)
	assert.Equal(t, "/dev/hidraw5", found[0].Path)
}

// AC2. A vendor collection behind three standard ones is still found, which a
// reader that gave up at the first usage page would miss.
func TestARazerVendorCollectionIsFoundBehindTheStandardOnes(t *testing.T) {
	root := withTree(t)
	namedNode(t, root, "hidraw0", "1532", "00A4", "Razer Razer Mouse Dock Pro", razerDock)

	found, err := nodes(razerVendor, speaksRazer)

	require.NoError(t, err)
	require.Len(t, found, 1)
	assert.Equal(t, "Razer Mouse Dock Pro", found[0].Name,
		"a vendor that writes its name into both descriptor strings should not be read out twice")
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
	require.Len(t, found, 1)
	assert.Equal(t, "/dev/hidraw12", found[0].Path)
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
