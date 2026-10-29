package panel_test

import (
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/ushineko/sanshoku/battery"
)

// The kernel's battery driver is a fallback that reads on Linux only.
func onLinux(t *testing.T) {
	t.Helper()
	if runtime.GOOS != "linux" {
		t.Skip("sanshoku's powersupply driver reads on Linux only")
	}
}

// kernelPad is a gamepad as sanshoku's powersupply driver lists it.
func kernelPad(name, key string, b battery.Battery) *peripheral {
	return &peripheral{driver: "powersupply", name: name, path: "/sys/devices/fake/" + name, key: key,
		says: []battery.Battery{b}}
}

// A DualSense on its cable is "DualSense Wireless Controller" to the sony
// driver and "Sony Interactive Entertainment DualSense Wireless Controller"
// to the kernel. It is one cell, the sony driver's, and the kernel's
// candidate is never opened, because this section scans one driver at a time
// and sanshoku.Scan's own drop never sees the two together.
func TestADeviceTheKernelAlsoReadsIsOneCell(t *testing.T) {
	onLinux(t)
	pad := battery.Battery{Name: "DualSense Wireless Controller", Level: 75, HasLevel: true, Kind: battery.KindGamepad}
	kernel := kernelPad("Sony Interactive Entertainment DualSense Wireless Controller", "usb:1-4",
		battery.Battery{Name: "Sony Interactive Entertainment DualSense Wireless Controller", Level: 75, HasLevel: true, Kind: battery.KindGamepad})
	k := &desk{devices: []*peripheral{
		kernel,
		{driver: "sony", name: "DualSense Wireless Controller", path: "/dev/hidraw17", key: "usb:1-4",
			says: []battery.Battery{pad}},
	}}
	p := k.section(nil)

	poll(t, p)
	poll(t, p)

	found := devices(p)
	require.Len(t, found, 1)
	assert.Equal(t, "DualSense Wireless Controller", found[0].Name)
	assert.Zero(t, kernel.opens, "the kernel's candidate is left out before it is opened")
}

// A device only the kernel reads, the 8BitDo in its Switch pose, is a cell,
// drawn as the band it reads.
func TestADeviceOnlyTheKernelReadsIsACell(t *testing.T) {
	onLinux(t)
	k := &desk{devices: []*peripheral{
		kernelPad("Pro Controller", "bt:aa:bb:cc:dd:ee:ff",
			battery.Battery{Name: "Pro Controller", Band: battery.BandGood, HasBand: true, Kind: battery.KindGamepad}),
		{driver: "sony", name: "DualSense Wireless Controller", path: "/dev/hidraw17", key: "bt:aa:bb:cc:dd:ee:01",
			says: []battery.Battery{{Name: "DualSense Wireless Controller", Level: 75, HasLevel: true, Kind: battery.KindGamepad}}},
	}}
	p := k.section(nil)

	poll(t, p)
	poll(t, p)

	found := devices(p)
	require.Len(t, found, 2)
	names := []string{found[0].Name, found[1].Name}
	assert.ElementsMatch(t, []string{"DualSense Wireless Controller", "Pro Controller"}, names)
	for _, d := range found {
		if d.Name == "Pro Controller" {
			assert.Equal(t, 3, d.Segments)
		}
	}
}

// A candidate with no key never gives way: two keyless devices are two.
func TestNoKeyIsNoDrop(t *testing.T) {
	onLinux(t)
	k := &desk{devices: []*peripheral{
		kernelPad("Pro Controller", "",
			battery.Battery{Name: "Pro Controller", Band: battery.BandLow, HasBand: true, Kind: battery.KindGamepad}),
		{driver: "sony", name: "DualSense Wireless Controller", path: "/dev/hidraw17",
			says: []battery.Battery{{Name: "DualSense Wireless Controller", Level: 75, HasLevel: true, Kind: battery.KindGamepad}}},
	}}
	p := k.section(nil)

	poll(t, p)
	poll(t, p)

	assert.Len(t, devices(p), 2)
}

// The kernel's candidate held from a poll when the sony driver did not list
// the controller is closed once the sony driver lists it again.
func TestTheKernelsCandidateIsClosedWhenTheDirectDriverReturns(t *testing.T) {
	onLinux(t)
	pad := battery.Battery{Name: "DualSense Wireless Controller", Level: 75, HasLevel: true, Kind: battery.KindGamepad}
	kernel := kernelPad("DualSense Wireless Controller", "bt:aa:bb:cc:dd:ee:01", pad)
	direct := &peripheral{driver: "sony", name: "DualSense Wireless Controller", path: "/dev/hidraw17",
		key: "bt:aa:bb:cc:dd:ee:01", says: []battery.Battery{pad}}
	k := &desk{devices: []*peripheral{kernel}}
	p := k.section(nil)

	poll(t, p)
	require.Equal(t, 1, kernel.opens)

	k.devices = append(k.devices, direct)
	poll(t, p)

	assert.Equal(t, 1, kernel.closes)
	assert.Equal(t, 1, direct.opens)
}
