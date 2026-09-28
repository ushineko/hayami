package peripherals

import (
	"testing"

	"github.com/godbus/dbus/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// AC16. HID++ device types map onto the kinds the panel orders by.
//
// The mapping is the only thing in this program with an opinion about what a
// trackball is, so it is checked rather than read.
func TestTheHidppDeviceTypesMapOntoKinds(t *testing.T) {
	for _, c := range []struct {
		reported byte
		want     Kind
	}{
		{typeMouse, KindMouse},
		{typeTrackball, KindMouse},
		{typeTouchpad, KindMouse},
		{typeKeyboard, KindKeyboard},
		{typeNumpad, KindKeyboard},
		{0x01, KindOther}, // a remote control
		{0x07, KindOther}, // the receiver itself
		{0x5A, KindOther}, // a type this program has never seen
	} {
		const feature = 0x07
		e := &fakeEndpoint{respond: func(req []byte) [][]byte {
			return [][]byte{reportOf(reportShort, req[1], req[2], req[3], c.reported)}
		}}
		assert.Equal(t, c.want, kind(e, RequestTimeout, 1, feature), "for type %#x", c.reported)
	}
}

// AC16. A device that will not say what it is is KindOther, which orders it
// after the ones that did.
//
// Not an error: the kind is a label like the name, and a mouse whose type
// request was lost is still a mouse with a percentage.
func TestADeviceThatWillNotSayItsTypeIsKindOther(t *testing.T) {
	e := &fakeEndpoint{respond: func(req []byte) [][]byte {
		return [][]byte{{reportShort, req[1], 0xFF, req[2], req[3], err20UnsupportedFeature, 0x00}}
	}}
	assert.Equal(t, KindOther, kind(e, RequestTimeout, 1, 0x07))
}

// AC16. BlueZ's icon says what sort of device it has.
//
// The icon and not the class of device: the class is a bit field this program
// would have to decode, the icon is BlueZ's own decoding of it, and a device
// over LE has an icon and no class at all.
func TestTheBluezIconSaysWhatTheDeviceIs(t *testing.T) {
	for _, c := range []struct {
		icon string
		want Kind
	}{
		{"input-mouse", KindMouse},
		{"input-tablet", KindOther}, // a graphics tablet is not the pointing device
		{"input-keyboard", KindKeyboard},
		{"audio-headset", KindHeadset},
		{"audio-headphones", KindHeadset},
		{"Audio-Headphones", KindHeadset}, // the case is not the contract
		{"phone", KindOther},
		{"", KindOther},
	} {
		d := describe(map[string]dbus.Variant{
			"Alias":     dbus.MakeVariant("A Device"),
			"Address":   dbus.MakeVariant("AA:BB:CC:DD:EE:FF"),
			"Icon":      dbus.MakeVariant(c.icon),
			"Connected": dbus.MakeVariant(true),
		}, nil)
		assert.Equal(t, c.want, d.Kind, "for icon %q", c.icon)
	}
}

// AC16. A device with no icon falls back to its profiles.
//
// BlueZ gives a device an icon only when it can pick one, and a pair of Bose
// headphones paired to the machine this was written on has an Audio Sink and no
// Icon property at all.
func TestADeviceWithNoIconFallsBackToItsProfiles(t *testing.T) {
	d := describe(map[string]dbus.Variant{
		"Alias":   dbus.MakeVariant("Some Headphones"),
		"Address": dbus.MakeVariant("AA:BB:CC:DD:EE:FF"),
		"UUIDs":   dbus.MakeVariant([]string{"0000110b-0000-1000-8000-00805f9b34fb"}),
	}, nil)
	assert.Equal(t, KindHeadset, d.Kind)
}

// AC16. A device with neither an icon nor an audio profile is KindOther, which
// orders it last and says nothing false about it.
func TestADeviceWithNothingToGoOnIsKindOther(t *testing.T) {
	d := describe(map[string]dbus.Variant{
		"Alias":   dbus.MakeVariant("Something"),
		"Address": dbus.MakeVariant("AA:BB:CC:DD:EE:FF"),
	}, nil)
	assert.Equal(t, KindOther, d.Kind)
}

// AC16. Everything headsetcontrol reports is a headset, which is what the
// program is for. Nothing is read to establish it.
func TestEveryHeadsetcontrolDeviceIsAHeadset(t *testing.T) {
	found, err := ParseHeadsets([]byte(`{"devices":[
		{"product":"Arctis Nova Pro Wireless","battery":{"status":"BATTERY_AVAILABLE","level":47}}]}`))
	require.NoError(t, err)
	require.Len(t, found, 1)
	assert.Equal(t, KindHeadset, found[0].Kind)
}

// AC16. A Bluetooth reading carries the kind of the device it came from, by
// either route out of the radio.
func TestABluetoothReadingCarriesItsDevicesKind(t *testing.T) {
	b := reader([]BluetoothDevice{
		{Name: "WH-1000XM6", Address: "AA:BB:CC:DD:EE:FF", Level: 47, HasLevel: true, Kind: KindHeadset},
	}, refusing)

	found, err := b.Batteries()
	require.NoError(t, err)
	require.Len(t, found, 1)
	assert.Equal(t, KindHeadset, found[0].Kind)
}
