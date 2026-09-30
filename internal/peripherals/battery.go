package peripherals

import "fmt"

// State is what a battery is doing, as distinct from how full it is.
//
// It is kept apart from the level because the two answer different questions
// and because a change in it invalidates a remembered level: a device that has
// gone from discharging to charging has not merely moved a few percent.
type State int

const (
	// Discharging is the ordinary case and says nothing worth drawing.
	Discharging State = iota
	// Charging is plugged in and filling.
	Charging
	// Full is charged, and is separate from Charging because a device says so
	// itself and a panel that showed it as still filling would be wrong for
	// as long as it stayed on the cable.
	Full
)

// String names a state for the panel and for a test's failure message.
func (s State) String() string {
	switch s {
	case Charging:
		return "charging"
	case Full:
		return "full"
	default:
		return "discharging"
	}
}

// Battery is one device's reading.
type Battery struct {
	// Name is the device's own name, as the device or the tool reporting it
	// gives it. It identifies the device across polls, so a row's level is
	// never carried onto a different device.
	Name string

	// Level is a percentage. HasLevel is false for a device that is present
	// and connected but has not said how full it is, which a headset on a
	// charging cradle does.
	Level    int
	HasLevel bool

	State State

	// Band is how full a device without a fuel gauge says it is, in the four
	// steps such a device knows. HasBand is false for everything with a
	// percentage.
	//
	// **Not a substitute for Level and never converted into one.** A device
	// saying "good" does not mean 75 %, and spec 008 refused to invent that
	// figure once already; spec 018 draws the four steps instead, which is how
	// the device's own indicator shows it.
	Band    Band
	HasBand bool

	// Kind is what sort of device this is, where the source could say. It is
	// not part of the reading and is only used to order the panel's cells;
	// a source that cannot tell leaves it KindOther.
	Kind Kind

	// Cells are the batteries inside the device, for one that has more than
	// one: two earbuds and a case. Empty for the ordinary device with a single
	// battery, which is every device spec 008 reads.
	//
	// Level is still the row's number — the lower of the two ears — and these
	// are what goes on the quiet line beneath it. They are kept apart rather
	// than folded into a string here because formatting is the view's job.
	Cells []CellReading
}

// decodeUnifiedBattery reads a 0x1004 get_status reply.
//
// The device reports both a state of charge and a level band, and the band is
// what a device with no fuel gauge actually knows. The percentage is used when
// there is one; the band is not turned into a number, because a device saying
// "good" does not mean 75 %.
func decodeUnifiedBattery(p []byte) (Battery, error) {
	if len(p) < 3 {
		return Battery{}, fmt.Errorf("a unified battery reply of %d bytes", len(p))
	}

	var b Battery
	if soc := int(p[0]); soc <= 100 {
		b.Level, b.HasLevel = soc, true
	}

	switch p[2] {
	case 0x01, 0x02: // charging, charging slowly
		b.State = Charging
	case 0x03: // charging complete
		b.State = Full
	default:
		b.State = Discharging
	}
	return b, nil
}

// decodeBatteryStatus reads a 0x1000 get_battery_level_status reply, which is
// what a device without a fuel gauge has instead.
func decodeBatteryStatus(p []byte) (Battery, error) {
	if len(p) < 3 {
		return Battery{}, fmt.Errorf("a battery status reply of %d bytes", len(p))
	}

	var b Battery
	if level := int(p[0]); level <= 100 {
		b.Level, b.HasLevel = level, true
	}

	switch p[2] {
	case 0x01, 0x04: // recharging, slow recharge
		b.State = Charging
	case 0x02, 0x03: // almost full, full
		b.State = Full
	default:
		// 0x05 invalid battery, 0x06 thermal error and 0x07 charging error all
		// land here. They are faults of the charger, not readings, and the
		// level beside them is still the level.
		b.State = Discharging
	}
	return b, nil
}
