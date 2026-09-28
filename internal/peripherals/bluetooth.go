package peripherals

import (
	"errors"
	"time"
)

// Bluetooth reads the batteries of the connected Bluetooth devices.
//
// Two sources behind one: the accessory protocol for Apple audio devices,
// which report nothing any other way, and BlueZ's own battery for everything
// that has one.
type Bluetooth struct {
	// devices and dial are the two ways out of this package, replaced by a
	// test so the suite touches neither the system bus nor a radio.
	devices func() ([]BluetoothDevice, error)
	dial    func(addr [6]byte, timeout time.Duration) (channel, error)

	timeout time.Duration
}

// NewBluetooth builds the reader.
func NewBluetooth() *Bluetooth {
	return &Bluetooth{devices: bluetoothDevices, dial: dial, timeout: AAPTimeout}
}

/*
Batteries reads every connected device that has something to say.

A device that reports neither an accessory-protocol battery nor one through
BlueZ is not a row. Most connected Bluetooth devices are in that position —
a phone, a car, a speaker — and a panel listing them all at "--" would be a
list of everything paired rather than a reading.
*/
func (b *Bluetooth) Batteries() ([]Battery, error) {
	devices, err := b.devices()
	if err != nil {
		return nil, err
	}

	var (
		found []Battery
		errs  []error
	)
	for _, d := range devices {
		battery, err := b.read(d)
		if err != nil {
			// A device that would not answer is a device with no row. It is
			// worth reporting once, but it is not a failure of the section:
			// AirPods in a pocket decline the channel, and that is Tuesday.
			if !errors.Is(err, ErrNoBatteryPacket) {
				errs = append(errs, err)
			}
			continue
		}
		found = append(found, battery)
	}
	return found, errors.Join(errs...)
}

// read takes one device's battery, by whichever route it has.
//
// The accessory protocol is tried first and its answer wins, because it is the
// one with the cells in it: a device that reports both would otherwise be
// drawn as a single percentage when it could say which ear is low.
func (b *Bluetooth) read(d BluetoothDevice) (Battery, error) {
	if d.Apple && d.Audio {
		battery, err := b.readAccessory(d)
		if err == nil {
			return battery, nil
		}
		if !d.HasLevel {
			return Battery{}, err
		}
		// It has a BlueZ battery after all. Better a percentage than no row.
	}

	if !d.HasLevel {
		return Battery{}, ErrNoBatteryPacket
	}
	return Battery{Name: d.Name, Level: d.Level, HasLevel: true, Kind: d.Kind}, nil
}

// readAccessory runs the accessory protocol against one device.
func (b *Bluetooth) readAccessory(d BluetoothDevice) (Battery, error) {
	addr, err := parseAddress(d.Address)
	if err != nil {
		return Battery{}, err
	}

	channel, err := b.dial(addr, b.timeout)
	if err != nil {
		return Battery{}, err
	}
	defer func() { _ = channel.Close() }()

	cells, err := readAAP(channel, b.timeout)
	if err != nil {
		return Battery{}, err
	}
	battery, err := batteryOf(d.Name, cells)
	if err != nil {
		return Battery{}, err
	}
	battery.Kind = d.Kind
	return battery, nil
}
