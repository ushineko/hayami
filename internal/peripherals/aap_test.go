package peripherals

import (
	"encoding/hex"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The packet a real pair of AirPods Pro sent, with the levels changed so the
// three cells are told apart and nothing here is a recording of anybody's
// hardware: right 90, left 80, case not present.
//
//	04 00 04 00 04 00  prefix
//	03                 three cells
//	02 01 5a 02 01     right, 90, draining
//	04 01 50 02 01     left, 80, draining
//	08 01 00 04 01     case, 0, not present
const airpodsPacket = "040004000400030201" + "5a0201" + "0401500201" + "080100" + "0401"

func packet(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	require.NoError(t, err)
	return b
}

// AC1. A battery packet decodes to the cells it carries.
func TestAnAccessoryBatteryPacketDecodesToItsCells(t *testing.T) {
	cells, err := decodeAAPBattery(packet(t, airpodsPacket))
	require.NoError(t, err)

	// Two, not three: the case is not there and is not a cell.
	require.Len(t, cells, 2)
	assert.Equal(t, CellReading{Cell: Right, Level: 90}, cells[0])
	assert.Equal(t, CellReading{Cell: Left, Level: 80}, cells[1])
}

// AC2. A cell that is not there is not a cell at zero.
//
// The case reports level 0 with the "not on body" status when it is left on a
// desk, and a reader taking that at face value draws a flat battery for a
// thing in a drawer.
func TestACellThatIsNotPresentIsNotACellAtZero(t *testing.T) {
	cells, err := decodeAAPBattery(packet(t, airpodsPacket))
	require.NoError(t, err)

	for _, c := range cells {
		assert.NotEqual(t, Case, c.Cell, "an absent case was read as a reading")
	}
}

// AC1. Charging is carried per cell.
func TestACellOnTheCableSaysSo(t *testing.T) {
	// One cell: left, 50, charging.
	cells, err := decodeAAPBattery(packet(t, "040004000400"+"01"+"0401320101"))
	require.NoError(t, err)
	require.Len(t, cells, 1)
	assert.True(t, cells[0].Charging)
}

// AC3. A device that is one piece reports one cell, and that is its level with
// no per-cell line to draw.
func TestADeviceWithOneCellIsOneLevelAndNoDetail(t *testing.T) {
	cells, err := decodeAAPBattery(packet(t, "040004000400"+"01"+"0101400201"))
	require.NoError(t, err)

	b, err := batteryOf("Some Headphones", cells)
	require.NoError(t, err)

	assert.Equal(t, 64, b.Level)
	assert.True(t, b.HasLevel)
	require.Len(t, b.Cells, 1)
	assert.Equal(t, "headset", b.Cells[0].Cell.String())
}

// AC4. A packet that is not one, or is cut short, is an error rather than a
// partial reading.
func TestAMalformedBatteryPacketIsAnError(t *testing.T) {
	_, err := decodeAAPBattery(packet(t, "040004000900"+"01"+"0401320101"))
	require.Error(t, err, "a packet that is not a battery packet was decoded as one")

	// Promises three cells, carries one.
	_, err = decodeAAPBattery(packet(t, "040004000400"+"03"+"0401320101"))
	require.Error(t, err, "a truncated packet was decoded")
}

// AC1. A cell this build has never heard of is skipped, not guessed at, and is
// not an error. A future device with another battery in it is not a fault.
func TestAnUnknownCellIsSkippedRatherThanGuessedAt(t *testing.T) {
	cells, err := decodeAAPBattery(packet(t, "040004000400"+"02"+"0401320201"+"7f01630201"))
	require.NoError(t, err)
	require.Len(t, cells, 1)
	assert.Equal(t, Left, cells[0].Cell)
}

// The row's number is the lower ear.
//
// It is the one that will stop working first, which is what somebody glancing
// at a panel wants to know.
func TestTheRowsNumberIsTheLowerEar(t *testing.T) {
	b, err := batteryOf("AirPods", []CellReading{
		{Cell: Right, Level: 90},
		{Cell: Left, Level: 55},
	})
	require.NoError(t, err)
	assert.Equal(t, 55, b.Level)
}

// The case does not set the row's number.
//
// A case at 5 % while the ears are full is not a warning about anything the
// wearer is doing, and letting it set the number would make the panel shout
// about a thing in a drawer.
func TestTheCaseDoesNotSetTheRowsNumber(t *testing.T) {
	b, err := batteryOf("AirPods", []CellReading{
		{Cell: Left, Level: 80},
		{Cell: Right, Level: 90},
		{Cell: Case, Level: 5},
	})
	require.NoError(t, err)
	assert.Equal(t, 80, b.Level)
}

// The cells are drawn in a fixed order, not the order the device sent them.
// The AirPods this was written against report right before left.
func TestTheCellsAreOrderedForReadingAndNotAsSent(t *testing.T) {
	b, err := batteryOf("AirPods", []CellReading{
		{Cell: Case, Level: 50},
		{Cell: Right, Level: 90},
		{Cell: Left, Level: 80},
	})
	require.NoError(t, err)

	var order []string
	for _, c := range b.Cells {
		order = append(order, c.Cell.String())
	}
	assert.Equal(t, []string{"L", "R", "case"}, order)
}

// A device reporting nothing at all is not a row.
func TestNoCellsIsNotAReading(t *testing.T) {
	_, err := batteryOf("AirPods", nil)
	require.ErrorIs(t, err, ErrNoBatteryPacket)
}

// fakeChannel answers the exchange the way a device does, and can be told to
// say nothing at all.
type fakeChannel struct {
	sent    [][]byte
	replies [][]byte
	silent  bool
}

func (f *fakeChannel) Send(p []byte) error {
	f.sent = append(f.sent, append([]byte(nil), p...))
	return nil
}

func (f *fakeChannel) Receive(within time.Duration) ([]byte, error) {
	if f.silent {
		time.Sleep(within)
		return nil, errNoPacket
	}
	if len(f.replies) == 0 {
		return nil, errNoPacket
	}
	r := f.replies[0]
	f.replies = f.replies[1:]
	return r, nil
}

func (f *fakeChannel) Close() error { return nil }

// The exchange is walked through in order, and the chatter in between — the
// device has a great deal to say about its firmware and its serial numbers —
// is waited through rather than mistaken for an answer.
func TestTheExchangeIsWalkedThroughAndTheChatterIgnored(t *testing.T) {
	c := &fakeChannel{replies: [][]byte{
		packet(t, "01000400000001"),           // handshake ack
		packet(t, "040004000c00d443010101"),   // chatter
		packet(t, "040004002b000144000b0705"), // features ack
		packet(t, "0400040009000d03000000"),   // more chatter
		packet(t, airpodsPacket),
	}}

	cells, err := readAAP(c, 2*time.Second)
	require.NoError(t, err)
	require.Len(t, cells, 2)

	require.Len(t, c.sent, 3, "the exchange did not send all three of its steps")
	assert.Equal(t, aapHandshake, c.sent[0])
	assert.Equal(t, aapSetFeatures, c.sent[1])
	assert.Equal(t, aapNotifications, c.sent[2])
}

// AC8. A device that connects and never reports gives up at the deadline
// rather than holding the poll loop.
func TestADeviceThatNeverReportsGivesUpAtItsDeadline(t *testing.T) {
	c := &fakeChannel{silent: true}

	started := time.Now()
	_, err := readAAP(c, 50*time.Millisecond)

	require.ErrorIs(t, err, ErrNoBatteryPacket)
	assert.Less(t, time.Since(started), 2*time.Second)
}

// An address is read in the order it is written.
//
// SockaddrL2 reverses it on the way to the kernel. Reversing it here as well
// dials an address nothing answers on, and the kernel calls that ECONNREFUSED
// — which reads like the device declining rather than like a wrong number.
func TestAnAddressIsReadInTheOrderItIsWritten(t *testing.T) {
	// Six distinct bytes, none of them anybody's: the whole point is that the
	// order is preserved, so an address that read the same backwards would
	// assert nothing.
	addr, err := parseAddress("A1:B2:C3:D4:E5:F6")
	require.NoError(t, err)
	assert.Equal(t, [6]byte{0xA1, 0xB2, 0xC3, 0xD4, 0xE5, 0xF6}, addr)

	_, err = parseAddress("not an address")
	require.Error(t, err)

	_, err = parseAddress("A1:B2:C3:D4:E5:ZZ")
	require.Error(t, err)
}

// errNoPacket is the channel's own "nothing yet" and must not escape as a
// failure: the exchange waits through many of them.
func TestNothingYetIsNotAFailure(t *testing.T) {
	c := &fakeChannel{replies: [][]byte{packet(t, airpodsPacket)}}
	_, err := readAAP(c, time.Second)
	require.NoError(t, err)
	assert.False(t, errors.Is(err, errNoPacket))
}
