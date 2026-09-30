package peripherals

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeReport is a control endpoint whose answers the test decides.
type fakeReport struct {
	// reply maps a command byte to what the device sends back. A command with
	// no entry is answered with silence, which is what a device that does not
	// know it does.
	reply map[byte][]byte

	// queued is what the next Read hands over, including packets that are not
	// answers to anything.
	queued [][]byte

	// asked records every command written, so a test can say what was sent.
	asked []byte

	closed bool
}

func (f *fakeReport) Write(b []byte) (int, error) {
	cmd := b[1]
	f.asked = append(f.asked, cmd)
	if answer, ok := f.reply[cmd]; ok {
		f.queued = append(f.queued, answer)
	}
	return len(b), nil
}

func (f *fakeReport) Read(b []byte) (int, error) {
	if len(f.queued) == 0 {
		return 0, errors.New("nothing to read")
	}
	next := f.queued[0]
	f.queued = f.queued[1:]
	return copy(b, next), nil
}

func (f *fakeReport) SetReadDeadline(time.Time) error { return nil }
func (f *fakeReport) Close() error                    { f.closed = true; return nil }

// withApex points the package at a tree holding one Apex control endpoint and
// returns a reader wired to the device the test supplies.
func withApex(t *testing.T, d *fakeReport) *SteelSeries {
	t.Helper()
	root := withTree(t)
	namedNode(t, root, "hidraw5", "1038", "1644", "SteelSeries Apex Pro TKL Wireless Gen 3", apexControl)
	return &SteelSeries{
		open:    func(string) (reportDevice, error) { return d, nil },
		timeout: 50 * time.Millisecond,
	}
}

/*
AC6. The battery is rivalcfg's command, and its level has a step of five.

0x95 is what the keyboard answered with its cable in: bit 7 charging, and
((0x95 & 0x7f) - 1) * 5 = 100. The arithmetic is the device's own resolution
and is asserted here so a future simplification to "the byte is a percentage"
cannot pass.
*/
func TestTheSteelSeriesBatteryIsReadFromTheValueByte(t *testing.T) {
	for _, c := range []struct {
		value byte
		level int
		state State
	}{
		{0x95, 100, Full},    // charging bit set, and already full
		{0x8d, 60, Charging}, // charging bit set, part way up
		{0x14, 95, Discharging},
		{0x15, 100, Discharging},
		{0x02, 5, Discharging},
	} {
		d := &fakeReport{reply: map[byte][]byte{batteryCommand: {batteryCommand, c.value}}}
		s := withApex(t, d)

		found, err := s.Batteries()

		require.NoError(t, err)
		require.Len(t, found, 1, "value %#02x gave no reading", c.value)
		assert.Equal(t, c.level, found[0].Level, "value %#02x", c.value)
		assert.Equal(t, c.state, found[0].State, "value %#02x", c.value)
		assert.Equal(t, "SteelSeries Apex Pro TKL Wireless Gen 3", found[0].Name)
	}
}

/*
AC6. A device that does not answer the wired form is asked the wireless one.

The two differ by rivalcfg's `_WIRELESS_FLAG`, and which one a device wants
depends on whether its keyboard is on the cable or behind the dongle -- which
this build cannot tell from the product ID, because that moves too. Asking
twice is cheaper than tracking it and being wrong.
*/
func TestTheWirelessFormIsAskedWhenTheWiredOneIsNotAnswered(t *testing.T) {
	d := &fakeReport{reply: map[byte][]byte{batteryCommand | wirelessFlag: {batteryCommand | wirelessFlag, 0x14}}}
	s := withApex(t, d)

	found, err := s.Batteries()

	require.NoError(t, err)
	require.Len(t, found, 1)
	assert.Equal(t, 95, found[0].Level)
	assert.Equal(t, []byte{batteryCommand, batteryCommand | wirelessFlag}, d.asked,
		"the wired form is asked first and the wireless one only after it fails")
}

/*
AC7. A packet that is not the answer to the question is not read as one.

This endpoint carries late replies to earlier questions: during this spec's
investigation a `d2 14` arrived while a different command was outstanding and
was written off as noise, which is how the battery was missed for an afternoon.
The echoed command byte is the whole of the addressing.
*/
func TestAStalePacketIsNotMistakenForTheAnswer(t *testing.T) {
	d := &fakeReport{
		reply:  map[byte][]byte{batteryCommand: {batteryCommand, 0x15}},
		queued: [][]byte{{0xf5, 0x00, 0x03}}, // a region reply still in flight
	}
	s := withApex(t, d)

	found, err := s.Batteries()

	require.NoError(t, err)
	require.Len(t, found, 1)
	assert.Equal(t, 100, found[0].Level, "the region reply was decoded as a battery")
}

// AC6. A value that cannot be a level is not turned into one.
func TestAnImpossibleValueIsNotAReading(t *testing.T) {
	for _, value := range []byte{0x00, 0x80, 0x7f} {
		d := &fakeReport{reply: map[byte][]byte{
			batteryCommand:                {batteryCommand, value},
			batteryCommand | wirelessFlag: {batteryCommand | wirelessFlag, value},
		}}
		s := withApex(t, d)

		found, err := s.Batteries()

		require.NoError(t, err)
		assert.Empty(t, found, "value %#02x was decoded as a level", value)
	}
}

// A machine with no SteelSeries device reads nothing and is not a failure.
func TestNoSteelSeriesDeviceIsNotAFailure(t *testing.T) {
	withTree(t)
	s := &SteelSeries{
		open:    func(string) (reportDevice, error) { return nil, errors.New("should not be opened") },
		timeout: time.Millisecond,
	}

	found, err := s.Batteries()

	require.NoError(t, err)
	assert.Empty(t, found)
}

// The node is closed however the read went, because a panel polls on a timer
// and a descriptor leaked every thirty seconds is a descriptor leaked.
func TestTheNodeIsClosedEvenWhenNothingAnswers(t *testing.T) {
	d := &fakeReport{}
	s := withApex(t, d)

	_, err := s.Batteries()

	require.NoError(t, err)
	assert.True(t, d.closed)
}
