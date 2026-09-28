package peripherals

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// receiver answers as a Logitech receiver does: the device at liveIndex has a
// battery, a name and a device type, and every other index is refused.
//
// The type is a mouse, because the device these fakes are named after is one.
// A receiver that did not answer the type at all is the deviceType case in
// kind_test.go.
//
// The refusals are the point. The two error forms are answered differently by
// design — the 1.0 form for an empty index, which is what the real receiver
// sends, and the 2.0 form for a device that exists without the feature — and a
// reader that recognised only one of them would wait out its timeout instead.
func receiver(liveIndex byte, level byte, name string) func([]byte) [][]byte {
	const featureIdx = 0x06
	const nameIdx = 0x07

	return func(req []byte) [][]byte {
		device, feature, function := req[1], req[2], req[3]

		if device != liveIndex {
			// HID++ 1.0: sub-id 0x8F, the failing sub-id and address, then
			// "unknown device".
			return [][]byte{{reportShort, device, 0x8F, feature, function, err10UnknownDevice, 0x00}}
		}

		switch {
		case feature == rootFeature:
			wanted := uint16(req[4])<<8 | uint16(req[5])
			switch wanted {
			case featureUnifiedBattery:
				return [][]byte{reportOf(reportShort, device, feature, function, featureIdx)}
			case featureDeviceName:
				return [][]byte{reportOf(reportShort, device, feature, function, nameIdx)}
			default:
				// A feature this device does not have: index zero.
				return [][]byte{reportOf(reportShort, device, feature, function, 0x00)}
			}

		case feature == featureIdx:
			return [][]byte{reportOf(reportLong, device, feature, function, level, 0x08, 0x00, 0x00)}

		case feature == nameIdx && function>>4 == functionDeviceType:
			return [][]byte{reportOf(reportShort, device, feature, function, typeMouse)}

		case feature == nameIdx && function>>4 == 0x00:
			return [][]byte{reportOf(reportShort, device, feature, function, byte(len(name)))}

		case feature == nameIdx && function>>4 == 0x01:
			offset := int(req[4])
			chunk := make([]byte, 16)
			copy(chunk, name[min(offset, len(name)):])
			return [][]byte{reportOf(reportLong, device, feature, function, chunk...)}
		}

		return [][]byte{{reportShort, device, 0xFF, feature, function, err20UnsupportedFeature, 0x00}}
	}
}

// AC3. Every index is asked once, the one device is found, and no index costs
// the timeout — because both error forms end a request.
func TestDiscoveryFindsTheOneDeviceWithoutWaitingOnTheEmptyIndices(t *testing.T) {
	root := withTree(t)
	node(t, root, "hidraw12", "046D", descriptorHIDPP)

	// A timeout long enough to be obvious if it were ever reached: seven
	// indices at this would be seven seconds, and the test would not finish
	// inside its own budget below.
	l := &Logitech{
		timeout: time.Second,
		open: func(string) (endpoint, error) {
			return &fakeEndpoint{respond: receiver(1, 0x56, "G502 X PLUS")}, nil
		},
	}

	started := time.Now()
	found, err := l.Batteries()
	require.NoError(t, err)
	assert.Less(t, time.Since(started), 500*time.Millisecond,
		"discovery waited on a refusal instead of reading it")

	require.Len(t, found, 1)
	assert.Equal(t, "G502 X PLUS", found[0].Name)
	assert.Equal(t, 86, found[0].Level)
	assert.True(t, found[0].HasLevel)
	assert.Equal(t, KindMouse, found[0].Kind, "the device type came back as something else")
}

// AC3. An endpoint that answers nothing at all is bounded by the timeout
// rather than waiting forever. A paired device that is asleep does this.
func TestAnEndpointThatAnswersNothingIsBoundedByTheTimeout(t *testing.T) {
	root := withTree(t)
	node(t, root, "hidraw12", "046D", descriptorHIDPP)

	const timeout = 20 * time.Millisecond
	l := &Logitech{
		timeout: timeout,
		open: func(string) (endpoint, error) {
			return &fakeEndpoint{respond: func([]byte) [][]byte { return nil }}, nil
		},
	}

	started := time.Now()
	found, err := l.Batteries()
	require.NoError(t, err)
	assert.Empty(t, found)

	// Seven indices, each bounded by the timeout, and nothing beyond that.
	assert.Less(t, time.Since(started), 7*timeout+time.Second)
}

// AC3. The index that answered is remembered, so the second poll asks it
// directly instead of walking every index again.
func TestASecondPollAsksOnlyTheIndexTheFirstOneFound(t *testing.T) {
	root := withTree(t)
	node(t, root, "hidraw12", "046D", descriptorHIDPP)

	var opened []*fakeEndpoint
	l := &Logitech{
		timeout: time.Second,
		open: func(string) (endpoint, error) {
			e := &fakeEndpoint{respond: receiver(1, 0x56, "G502 X PLUS")}
			opened = append(opened, e)
			return e, nil
		},
	}

	_, err := l.Batteries()
	require.NoError(t, err)

	before := len(opened)
	_, err = l.Batteries()
	require.NoError(t, err)

	// The second poll opened exactly one endpoint — the node it already knew —
	// rather than reopening to discover.
	require.Len(t, opened, before+1)

	second := opened[len(opened)-1]
	for _, req := range second.writes {
		assert.Equal(t, byte(1), req[1], "the second poll asked an index it had not found")
	}
}

// AC3. A device that has gone is not remembered. The next poll rediscovers
// rather than asking an index that is no longer there for ever.
func TestADeviceThatStopsAnsweringIsNotRemembered(t *testing.T) {
	root := withTree(t)
	node(t, root, "hidraw12", "046D", descriptorHIDPP)

	live := true
	l := &Logitech{
		timeout: 20 * time.Millisecond,
		open: func(string) (endpoint, error) {
			if !live {
				return &fakeEndpoint{respond: func([]byte) [][]byte { return nil }}, nil
			}
			return &fakeEndpoint{respond: receiver(1, 0x56, "G502 X PLUS")}, nil
		},
	}

	found, err := l.Batteries()
	require.NoError(t, err)
	require.Len(t, found, 1)

	live = false
	found, err = l.Batteries()
	require.NoError(t, err)
	assert.Empty(t, found)
	assert.Empty(t, l.known)
}

// AC1. A device with no fuel gauge falls back to the older feature rather than
// disappearing.
func TestADeviceWithoutAFuelGaugeIsReadThroughTheOlderFeature(t *testing.T) {
	const legacyIdx = 0x05

	respond := func(req []byte) [][]byte {
		device, feature, function := req[1], req[2], req[3]
		if device != 1 {
			return [][]byte{{reportShort, device, 0x8F, feature, function, err10UnknownDevice, 0x00}}
		}
		if feature == rootFeature {
			switch uint16(req[4])<<8 | uint16(req[5]) {
			case featureBatteryStatus:
				return [][]byte{reportOf(reportShort, device, feature, function, legacyIdx)}
			default:
				// Neither the unified battery nor a name: index zero.
				return [][]byte{reportOf(reportShort, device, feature, function, 0x00)}
			}
		}
		if feature == legacyIdx {
			// 50 %, recharging.
			return [][]byte{reportOf(reportShort, device, feature, function, 0x32, 0x00, 0x01)}
		}
		return [][]byte{{reportShort, device, 0xFF, feature, function, err20UnsupportedFeature, 0x00}}
	}

	root := withTree(t)
	node(t, root, "hidraw12", "046D", descriptorHIDPP)

	l := &Logitech{
		timeout: time.Second,
		open:    func(string) (endpoint, error) { return &fakeEndpoint{respond: respond}, nil },
	}

	found, err := l.Batteries()
	require.NoError(t, err)
	require.Len(t, found, 1)
	assert.Equal(t, 50, found[0].Level)
	assert.Equal(t, Charging, found[0].State)
	// It would not say what it is called, so it is labelled rather than blank.
	assert.Equal(t, defaultName, found[0].Name)
}
