package peripherals

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

/*
The four bands decode, and nothing else does.

The register answers one of four values and the mapping is solaar's, checked
against a K800 that answered 0x05. A value outside them is not the nearest
band: the whole point of a band is that it is the device's own word, so an
unrecognised one is no reading rather than a guess (spec 018).
*/
func TestTheFourBandsDecodeAndNothingElseDoes(t *testing.T) {
	for value, want := range map[byte]Band{
		0x01: BandCritical, 0x03: BandLow, 0x05: BandGood, 0x07: BandFull,
	} {
		e := &fakeEndpoint{respond: registerReply(registerBatteryStatus, value, 0x00)}

		b, err := readStatusRegister(e, testTimeout, 1)

		require.NoError(t, err, "value %#02x", value)
		assert.Equal(t, want, b.Band)
		assert.True(t, b.HasBand)
		assert.False(t, b.HasLevel, "a band is never also a percentage")
		assert.Zero(t, b.Level)
	}

	for _, value := range []byte{0x00, 0x02, 0x04, 0x06, 0x08, 0xFF} {
		e := &fakeEndpoint{respond: registerReply(registerBatteryStatus, value, 0x00)}

		_, err := readStatusRegister(e, testTimeout, 1)

		require.Error(t, err, "value %#02x was decoded as a band", value)
	}
}

// The K800's own answer, byte for byte: good, and discharging.
func TestTheKeyboardsOwnReply(t *testing.T) {
	e := &fakeEndpoint{respond: registerReply(registerBatteryStatus, 0x05, 0x00, 0x00)}

	b, err := readStatusRegister(e, testTimeout, 1)

	require.NoError(t, err)
	assert.Equal(t, BandGood, b.Band)
	assert.Equal(t, 3, b.Band.Segments())
	assert.Equal(t, "Good", b.Band.String())
	assert.Equal(t, Discharging, b.State)
}

// A charge byte that is not zero is a device on the cable.
func TestANonZeroChargeByteIsCharging(t *testing.T) {
	e := &fakeEndpoint{respond: registerReply(registerBatteryStatus, 0x05, 0x21)}

	b, err := readStatusRegister(e, testTimeout, 1)

	require.NoError(t, err)
	assert.Equal(t, Charging, b.State)
}

// A device with a real gauge is read as one, and its percentage is preferred
// to any band.
func TestAGaugeIsPreferredToABand(t *testing.T) {
	e := &fakeEndpoint{respond: registerReply(registerBatteryCharge, 72, 0x00)}

	b, err := readOldBattery(e, testTimeout, 1)

	require.NoError(t, err)
	assert.Equal(t, 72, b.Level)
	assert.True(t, b.HasLevel)
	assert.False(t, b.HasBand, "a percentage is not also a band")
}

/*
Only getters are sent.

Sub-id 0x81 reads a register and 0x80 writes one. A package that has just
learned to speak an older protocol to somebody's ten-year-old keyboard should
be provably incapable of setting anything on it.
*/
func TestOnlyGettersAreSent(t *testing.T) {
	var sent []byte
	e := &fakeEndpoint{respond: func(req []byte) [][]byte {
		sent = append(sent, req[2])
		return registerReply(registerBatteryStatus, 0x05, 0x00)(req)
	}}

	_, err := readOldBattery(e, testTimeout, 1)
	require.NoError(t, err)

	require.NotEmpty(t, sent)
	for _, sub := range sent {
		assert.Equal(t, byte(subGetRegister), sub, "something other than a register read was sent")
	}
}

// registerReply answers a register read with the given parameters, and refuses
// any other register the way a device without it does.
func registerReply(reg byte, params ...byte) func([]byte) [][]byte {
	return func(req []byte) [][]byte {
		if req[3] != reg {
			return [][]byte{{reportShort, req[1], 0x8F, req[2], req[3], err10InvalidAddress, 0x00}}
		}
		r := make([]byte, 7)
		r[0], r[1], r[2], r[3] = reportShort, req[1], subGetRegister, reg
		copy(r[4:], params)
		return [][]byte{r}
	}
}

// testTimeout is short: every device here is a fake and answers at once.
const testTimeout = 50 * time.Millisecond
