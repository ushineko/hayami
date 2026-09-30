package panel_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/ushineko/sanshoku"
	"github.com/ushineko/sanshoku/battery"
	"github.com/ushineko/sanshoku/bluez"
	"github.com/ushineko/sanshoku/logitech"

	"github.com/ushineko/hayami/internal/panel"
	"github.com/ushineko/hayami/internal/view"
)

// clock is a time the test moves itself, so a rule about ten minutes does not
// cost ten minutes to check.
type clock struct{ at time.Time }

func (c *clock) now() time.Time       { return c.at }
func (c *clock) tick(d time.Duration) { c.at = c.at.Add(d) }

/*
peripheral is a fake device: the candidate a fake scan lists for its driver,
and the device that candidate's Open yields.

It carries only what the section's reasons turn on: what it reads, an error
from Open or from a read (sanshoku.ErrGone, ErrUnsupported and a permission
error among them), and for a Logitech receiver its Presence. The fields are
changed between polls to move the desk on.
*/
type peripheral struct {
	driver string
	name   string
	path   string
	phys   string

	says     []battery.Battery
	openErr  error
	readErr  error
	presence logitech.Presence

	opens, closes int
}

func (d *peripheral) Identity() sanshoku.Identity {
	return sanshoku.Identity{Name: d.name, Path: d.path, Phys: d.phys}
}

func (d *peripheral) Close() error { d.closes++; return nil }

func (d *peripheral) Batteries(context.Context) ([]battery.Battery, error) {
	return d.says, d.readErr
}

func (d *peripheral) Presence() logitech.Presence { return d.presence }

// desk is what a fake scan finds: the devices on it, and a driver's own
// failure to list, by driver name.
type desk struct {
	devices []*peripheral
	failing map[string]error
}

// scan is sanshoku.Scan's shape over the desk. It asks each driver only its
// name, which touches no device.
func (k *desk) scan(_ context.Context, drivers ...sanshoku.Driver) ([]sanshoku.Candidate, error) {
	var out []sanshoku.Candidate
	var errs []error
	for _, d := range drivers {
		if err := k.failing[d.Name()]; err != nil {
			errs = append(errs, err)
		}
		for _, dev := range k.devices {
			if dev.driver != d.Name() {
				continue
			}
			out = append(out, sanshoku.Candidate{
				Identity: dev.Identity(),
				Driver:   dev.driver,
				Open: func(context.Context) (sanshoku.Device, error) {
					dev.opens++
					if dev.openErr != nil {
						return nil, dev.openErr
					}
					return dev, nil
				},
			})
		}
	}
	return out, errors.Join(errs...)
}

// section is a peripherals section over the desk.
func (k *desk) section(c *clock) *panel.Peripherals {
	if c == nil {
		c = &clock{at: time.Now()}
	}
	return panel.NewPeripheralsOver(k.scan, c.now)
}

// receiver is a Logitech receiver on the desk, reading what says returns.
func receiver(says ...battery.Battery) *peripheral {
	return &peripheral{
		driver: "logitech", name: "Logitech USB Receiver", path: "/dev/hidraw10",
		says: says, presence: logitech.Presence{Nodes: 1},
	}
}

// poll polls once and fails the test on an error.
func poll(t *testing.T, p panel.Source) bool {
	t.Helper()
	drawn, err := p.Poll(t.Context())
	require.NoError(t, err)
	return drawn
}

// devices reads the section's readings back as the view sees them.
func devices(p *panel.Peripherals) []view.PeripheralReading {
	return p.Data().(view.PeripheralsReading).Devices
}

// AC2. A device this panel has never had a level from is not a cell.
//
// An Arctis whose base station is in with the headset switched off is
// reported as present and silent for a whole session. Drawn, it is a name, a
// dash and a word explaining that there is nothing to say — and, once the
// cells are ordered, it sits in front of the mouse.
func TestADeviceThatHasNeverGivenALevelIsNotDrawn(t *testing.T) {
	k := &desk{devices: []*peripheral{
		receiver(battery.Battery{Name: "G502 X PLUS", Level: 78, HasLevel: true, Kind: battery.KindMouse}),
		{driver: "steelseries", name: "SteelSeries Arctis Nova Pro Wireless", path: "/dev/hidraw13",
			says: []battery.Battery{{Name: "SteelSeries Arctis Nova Pro Wireless", Kind: battery.KindHeadset}}},
	}}
	p := k.section(nil)

	assert.True(t, poll(t, p))

	found := devices(p)
	require.Len(t, found, 1)
	assert.Equal(t, "G502 X PLUS", found[0].Name)
}

// R3.6. The Arctis switched off is a reading with no level, and the card shows
// it as it showed headsetcontrol's BATTERY_UNAVAILABLE: the level it last gave
// is kept while the charge state holds, and no reason is given for it.
func TestAnArctisSwitchedOffKeepsItsLastLevel(t *testing.T) {
	arctis := &peripheral{driver: "steelseries", name: "SteelSeries Arctis Nova Pro Wireless", path: "/dev/hidraw13",
		says: []battery.Battery{{Name: "SteelSeries Arctis Nova Pro Wireless", Level: 62, HasLevel: true, Kind: battery.KindHeadset}}}
	k := &desk{devices: []*peripheral{arctis}}
	c := &clock{at: time.Now()}
	p := k.section(c)
	poll(t, p)

	arctis.says = []battery.Battery{{Name: "SteelSeries Arctis Nova Pro Wireless", Kind: battery.KindHeadset}}
	c.tick(time.Minute)
	assert.True(t, poll(t, p))

	found := devices(p)
	require.Len(t, found, 1)
	assert.Equal(t, 62, found[0].Level)
	assert.False(t, found[0].Stale, "the base station answered; it is the headset that is off")
	assert.Empty(t, p.Section().Reasons)
}

// AC2. A device that answered and has gone quiet keeps its level and is drawn
// dim.
//
// This is the case the memory exists for. A wireless mouse that has been still
// for a minute answers nothing — measured on the receiver this was written
// against, where one poll in fourteen came back empty — and a panel that
// dropped its cell would reflow on a desk where nothing was wrong.
func TestADeviceThatStopsAnsweringKeepsItsLevelAndGoesDim(t *testing.T) {
	mouse := receiver(battery.Battery{Name: "G502 X PLUS", Level: 86, HasLevel: true})
	k := &desk{devices: []*peripheral{mouse}}
	c := &clock{at: time.Now()}
	p := k.section(c)
	poll(t, p)

	mouse.says = nil
	c.tick(time.Minute)
	assert.True(t, poll(t, p), "a device that has gone quiet is still drawn")

	found := devices(p)
	require.Len(t, found, 1)
	assert.Equal(t, 86, found[0].Level, "the last level was dropped rather than kept")
	assert.True(t, found[0].Stale)

	// The verdict survives the dimming, which is the shells' to apply.
	cells := p.Section().Cells
	require.Len(t, cells, 1)
	assert.True(t, cells[0].Stale)
	assert.Equal(t, "Offline", cells[0].Note)
	assert.Contains(t, cells[0].Value, "86")
}

// AC2. A device quiet for long enough is gone rather than quiet, and stops
// being drawn. A mouse put away this morning is not a mouse that is idle.
func TestADeviceQuietForLongEnoughIsForgotten(t *testing.T) {
	mouse := receiver(battery.Battery{Name: "G502 X PLUS", Level: 86, HasLevel: true})
	k := &desk{devices: []*peripheral{mouse}}
	c := &clock{at: time.Now()}
	p := k.section(c)
	poll(t, p)

	mouse.says = nil
	c.tick(panel.PeripheralsForget + time.Minute)

	assert.False(t, poll(t, p))
	assert.Empty(t, devices(p))
}

// AC2. A device that is connected and not saying how full it is keeps the
// level it last gave, as the monitor's own carry-over does.
func TestAConnectedDeviceThatIsNotSayingKeepsItsLastLevel(t *testing.T) {
	headset := receiver(battery.Battery{Name: "Arctis Nova Pro Wireless", Level: 72, HasLevel: true})
	k := &desk{devices: []*peripheral{headset}}
	c := &clock{at: time.Now()}
	p := k.section(c)
	poll(t, p)

	headset.says = []battery.Battery{{Name: "Arctis Nova Pro Wireless"}}
	c.tick(time.Minute)
	poll(t, p)

	found := devices(p)
	require.Len(t, found, 1)
	assert.Equal(t, 72, found[0].Level)
	assert.False(t, found[0].Stale, "the device is answering; it is its battery that is quiet")
}

// AC2. The carried level is dropped when the battery crosses between charging
// and discharging. Then it is not stale, it is wrong: a headset that has been
// on its cable is not at the level it came off the head with.
func TestTheCarriedLevelIsDroppedWhenTheChargeStateChanges(t *testing.T) {
	headset := receiver(battery.Battery{Name: "Arctis Nova Pro Wireless", Level: 72, HasLevel: true, State: battery.Discharging})
	k := &desk{devices: []*peripheral{headset}}
	c := &clock{at: time.Now()}
	p := k.section(c)
	poll(t, p)

	// On the cable now, and not saying how full it is. The old level belongs
	// to before the cable, so there is nothing to draw and no cell.
	headset.says = []battery.Battery{{Name: "Arctis Nova Pro Wireless", State: battery.Charging}}
	c.tick(time.Minute)
	poll(t, p)

	for _, d := range devices(p) {
		assert.NotEqual(t, 72, d.Level, "a level from before the cable was carried over")
	}
}

// AC2. A different device does not inherit the last one's level. The name is
// what tells them apart.
func TestADifferentDeviceDoesNotInheritTheLastOnesLevel(t *testing.T) {
	headset := receiver(battery.Battery{Name: "Arctis Nova Pro Wireless", Level: 72, HasLevel: true})
	k := &desk{devices: []*peripheral{headset}}
	c := &clock{at: time.Now()}
	p := k.section(c)
	poll(t, p)

	headset.says = []battery.Battery{{Name: "Some Other Headset"}}
	c.tick(time.Minute)
	poll(t, p)

	for _, d := range devices(p) {
		assert.NotEqual(t, "Some Other Headset", d.Name,
			"a device with no level of its own took the previous one's")
	}
}

// AC15. The mouse is the first cell, however the drivers hand the devices over
// and whatever the devices are called.
func TestTheMouseIsTheFirstCell(t *testing.T) {
	// Given headset-first, as a driver may, and named so that a sort by name
	// alone would keep it that way.
	k := &desk{devices: []*peripheral{receiver(
		battery.Battery{Name: "Arctis Nova Pro Wireless", Level: 47, HasLevel: true, Kind: battery.KindHeadset},
		battery.Battery{Name: "G502 X PLUS", Level: 78, HasLevel: true, Kind: battery.KindMouse},
	)}}
	p := k.section(nil)
	poll(t, p)

	found := devices(p)
	require.Len(t, found, 2)
	assert.Equal(t, "G502 X PLUS", found[0].Name)
	assert.Equal(t, view.KindMouse, found[0].Kind)

	// The section the shells draw is in the same order as the JSON the command
	// line prints, which is the reason the panel orders rather than the view.
	cells := p.Section().Cells
	require.Len(t, cells, 2)
	assert.Equal(t, "G502 X PLUS", cells[0].Label)
}

// AC15. Cells keep their order as devices come and go, because a panel is read
// at a glance and a cell that moves has to be found again.
func TestCellsKeepTheirOrderAsDevicesComeAndGo(t *testing.T) {
	mouse := battery.Battery{Name: "G502 X PLUS", Level: 86, HasLevel: true, Kind: battery.KindMouse}
	rx := receiver(mouse)
	k := &desk{devices: []*peripheral{rx}}
	p := k.section(nil)
	poll(t, p)

	rx.says = []battery.Battery{{Name: "Arctis", Level: 50, HasLevel: true, Kind: battery.KindHeadset}, mouse}
	poll(t, p)

	found := devices(p)
	require.Len(t, found, 2)
	assert.Equal(t, "G502 X PLUS", found[0].Name, "the headset took the mouse's place")
	assert.Equal(t, "Arctis", found[1].Name)
}

// A desk with nothing on it is a machine with no section, not a machine with
// an empty heading.
func TestADeskWithNothingOnItDrawsNoSection(t *testing.T) {
	p := (&desk{}).section(nil)

	assert.False(t, poll(t, p), "absent hardware is not an error and not a section")
	assert.Empty(t, devices(p))
}

// AC. Since is when a device arrived, not when it last answered.
//
// The right-hand slot goes to the device detected most recently, so a Since
// that moved with every poll would make every device equally new and the slot
// would be decided by the name again.
func TestSinceIsWhenTheDeviceArrivedNotWhenItLastAnswered(t *testing.T) {
	k := &desk{devices: []*peripheral{receiver(battery.Battery{Name: "G502 X PLUS", Level: 86, HasLevel: true})}}
	c := &clock{at: time.Now()}
	arrived := c.at
	p := k.section(c)
	poll(t, p)
	c.tick(time.Minute)
	poll(t, p)

	found := devices(p)
	require.Len(t, found, 1)
	assert.Equal(t, arrived, found[0].Since, "the arrival moved with the poll")
	assert.Equal(t, c.at, found[0].Seen, "the last answer did not move with the poll")
}

// AC. A device that was forgotten and comes back is new, so switching a
// headset off and on again puts it back in the slot.
func TestADeviceThatComesBackIsNewAgain(t *testing.T) {
	mouse := battery.Battery{Name: "G502 X PLUS", Level: 86, HasLevel: true}
	rx := receiver(mouse)
	k := &desk{devices: []*peripheral{rx}}
	c := &clock{at: time.Now()}
	p := k.section(c)
	poll(t, p)
	arrived := c.at

	rx.says = nil
	c.tick(panel.PeripheralsForget + time.Minute)
	poll(t, p)
	require.Empty(t, devices(p), "the device was not forgotten")

	rx.says = []battery.Battery{mouse}
	c.tick(time.Minute)
	poll(t, p)

	found := devices(p)
	require.Len(t, found, 1)
	assert.Equal(t, c.at, found[0].Since, "a device that came back kept its old arrival")
	assert.NotEqual(t, arrived, found[0].Since)
}

/*
R2.1. A device is opened once and read on every poll after.

The drivers remember things worth keeping between reads -- a receiver's
located indices, a Razer device's transaction ID -- and a section that opened
every device on every poll would throw that away each time.
*/
func TestADeviceIsOpenedOnceAndHeldAcrossPolls(t *testing.T) {
	rx := receiver(battery.Battery{Name: "G502 X PLUS", Level: 86, HasLevel: true})
	p := (&desk{devices: []*peripheral{rx}}).section(nil)

	for range 3 {
		poll(t, p)
	}

	assert.Equal(t, 1, rx.opens, "the device was opened again rather than held")
	assert.Zero(t, rx.closes)
}

// R2.1. A device whose candidate is no longer listed is closed and dropped,
// and opened afresh when it is listed again. That is hotplug by pull.
func TestADeviceNoLongerListedIsClosedAndReopenedWhenBack(t *testing.T) {
	rx := receiver(battery.Battery{Name: "G502 X PLUS", Level: 86, HasLevel: true})
	k := &desk{devices: []*peripheral{rx}}
	p := k.section(nil)
	poll(t, p)

	k.devices = nil
	poll(t, p)
	assert.Equal(t, 1, rx.closes, "a device that was unplugged was kept open")

	k.devices = []*peripheral{rx}
	poll(t, p)
	assert.Equal(t, 2, rx.opens, "a device plugged back in was not opened")
}

// R2.2. A read that says the device has gone closes it; the next scan finds it
// again if it is back, and it is opened afresh.
func TestADeviceThatHasGoneIsClosedAndFoundAgain(t *testing.T) {
	rx := receiver(battery.Battery{Name: "G502 X PLUS", Level: 86, HasLevel: true})
	p := (&desk{devices: []*peripheral{rx}}).section(nil)
	poll(t, p)

	rx.readErr = fmt.Errorf("reading /dev/hidraw10: %w", sanshoku.ErrGone)
	drawn, err := p.Poll(t.Context())
	require.NoError(t, err, "a device that has gone is not a failure to log")
	assert.True(t, drawn, "its last level is still drawn, dim")
	assert.Equal(t, 1, rx.closes)
	assert.Empty(t, p.Section().Reasons, "gone is not a reason")

	rx.readErr = nil
	poll(t, p)
	assert.Equal(t, 2, rx.opens)
	require.Len(t, devices(p), 1)
	assert.False(t, devices(p)[0].Stale)
}

/*
R3.1. A section on a desk with nothing on it says what did not answer, one
vendor at a time.

One per vendor rather than one for the section, because they are different
things to go and do something about: a receiver that is not plugged in,
hardware of a given make that is not there, and nothing on Bluetooth with a
battery to report.
*/
func TestPeripheralsNamesEachVendorThatFoundNothing(t *testing.T) {
	p := (&desk{}).section(nil)

	assert.False(t, poll(t, p), "there is nothing to draw")

	sec := p.Section()
	assert.Equal(t, []string{
		"no Logitech receiver",
		"no Razer device",
		"no SteelSeries device",
		"no Bluetooth device with a battery",
	}, reasonTexts(sec))
	for _, r := range sec.Reasons {
		assert.Equal(t, view.Info, r.Status, "%q was marked as a failure", r.Text)
	}
}

// R3.7. BlueZ not answering is a machine with no Bluetooth adapter: absence,
// not failure.
func TestNoBluezIsNoBluetoothAdapter(t *testing.T) {
	// As sanshoku.Scan returns it: prefixed with the driver's name, and
	// wrapping sanshoku.ErrUnavailable, which Scan does not drop.
	noBluez := fmt.Errorf("bluez: %w", bluez.ErrNoBlueZ)
	p := (&desk{failing: map[string]error{"bluez": noBluez, "apple": noBluez}}).section(nil)

	poll(t, p)

	_, err := p.Poll(t.Context())
	require.NoError(t, err, "no adapter is not a failure to log")
	r := find(t, p.Section(), "no Bluetooth adapter")
	assert.Equal(t, view.Info, r.Status)
	assert.Len(t, p.Section().Reasons, 4, "one line for the adapter, not one per driver")
	assert.NotContains(t, reasonTexts(p.Section()), "no Bluetooth device with a battery",
		"no adapter and nothing connected are two different answers")
}

// R3.4. A driver that failed to list is a vendor that would not answer, and
// the failure reaches the caller as well.
func TestADriverThatFailsToListIsAVendorThatWouldNotAnswer(t *testing.T) {
	boom := errors.New("listing /sys/class/hidraw: input/output error")
	p := (&desk{failing: map[string]error{"bluez": boom}}).section(nil)

	_, err := p.Poll(t.Context())

	require.ErrorIs(t, err, boom)
	r := find(t, p.Section(), "a Bluetooth device would not answer")
	assert.Equal(t, view.Warn, r.Status)
	assert.Contains(t, r.Detail, "input/output error")
}

// R3.4. An Open that fails for a reason that is neither permission nor an
// unsupported product is marked, with the error as its detail.
func TestAnOpenThatFailsIsAVendorThatWouldNotAnswer(t *testing.T) {
	dock := &peripheral{driver: "razer", name: "Razer Mouse Dock Pro", path: "/dev/hidraw4",
		openErr: errors.New("opening /dev/hidraw4: no such device or address")}
	p := (&desk{devices: []*peripheral{dock}}).section(nil)

	_, err := p.Poll(t.Context())

	require.Error(t, err, "a failure is still reported to the caller that logs it")
	r := find(t, p.Section(), "a Razer device would not answer")
	assert.Equal(t, view.Warn, r.Status)
	assert.Contains(t, r.Detail, "no such device or address")
}

// R3.4. A read that fails is marked too, and what the device did read beside
// the failure is still drawn.
func TestAReadThatFailsIsMarkedAndItsPartialAnswerKept(t *testing.T) {
	rx := receiver(battery.Battery{Name: "MX Keys", Level: 55, HasLevel: true})
	rx.readErr = errors.New("device 2: hid++ error 0x05")
	p := (&desk{devices: []*peripheral{rx}}).section(nil)

	drawn, err := p.Poll(t.Context())

	require.Error(t, err)
	assert.True(t, drawn, "a partial answer is still an answer")
	require.Len(t, devices(p), 1)
	// The card is drawing, so absences and failures alike are doctor's.
	assert.Empty(t, p.Section().Reasons)
}

/*
R3.2. A device that may not be opened says so, and says what to do about it.

liquidctl's and OpenRazer's packages used to install the udev rule that lets
the logged-in user open these nodes; reading them directly, nothing does but
hayami's installer, and a device found and not opened otherwise looks like no
device at all.
*/
func TestADeviceThatMayNotBeOpenedNamesTheUdevRule(t *testing.T) {
	dock := &peripheral{driver: "razer", name: "Razer Mouse Dock Pro", path: "/dev/hidraw4",
		openErr: &fsError{syscall.EACCES}}
	mouse := receiver(battery.Battery{Name: "G502 X PLUS", Level: 86, HasLevel: true})
	other := receiver(battery.Battery{Name: "MX Keys", Level: 50, HasLevel: true})
	other.path = "/dev/hidraw11"
	p := (&desk{devices: []*peripheral{mouse, other, dock}}).section(nil)

	drawn, err := p.Poll(t.Context())

	require.NoError(t, err, "nothing will change until the rule is installed; it is a reason, not a log line")
	require.True(t, drawn)
	r := find(t, p.Section(), "Razer Mouse Dock Pro is not permitted")
	assert.Equal(t, view.Warn, r.Status)
	assert.Contains(t, r.Detail, "60-sanshoku.rules")
	assert.Contains(t, r.Detail, "udev rule")
	assert.False(t, r.Aside, "the one line a reader can act on stays on a full card")
}

// fsError is how an Open reports EACCES: wrapped, as os.PathError does.
type fsError struct{ errno syscall.Errno }

func (e *fsError) Error() string { return "open /dev/hidraw4: " + e.errno.Error() }
func (e *fsError) Unwrap() error { return e.errno }

// A card that is showing hardware does not also explain what it is not
// showing. That belongs in doctor.
func TestPeripheralsWithADeviceSaysNothingExtra(t *testing.T) {
	p := (&desk{devices: []*peripheral{receiver(battery.Battery{Name: "MX Master", Level: 70, HasLevel: true})}}).section(nil)

	require.True(t, poll(t, p))
	assert.Empty(t, p.Section().Reasons)
}

// arctisUnsupported is a SteelSeries product this build does not speak to.
func arctisUnsupported() *peripheral {
	return &peripheral{driver: "steelseries", name: "SteelSeries Arctis 7", path: "/dev/hidraw13",
		openErr: fmt.Errorf("SteelSeries Arctis 7 (1038:12ad): not a product this driver speaks to: %w", sanshoku.ErrUnsupported)}
}

/*
R3.3. A device that is present and unreadable is named even when others are
drawing.

The suppression rule is right for absences — "no Logitech receiver" is noise
beside a mouse that is showing — and wrong for a device that is on the desk and
being deliberately left alone. Without this, a build that had quietly stopped
recognising a device looked exactly like one that never met it (spec 017).
*/
func TestAnUnsupportedDeviceIsNamedEvenWhenOthersAreDrawing(t *testing.T) {
	p := (&desk{devices: []*peripheral{
		receiver(battery.Battery{Name: "MX Master", Level: 70, HasLevel: true}),
		arctisUnsupported(),
	}}).section(nil)

	drawn, err := p.Poll(t.Context())

	require.NoError(t, err)
	require.True(t, drawn)

	sec := p.Section()
	require.Len(t, sec.Reasons, 1, "the absences should still be suppressed: %v", reasonTexts(sec))
	assert.Equal(t, "SteelSeries Arctis 7", sec.Reasons[0].Label)
	assert.Equal(t, "unsupported", sec.Reasons[0].Text,
		"one word: the name and a sentence was a line as wide as the panel (issue #77)")
	assert.False(t, sec.Reasons[0].Aside, "one device drawn leaves room, and the line is drawn")
	assert.Contains(t, sec.Lines(), view.Row{Label: "SteelSeries Arctis 7", Value: "unsupported", Status: view.Dim})
}

/*
With two devices drawing, an unsupported one steps aside (issue #77).

The card is full and doing its job; a headset this build cannot read is a
footnote there. It is not dropped: the hover note and doctor still name it,
which is what spec 017 asked for.
*/
func TestAnUnsupportedDeviceStepsAsideWhenTwoOthersAreDrawing(t *testing.T) {
	p := (&desk{devices: []*peripheral{
		receiver(
			battery.Battery{Name: "MX Master", Level: 70, HasLevel: true},
			battery.Battery{Name: "G502", Level: 40, HasLevel: true},
		),
		arctisUnsupported(),
	}}).section(nil)

	require.True(t, poll(t, p))

	sec := p.Section()
	require.Len(t, sec.Reasons, 1, "kept for doctor")
	assert.True(t, sec.Reasons[0].Aside)
	for _, l := range sec.Lines() {
		assert.NotEqual(t, "SteelSeries Arctis 7", l.Label, "an aside is not a line of the card")
	}
	assert.Contains(t, sec.Hover(), "SteelSeries Arctis 7", "the hover note still names it")
}

// receiverSays is what a section says about a receiver that read nothing and
// reported the given Presence, which is the whole of what these sentences
// turn on.
func receiverSays(t *testing.T, presence logitech.Presence) []string {
	t.Helper()
	rx := receiver()
	rx.presence = presence
	p := (&desk{devices: []*peripheral{rx}}).section(nil)
	poll(t, p)
	return reasonTexts(p.Section())
}

/*
R3.5. Situations that used to read as one sentence.

"No Logitech receiver" was said about a receiver with a ten-year-old keyboard
on it and a pairing slot left over from hardware that was never on the desk
(issue #66). Each of these is a different thing for a reader to do something
about, so each gets its own words.
*/
func TestTheReceiverSaysWhichOfTheseItIs(t *testing.T) {
	for _, c := range []struct {
		name     string
		presence logitech.Presence
		want     string
	}{
		{"a receiver with empty slots", logitech.Presence{Nodes: 1},
			"a Logitech receiver, with nothing paired to it"},
		{"a receiver whose devices are quiet", logitech.Presence{Nodes: 1, Quiet: 2},
			"a Logitech receiver, with nothing awake on it"},
		{"a device whose HID++ 1.0 register would not read", logitech.Presence{Nodes: 1, TooOld: []string{"Logitech K800"}},
			"speaks HID++ 1.0"},
	} {
		assert.Contains(t, receiverSays(t, c.presence), c.want, c.name)
	}
	assert.Contains(t, reasonTexts(func() view.Section {
		p := (&desk{}).section(nil)
		poll(t, p)
		return p.Section()
	}()), "no Logitech receiver", "no hardware at all")
}

// R3.5. Two receivers' quiet indices are one count: Presence is summed over the
// receiver's devices.
func TestQuietIndicesAreSummedAcrossReceivers(t *testing.T) {
	a, b := receiver(), receiver()
	b.path = "/dev/hidraw11"
	a.presence = logitech.Presence{Nodes: 1, Quiet: 1}
	b.presence = logitech.Presence{Nodes: 1, Quiet: 2}
	p := (&desk{devices: []*peripheral{a, b}}).section(nil)
	poll(t, p)

	r := find(t, p.Section(), "a Logitech receiver, with nothing awake on it")
	assert.True(t, strings.HasPrefix(r.Detail, "3 indices"), r.Detail)
}

/*
A quiet slot is counted and never named.

A pairing table outlives the hardware in it: the receiver this was written
against carries a slot for a mouse its owner has never owned, because the
dongle was paired to one years ago and a slot is only cleared by an explicit
unpair. Naming it would put a device on the panel that was never on the desk —
which this build has drawn once already, and once is enough.
*/
func TestAQuietSlotIsCountedAndNeverNamed(t *testing.T) {
	texts := receiverSays(t, logitech.Presence{Nodes: 1, Quiet: 1})

	for _, text := range texts {
		assert.NotContains(t, text, "Performance MX")
	}
	assert.Contains(t, texts, "a Logitech receiver, with nothing awake on it")
}

// sleepingDock is a Razer dock that is listed, opens, and reads nothing: the
// mouse on it is asleep.
func sleepingDock() *peripheral {
	return &peripheral{driver: "razer", name: "Razer Mouse Dock", path: "/dev/hidraw0"}
}

/*
R3.9. A vendor whose devices are listed and answer nothing says so.

The Unifying machine's Razer dock and dongle, with the mouse asleep, gave no
Razer line at all: neither absent, because they were listed, nor read. The old
reader said "no Razer device" about a dock on the desk.
*/
func TestAVendorThatAnsweredNothingSaysSo(t *testing.T) {
	p := (&desk{devices: []*peripheral{sleepingDock()}}).section(nil)

	poll(t, p)

	texts := reasonTexts(p.Section())
	assert.Contains(t, texts, "a Razer device answered nothing")
	assert.NotContains(t, texts, "no Razer device", "a dock that is on the desk was called absent")
	assert.Equal(t, view.Info, find(t, p.Section(), "a Razer device answered nothing").Status)
}

// R3.9. It is about absence, so a card that is drawing drops it; and a vendor
// whose only device was not read -- unsupported, or not permitted -- has
// already said why and does not also say it answered nothing.
func TestAnsweredNothingIsOnlyForADeviceThatWasAsked(t *testing.T) {
	p := (&desk{devices: []*peripheral{
		sleepingDock(), receiver(battery.Battery{Name: "G502 X PLUS", Level: 86, HasLevel: true}),
	}}).section(nil)
	poll(t, p)
	assert.NotContains(t, reasonTexts(p.Section()), "a Razer device answered nothing")

	p = (&desk{devices: []*peripheral{arctisUnsupported()}}).section(nil)
	poll(t, p)
	assert.NotContains(t, reasonTexts(p.Section()), "a SteelSeries device answered nothing")
}

/*
R3.5. A paired child's quiet is not counted again.

A Unifying receiver's paired devices have nodes of their own, and each is asked
there as well as on the receiver's node, which asked every index. The Unifying
machine reported "8 indices" on a receiver that numbers six.
*/
func TestAChildNodesQuietIsNotCountedTwice(t *testing.T) {
	rx := receiver()
	rx.phys = "usb-0000:00:14.0-3/input2"
	rx.presence = logitech.Presence{Nodes: 1, Quiet: 2}
	child := receiver()
	child.name, child.path, child.phys = "Logitech K800", "/dev/hidraw7", "usb-0000:00:14.0-3/input2:1"
	child.presence = logitech.Presence{Nodes: 1, Quiet: 1}
	p := (&desk{devices: []*peripheral{rx, child}}).section(nil)

	poll(t, p)

	r := find(t, p.Section(), "a Logitech receiver, with nothing awake on it")
	assert.True(t, strings.HasPrefix(r.Detail, "2 indices"), r.Detail)
}
