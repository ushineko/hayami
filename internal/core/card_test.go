package core

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Spec 096 R4. nvidia-smi's answer: memory in MiB, clocks in MHz, power and
// its limit in watts.
func TestNvidiaSMIsCardAnswer(t *testing.T) {
	c := ParseSMICard("11169, 24564, 780, 810, 57.52, 400.00\n")

	assert.True(t, c.HasVRAM)
	assert.InDelta(t, 11169*float64(1<<20), c.VRAMUsed, 1)
	assert.InDelta(t, 24564*float64(1<<20), c.VRAMTotal, 1)
	assert.Equal(t, Card{
		VRAMUsed: c.VRAMUsed, VRAMTotal: c.VRAMTotal, HasVRAM: true,
		Clock: 780, HasClock: true, MemClock: 810, HasMemClock: true,
		Power: 57.52, PowerLimit: 400, HasPower: true,
	}, c)
}

// R2. A field nvidia-smi does not have is that part absent, and the rest are
// read; a card without a power limit has its draw.
func TestAnSMIFieldItLacksIsAbsent(t *testing.T) {
	c := ParseSMICard("[N/A], 24564, 780, [Not Supported], 57.52, [N/A]\n")
	assert.False(t, c.HasVRAM)
	assert.True(t, c.HasClock)
	assert.False(t, c.HasMemClock)
	assert.True(t, c.HasPower)
	assert.Zero(t, c.PowerLimit)

	assert.Equal(t, Card{}, ParseSMICard(""))
	assert.False(t, ParseSMICard("30000, 20000, 1, 1, 1, 1").HasVRAM, "more used than there is")
}

// amdDevice lays out an AMD card's files under a test's directory, as the
// kernel's amdgpu documentation describes them, and returns the busy glob.
func amdDevice(t *testing.T, files map[string]string) string {
	t.Helper()
	device := filepath.Join(t.TempDir(), "card0", "device")
	files["gpu_busy_percent"] = "7\n"
	for name, body := range files {
		path := filepath.Join(device, name)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o750))
		require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
	}
	return filepath.Join(filepath.Dir(filepath.Dir(device)), "card*", "device", "gpu_busy_percent")
}

// R5. An AMD card: VRAM in bytes, the clocks by their labels in Hz, the power
// and its cap in microwatts.
func TestAnAMDCardsFiles(t *testing.T) {
	glob := amdDevice(t, map[string]string{
		"mem_info_vram_used":          "2147483648\n",
		"mem_info_vram_total":         "17163091968\n",
		"hwmon/hwmon3/freq1_label":    "sclk\n",
		"hwmon/hwmon3/freq1_input":    "2500000000\n",
		"hwmon/hwmon3/freq2_label":    "mclk\n",
		"hwmon/hwmon3/freq2_input":    "1000000000\n",
		"hwmon/hwmon3/power1_average": "45000000\n",
		"hwmon/hwmon3/power1_cap":     "203000000\n",
	})

	c, err := AMDCard(glob)
	require.NoError(t, err)
	assert.Equal(t, Card{
		VRAMUsed: 2147483648, VRAMTotal: 17163091968, HasVRAM: true,
		Clock: 2500, HasClock: true, MemClock: 1000, HasMemClock: true,
		Power: 45, PowerLimit: 203, HasPower: true,
	}, c)
}

// R5. Newer kernels give power1_input where older ones gave power1_average;
// labels, not numbers, say which clock is which.
func TestAnAMDCardsOtherForms(t *testing.T) {
	glob := amdDevice(t, map[string]string{
		"hwmon/hwmon0/freq1_label":  "mclk\n",
		"hwmon/hwmon0/freq1_input":  "875000000\n",
		"hwmon/hwmon0/power1_input": "12000000\n",
	})

	c, err := AMDCard(glob)
	require.NoError(t, err)
	assert.False(t, c.HasVRAM)
	assert.False(t, c.HasClock)
	assert.InDelta(t, 875, c.MemClock, 1e-9)
	assert.InDelta(t, 12, c.Power, 1e-9)
	assert.Zero(t, c.PowerLimit)

	_, err = AMDCard(filepath.Join(t.TempDir(), "none*"))
	assert.Error(t, err)
}

// R3. The card is read by the route its reading came from: nvidia-smi for a
// card read through it, the AMD files otherwise; Windows asks nvidia-smi.
func TestTheCardIsReadByItsRoute(t *testing.T) {
	asked := 0
	smi := func(context.Context) (string, error) { asked++; return "1, 2, 3, 4, 5, 6", nil }
	glob := amdDevice(t, map[string]string{"hwmon/hwmon0/power1_input": "9000000\n"})

	c, err := linuxCard(glob, smi)(t.Context(), smiName)
	require.NoError(t, err)
	assert.InDelta(t, 3, c.Clock, 1e-9)
	assert.Equal(t, 1, asked)

	c, err = linuxCard(glob, smi)(t.Context(), "")
	require.NoError(t, err)
	assert.InDelta(t, 9, c.Power, 1e-9)
	assert.Equal(t, 1, asked, "an AMD card runs no nvidia-smi")

	_, err = windowsCard(func(context.Context) (string, error) { return "", errors.New("no nvidia-smi") })(t.Context(), "")
	assert.Error(t, err)
}

func FuzzParseSMICard(f *testing.F) {
	f.Add("11169, 24564, 780, 810, 57.52, 400.00\n")
	f.Add("[N/A], 24564, 780, [Not Supported], 57.52, [N/A]")
	f.Fuzz(func(t *testing.T, s string) {
		c := ParseSMICard(s)
		if c.HasVRAM && c.VRAMUsed > c.VRAMTotal {
			t.Fatalf("%q: %v used of %v", s, c.VRAMUsed, c.VRAMTotal)
		}
		if c.Clock < 0 || c.MemClock < 0 || c.Power < 0 || c.PowerLimit < 0 {
			t.Fatalf("%q: a negative figure: %+v", s, c)
		}
	})
}
