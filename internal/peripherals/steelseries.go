package peripherals

import (
	"errors"
	"fmt"
	"os"
	"time"
)

/*
SteelSeries batteries, over the vendor's own HID protocol.

**The command is rivalcfg's, and it is not documented anywhere else.** Every
wireless mouse profile in that project carries `0x92` with a two-byte reply --
bit 7 of the value is charging, the rest is a level in steps of five -- and its
wireless variants OR `0x40` into the command byte so the same getter reaches a
device behind a dongle. Nothing published says this works on a keyboard. It
does: the Apex Pro TKL Wireless Gen 3 answers both forms, which is how spec 016
read a battery that HeadsetControl, the kernel, OpenRGB, Solaar and SDL
between them have no protocol for.

So everything here is inferred from one device on one machine. A reply that is
not the shape this expects yields no reading rather than a guessed one.
*/

// The SteelSeries vendor protocol, as measured.
const (
	// SteelSeriesPage is the usage page of the control endpoint. A keyboard
	// presents six interfaces and this is the only one that answers.
	SteelSeriesPage = 0xFFC0

	// batteryCommand asks for the battery. batteryWireless is the same getter
	// addressed through a dongle: rivalcfg's `_WIRELESS_FLAG`, which is also
	// exactly the offset between the two aliased command banks the device
	// exposes.
	batteryCommand = 0x92
	wirelessFlag   = 0x40

	// chargingFlag is bit 7 of the value byte.
	chargingFlag = 0x80

	// reportSize is the output and input report width. There is no report ID
	// in this descriptor, so a write carries a leading zero the kernel strips
	// and a read does not.
	reportSize = 64
)

/*
batteryProtocol is which battery command a SteelSeries device answers.

There is more than one and they are not compatible, so this is a fact about a
model rather than about the vendor.
*/
type batteryProtocol int

const (
	// batteryUnknown is a device this build has never been told about. It is
	// the default on purpose: a product that is not named below is not written
	// to at all.
	batteryUnknown batteryProtocol = iota

	// batteryModern is command 0x92 with a two-byte reply: bit 7 charging and
	// the rest a level in steps of five.
	batteryModern

	// batteryLegacy is command 0xAA 0x01 with a three-byte reply. rivalcfg
	// names the devices that speak it; this build cannot read it.
	//
	// **Named but not implemented, on purpose.** Its framing differs from the
	// newer family in more than the command byte -- rivalcfg reads the level
	// out of the first byte of the reply, so there is no command echo to match
	// an answer on, which is the whole of how the newer family tells its reply
	// from the other traffic on that endpoint. Writing that from documentation
	// alone would be shipping a guess about framing to hardware nobody here
	// has. It is in the table so those devices get told apart from ones this
	// build has never heard of.
	batteryLegacy
)

/*
steelseriesBattery says which command each known product answers.

**A product not in here is never written to.** Finding a node by vendor and
usage page is broad -- broad enough that the Arctis Nova Pro Wireless on the
development machine matched it and was being sent `0x92` on every poll, a
command from a family it does not speak (issue #62). Finding a device and
being entitled to talk to it are separate questions and this is the second one.

The Apex appears twice because its product ID moves with its connection, and
both were measured. Everything else is from rivalcfg's device profiles, which
is the only place this protocol is written down at all.
*/
var steelseriesBattery = map[uint64]batteryProtocol{
	// Measured here, spec 016: 2.4 GHz and cable.
	0x1644: batteryModern, // Apex Pro TKL Wireless Gen 3
	0x1646: batteryModern, // the same keyboard, wired

	// rivalcfg's 0x92 family. Each mouse has a product for each connection.
	0x1838: batteryModern, // Aerox 3 Wireless
	0x183A: batteryModern, // Aerox 3 Wireless, wired
	0x1852: batteryModern, // Aerox 5 Wireless
	0x1854: batteryModern, // Aerox 5 Wireless, wired
	0x1858: batteryModern, // Aerox 9 Wireless
	0x185A: batteryModern, // Aerox 9 Wireless, wired
	0x1840: batteryModern, // Prime Wireless
	0x1842: batteryModern, // Prime Wireless, wired

	// rivalcfg's 0xAA family. Unverified: no hardware here speaks it, which is
	// exactly why the table exists -- it cannot reach anything else.
	0x1830: batteryLegacy, // Rival 3 Wireless
	0x1872: batteryLegacy, // Rival 3 Wireless Gen 2
	0x172B: batteryLegacy, // Rival 650 Wireless
}

// SteelSeriesTimeout is how long the device has to answer. Short: a panel
// polls on a timer, and a keyboard that is not going to reply does not start.
const SteelSeriesTimeout = 300 * time.Millisecond

// ErrNoSteelSeries is the absence of a device to ask. Not a problem.
var ErrNoSteelSeries = errors.New("no SteelSeries device with a control endpoint")

// SteelSeries reads SteelSeries batteries.
type SteelSeries struct {
	// open is how a node is obtained, replaced by a test so the suite touches
	// no device.
	open func(path string) (reportDevice, error)

	// timeout is a field rather than the constant so a test can exercise the
	// bound without waiting for it.
	timeout time.Duration

	// unknown is what the last poll found and would not write to.
	unknown []string
}

// NewSteelSeries builds the reader.
func NewSteelSeries() *SteelSeries {
	return &SteelSeries{open: openReportDevice, timeout: SteelSeriesTimeout}
}

/*
Batteries reads every SteelSeries device that answers.

Both command forms are tried because the device does not say which it is: the
product ID moves with the connection -- 0x1644 on 2.4 GHz and 0x1646 on the
cable -- and asking twice is cheaper than tracking that and being wrong.

A device that answers neither is absent from the result rather than reported at
zero. What to do about one that was here last time is the section's question,
not this layer's.
*/
func (s *SteelSeries) Batteries() ([]Battery, error) {
	found, err := nodes(steelseriesVendor, usagePage(SteelSeriesPage))
	if err != nil {
		return nil, err
	}

	var (
		out     []Battery
		unknown []string
		errs    []error
	)
	for _, n := range found {
		if steelseriesBattery[n.Product] == batteryUnknown {
			// Detected and deliberately not spoken to. Named rather than
			// dropped, so a person with a device this build has not met can
			// see that it was found and why nothing was asked of it.
			unknown = append(unknown, n.Name)
			continue
		}
		b, err := s.read(n)
		if err != nil {
			if !errors.Is(err, errSilent) {
				errs = append(errs, err)
			}
			continue
		}
		out = append(out, b)
	}
	s.unknown = unknown
	return out, errors.Join(errs...)
}

// Unsupported names the SteelSeries devices the last poll found and did not
// speak to, for the section to say so.
func (s *SteelSeries) Unsupported() []string { return s.unknown }

// commands are the command bytes to try for a protocol, wired form first.
//
// The wireless form is the same getter with rivalcfg's `_WIRELESS_FLAG` in it,
// and which one a device wants depends on how its keyboard or mouse is
// connected -- which this build cannot tell from the product ID, because that
// moves too. Asking twice is cheaper than tracking it and being wrong.
func (p batteryProtocol) commands() []byte {
	if p != batteryModern {
		return nil
	}
	return []byte{batteryCommand, batteryCommand | wirelessFlag}
}

// read asks one node for its battery, in the protocol its product speaks.
func (s *SteelSeries) read(n hidNode) (Battery, error) {
	protocol := steelseriesBattery[n.Product]

	d, err := s.open(n.Path)
	if err != nil {
		return Battery{}, err
	}
	defer func() { _ = d.Close() }()

	for _, cmd := range protocol.commands() {
		reply, err := ask(d, s.timeout, cmd)
		if err != nil {
			continue
		}
		b, err := decodeModernBattery(reply)
		if err != nil {
			continue
		}
		b.Name = n.Name
		// KindOther, and deliberately not a guess. The control endpoint says
		// nothing about what the device is, and the interfaces beside it are
		// ambiguous in both directions: this keyboard presents a mouse
		// interface for its media controls, and plenty of mice present a
		// keyboard one for their macro buttons. Reading either as the answer
		// gets the other wrong, and the only thing it costs to admit that is
		// where the cell sorts.
		b.Kind = KindOther
		return b, nil
	}
	return Battery{}, errSilent
}

/*
ask writes one command and waits for the reply that echoes it.

**The echo is the whole of the addressing.** This endpoint carries unsolicited
traffic and late answers to earlier questions, and a reader that took the next
packet as its own would attribute one command's reply to another -- which it
did during spec 016's investigation, where a late `d2 14` was read as noise
from a different command and the battery was missed for it.
*/
func ask(d reportDevice, timeout time.Duration, cmd byte) ([]byte, error) {
	out := make([]byte, reportSize+1) // a leading report number of zero
	out[1] = cmd
	if _, err := d.Write(out); err != nil {
		return nil, fmt.Errorf("asking a SteelSeries device for %#02x: %w", cmd, err)
	}

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if err := d.SetReadDeadline(deadline); err != nil {
			return nil, fmt.Errorf("setting a read deadline: %w", err)
		}
		buf := make([]byte, reportSize)
		n, err := d.Read(buf)
		if err != nil {
			return nil, errSilent
		}
		if n > 1 && buf[0] == cmd {
			return buf[:n], nil
		}
	}
	return nil, errSilent
}

/*
decodeSteelSeriesBattery reads the two-byte reply.

The level has a step of **five**: `((v & 0x7f) - 1) * 5` is rivalcfg's
arithmetic and the device's own resolution, so a panel goes 95 % and then
100 % with nothing between. That looks like a dropped reading and is not one.

A value of zero would decode to -5, which is how a reply that is present and
means nothing announces itself.
*/
func decodeModernBattery(reply []byte) (Battery, error) {
	if len(reply) < 2 {
		return Battery{}, fmt.Errorf("a SteelSeries battery reply of %d bytes", len(reply))
	}

	v := reply[1]
	level := (int(v&^chargingFlag) - 1) * 5
	if level < 0 || level > 100 {
		return Battery{}, fmt.Errorf("a SteelSeries battery value of %#02x", v)
	}

	b := Battery{Level: level, HasLevel: true}
	switch {
	case v&chargingFlag == 0:
		b.State = Discharging
	case level == 100:
		// Charging and full are the same bit here, so the level is what tells
		// them apart. A card that said "charging" against 100 % for the rest
		// of the day would be wrong for as long as the cable stayed in.
		b.State = Full
	default:
		b.State = Charging
	}
	return b, nil
}

// reportDevice is a hidraw node written and read as whole reports.
type reportDevice interface {
	Write([]byte) (int, error)
	Read([]byte) (int, error)
	SetReadDeadline(time.Time) error
	Close() error
}

// openReportDevice opens a node for the conversation this protocol is.
func openReportDevice(path string) (reportDevice, error) {
	f, err := os.OpenFile(path, os.O_RDWR, 0) //nolint:gosec // a hidraw node the kernel named
	if err != nil {
		return nil, fmt.Errorf("opening %s: %w", path, err)
	}
	return f, nil
}
