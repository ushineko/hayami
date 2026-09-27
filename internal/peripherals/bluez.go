package peripherals

import (
	"errors"
	"fmt"
	"strings"

	"github.com/godbus/dbus/v5"
)

/*
BlueZ, over its own D-Bus interface.

The program this succeeds runs `upower -i` and parses its output. BlueZ carries
the same numbers with the device's name, its address and whether it is
connected beside them, and it is an interface rather than a human-readable
report, so it is asked directly.

Two things come from here. Which devices are worth talking to at all — that is
how the AirPods are found — and, for everything that reports one, the battery
BlueZ already has.
*/

// appleVendor is Apple's Bluetooth company identifier, as BlueZ writes it into
// a device's Modalias: "bluetooth:vNNNNpNNNNdNNNN".
const appleVendor = "v004C"

// audioUUIDs are the profiles that make a device a pair of headphones rather
// than a phone that happens to be paired.
var audioUUIDs = map[string]bool{
	"0000110b-0000-1000-8000-00805f9b34fb": true, // Audio Sink
	"0000110d-0000-1000-8000-00805f9b34fb": true, // Advanced Audio Distribution
}

// ErrNoBluez is BlueZ not being there to ask: no daemon, no system bus, a
// machine with no Bluetooth at all. Not a problem, and not a section that
// fails.
var ErrNoBluez = errors.New("bluez is not answering")

// BluetoothDevice is what BlueZ knows about one device.
type BluetoothDevice struct {
	// Name is what the device is called, preferring the alias its owner gave
	// it over the name its manufacturer did.
	Name string

	// Address is the printed Bluetooth address.
	Address string

	// Level is the battery BlueZ has, where it has one. HasLevel is false for
	// a device with no org.bluez.Battery1 — which is most of them, and is the
	// whole reason the accessory protocol exists in this package.
	Level    int
	HasLevel bool

	// Apple and Audio say whether this is worth trying the accessory protocol
	// on.
	Apple bool
	Audio bool
}

// bluetoothDevices lists every connected device BlueZ knows about.
//
// A device that is not connected is not listed: it is paired and elsewhere,
// which is not a device this panel has anything to say about.
func bluetoothDevices() ([]BluetoothDevice, error) {
	conn, err := dbus.SystemBus()
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrNoBluez, err)
	}

	var objects map[dbus.ObjectPath]map[string]map[string]dbus.Variant
	err = conn.Object("org.bluez", "/").
		Call("org.freedesktop.DBus.ObjectManager.GetManagedObjects", 0).
		Store(&objects)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrNoBluez, err)
	}

	var found []BluetoothDevice
	for _, interfaces := range objects {
		device, ok := interfaces["org.bluez.Device1"]
		if !ok {
			continue
		}
		if !boolOf(device["Connected"]) {
			continue
		}
		found = append(found, describe(device, interfaces))
	}
	return found, nil
}

// describe reads one device's properties.
func describe(device map[string]dbus.Variant, interfaces map[string]map[string]dbus.Variant) BluetoothDevice {
	out := BluetoothDevice{
		Name:    deviceName(device),
		Address: stringOf(device["Address"]),
		Apple:   strings.Contains(stringOf(device["Modalias"]), appleVendor),
		Audio:   isAudio(device),
	}

	// The battery interface is a sibling of Device1 on the same object, and
	// most devices do not have one.
	if battery, ok := interfaces["org.bluez.Battery1"]; ok {
		if level, ok := intOf(battery["Percentage"]); ok && level >= 0 && level <= 100 {
			out.Level, out.HasLevel = level, true
		}
	}
	return out
}

// deviceName is what to call a device: the alias if its owner set one,
// otherwise the name it came with.
func deviceName(device map[string]dbus.Variant) string {
	if alias := strings.TrimSpace(stringOf(device["Alias"])); alias != "" {
		return alias
	}
	if n := strings.TrimSpace(stringOf(device["Name"])); n != "" {
		return n
	}
	return "Bluetooth device"
}

// isAudio reports whether a device is headphones, by its profiles or by the
// icon BlueZ picked for it.
func isAudio(device map[string]dbus.Variant) bool {
	if strings.HasPrefix(stringOf(device["Icon"]), "audio-") {
		return true
	}
	var uuids []string
	if err := device["UUIDs"].Store(&uuids); err == nil {
		for _, u := range uuids {
			if audioUUIDs[strings.ToLower(u)] {
				return true
			}
		}
	}
	return false
}

func stringOf(v dbus.Variant) string {
	s, _ := v.Value().(string)
	return s
}

func boolOf(v dbus.Variant) bool {
	b, _ := v.Value().(bool)
	return b
}

// intOf reads a number BlueZ may have sent as any of several widths.
func intOf(v dbus.Variant) (int, bool) {
	switch n := v.Value().(type) {
	case uint8:
		return int(n), true
	case int16:
		return int(n), true
	case uint16:
		return int(n), true
	case int32:
		return int(n), true
	case uint32:
		return int(n), true
	case int64:
		return int(n), true
	default:
		return 0, false
	}
}
