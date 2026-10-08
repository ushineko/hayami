package core_test

import (
	"io/fs"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/ushineko/sanshoku/hwmon"

	"github.com/ushineko/hayami/internal/core"
)

// Spec 043. Linux's table reads the kernel's sensors under hwmon.Root and tells
// a device the logged-in user may not open about the udev rule. Which device
// drivers read here is theirs to say (spec 048; the panel's vendor tests).
func TestLinuxsTable(t *testing.T) {
	h := core.NewHost(core.HostConfig{})
	assert.Equal(t, "linux", h.Platform)

	assert.Contains(t, h.CPUSensorDetail(), hwmon.Root)
	for _, s := range hwmon.CPU {
		assert.Contains(t, h.CPUSensorDetail(), s.String())
	}

	a, ok := h.Permission(&fs.PathError{Op: "open", Path: "/dev/hidraw4", Err: syscall.EACCES})
	require.True(t, ok)
	assert.Contains(t, a.Detail, "udev rule")
	assert.Contains(t, a.Detail, "60-sanshoku.rules")
}
