package core_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/hayami/internal/core"
)

// stat writes a /proc/stat with the aggregate line given, and a per-core line
// that must not be read in its place.
func stat(t *testing.T, path, aggregate string) {
	t.Helper()
	body := "cpu  " + aggregate + "\ncpu0 9 9 9 9 9 9 9 9 0 0\nintr 1 2 3\n"
	require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
}

// The load is the busy fraction between two samples. The first poll takes
// both itself, a warm-up apart, so a one-shot command has a load to say; later
// polls difference against the one before.
func TestTheProcessorsLoadIsTheBusyFractionBetweenTwoSamples(t *testing.T) {
	path := filepath.Join(t.TempDir(), "stat")
	//          user nice system idle iowait irq softirq steal guest guest_nice
	stat(t, path, "100 0 50 800 50 0 0 0 0 0")
	c := core.NewCPULoad(path)
	waited := 0
	c.SetWait(func() {
		waited++
		// 30 more busy (20 user, 10 system), 70 more idle (60 idle, 10
		// iowait), and 40 of guest time already in user, not counted twice.
		stat(t, path, "120 0 60 860 60 0 0 0 40 0")
	})

	load, ok := c.Load()
	require.True(t, ok, "the first poll has a load")
	assert.InDelta(t, 30, load, 0.001)

	// 10 busy and 90 idle since, with no second warm-up.
	stat(t, path, "130 0 60 950 60 0 0 0 40 0")
	load, ok = c.Load()
	require.True(t, ok)
	assert.InDelta(t, 10, load, 0.001)
	assert.Equal(t, 1, waited, "only the first poll waits")
}

// The real warm-up is CPUWarmup, and nothing longer.
func TestTheFirstPollWaitsTheWarmUp(t *testing.T) {
	path := filepath.Join(t.TempDir(), "stat")
	stat(t, path, "100 0 50 800 50 0 0 0 0 0")
	c := core.NewCPULoad(path)

	start := time.Now()
	c.Load()
	assert.GreaterOrEqual(t, time.Since(start), core.CPUWarmup)
}

// A missing file is no load rather than an error: the row is drawn without it.
func TestAMissingProcStatIsNoLoad(t *testing.T) {
	c := core.NewCPULoad(filepath.Join(t.TempDir(), "absent"))
	c.Load()
	_, ok := c.Load()
	assert.False(t, ok)
}

// A line nvidia-smi printed on the desk this was written on, with nounits.
func TestAnSMILineIsTheTemperatureAndTheLoad(t *testing.T) {
	temp, load, gotTemp, gotLoad := core.ParseSMI("40, 31\n")
	require.True(t, gotTemp)
	require.True(t, gotLoad)
	assert.InDelta(t, 40, temp, 0.001)
	assert.InDelta(t, 31, load, 0.001)

	// And without nounits, where the percent sign travels with the number.
	_, load, _, gotLoad = core.ParseSMI("40, 3 %\n")
	require.True(t, gotLoad)
	assert.InDelta(t, 3, load, 0.001)
}

// A format is not an API. What does not parse is absence.
func TestAnSMIAnswerThatIsNotANumberIsAbsence(t *testing.T) {
	for _, out := range []string{"", "\n", "[N/A], [N/A]", "No devices were found"} {
		_, _, gotTemp, gotLoad := core.ParseSMI(out)
		assert.False(t, gotTemp, "%q was read as a temperature", out)
		assert.False(t, gotLoad, "%q was read as a load", out)
	}
}

// amdgpu builds an hwmon tree with an amdgpu edge sensor at 45 degrees, and a
// busy file at 12 percent, under dir.
func amdgpu(t *testing.T, dir string) (root, busy string) {
	t.Helper()
	root = filepath.Join(dir, "hwmon")
	chip := filepath.Join(root, "hwmon3")
	require.NoError(t, os.MkdirAll(chip, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(chip, "name"), []byte("amdgpu\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(chip, "temp1_label"), []byte("edge\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(chip, "temp1_input"), []byte("45000\n"), 0o600))

	card := filepath.Join(dir, "drm", "card1", "device")
	require.NoError(t, os.MkdirAll(card, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(card, "gpu_busy_percent"), []byte("12\n"), 0o600))
	return root, filepath.Join(dir, "drm", "card*", "device", "gpu_busy_percent")
}

// An AMD card is read from the kernel alone, and nvidia-smi is not asked.
func TestAnAMDCardIsReadFromTheKernel(t *testing.T) {
	root, busy := amdgpu(t, t.TempDir())
	asked := false
	g := core.GraphicsReader{Root: root, Busy: busy, SMI: func(context.Context) (string, error) {
		asked = true
		return "", errors.New("not here")
	}}

	got := g.Read(t.Context())

	assert.Equal(t, core.Graphics{Temperature: 45, HasTemperature: true, Load: 12, HasLoad: true}, got)
	assert.False(t, asked, "a card the kernel answers for is not asked about again")
}

// Where the kernel has nothing, nvidia-smi answers both.
func TestACardTheKernelCannotReadIsAskedOfNvidiaSMI(t *testing.T) {
	dir := t.TempDir()
	g := core.GraphicsReader{
		Root: filepath.Join(dir, "hwmon"), Busy: filepath.Join(dir, "none", "gpu_busy_percent"),
		SMI: func(context.Context) (string, error) { return "40, 31\n", nil },
	}

	assert.Equal(t, core.Graphics{Temperature: 40, HasTemperature: true, Load: 31, HasLoad: true}, g.Read(t.Context()))
}

// No sensor and no nvidia-smi is no GPU, and not an error: most machines
// without a discrete card are this.
func TestNoSensorAndNoNvidiaSMIIsNoGPU(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PATH", dir)
	g := core.GraphicsReader{
		Root: filepath.Join(dir, "hwmon"), Busy: filepath.Join(dir, "none", "gpu_busy_percent"),
		SMI: core.NvidiaSMI,
	}

	assert.Equal(t, core.Graphics{}, g.Read(t.Context()))
}

// The real runner, against a script standing in for nvidia-smi on PATH: the
// flags it is given are the ones ParseSMI expects the answer to.
func TestNvidiaSMIIsAskedForThreeColumnsWithoutUnits(t *testing.T) {
	dir := t.TempDir()
	script := "#!/bin/sh\n" +
		`[ "$1" = "--query-gpu=temperature.gpu,utilization.gpu,name" ] && ` +
		`[ "$2" = "--format=csv,noheader,nounits" ] && echo "52, 7, NVIDIA GeForce RTX 3080" && exit 0` + "\n" +
		"exit 9\n"
	//nolint:gosec // an executable the test needs to run
	require.NoError(t, os.WriteFile(filepath.Join(dir, "nvidia-smi"), []byte(script), 0o700))
	t.Setenv("PATH", dir)

	out, err := core.NvidiaSMI(t.Context())
	require.NoError(t, err)
	temp, load, gotTemp, gotLoad := core.ParseSMI(out)
	assert.True(t, gotTemp && gotLoad)
	assert.InDelta(t, 52, temp, 0.001)
	assert.InDelta(t, 7, load, 0.001)
	assert.Equal(t, "NVIDIA GeForce RTX 3080", core.SMIName(out))
}
