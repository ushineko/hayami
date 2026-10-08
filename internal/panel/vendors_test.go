package panel_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/ushineko/hayami/internal/panel"
)

/*
Spec 048. The vendors the peripherals section asks, and what it says for each
when nothing is found, come from sanshoku's drivers describing themselves:
the battery drivers that read on the host's platform, grouped by name, in the
module's order. Pinned per platform, because these lines are what doctor and
an empty card say, and they must be the ones hayami said from its own table
(spec 035) -- including the Bluetooth line on Linux only.

The order is sanshoku's: on Linux the Bluetooth line now comes before AULA's,
where hayami's table put AULA last but one. Everything else is as it was.
*/
func TestTheVendorListComesFromTheDrivers(t *testing.T) {
	hid := []panel.Vendor{
		{Name: "Logitech", Absent: "no Logitech receiver", Drivers: []string{"logitech"}},
		{Name: "Razer", Absent: "no Razer device", Quiet: true, Drivers: []string{"razer"}},
		{Name: "SteelSeries", Absent: "no SteelSeries device", Quiet: true, Drivers: []string{"steelseries"}},
	}
	aula := panel.Vendor{Name: "AULA", Absent: "no AULA receiver", Quiet: true, Drivers: []string{"aula"}}
	bluetooth := panel.Vendor{Name: "Bluetooth", Absent: "no Bluetooth device with a battery", Drivers: []string{"bluez", "apple"}}

	assert.Equal(t, append(append([]panel.Vendor{}, hid...), aula), panel.Vendors("windows"),
		"Windows: the HID vendors and no Bluetooth")
	assert.Equal(t, append(append([]panel.Vendor{}, hid...), bluetooth, aula), panel.Vendors("linux"),
		"Linux: the HID vendors and Bluetooth")
	assert.Empty(t, panel.Vendors("other"), "a system no driver reads on asks none")
}
