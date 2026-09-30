package peripherals

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

/*
A SteelSeries product this build does not know is never written to.

The fault: nodes were found by vendor and usage page, which is broad, and then
one command family was spoken at whatever turned up. On the development machine
an Arctis Nova Pro Wireless matched — it exposes 0xFFC0 — and was being sent
0x92 on every poll, from a family it does not speak (issue #62). It never
answered, which is not the same as the command being safe.

The fake fails the test if it is written to at all, because "wrote and got
nothing back" is exactly the outcome that hid this.
*/
func TestAnUnknownSteelSeriesProductIsNeverWrittenTo(t *testing.T) {
	root := withTree(t)
	// 0x12E5 is the Arctis Nova Pro Wireless, which is real, matches the usage
	// page, and speaks a different protocol entirely.
	namedNode(t, root, "hidraw14", "1038", "12E5", "SteelSeries Arctis Nova Pro Wireless", apexControl)

	s := &SteelSeries{
		timeout: 50 * time.Millisecond,
		open: func(string) (reportDevice, error) {
			t.Fatal("a device whose protocol this build does not know was opened")
			return nil, nil
		},
	}

	found, err := s.Batteries()

	require.NoError(t, err, "an unknown device is not a failure")
	assert.Empty(t, found)
	assert.Equal(t, []string{"SteelSeries Arctis Nova Pro Wireless"}, s.Unsupported(),
		"a device found and left alone must be named, or it looks like a bug")
}

// Both of the Apex's product IDs reach the same protocol, because the ID moves
// with the connection and both were measured.
func TestBothOfTheApexProductIDsAreKnown(t *testing.T) {
	for _, product := range []uint64{0x1644, 0x1646} {
		assert.Equal(t, batteryModern, steelseriesBattery[product], "product %#04x", product)
	}
}

// A device rivalcfg names but this build cannot read is told apart from one it
// has never heard of: both are left alone, and only the shape of the table
// distinguishes them for whoever adds the protocol later.
func TestTheLegacyFamilyIsNamedAndNotSpokenTo(t *testing.T) {
	for _, product := range []uint64{0x1830, 0x1872, 0x172B} {
		assert.Equal(t, batteryLegacy, steelseriesBattery[product], "product %#04x", product)
	}
	assert.Nil(t, batteryLegacy.commands(),
		"a protocol this build cannot read must have no command to send")
	assert.Nil(t, batteryUnknown.commands())
}

/*
The Razer reader finds the transaction ID by trying, and then stops trying.

OpenRazer picks one per model — 0x3F for the older Mouse Dock and 0xFF for the
Dock Pro, while 0x1F is what reaches the mouse behind the Pro. A hardcoded ID
reads nothing on the next device and looks like absent hardware (issue #63).
*/
func TestTheTransactionIDIsFoundByTryingAndThenRemembered(t *testing.T) {
	const answers = 0x3F // the older Mouse Dock's, and not the first tried

	tried := []byte{}
	d := &fakeFeature{answer: func(byte, byte) ([]byte, bool) { return nil, false }}
	d.observe = func(req []byte) { tried = append(tried, req[2]) }
	d.answer = func(class, command byte) ([]byte, bool) {
		if d.lastTransaction != answers {
			return razerReply(statusTimeout), true
		}
		if class == classPower && command == commandBatteryLevel {
			return razerReply(statusOK, 0x00, 0xFF), true
		}
		return razerReply(statusOK, 0x00, 0x00), true
	}
	r := withDock(t, d)

	found, err := r.Batteries()
	require.NoError(t, err)
	require.Len(t, found, 1)
	assert.Equal(t, 100, found[0].Level)
	assert.Equal(t, []byte{0x1F, 0x3F}, tried[:2],
		"the list is tried in order until one answers")

	// A second poll asks once, on the ID that worked.
	tried = tried[:0]
	_, err = r.Batteries()
	require.NoError(t, err)
	assert.Equal(t, answers, int(tried[0]), "the remembered ID was not used first")
	assert.NotContains(t, tried, byte(0x1F), "the search ran again on a device that had answered")
}

/*
A device that goes quiet keeps the transaction ID it answered on.

busy and timeout are what a contended or sleeping Razer device says on the
*right* ID, several times an hour. Unlearning on those would turn an ordinary
quiet minute into a burst of writes while the list was searched again.
*/
func TestASilentPollDoesNotUnlearnTheTransactionID(t *testing.T) {
	tried := []byte{}
	d := &fakeFeature{}
	d.observe = func(req []byte) { tried = append(tried, req[2]) }
	d.answer = func(_, _ byte) ([]byte, bool) {
		if d.lastTransaction != 0x1F {
			return razerReply(statusTimeout), true
		}
		return razerReply(statusOK, 0x00, 0xFF), true
	}
	r := withDock(t, d)

	_, err := r.Batteries()
	require.NoError(t, err)

	// The mouse goes to sleep.
	d.answer = func(byte, byte) ([]byte, bool) { return razerReply(statusBusy), true }
	tried = tried[:0]
	found, err := r.Batteries()

	require.NoError(t, err)
	assert.Empty(t, found)
	assert.Equal(t, []byte{0x1F}, tried,
		"a quiet device made the reader search the whole list again")
}

// Kind comes from the product table, and an unmet product is KindOther rather
// than a guess.
func TestTheDeviceKindComesFromTheProductTable(t *testing.T) {
	assert.Equal(t, KindMouse, razerKinds[0x00A4], "Mouse Dock Pro")
	assert.Equal(t, KindMouse, razerKinds[0x007E], "the older Mouse Dock")
	assert.Equal(t, KindOther, razerKinds[0x0000], "a product this build has not met")
}
