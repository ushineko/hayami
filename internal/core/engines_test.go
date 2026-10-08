package core_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/hayami/internal/core"
)

// The adapter in these names is the card D3DKMT reported on the desk spec 034
// was measured on; the processes are invented.
const card3060 = "0x00000000_0x0000e985"

// An instance name is the adapter, the engine and the engine's type.
func TestAGPUEngineInstanceNamesItsAdapterAndEngine(t *testing.T) {
	in, ok := core.ParseEngineInstance("pid_4120_luid_0x00000000_0x0000E985_phys_0_eng_3_engtype_3D")
	require.True(t, ok)
	assert.Equal(t, core.EngineInstance{LUID: card3060, Engine: "phys_0_eng_3", Type: "3D"}, in)

	_, ok = core.ParseEngineInstance("_Total")
	assert.False(t, ok, "a name that is not an instance is not read as one")
}

// D3DKMT's LUID and the counter's are the same adapter, written the same way.
func TestAnAdaptersLUIDIsWrittenAsTheCounterWritesIt(t *testing.T) {
	assert.Equal(t, card3060, core.LUIDString(0, 0xE985))
	assert.Equal(t, "0xffffffff_0x00000001", core.LUIDString(-1, 1))
}

// The load is the busiest engine, each engine summed over the processes using
// it, as Task Manager gives it: not the sum of every engine, and not the mean.
// Another adapter's engines are not this one's.
func TestTheCardsLoadIsItsBusiestEngine(t *testing.T) {
	values := map[string]float64{
		"pid_10_luid_0x00000000_0x0000E985_phys_0_eng_0_engtype_3D":          20,
		"pid_11_luid_0x00000000_0x0000E985_phys_0_eng_0_engtype_3D":          15,
		"pid_12_luid_0x00000000_0x0000E985_phys_0_eng_4_engtype_VideoDecode": 30,
		"pid_12_luid_0x00000000_0x0000E985_phys_0_eng_1_engtype_Copy":        5,
		"pid_13_luid_0x00000000_0x0000FB45_phys_0_eng_0_engtype_3D":          90,
	}

	load, ok := core.BusiestEngine(values, card3060)

	require.True(t, ok)
	assert.InDelta(t, 35, load, 0.001)
}

// An engine summed past 100 on the counters' rounding is held to 100, and an
// adapter with no instance has no load rather than a load of zero.
func TestTheCardsLoadIsHeldTo100AndAbsentWithoutInstances(t *testing.T) {
	values := map[string]float64{
		"pid_10_luid_0x00000000_0x0000E985_phys_0_eng_0_engtype_3D": 70,
		"pid_11_luid_0x00000000_0x0000E985_phys_0_eng_0_engtype_3D": 40,
	}
	load, ok := core.BusiestEngine(values, card3060)
	require.True(t, ok)
	assert.InDelta(t, 100, load, 0.001)

	_, ok = core.BusiestEngine(values, "0x00000000_0x00000001")
	assert.False(t, ok)
}

// noKernel is a GraphicsReader with no hwmon tree and no busy file, which is a
// Windows machine's, and an SMI that records being asked.
func noKernel(t *testing.T, native func(context.Context) core.Graphics, smi string) (*core.GraphicsReader, *bool) {
	t.Helper()
	dir := t.TempDir()
	asked := false
	return &core.GraphicsReader{
		Native: native,
		Root:   filepath.Join(dir, "hwmon"), Busy: filepath.Join(dir, "none", "gpu_busy_percent"),
		SMI: func(context.Context) (string, error) {
			asked = true
			if smi == "" {
				return "", errors.New("not here")
			}
			return smi, nil
		},
	}, &asked
}

// Spec 034. A card the platform reports in full is not asked of nvidia-smi:
// on Windows D3DKMT and the engine counters are the reading, and the
// subprocess is the fallback.
func TestACardThePlatformReadsIsNotAskedOfNvidiaSMI(t *testing.T) {
	full := core.Graphics{Temperature: 43.2, HasTemperature: true, Load: 24, HasLoad: true, Name: "NVIDIA GeForce RTX 3060 Ti"}
	g, asked := noKernel(t, func(context.Context) core.Graphics { return full }, "40, 31, Other\n")

	assert.Equal(t, full, g.Read(t.Context()))
	assert.False(t, *asked, "nvidia-smi was run for a card D3DKMT had already read")
}

// Spec 034. What the platform left out, and only that, is asked of
// nvidia-smi: a driver that reports no temperature to D3DKMT still has its
// load from the counters, and its name from D3DKMT over nvidia-smi's.
func TestWhatThePlatformLeftOutIsAskedOfNvidiaSMI(t *testing.T) {
	partial := core.Graphics{Load: 24, HasLoad: true, Name: "NVIDIA GeForce RTX 3060 Ti"}
	g, asked := noKernel(t, func(context.Context) core.Graphics { return partial }, "40, 31, Other\n")

	got := g.Read(t.Context())

	assert.True(t, *asked)
	assert.Equal(t, core.Graphics{Temperature: 40, HasTemperature: true, Load: 24, HasLoad: true, Name: "NVIDIA GeForce RTX 3060 Ti"}, got)
}
