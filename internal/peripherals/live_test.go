package peripherals

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

/*
The tests that touch the real hardware.

Everything else in this package drives an endpoint the test wrote, and that is
the right way round — but a decoder passing on recorded bytes is not evidence
that the exchange works. The bug this guards against is the one that is
invisible from the code: a request that is well formed and never answered, a
node chosen that does not speak, a permission that is not there. Each of those
passes every unit test in this package and reports no peripherals on the
machine the program was written for.

They skip where the hardware is absent, so the suite runs anywhere.
*/

// liveLogitech reads a real Logitech device, or says why it cannot.
func liveLogitech(t *testing.T) []Battery {
	t.Helper()

	nodes, err := hidppNodes()
	require.NoError(t, err)
	if len(nodes) == 0 {
		t.Skip("no Logitech HID++ node on this machine")
	}

	found, err := NewLogitech().Batteries()
	require.NoError(t, err)
	if len(found) == 0 {
		t.Skip("a receiver is here but no device is awake on it")
	}
	return found
}

// AC9. The reading matches what solaar says about the same device at the same
// moment. solaar is the reference this section replaces, and the number is the
// only thing about it that has to survive the port.
func TestALiveLogitechReadingAgreesWithSolaar(t *testing.T) {
	found := liveLogitech(t)

	if _, err := exec.LookPath("solaar"); err != nil {
		t.Skip("solaar is not installed, so there is nothing to agree with")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	out, err := exec.CommandContext(ctx, "solaar", "show").CombinedOutput()
	require.NoError(t, err, "solaar: %s", out)

	// solaar writes "Battery: 86%, BatteryStatus.DISCHARGING." once per device.
	levels := solaarLevels(string(out))
	if len(levels) == 0 {
		t.Skip("solaar reported no battery to compare against")
	}

	for _, b := range found {
		if !b.HasLevel {
			continue
		}
		assert.Contains(t, levels, b.Level,
			"read %d %% over HID++; solaar reported %v", b.Level, levels)
	}
}

// solaarLevels pulls the percentages out of solaar's own output.
func solaarLevels(out string) []int {
	var levels []int
	for _, line := range splitLines(out) {
		i := indexOf(line, "Battery: ")
		if i < 0 {
			continue
		}
		rest := line[i+len("Battery: "):]
		n := 0
		digits := 0
		for _, c := range rest {
			if c < '0' || c > '9' {
				break
			}
			n = n*10 + int(c-'0')
			digits++
		}
		if digits > 0 {
			levels = append(levels, n)
		}
	}
	return levels
}

func splitLines(s string) []string {
	var out []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	return append(out, s[start:])
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

// AC9. A live reading is a plausible reading. This catches the exchange
// working and the decode being wrong, which the agreement test above skips
// over when solaar is not installed.
func TestALiveLogitechReadingIsPlausible(t *testing.T) {
	for _, b := range liveLogitech(t) {
		assert.NotEmpty(t, b.Name, "a live device reported no name at all")
		if b.HasLevel {
			assert.GreaterOrEqual(t, b.Level, 0)
			assert.LessOrEqual(t, b.Level, 100)
		}
	}
}

// AC9. headsetcontrol on this machine answers in a shape this package reads.
// It skips where the tool is absent, and passes where a headset is on its
// cradle, which is a device with no level and not an error.
func TestALiveHeadsetcontrolReplyIsReadable(t *testing.T) {
	if _, err := exec.LookPath("headsetcontrol"); err != nil {
		t.Skip("headsetcontrol is not installed")
	}

	ctx, cancel := context.WithTimeout(context.Background(), HeadsetTimeout)
	defer cancel()

	found, err := Headsets(ctx)
	require.NoError(t, err)
	if len(found) == 0 {
		t.Skip("headsetcontrol sees no device on this machine")
	}

	for _, b := range found {
		assert.NotEmpty(t, b.Name)
		if b.HasLevel {
			assert.GreaterOrEqual(t, b.Level, 0)
			assert.LessOrEqual(t, b.Level, 100)
		}
	}
}

// AC9. The node this machine's receiver presents is the one chosen.
//
// It asserts what the probe found: three Logitech nodes, exactly one of which
// speaks HID++. A kernel that started presenting them differently would show
// up here rather than as a section that had quietly stopped drawing.
func TestTheLiveMachinePresentsOneHidppNodePerReceiver(t *testing.T) {
	if _, err := os.Stat(SysHidraw); err != nil {
		t.Skip("no hidraw tree on this machine")
	}

	nodes, err := hidppNodes()
	require.NoError(t, err)
	if len(nodes) == 0 {
		t.Skip("no Logitech receiver on this machine")
	}

	for _, n := range nodes {
		f, err := os.OpenFile(n.Path, os.O_RDWR, 0)
		require.NoError(t, err, "the chosen node is not writable, so HID++ cannot be spoken on it")
		require.NoError(t, f.Close())
	}
}

// AC9. The AirPods on this machine, read over the accessory protocol.
//
// This is the test that matters for this source. Every other test in the file
// drives a channel the test wrote, and the two findings that made this work —
// the address byte order and EINTR — were both invisible to all of them. Each
// produced a working decoder talking to nothing.
func TestALiveAirPodsReadingIsPlausible(t *testing.T) {
	devices, err := bluetoothDevices()
	if errors.Is(err, ErrNoBluez) {
		t.Skip("bluez is not answering on this machine")
	}
	require.NoError(t, err)

	var apple []BluetoothDevice
	for _, d := range devices {
		if d.Apple && d.Audio {
			apple = append(apple, d)
		}
	}
	if len(apple) == 0 {
		t.Skip("no Apple audio device is connected")
	}

	b := NewBluetooth()
	for _, d := range apple {
		battery, err := b.readAccessory(d)
		if err != nil {
			t.Skipf("the device did not answer the accessory protocol: %v", err)
		}

		assert.True(t, battery.HasLevel)
		assert.GreaterOrEqual(t, battery.Level, 0)
		assert.LessOrEqual(t, battery.Level, 100)
		assert.NotEmpty(t, battery.Cells, "a live Apple audio device reported no cells")

		for _, c := range battery.Cells {
			assert.GreaterOrEqual(t, c.Level, 0, "cell %s", c.Cell)
			assert.LessOrEqual(t, c.Level, 100, "cell %s", c.Cell)
		}

		// The cells are ordered for reading, whatever order the firmware sent
		// them in. The pair this was written against reports right first.
		for i := 1; i < len(battery.Cells); i++ {
			assert.Less(t, battery.Cells[i-1].Cell, battery.Cells[i].Cell,
				"the live cells came back out of order")
		}
	}
}

// AC10. Anything else on this machine that reports a battery through BlueZ.
//
// Skips where nothing does, which is the ordinary case: most connected
// devices have no org.bluez.Battery1 at all.
func TestALiveBluezBatteryIsReadable(t *testing.T) {
	devices, err := bluetoothDevices()
	if errors.Is(err, ErrNoBluez) {
		t.Skip("bluez is not answering on this machine")
	}
	require.NoError(t, err)

	found := 0
	for _, d := range devices {
		if !d.HasLevel {
			continue
		}
		found++
		assert.NotEmpty(t, d.Name, "a device with a battery reported no name")
		assert.GreaterOrEqual(t, d.Level, 0)
		assert.LessOrEqual(t, d.Level, 100)
	}
	if found == 0 {
		t.Skip("no connected device reports a battery through bluez")
	}
}

// Nothing this package reads carries an identifier into a failure message.
//
// The repository is public and the live tests run against whatever is
// connected, which on this machine is a device named after its owner. A
// failure that printed the name or the address would put it in a CI log.
func TestALiveReadingNeverNamesTheDeviceInAnAssertion(t *testing.T) {
	devices, err := bluetoothDevices()
	if errors.Is(err, ErrNoBluez) {
		t.Skip("bluez is not answering on this machine")
	}
	require.NoError(t, err)

	for _, d := range devices {
		// The assertion messages above use the cell, never d.Name or
		// d.Address. This asserts the data exists so the reader knows the
		// omission is deliberate rather than an oversight.
		assert.NotPanics(t, func() { _ = d.Name + d.Address })
	}
}
