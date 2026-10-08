package core_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/ushineko/sanshoku/hwmon"

	"github.com/ushineko/hayami/internal/core"
)

// chip writes one hwmon chip under root: its name and one labelled
// temperature, in millidegrees.
func chip(t *testing.T, root, dir, name, label, milli string) {
	t.Helper()
	d := filepath.Join(root, dir)
	require.NoError(t, os.MkdirAll(d, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(d, "name"), []byte(name+"\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(d, "temp1_label"), []byte(label+"\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(d, "temp1_input"), []byte(milli+"\n"), 0o600))
}

// smi is an nvidia-smi that is never run.
func smi(context.Context) (string, error) { return "", errors.New("not run") }

// native is a platform report that is never asked.
func native(context.Context) core.Graphics { return core.Graphics{} }

/*
Spec 043. Linux's processor chain is hwmon.CPU in its order, one provider per
sensor, and the account of a machine with none is the sentence it always was
(spec 040's pin): every sensor looked for, and where -- now from the chain's
own record.
*/
func TestLinuxAsksTheProcessorSensorsInHwmonsOrder(t *testing.T) {
	ch := core.LinuxCPUTemperature("/sys/class/hwmon")
	want := make([]string, 0, len(hwmon.CPU))
	for _, s := range hwmon.CPU {
		want = append(want, s.String())
	}
	assert.Equal(t, want, ch.Names())

	a := core.LinuxCPUMissing("/sys/class/hwmon")(ch.Names(), nil)
	assert.Equal(t, core.AbsenceCPUSensor, a.Code)
	assert.Equal(t,
		"looked under /sys/class/hwmon for coretemp/Package id 0, k10temp/Tdie, k10temp/Tctl, zenpower/Tdie",
		a.Detail)
}

// Spec 043. A tree with no processor sensor is asked of every one, and the
// account names them all; a tree with the second has its value.
func TestLinuxsProcessorChainReadsATree(t *testing.T) {
	empty := t.TempDir()
	o := core.LinuxCPUTemperature(empty).Read(t.Context())
	assert.False(t, o.Answered)
	assert.Len(t, o.Tried, len(hwmon.CPU))

	root := t.TempDir()
	chip(t, root, "hwmon0", "k10temp", "Tdie", "51500")
	o = core.LinuxCPUTemperature(root).Read(t.Context())
	require.True(t, o.Answered)
	assert.InDelta(t, 51.5, o.Value, 0.001)
	assert.Equal(t, []string{"coretemp/Package id 0", "k10temp/Tdie"}, o.Tried)
}

/*
Spec 043. Linux asks the card through hwmon's GPU sensors, AMD's busy file,
then nvidia-smi; Windows through D3DKMT and the GPU Engine counters, then
nvidia-smi, and never under /sys (the review after spec 037 found Windows
asking hwmon). Windows' account is the sentence it always was.
*/
func TestEachPlatformAsksTheCardInItsOwnOrder(t *testing.T) {
	linux := core.LinuxGraphics("/sys/class/hwmon", core.BusyGlob, "", smi).Chain().Names()
	assert.Equal(t, []string{"amdgpu/edge", "amdgpu", "nouveau", "gpu_busy_percent", "nvidia-smi"}, linux)
	assert.Equal(t,
		"looked under /sys/class/hwmon and tried amdgpu/edge, amdgpu, nouveau, gpu_busy_percent, nvidia-smi",
		core.LinuxGPUMissing("/sys/class/hwmon")(linux))

	windows := core.WindowsGraphics(native, smi).Chain().Names()
	assert.Equal(t, []string{"D3DKMT and the GPU Engine counters", "nvidia-smi"}, windows)
	assert.Equal(t, "asked D3DKMT and the GPU Engine counters, and tried nvidia-smi", core.WindowsGPUMissing(windows))
}

// Spec 043. Windows' processor chain is LibreHardwareMonitor alone: its own
// account of why it has nothing passes through, and anything it does not
// account for is the platform's sentence (spec 034).
func TestWindowsAsksLibreHardwareMonitorForTheProcessor(t *testing.T) {
	off := &core.Absence{Code: core.AbsenceLHMServerOff, Detail: "web server off"}
	ch := core.WindowsCPUTemperature(func(context.Context) (float64, error) { return 0, off })
	assert.Equal(t, []string{"LibreHardwareMonitor"}, ch.Names())

	h := core.Host{CPUTemperature: ch, CPUMissing: core.WindowsCPUMissing}
	_, err := h.ReadCPUTemperature(t.Context())
	assert.ErrorIs(t, err, off, "LibreHardwareMonitor's own account is lost")

	h.CPUTemperature = core.WindowsCPUTemperature(func(context.Context) (float64, error) {
		return 0, context.Canceled
	})
	_, err = h.ReadCPUTemperature(t.Context())
	a, ok := core.AbsenceOf(err)
	require.True(t, ok)
	assert.Equal(t, core.AbsenceCPUSensor, a.Code)
	assert.Equal(t, "Windows offers no CPU temperature without a kernel driver", a.Detail)
	assert.ErrorIs(t, err, context.Canceled)
}

// Spec 043. A card the chain found no temperature for is an absence naming
// every route it tried; a card with one is no error.
func TestAHostsCardIsAnAbsenceNamingWhatWasTried(t *testing.T) {
	g := core.WindowsGraphics(func(context.Context) core.Graphics { return core.Graphics{} }, smi)
	h := core.Host{Graphics: g, GPUMissing: core.WindowsGPUMissing}

	_, err := h.ReadGraphics(t.Context())
	a, ok := core.AbsenceOf(err)
	require.True(t, ok)
	assert.Equal(t, core.AbsenceGPUSensor, a.Code)
	assert.Equal(t, "asked D3DKMT and the GPU Engine counters, and tried nvidia-smi", a.Detail)

	h.Graphics = core.WindowsGraphics(func(context.Context) core.Graphics {
		return core.Graphics{Temperature: 43, HasTemperature: true, Load: 21, HasLoad: true, Name: "a card"}
	}, smi)
	g2, err := h.ReadGraphics(t.Context())
	require.NoError(t, err)
	assert.Equal(t, "a card", g2.Name)
}

// Spec 043. A platform with no readers says so, in words that name no Linux
// path.
func TestAnUnknownPlatformAssumesNoLinuxPath(t *testing.T) {
	assert.NotContains(t, core.OtherCPUMissing(nil, nil).Detail, "/sys")
	assert.NotContains(t, core.OtherGPUMissing(nil), "/sys")
	assert.Empty(t, (&core.GraphicsReader{}).Chain().Names(), "a reader with no fields set asks nothing")
}
