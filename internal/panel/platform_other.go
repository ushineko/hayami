//go:build !windows

package panel

import (
	"github.com/ushineko/sanshoku"
	"github.com/ushineko/sanshoku/apple"
	"github.com/ushineko/sanshoku/bluez"
)

// permissionDetail is what a device that may not be opened is told to do. The
// rule used to arrive with liquidctl's and OpenRazer's packages; reading the
// devices directly, nothing installs it but hayami's own installer.
const permissionDetail = "install the udev rule (60-sanshoku.rules) and replug; see the README"

// permitted is whether an Open failed for want of permission: a hidraw node
// the logged-in user may not open, which is what the udev rule grants.
func permitted(err error) bool { return sanshoku.IsPermission(err) }

// bluetooth is the Bluetooth vendor: one line for two drivers, because to a
// reader "no Bluetooth device with a battery" is one fact whichever protocol
// would have read it.
func bluetooth() []vendor {
	return []vendor{{
		name: "Bluetooth", absent: "no Bluetooth device with a battery",
		drivers: []sanshoku.Driver{apple.Driver{}, bluez.Driver{}},
	}}
}
