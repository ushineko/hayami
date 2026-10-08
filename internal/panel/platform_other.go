//go:build !windows

package panel

import (
	"github.com/ushineko/sanshoku"
	"github.com/ushineko/sanshoku/apple"
	"github.com/ushineko/sanshoku/bluez"
)

// bluetooth is the Bluetooth vendor: one line for two drivers, because to a
// reader "no Bluetooth device with a battery" is one fact whichever protocol
// would have read it.
func bluetooth() []vendor {
	return []vendor{{
		name: "Bluetooth", absent: "no Bluetooth device with a battery",
		drivers: []sanshoku.Driver{apple.Driver{}, bluez.Driver{}},
	}}
}
