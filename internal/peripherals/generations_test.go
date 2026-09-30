package peripherals

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

/*
The two HID++ error spaces are read apart.

They arrive through the same reply and are told apart only by where in it they
sit — sub-id 0x8F for 1.0, feature index 0xFF for 2.0 — and they overlap. One
table read them both, so 0x01 from a ten-year-old keyboard that does not speak
2.0 at all was taken for "this device has no such feature", and 0x09 from a
receiver slot with nothing behind it was taken for a failed connection. The
panel then said "no Logitech receiver" about a receiver with a keyboard on it
(issue #66).
*/
func TestTheTwoErrorSpacesAreReadApart(t *testing.T) {
	// 0x01 is the collision: unsupported feature in 2.0, invalid sub-id in 1.0.
	assert.ErrorIs(t, translate20(0x01), errUnknownFeature)
	assert.ErrorIs(t, translate10(0x01), errOldProtocol)

	// 0x09 is a paired slot that is not answering, not a connection failure.
	assert.ErrorIs(t, translate10(0x09), errNotReachable)
	assert.ErrorIs(t, translate10(0x07), errNotReachable, "busy is not answering either")
	assert.ErrorIs(t, translate10(0x04), errNotReachable, "connect fail is the same to a reader")

	// And an index with nothing paired to it at all stays what it was.
	assert.ErrorIs(t, translate10(0x08), errNoDevice)
}

// A code neither space names is a failure rather than an answer, in both.
func TestAnUnknownCodeIsStillAFailure(t *testing.T) {
	assert.NotErrorIs(t, translate10(0x7F), errNoDevice)
	assert.NotErrorIs(t, translate20(0x7F), errUnknownFeature)
}

/*
A name is taken from a device's own node, never from the receiver's.

Two exclusions, and the hardware taught both. Index 0xFF addresses the thing
being spoken to, and a Unifying receiver answers a 2.0 root request with
"invalid sub-id" because it *is* a 1.0 device — always true, never worth
saying. And a receiver node answers for every device paired to it, so an answer
arriving there carries the receiver's name: the K800 was reported twice, once
correctly and once as "Logitech USB Receiver".

`hid-logitech-dj` appends `:index` to a child's HID_PHYS, which is how the two
are told apart.
*/
func TestOnlyADevicesOwnNodeLendsItAName(t *testing.T) {
	root := withTree(t)
	// The receiver, and the node the kernel makes for the keyboard on it.
	physNode(t, root, "hidraw1", "046D", "C52B", "Logitech USB Receiver",
		"usb-0000:03:00.0-3/input2", descriptorHIDPP)
	physNode(t, root, "hidraw7", "046D", "2010", "Logitech K800",
		"usb-0000:03:00.0-3/input2:1", descriptorHIDPP)

	l := &Logitech{
		timeout: 50 * time.Millisecond,
		open: func(string) (endpoint, error) {
			return &fakeEndpoint{respond: func(req []byte) [][]byte {
				// Everything, on every index: invalid sub-id.
				return [][]byte{{reportShort, req[1], 0x8F, req[2], req[3], err10InvalidSubID, 0x00}}
			}}, nil
		},
	}

	_, err := l.Batteries()
	require.NoError(t, err)

	assert.Equal(t, []string{"Logitech K800"}, l.Presence().TooOld,
		"a device is named from its own node and not from the receiver's")
	assert.Equal(t, 2, l.Presence().Nodes)
}

// The rule itself, against the shapes the kernel actually writes.
func TestPairedDeviceIsToldFromTheReceiver(t *testing.T) {
	assert.False(t, pairedDevice("usb-0000:03:00.0-3/input2"), "the receiver")
	assert.True(t, pairedDevice("usb-0000:03:00.0-3/input2:1"), "a device on it")
	assert.True(t, pairedDevice("usb-0000:03:00.0-3/input2:2"))
	assert.False(t, pairedDevice("usb-0000:00:14.0-7.4/input0"), "an ordinary interface")
	assert.False(t, pairedDevice(""))
}
