package cooler_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/hayami/internal/cooler"
)

// chip writes one hwmon directory: a name, and labelled temperatures.
func chip(t *testing.T, root, dir, name string, temps map[string]string) {
	t.Helper()
	path := filepath.Join(root, dir)
	require.NoError(t, os.MkdirAll(path, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(path, "name"), []byte(name+"\n"), 0o600))

	i := 1
	for label, milli := range temps {
		base := filepath.Join(path, "temp"+itoa(i))
		require.NoError(t, os.WriteFile(base+"_label", []byte(label+"\n"), 0o600))
		require.NoError(t, os.WriteFile(base+"_input", []byte(milli+"\n"), 0o600))
		i++
	}
}

func itoa(i int) string { return string(rune('0' + i)) }

// The numbers move between boots. A program that remembered hwmon10 would
// report another chip's temperature after a reboot rather than failing, which
// is the worst way to be wrong -- so the same tree renumbered must give the
// same answer.
func TestASensorIsFoundByLabelWhateverItsHwmonNumberIs(t *testing.T) {
	for _, dir := range []string{"hwmon0", "hwmon10", "hwmon7"} {
		root := t.TempDir()
		chip(t, root, "hwmon3", "nct6798", map[string]string{"SYSTIN": "31000"})
		chip(t, root, dir, "coretemp", map[string]string{"Package id 0": "56000"})

		got, err := cooler.Sensor{Chip: "coretemp", Label: "Package id 0"}.Read(root)

		require.NoError(t, err, "coretemp at %s", dir)
		assert.InDelta(t, 56.0, got, 0.001, "the kernel writes thousandths")
	}
}

// A machine with a different processor is a machine that draws the rest.
func TestASensorThisMachineDoesNotHaveIsAbsentRatherThanAnError(t *testing.T) {
	root := t.TempDir()
	chip(t, root, "hwmon0", "acpitz", map[string]string{"": "27800"})

	_, err := cooler.Sensor{Chip: "coretemp", Label: "Package id 0"}.Read(root)

	assert.ErrorIs(t, err, cooler.ErrNoSensor)
}

func TestTheWrongLabelInTheRightChipIsAbsent(t *testing.T) {
	root := t.TempDir()
	chip(t, root, "hwmon0", "coretemp", map[string]string{"Core 8": "45000"})

	_, err := cooler.Sensor{Chip: "coretemp", Label: "Package id 0"}.Read(root)

	assert.ErrorIs(t, err, cooler.ErrNoSensor)
}

// liquidctl's own shape, with the two devices this machine actually reports.
// The power supply is the trap: it has a "VRM temperature" and a "Case
// temperature", and a decoder matching on temperature keys alone would put its
// numbers under a heading that says coolant.
const reply = `[
  {"description": "Corsair HX1000i (2022)", "status": [
    {"key": "VRM temperature",  "value": 53.5, "unit": "°C"},
    {"key": "Case temperature", "value": 51.0, "unit": "°C"},
    {"key": "Fan speed",        "value": 0,    "unit": "rpm"}
  ]},
  {"description": "NZXT Kraken 2024 Elite RGB", "status": [
    {"key": "Liquid temperature", "value": 40.2, "unit": "°C"},
    {"key": "Pump speed",         "value": 2717, "unit": "rpm"},
    {"key": "Pump duty",          "value": 90,   "unit": "%"},
    {"key": "Fan speed",          "value": 1507, "unit": "rpm"}
  ]}
]`

func TestThePowerSupplyIsNotMistakenForTheCooler(t *testing.T) {
	got, err := cooler.Parse([]byte(reply))

	require.NoError(t, err)
	assert.InDelta(t, 40.2, got.Coolant, 0.001,
		"the power supply's case temperature was read as the coolant")
	assert.Equal(t, 2717, got.PumpRPM)
	assert.Equal(t, 1507, got.FanRPM, "the power supply's stopped fan was read as the cooler's")
}

// A machine with no liquid cooler has no cooler as far as this program is
// concerned, however many other devices liquidctl can see.
func TestADeviceWithNoLiquidTemperatureIsNotACooler(t *testing.T) {
	_, err := cooler.Parse([]byte(`[{"description": "Corsair HX1000i (2022)", "status": [
	  {"key": "VRM temperature", "value": 53.5, "unit": "°C"}]}]`))

	assert.ErrorIs(t, err, cooler.ErrNoCooler)
}

// A device that reports a speed as null is a device with nothing to say about
// it, which is not the same as a stopped fan.
func TestASpeedTheDeviceDidNotReportIsAbsentRatherThanZero(t *testing.T) {
	got, err := cooler.Parse([]byte(`[{"description": "x", "status": [
	  {"key": "Liquid temperature", "value": 41.0},
	  {"key": "Fan speed", "value": null}]}]`))

	require.NoError(t, err)
	assert.False(t, got.HasFan)
	assert.False(t, got.HasPump)
}

// Without a match liquidctl opens every device it can drive -- here a power
// supply and an RGB controller as well as the cooler -- and each is a hidraw
// node held open for as long as the read takes. This machine has a documented
// history of contention on those, and the symptom is not an error: it is a
// coolant temperature that goes missing for a poll or two at random.
func TestTheCoolerIsNarrowedToTheCoolerByDefault(t *testing.T) {
	// Setenv first so the test framework restores whatever the environment
	// had; the unset is what the case is actually about, which is a machine
	// that has never set the variable at all.
	t.Setenv(cooler.MatchEnv, "")
	require.NoError(t, os.Unsetenv(cooler.MatchEnv))

	assert.Equal(t, cooler.DefaultMatch, cooler.Match())
}

// A machine with another cooler says so rather than editing a constant.
func TestTheMatchComesFromTheEnvironmentWhenItIsSet(t *testing.T) {
	t.Setenv(cooler.MatchEnv, "corsair")

	assert.Equal(t, "corsair", cooler.Match())
}

// An empty value is a deliberate "do not narrow", which is how a machine this
// default does not name gets every device looked at again. It has to be
// distinguishable from unset, which is why it is LookupEnv and not Getenv.
func TestAnEmptyMatchLooksAtEveryDevice(t *testing.T) {
	t.Setenv(cooler.MatchEnv, "")

	assert.Empty(t, cooler.Match())
}

/*
An AMD processor is read, not declared absent.

One constant named `coretemp`, which is Intel's driver, so a Ryzen read nothing
and the section said "no coretemp/Package id 0" — precise about what it looked
for and silent about only looking for one thing (issue #72).
*/
func TestAnAMDProcessorIsRead(t *testing.T) {
	root := t.TempDir()
	chip(t, root, "hwmon1", "k10temp", map[string]string{"Tctl": "32625"})

	v, err := cooler.ReadCPU(root)

	require.NoError(t, err)
	assert.InDelta(t, 32.625, v, 0.001)
}

/*
Tdie is preferred to Tctl.

Tctl is not a temperature: it carries a per-model offset the firmware uses for
fan curves and reads above the die on some chips. Where a processor exposes
both, the real one wins.
*/
func TestTdieIsPreferredToTctl(t *testing.T) {
	root := t.TempDir()
	chip(t, root, "hwmon1", "k10temp", map[string]string{"Tctl": "52000", "Tdie": "42000"})

	v, err := cooler.ReadCPU(root)

	require.NoError(t, err)
	assert.InDelta(t, 42, v, 0.001, "Tctl was taken over Tdie")
}

// Intel still comes first, so nothing changes on the machine this was written
// on.
func TestIntelIsStillPreferred(t *testing.T) {
	root := t.TempDir()
	chip(t, root, "hwmon1", "k10temp", map[string]string{"Tctl": "52000"})
	chip(t, root, "hwmon2", "coretemp", map[string]string{"Package id 0": "38000"})

	v, err := cooler.ReadCPU(root)

	require.NoError(t, err)
	assert.InDelta(t, 38, v, 0.001)
}

// A machine with no processor sensor names every one looked for, so nobody
// goes hunting for a driver that was never going to be there.
func TestAnAbsentSensorNamesEveryOneLookedFor(t *testing.T) {
	root := t.TempDir()
	chip(t, root, "hwmon1", "nct6798", map[string]string{"SYSTIN": "31000"})

	_, err := cooler.ReadCPU(root)

	require.Error(t, err)
	assert.ErrorIs(t, err, cooler.ErrNoSensor)
	for _, want := range []string{"coretemp", "k10temp", "zenpower", "Tdie", "Tctl"} {
		assert.Contains(t, err.Error(), want)
	}
}
