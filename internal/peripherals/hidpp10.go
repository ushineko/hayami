package peripherals

import (
	"fmt"
	"time"
)

/*
HID++ 1.0 batteries, for devices older than the feature protocol.

A Logitech K800 in daily use since about 2016 answers a HID++ 2.0 root request
with "invalid sub-id" -- it has no features at all, so the 0x1004 and 0x1000
this package reads elsewhere do not exist on it. What it has instead is a
register, and what the register gives is a band rather than a percentage.

**Only getters are sent.** Sub-id 0x81 is GET_REGISTER_REQ. Its neighbour 0x80
writes, and appears nowhere in this file or this package.
*/

// The HID++ 1.0 sub-ids and registers this package reads.
const (
	// subGetRegister reads a short register. There is a long form, 0x83, which
	// nothing here needs.
	subGetRegister = 0x81

	// registerBatteryCharge carries a percentage on the devices that have a
	// fuel gauge, and answers an error on the ones that do not.
	registerBatteryCharge = 0x0D

	// registerBatteryStatus carries the band, which is what a device without a
	// gauge knows about itself.
	registerBatteryStatus = 0x07
)

/*
Band is a battery as a device without a gauge reports it.

Four values and no more, which is why the panel draws four segments: it is the
device's own resolution, and the shape its indicator uses.
*/
type Band int

// The bands, low to high. BandUnknown is the zero value and is not a reading.
const (
	BandUnknown Band = iota
	BandCritical
	BandLow
	BandGood
	BandFull
)

// Segments is how many of four are filled.
func (b Band) Segments() int {
	switch b {
	case BandCritical:
		return 1
	case BandLow:
		return 2
	case BandGood:
		return 3
	case BandFull:
		return 4
	default:
		return 0
	}
}

// String is the word under the cell.
func (b Band) String() string {
	switch b {
	case BandCritical:
		return "Critical"
	case BandLow:
		return "Low"
	case BandGood:
		return "Good"
	case BandFull:
		return "Full"
	default:
		return ""
	}
}

/*
bands maps the register's values.

solaar's, checked against one keyboard, which answered 0x05 for "good". A value
that is not one of these is not a band: an unknown reading yields nothing
rather than the nearest guess, because the whole point of a band is that it is
the device's own word and not an interpolation.
*/
var bands = map[byte]Band{
	0x01: BandCritical,
	0x03: BandLow,
	0x05: BandGood,
	0x07: BandFull,
}

// readOldBattery reads a HID++ 1.0 battery, preferring a gauge to a band.
func readOldBattery(e endpoint, timeout time.Duration, index byte) (Battery, error) {
	if b, err := readChargeRegister(e, timeout, index); err == nil {
		return b, nil
	}
	return readStatusRegister(e, timeout, index)
}

// readChargeRegister reads register 0x0D, which is a percentage and a state.
func readChargeRegister(e endpoint, timeout time.Duration, index byte) (Battery, error) {
	params, err := register(e, timeout, index, registerBatteryCharge)
	if err != nil {
		return Battery{}, err
	}
	if len(params) < 2 {
		return Battery{}, fmt.Errorf("a battery charge reply of %d bytes", len(params))
	}
	level := int(params[0])
	if level <= 0 || level > 100 {
		return Battery{}, fmt.Errorf("a battery charge of %d", level)
	}
	return Battery{Level: level, HasLevel: true, State: oldState(params[1])}, nil
}

/*
readStatusRegister reads register 0x07, which is the band.

The reply is `band, charge, 0`. The K800 this was written against answered
`05 00 00`: good, and discharging.
*/
func readStatusRegister(e endpoint, timeout time.Duration, index byte) (Battery, error) {
	params, err := register(e, timeout, index, registerBatteryStatus)
	if err != nil {
		return Battery{}, err
	}
	if len(params) < 2 {
		return Battery{}, fmt.Errorf("a battery status reply of %d bytes", len(params))
	}
	band, ok := bands[params[0]]
	if !ok {
		return Battery{}, fmt.Errorf("a battery band of %#02x", params[0])
	}
	return Battery{Band: band, HasBand: true, State: oldState(params[1])}, nil
}

// oldState reads the charge byte, which is zero when a device is running on
// its battery and something else when it is not.
func oldState(charge byte) State {
	if charge == 0 {
		return Discharging
	}
	return Charging
}
