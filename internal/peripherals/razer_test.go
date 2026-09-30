package peripherals

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeFeature is a Razer node whose answers the test decides.
type fakeFeature struct {
	// answer builds the reply to one request, given the class and command it
	// carried. Returning false is a device that says nothing.
	answer func(class, command byte) ([]byte, bool)

	// sent records every request, so a test can say what was asked and with
	// which transaction ID.
	sent [][]byte

	// observe is called with each request as it arrives, for a test that
	// wants to watch the transaction IDs go by rather than pick them out
	// afterwards.
	observe func(request []byte)

	// lastTransaction is the transaction ID of the request being answered, so
	// a fake can behave like a device that answers on one and not the others.
	lastTransaction byte

	pending []byte
	closed  bool
}

func (f *fakeFeature) SetFeature(b []byte) error {
	f.sent = append(f.sent, append([]byte(nil), b...))
	body := b[1:]
	f.lastTransaction = body[1]
	if f.observe != nil {
		f.observe(b)
	}
	if reply, ok := f.answer(body[6], body[7]); ok {
		f.pending = reply
		return nil
	}
	f.pending = nil
	return nil
}

func (f *fakeFeature) GetFeature(b []byte) error {
	if f.pending == nil {
		return errors.New("nothing to read")
	}
	copy(b, f.pending)
	return nil
}

func (f *fakeFeature) Close() error { f.closed = true; return nil }

// razerReply builds a well-formed answer with a status and arguments, with its
// checksum computed the way the device computes it.
func razerReply(status byte, args ...byte) []byte {
	out := make([]byte, razerReportSize+1)
	body := out[1:]
	body[0] = status
	body[1] = relayTransaction
	body[5] = byte(len(args))
	copy(body[8:], args)
	body[razerReportSize-2] = razerCRC(body)
	return out
}

// withDock points the package at a tree holding one dock and returns a reader
// wired to the device the test supplies.
func withDock(t *testing.T, d *fakeFeature) *Razer {
	t.Helper()
	root := withTree(t)
	namedNode(t, root, "hidraw0", "1532", "00A4", "Razer Razer Mouse Dock Pro", razerDock)
	return &Razer{open: func(string) (featureDevice, error) { return d, nil }}
}

/*
AC3. The battery comes back from behind the dock, addressed by the relay.

The transaction ID is the whole of what makes this the mouse's answer rather
than the dock's: on the machine this was measured on, the serial that came back
at 0x1f was not the serial in the dock's own sysfs.
*/
func TestTheBatteryIsReadThroughTheRelay(t *testing.T) {
	d := &fakeFeature{answer: func(class, command byte) ([]byte, bool) {
		switch {
		case class == classPower && command == commandBatteryLevel:
			return razerReply(statusOK, 0x00, 0xFF), true
		case class == classPower && command == commandCharging:
			return razerReply(statusOK, 0x00, 0x00), true
		}
		return nil, false
	}}
	r := withDock(t, d)

	found, err := r.Batteries()

	require.NoError(t, err)
	require.Len(t, found, 1)
	assert.Equal(t, 100, found[0].Level, "0xFF is a full battery, not 99 %%")
	assert.Equal(t, Discharging, found[0].State)
	assert.Equal(t, KindMouse, found[0].Kind)
	assert.Equal(t, "Razer Mouse Dock Pro", found[0].Name)

	require.NotEmpty(t, d.sent)
	assert.Equal(t, byte(relayTransaction), d.sent[0][2],
		"the request was not addressed to the device behind the dock")
}

// AC3. The level is a byte over full scale, rounded rather than truncated.
func TestTheLevelIsAByteOverFullScale(t *testing.T) {
	for _, c := range []struct {
		raw   byte
		level int
	}{{0xFF, 100}, {0x80, 50}, {0x40, 25}, {0x00, 0}} {
		d := &fakeFeature{answer: func(class, command byte) ([]byte, bool) {
			if class == classPower && command == commandBatteryLevel {
				return razerReply(statusOK, 0x00, c.raw), true
			}
			return nil, false
		}}
		r := withDock(t, d)

		found, err := r.Batteries()

		require.NoError(t, err)
		require.Len(t, found, 1)
		assert.Equal(t, c.level, found[0].Level, "raw %#02x", c.raw)
	}
}

/*
AC4. A relay that is asleep, busy or empty is no reading and no error.

All three are ordinary on this hardware. The mouse sleeps on a five-minute idle
timer; the openrazer daemon polls the same node and collides; and a dock with
nothing paired to it answers that it does not support the question. A panel
that reported any of them as a fault would be reporting a fault most of the
day.
*/
func TestASleepingOrBusyRelayIsNotAFailure(t *testing.T) {
	for _, status := range []byte{statusBusy, statusTimeout, statusNotSupported} {
		d := &fakeFeature{answer: func(byte, byte) ([]byte, bool) {
			return razerReply(status, 0x00, 0x00), true
		}}
		r := withDock(t, d)

		found, err := r.Batteries()

		require.NoError(t, err, "status %#02x was reported as an error", status)
		assert.Empty(t, found, "status %#02x produced a reading", status)
	}
}

// AC5. A reply whose checksum does not match is not a reading. This is a radio
// link with a dock in the middle, and a corrupted level is a number a panel
// would draw without hesitating.
func TestAReplyWithABadChecksumIsNotAReading(t *testing.T) {
	d := &fakeFeature{answer: func(byte, byte) ([]byte, bool) {
		reply := razerReply(statusOK, 0x00, 0xFF)
		reply[razerReportSize-1] ^= 0xFF // the checksum, past the report number
		return reply, true
	}}
	r := withDock(t, d)

	found, err := r.Batteries()

	require.Error(t, err)
	assert.Empty(t, found)
	assert.Contains(t, err.Error(), "checksum")
}

// AC3. The request carries the checksum the device expects, computed over the
// addressed bytes and not over the whole report.
func TestTheRequestCarriesItsChecksum(t *testing.T) {
	req := razerReport(relayTransaction, classPower, commandBatteryLevel, 0x02)
	body := req[1:]

	var want byte
	for _, b := range body[2 : razerReportSize-2] {
		want ^= b
	}

	assert.Equal(t, want, body[razerReportSize-2])
	assert.Equal(t, byte(classPower), body[6])
	assert.Equal(t, byte(commandBatteryLevel), body[7])
	assert.Len(t, req, razerReportSize+1, "the report number is not part of the report")
}

// A device that answers the level and not the charge state still has a level
// worth drawing, and the missing answer must not read as "not charging".
func TestALevelWithoutAChargeStateIsStillAReading(t *testing.T) {
	d := &fakeFeature{answer: func(class, command byte) ([]byte, bool) {
		if class == classPower && command == commandBatteryLevel {
			return razerReply(statusOK, 0x00, 0x80), true
		}
		return nil, false
	}}
	r := withDock(t, d)

	found, err := r.Batteries()

	require.NoError(t, err)
	require.Len(t, found, 1)
	assert.Equal(t, 50, found[0].Level)
}

// A machine with no Razer hardware reads nothing and is not a failure.
func TestNoRazerDeviceIsNotAFailure(t *testing.T) {
	withTree(t)
	r := &Razer{open: func(string) (featureDevice, error) {
		return nil, errors.New("should not be opened")
	}}

	found, err := r.Batteries()

	require.NoError(t, err)
	assert.Empty(t, found)
}
