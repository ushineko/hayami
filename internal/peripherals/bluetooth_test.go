package peripherals

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// reader builds a Bluetooth over a fixed device list and a channel the test
// controls, so neither the system bus nor a radio is touched.
func reader(devices []BluetoothDevice, open func(addr [6]byte, timeout time.Duration) (channel, error)) *Bluetooth {
	return &Bluetooth{
		devices: func() ([]BluetoothDevice, error) { return devices, nil },
		dial:    open,
		timeout: 50 * time.Millisecond,
	}
}

// answering is a device that completes the exchange with the given cells.
func answering(t *testing.T, hexPacket string) func([6]byte, time.Duration) (channel, error) {
	t.Helper()
	return func([6]byte, time.Duration) (channel, error) {
		return &fakeChannel{replies: [][]byte{
			packet(t, "01000400000001"),
			packet(t, "040004002b000144"),
			packet(t, hexPacket),
		}}, nil
	}
}

// refusing is a device that will not open a channel, which is what AirPods in
// a pocket do.
func refusing([6]byte, time.Duration) (channel, error) {
	return nil, errors.New("connection refused")
}

// AC5, AC7. A device BlueZ has a battery for is a row with its own name.
func TestABluezBatteryIsARowWithTheDevicesName(t *testing.T) {
	b := reader([]BluetoothDevice{
		{Name: "A Keyboard", Address: "00:11:22:33:44:55", Level: 54, HasLevel: true},
	}, refusing)

	found, err := b.Batteries()
	require.NoError(t, err)
	require.Len(t, found, 1)

	assert.Equal(t, "A Keyboard", found[0].Name)
	assert.Equal(t, 54, found[0].Level)
	assert.True(t, found[0].HasLevel)
	assert.Empty(t, found[0].Cells, "a single-battery device has no cells to draw")
}

// AC7. A connected device with no battery anywhere is not a row.
//
// Most connected Bluetooth devices are in this position — a phone, a car, a
// speaker — and a panel listing them all at "--" would be a list of everything
// paired rather than a reading.
func TestADeviceWithNoBatteryAnywhereIsNotARow(t *testing.T) {
	b := reader([]BluetoothDevice{
		{Name: "A Car", Address: "00:11:22:33:44:55"},
	}, refusing)

	found, err := b.Batteries()
	require.NoError(t, err)
	assert.Empty(t, found)
}

// AC6. A device that has both is drawn once, from the accessory protocol,
// because that is the reading with the cells in it.
func TestADeviceWithBothIsDrawnOnceFromTheAccessoryProtocol(t *testing.T) {
	b := reader([]BluetoothDevice{{
		Name: "Some AirPods", Address: "00:11:22:33:44:55",
		Apple: true, Audio: true,
		Level: 42, HasLevel: true, // BlueZ also has an opinion
	}}, answering(t, airpodsPacket))

	found, err := b.Batteries()
	require.NoError(t, err)
	require.Len(t, found, 1)

	assert.Equal(t, 80, found[0].Level, "the BlueZ percentage won over the accessory protocol")
	assert.Len(t, found[0].Cells, 2)
}

// AC6. When the accessory protocol will not answer but BlueZ has a level, the
// level is drawn. Better a percentage than no row.
func TestAFailedAccessoryReadFallsBackToTheBluezLevel(t *testing.T) {
	b := reader([]BluetoothDevice{{
		Name: "Some AirPods", Address: "00:11:22:33:44:55",
		Apple: true, Audio: true,
		Level: 42, HasLevel: true,
	}}, refusing)

	found, err := b.Batteries()
	require.NoError(t, err)
	require.Len(t, found, 1)
	assert.Equal(t, 42, found[0].Level)
}

// A device that answers neither is not a row, and not a failure of the
// section: AirPods in a pocket decline the channel, and that is Tuesday.
func TestAnAppleDeviceThatAnswersNeitherIsNotAFailure(t *testing.T) {
	b := reader([]BluetoothDevice{{
		Name: "Some AirPods", Address: "00:11:22:33:44:55",
		Apple: true, Audio: true,
	}}, refusing)

	found, err := b.Batteries()
	assert.Empty(t, found)
	// The refusal is reported once, but it is not ErrNoBluez and it does not
	// stop the other devices being read.
	assert.Error(t, err)
}

// AC5. A device that is not Apple audio is never dialled. Opening an L2CAP
// channel to a keyboard is a thing to not do.
func TestOnlyAppleAudioDevicesAreDialled(t *testing.T) {
	dialled := 0
	open := func(addr [6]byte, d time.Duration) (channel, error) {
		dialled++
		return refusing(addr, d)
	}

	b := reader([]BluetoothDevice{
		{Name: "A Keyboard", Address: "00:11:22:33:44:55", Level: 54, HasLevel: true},
		{Name: "An Apple Phone", Address: "00:11:22:33:44:66", Apple: true},
		{Name: "Some Speakers", Address: "00:11:22:33:44:77", Audio: true, Level: 20, HasLevel: true},
	}, open)

	_, err := b.Batteries()
	require.NoError(t, err)
	assert.Zero(t, dialled, "a device that is not Apple audio was dialled")
}

// One device refusing does not stop the others being read.
func TestOneAwkwardDeviceDoesNotStopTheRest(t *testing.T) {
	b := reader([]BluetoothDevice{
		{Name: "Some AirPods", Address: "00:11:22:33:44:55", Apple: true, Audio: true},
		{Name: "A Keyboard", Address: "00:11:22:33:44:66", Level: 54, HasLevel: true},
	}, refusing)

	found, _ := b.Batteries()
	require.Len(t, found, 1)
	assert.Equal(t, "A Keyboard", found[0].Name)
}

// AC8. No BlueZ at all is not a failure of the section.
func TestNoBluezIsNotAFailure(t *testing.T) {
	b := &Bluetooth{
		devices: func() ([]BluetoothDevice, error) { return nil, ErrNoBluez },
		dial:    refusing,
		timeout: time.Millisecond,
	}

	_, err := b.Batteries()
	require.ErrorIs(t, err, ErrNoBluez)
}
