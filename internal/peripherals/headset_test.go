package peripherals

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// arctis is headsetcontrol's own reply on the machine this was written on,
// trimmed to the fields this package reads. The status is the one an Arctis
// gives on its charging cradle, which is the ordinary state at the end of a
// day and not an error.
const arctis = `{
  "device_count": 1,
  "devices": [
    {
      "status": "partial",
      "device": "SteelSeries Arctis Nova Pro Wireless",
      "vendor": "SteelSeries",
      "product": "Arctis Nova Pro Wireless",
      "capabilities": ["CAP_BATTERY_STATUS"],
      "battery": { "status": "BATTERY_UNAVAILABLE", "level": -1 }
    }
  ]
}`

// AC4. A headset that is answering gives its name and its level.
func TestAHeadsetThatIsAnsweringGivesItsNameAndLevel(t *testing.T) {
	reply := `{"devices":[{"device":"SteelSeries Arctis Nova Pro Wireless",
	            "product":"Arctis Nova Pro Wireless",
	            "battery":{"status":"BATTERY_AVAILABLE","level":72}}]}`

	found, err := ParseHeadsets([]byte(reply))
	require.NoError(t, err)
	require.Len(t, found, 1)

	// The short name, because the vendor is obvious to whoever owns it and a
	// row's label has a column to fit in.
	assert.Equal(t, "Arctis Nova Pro Wireless", found[0].Name)
	assert.Equal(t, 72, found[0].Level)
	assert.True(t, found[0].HasLevel)
	assert.Equal(t, Discharging, found[0].State)
}

// AC4. A headset on its cradle is a headset that is there and not saying, not
// a headset that is flat and not a headset that is gone.
func TestAHeadsetWithNoLevelIsStillADevice(t *testing.T) {
	found, err := ParseHeadsets([]byte(arctis))
	require.NoError(t, err)
	require.Len(t, found, 1)

	assert.Equal(t, "Arctis Nova Pro Wireless", found[0].Name)
	assert.False(t, found[0].HasLevel, "an unavailable battery was read as a level")
	assert.Zero(t, found[0].Level)
}

// AC4. Charging is reported as charging.
func TestAChargingHeadsetSaysSo(t *testing.T) {
	reply := `{"devices":[{"product":"Arctis","battery":{"status":"BATTERY_CHARGING","level":40}}]}`

	found, err := ParseHeadsets([]byte(reply))
	require.NoError(t, err)
	require.Len(t, found, 1)
	assert.Equal(t, Charging, found[0].State)
	assert.Equal(t, 40, found[0].Level)
}

// AC4. A level outside the range is not a level. -1 is what headsetcontrol
// writes beside a status it could not read, and read as a number it would be
// a battery below empty.
func TestALevelOutsideTheRangeIsNotALevel(t *testing.T) {
	reply := `{"devices":[{"product":"Arctis","battery":{"status":"BATTERY_AVAILABLE","level":-1}}]}`

	found, err := ParseHeadsets([]byte(reply))
	require.NoError(t, err)
	require.Len(t, found, 1)
	assert.False(t, found[0].HasLevel)
}

// AC4. No devices is no rows, and not an error.
func TestNoHeadsetsIsNotAnError(t *testing.T) {
	found, err := ParseHeadsets([]byte(`{"device_count":0,"devices":[]}`))
	require.NoError(t, err)
	assert.Empty(t, found)
}

// AC4. Something that is not the report at all is an error, because it means
// the tool changed under us rather than that the headset is quiet.
func TestAReplyThatIsNotJSONIsAnError(t *testing.T) {
	_, err := ParseHeadsets([]byte("Warning: short output deprecated, use the -o option instead"))
	require.Error(t, err)
}
