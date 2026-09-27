package peripherals

import (
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
)

/*
Apple's accessory protocol, as far as a battery reading needs it.

AirPods report nothing through BlueZ: there is no org.bluez.Battery1 on the
device, because the code that would put one there — BlueZ's battery provider,
reading Apple's HFP AT+IPHONEACCEV — is behind the Experimental setting, which
is off by default. A panel that needed a line added to a system configuration
file before it could draw would be blank on every machine nobody has prepared.

So the device is asked directly, over an L2CAP channel, the way LibrePods and
the program this succeeds both do it. The exchange is: connect, say hello, set
features, ask for notifications, and the device starts reporting. The
constants below are not documented by anyone; they are what one firmware
answers to, which is what "Risks & Assumptions" in the spec says about them.
*/

// AAPPSM is the L2CAP port the accessory protocol listens on.
const AAPPSM = 0x1001

// The exchange, in order. Each is sent and the next is sent when the one
// before it is acknowledged.
var (
	aapHandshake     = mustDecode("00000400010002000000000000000000")
	aapSetFeatures   = mustDecode("040004004d00d700000000000000")
	aapNotifications = mustDecode("040004000f00ffffffffff")
)

// What the device says back.
var (
	aapHandshakeAck = mustDecode("01000400")
	aapFeaturesAck  = mustDecode("040004002b00")
	aapBattery      = mustDecode("040004000400")
)

func mustDecode(s string) []byte {
	b, err := hex.DecodeString(s)
	if err != nil {
		panic("bad constant: " + s)
	}
	return b
}

// The cells a device can report, as the protocol numbers them.
const (
	aapHeadset = 0x01
	aapRight   = 0x02
	aapLeft    = 0x04
	aapCase    = 0x08
)

// What a cell says it is doing.
const (
	aapCharging  = 0x01
	aapDraining  = 0x02
	aapNotOnBody = 0x04
)

// ErrNoBatteryPacket is a device that connected and never reported.
//
// Not a failure of the machine or of this code: a firmware that does not
// answer this exchange is a device this build cannot read, and the row is not
// drawn.
var ErrNoBatteryPacket = errors.New("the device did not report a battery")

// Cell names one battery inside a device.
type Cell int

const (
	// Headset is a device that is one piece and has one battery.
	Headset Cell = iota
	// Left is the left earbud.
	Left
	// Right is the right earbud.
	Right
	// Case is the charging case, which is often not there at all.
	Case
)

// String names a cell for the panel and for a test's failure message.
func (c Cell) String() string {
	switch c {
	case Left:
		return "L"
	case Right:
		return "R"
	case Case:
		return "case"
	default:
		return "headset"
	}
}

// CellReading is one cell's level.
type CellReading struct {
	Cell     Cell
	Level    int
	Charging bool
}

/*
decodeAAPBattery reads a battery packet.

The layout is a six-byte prefix, a count, and then that many five-byte records:
the cell, a constant, the level, what it is doing, and another constant.

**A cell that is not there says so; it does not go missing.** A case left on
the desk is reported at level zero with the "not on body" status, and a reader
that took the level at face value would draw a flat battery for a case that is
simply elsewhere. Those records are dropped rather than read.
*/
func decodeAAPBattery(pkt []byte) ([]CellReading, error) {
	if len(pkt) < len(aapBattery)+1 || string(pkt[:len(aapBattery)]) != string(aapBattery) {
		return nil, fmt.Errorf("not a battery packet")
	}

	count := int(pkt[len(aapBattery)])
	body := pkt[len(aapBattery)+1:]
	if len(body) < count*5 {
		return nil, fmt.Errorf("a battery packet promising %d cells and carrying %d bytes", count, len(body))
	}

	var out []CellReading
	for i := 0; i < count; i++ {
		rec := body[i*5 : i*5+5]

		cell, ok := cellOf(rec[0])
		if !ok {
			// A cell this build has never heard of. Skipped rather than
			// guessed at, and not an error: a future device with a fourth
			// earbud is not a broken packet.
			continue
		}
		if rec[3] == aapNotOnBody {
			continue
		}
		if rec[2] > 100 {
			continue
		}

		out = append(out, CellReading{
			Cell:     cell,
			Level:    int(rec[2]),
			Charging: rec[3] == aapCharging,
		})
	}
	return out, nil
}

// cellOf names a cell number, reporting whether it is one this build knows.
func cellOf(b byte) (Cell, bool) {
	switch b {
	case aapHeadset:
		return Headset, true
	case aapLeft:
		return Left, true
	case aapRight:
		return Right, true
	case aapCase:
		return Case, true
	default:
		return 0, false
	}
}

/*
batteryOf turns a device's cells into the one reading a row carries.

The **lower of the two ears**, because that is the one that will stop working
first and it is what somebody glancing at a panel wants to know. The case is
deliberately not part of it: a case at 5 % while the ears are full is not a
warning about anything the wearer is doing, and letting it set the row's number
would make the panel shout about a thing in a drawer.

A device reporting a single cell is that cell, which is what a set of
headphones that is one piece looks like.
*/
func batteryOf(name string, cells []CellReading) (Battery, error) {
	if len(cells) == 0 {
		return Battery{}, ErrNoBatteryPacket
	}

	// In a fixed order, not the order the device happened to send them. The
	// AirPods on this desk report right before left, and a panel that drew
	// "R 100  L 100" makes a reader parse the labels instead of the numbers.
	// The Cell values are declared in the order they should be drawn.
	sorted := append([]CellReading(nil), cells...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Cell < sorted[j].Cell })

	b := Battery{Name: name, Cells: sorted}

	level, has := -1, false
	for _, c := range sorted {
		if c.Cell == Case {
			continue
		}
		if !has || c.Level < level {
			level, has = c.Level, true
		}
	}
	if !has {
		// Only the case answered. It is a reading, and it is the only one
		// there is, so it is drawn rather than dropped.
		level = sorted[0].Level
	}
	b.Level, b.HasLevel = level, true

	if charging(sorted) {
		b.State = Charging
	}
	return b, nil
}

// charging reports whether any cell that is not the case is on the cable.
func charging(cells []CellReading) bool {
	for _, c := range cells {
		if c.Cell != Case && c.Charging {
			return true
		}
	}
	return false
}
