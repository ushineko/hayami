package peripherals

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

/*
A name shorter than the device said it would be is refused.

The fault: a battery level of 81 reached the name read as a one-byte payload,
`0x51` is printable, and the panel drew a peripheral called "Q" beside the
mouse it had been read from — at the same percentage, and as confidently as the
real name next to it (issue #58).

Driven through the real reader with a receiver that declares a name of eleven
characters and hands back one, because the length check is the thing under test
and a fake that returned a whole name would exercise nothing.
*/
func TestANameShorterThanTheDeviceDeclaredIsRefused(t *testing.T) {
	root := withTree(t)
	node(t, root, "hidraw3", "046D", descriptorHIDPP)

	l := &Logitech{
		timeout: 50 * time.Millisecond,
		open: func(string) (endpoint, error) {
			return &fakeEndpoint{respond: truncatedName(1, 0x51, "G502 X PLUS")}, nil
		},
	}

	found, err := l.Batteries()

	require.NoError(t, err)
	require.NotEmpty(t, found)
	for _, b := range found {
		assert.Equal(t, defaultName, b.Name,
			"a stray byte was drawn as a device name")
		assert.NotEqual(t, "Q", b.Name)
	}
}

// And a name that arrives whole is still read, so the check above cannot be
// satisfied by refusing every name.
func TestANameThatArrivesWholeIsStillRead(t *testing.T) {
	root := withTree(t)
	node(t, root, "hidraw3", "046D", descriptorHIDPP)

	l := &Logitech{
		timeout: 50 * time.Millisecond,
		open: func(string) (endpoint, error) {
			return &fakeEndpoint{respond: receiver(1, 0x56, "G502 X PLUS")}, nil
		},
	}

	found, err := l.Batteries()

	require.NoError(t, err)
	require.NotEmpty(t, found)
	assert.Equal(t, "G502 X PLUS", found[0].Name)
}

/*
The software ID differs between processes.

HID++ reserves four bits of every request so concurrent clients can tell their
replies apart, and hayami is routinely several processes at once: the panel
polls the receiver every fifteen seconds while `readings`, a pane or `doctor`
asks the same node. A constant made those four bits useless between them, which
is how one process's battery reply could be read as another's name.

It is taken from the process ID, so what is asserted is the range — a value of
zero would mark a request as belonging to no software at all, and anything
above fifteen would not fit the nibble and would corrupt the function.
*/
func TestTheSoftwareIDIsInRangeAndNotZero(t *testing.T) {
	assert.GreaterOrEqual(t, softwareID, byte(1), "zero marks a request as nobody's")
	assert.LessOrEqual(t, softwareID, byte(15), "the software ID has to fit a nibble")
}

// And it follows this process, so two processes differ wherever their PIDs do.
func TestTheSoftwareIDFollowsTheProcess(t *testing.T) {
	assert.Equal(t, softwareIDs[os.Getpid()%len(softwareIDs)], softwareID)
}

/*
Solaar's own software ID is never used.

It is 0x0B — SOLAAR_SOFTWARE_ID in logitech_receiver/base.py — and solaar is
the program most likely to be talking to the same receiver. Picking freely from
1..15 landed on it one run in fifteen, and the live comparison caught the
consequence: hayami read 71 % in the same second solaar read 79 %, which is a
reply belonging to somebody else rather than a battery moving.
*/
func TestSolaarsSoftwareIDIsNeverUsed(t *testing.T) {
	const solaar = 0x0B

	assert.NotContains(t, softwareIDs, byte(solaar))
	assert.NotContains(t, softwareIDs, byte(0x00), "zero marks a request as nobody's")
	assert.Len(t, softwareIDs, 14, "every other value a nibble can hold")
}

/*
truncatedName is a receiver that declares a name of `declared` characters and
answers the first chunk with a single byte, as a reply from another
conversation would.
*/
func truncatedName(stray int, value byte, declared string) func([]byte) [][]byte {
	const (
		rootFeature = 0x00
		featureIdx  = 0x06
		nameIdx     = 0x07
		typeMouse   = 0x03
	)
	return func(req []byte) [][]byte {
		device, feature, function := req[1], req[2], req[3]
		if device != 1 {
			return [][]byte{{reportShort, device, 0x8F, feature, function, err10UnknownDevice, 0x00}}
		}

		switch {
		case feature == rootFeature:
			switch uint16(req[4])<<8 | uint16(req[5]) {
			case featureUnifiedBattery:
				return [][]byte{reportOf(reportShort, device, feature, function, featureIdx)}
			case featureDeviceName:
				return [][]byte{reportOf(reportShort, device, feature, function, nameIdx)}
			default:
				return [][]byte{reportOf(reportShort, device, feature, function, 0x00)}
			}

		case feature == featureIdx:
			return [][]byte{reportOf(reportLong, device, feature, function, value, 0x08, 0x00, 0x00)}

		case feature == nameIdx && function>>4 == functionDeviceType:
			return [][]byte{reportOf(reportShort, device, feature, function, typeMouse)}

		case feature == nameIdx && function>>4 == functionDeviceName:
			return [][]byte{reportOf(reportShort, device, feature, function, byte(len(declared)))}

		case feature == nameIdx && function>>4 == functionDeviceNameChunk:
			// One byte where a chunk was asked for: the shape a reply from
			// somebody else's request arrives in.
			short := make([]byte, stray)
			short[0] = value
			return [][]byte{reportOf(reportShort, device, feature, function, short...)}
		}
		return nil
	}
}

// A stray byte followed by the report's own padding is the exact shape the
// phantom arrived in: long enough to fill the declared length, and full of
// holes. Counting bytes alone would have let it through.
func TestAStrayByteAndPaddingIsNotAName(t *testing.T) {
	name, whole := printableName([]byte{0x51, 0x00, 0x00, 0x51, 0x00, 0x00})

	assert.Equal(t, "QQ", name, "the printable bytes are still recovered")
	assert.False(t, whole, "a run with holes in it was accepted as a name")
}

// A real name is whole, and a device whose name ends in a space keeps it.
func TestARealNameIsWholeAndKeepsItsShape(t *testing.T) {
	name, whole := printableName([]byte("G502 X PLUS"))

	assert.Equal(t, "G502 X PLUS", name)
	assert.True(t, whole)
}
