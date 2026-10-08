package core_test

import (
	"io/fs"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/windows"

	"github.com/ushineko/hayami/internal/core"
)

/*
Spec 048. Windows' table names its platform, which is what the device
drivers' descriptions are asked about: Bluetooth's say Linux, so none is asked
here (spec 035; the panel's vendor tests hold the list).
*/
func TestWindowsTableNamesItsPlatform(t *testing.T) {
	h := core.NewHost(core.HostConfig{})
	assert.Equal(t, "windows", h.Platform)
}

/*
Spec 035, moved here by spec 043. A device Windows will not open is not
permitted, and the advice is what is worth looking for here -- another program
holding it -- not a udev rule. Windows returns ERROR_ACCESS_DENIED, which is
not Go's EACCES.
*/
func TestWindowsRefusalIsNotPermittedAndNamesNoUdevRule(t *testing.T) {
	h := core.NewHost(core.HostConfig{})
	a, ok := h.Permission(&fs.PathError{Op: "open", Path: "hid", Err: windows.ERROR_ACCESS_DENIED})

	require.True(t, ok)
	assert.Equal(t, core.AbsencePermission, a.Code)
	assert.True(t, a.Actionable)
	assert.Contains(t, a.Detail, "another program")
	assert.NotContains(t, a.Detail, "udev")
}
