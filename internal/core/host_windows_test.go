package core_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/hayami/internal/core"
)

// Spec 034. GetSystemTimes is a load on any Windows machine, without
// privilege, and the first call has one.
func TestOnWindowsTheProcessorsLoadIsRead(t *testing.T) {
	load, ok := core.HostCPULoad().Load()

	require.True(t, ok)
	assert.GreaterOrEqual(t, load, 0.0)
	assert.LessOrEqual(t, load, 100.0)
}

// Spec 034. The processor is named from the registry, without the padding
// some parts carry.
func TestOnWindowsTheProcessorIsNamed(t *testing.T) {
	name := core.HostCPUName()

	require.NotEmpty(t, name)
	assert.Equal(t, strings.TrimSpace(name), name)
}

// Spec 034. On a machine whose card reports to D3DKMT, the card is read --
// temperature, load and name -- and nvidia-smi is never run. A machine with
// no such card (a CI runner's virtual adapter) skips: there is nothing to
// read, and that is not a failure.
func TestOnWindowsD3DKMTReadsTheCardWithoutNvidiaSMI(t *testing.T) {
	g := core.NewGraphicsReader()
	ran := false
	g.SMI = func(context.Context) (string, error) {
		ran = true
		return "", errors.New("not run")
	}

	got := g.Read(t.Context())
	if !got.HasTemperature {
		t.Skip("no adapter here reports a temperature to D3DKMT")
	}

	assert.False(t, ran, "nvidia-smi was run")
	assert.Greater(t, got.Temperature, 0.0)
	assert.True(t, got.HasLoad, "the GPU Engine counters gave no load")
	assert.NotEmpty(t, got.Name)
	t.Logf("D3DKMT and PDH: %.1f °C, %.0f %%, %s", got.Temperature, got.Load, got.Name)
}
