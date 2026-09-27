package peripherals

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeEndpoint is a hidraw node that is not one.
//
// It answers a written request with whatever the test's responder gives back,
// and a request nothing answers costs the deadline, as a sleeping device does.
// The suite touches no hardware; the one test that does is in live_test.go and
// skips where there is none.
type fakeEndpoint struct {
	respond  func(req []byte) [][]byte
	pending  [][]byte
	deadline time.Time
	writes   [][]byte
}

func (f *fakeEndpoint) Write(p []byte) (int, error) {
	req := append([]byte(nil), p...)
	f.writes = append(f.writes, req)
	if f.respond != nil {
		f.pending = append(f.pending, f.respond(req)...)
	}
	return len(p), nil
}

func (f *fakeEndpoint) Read(p []byte) (int, error) {
	if len(f.pending) == 0 {
		if d := time.Until(f.deadline); d > 0 {
			time.Sleep(d)
		}
		return 0, os.ErrDeadlineExceeded
	}
	r := f.pending[0]
	f.pending = f.pending[1:]
	return copy(p, r), nil
}

func (f *fakeEndpoint) SetReadDeadline(t time.Time) error {
	f.deadline = t
	return nil
}

// reportOf builds a reply of a given report kind: the kind, the device, the
// feature, the function byte and the parameters.
func reportOf(kind, device, feature, function byte, params ...byte) []byte {
	width := 7
	if kind == reportLong {
		width = 20
	}
	r := make([]byte, width)
	r[0], r[1], r[2], r[3] = kind, device, feature, function
	copy(r[4:], params)
	return r
}

// AC1. The bytes the real G502 X PLUS answered with, decoded.
func TestAUnifiedBatteryReplyDecodesToTheLevelTheDeviceReported(t *testing.T) {
	// 0x56 is 86, which is what solaar reports for the same device at the
	// same moment; 0x08 is the "full" level band and 0x00 is discharging.
	b, err := decodeUnifiedBattery([]byte{0x56, 0x08, 0x00, 0x00})
	require.NoError(t, err)

	assert.True(t, b.HasLevel)
	assert.Equal(t, 86, b.Level)
	assert.Equal(t, Discharging, b.State)
}

// AC1. Charging and charged are told apart, because a device on its cable that
// has finished is not a device still filling.
func TestAUnifiedBatteryReplyTellsChargingFromCharged(t *testing.T) {
	charging, err := decodeUnifiedBattery([]byte{0x32, 0x04, 0x01, 0x01})
	require.NoError(t, err)
	assert.Equal(t, Charging, charging.State)

	slow, err := decodeUnifiedBattery([]byte{0x32, 0x04, 0x02, 0x01})
	require.NoError(t, err)
	assert.Equal(t, Charging, slow.State)

	done, err := decodeUnifiedBattery([]byte{0x64, 0x08, 0x03, 0x01})
	require.NoError(t, err)
	assert.Equal(t, Full, done.State)
}

// AC1. **A short request may be answered with a long report.** The real device
// answers 0x1004 with 0x11, and a reader that matched on the report ID would
// drop it and time out — which looks exactly like absent hardware.
func TestAReplyArrivingAsALongReportDecodesTheSameAsAShortOne(t *testing.T) {
	const (
		device  = 0x01
		feature = 0x06
	)
	function := byte(0x01<<4 | softwareID)

	for _, kind := range []struct {
		name  string
		value byte
	}{
		{"short", reportShort},
		{"long", reportLong},
	} {
		t.Run(kind.name, func(t *testing.T) {
			e := &fakeEndpoint{respond: func([]byte) [][]byte {
				return [][]byte{reportOf(kind.value, device, feature, function, 0x56, 0x08, 0x00, 0x00)}
			}}

			params, err := request(e, time.Second, device, feature, 0x01)
			require.NoError(t, err)

			b, err := decodeUnifiedBattery(params)
			require.NoError(t, err)
			assert.Equal(t, 86, b.Level)
		})
	}
}

// AC1. A reply too short to hold a reading is an error, not a zero. Absent is
// not zero is a rule this repository has been bitten by three times.
func TestATruncatedBatteryReplyIsAnErrorRatherThanAZero(t *testing.T) {
	_, err := decodeUnifiedBattery([]byte{0x56})
	require.Error(t, err)

	_, err = decodeBatteryStatus([]byte{0x56, 0x00})
	require.Error(t, err)
}

// AC1. Somebody else's traffic on the node is waited through rather than
// mistaken for an answer. The mouse sends notifications unasked.
func TestANotificationOnTheNodeIsNotMistakenForAReply(t *testing.T) {
	const (
		device  = 0x01
		feature = 0x06
	)
	function := byte(0x01<<4 | softwareID)

	e := &fakeEndpoint{respond: func([]byte) [][]byte {
		return [][]byte{
			// A notification: the device's own, with no software ID.
			reportOf(reportShort, device, 0x00, 0x00, 0xFF, 0xFF, 0xFF),
			reportOf(reportShort, device, feature, function, 0x2A, 0x04, 0x00, 0x00),
		}
	}}

	params, err := request(e, time.Second, device, feature, 0x01)
	require.NoError(t, err)

	b, err := decodeUnifiedBattery(params)
	require.NoError(t, err)
	assert.Equal(t, 42, b.Level)
}
