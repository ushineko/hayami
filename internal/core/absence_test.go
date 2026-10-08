package core_test

import (
	"errors"
	"fmt"
	"os"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/hayami/internal/core"
)

// An absence is found through any wrapping, and two absences with the same
// code are the same error: a caller decides on the code (spec 040).
func TestAnAbsenceIsMatchedByItsCode(t *testing.T) {
	wrapped := fmt.Errorf("reading the WLAN: %w", core.ErrWirelessDenied)

	a, ok := core.AbsenceOf(wrapped)
	require.True(t, ok)
	assert.Equal(t, core.AbsenceWirelessDenied, a.Code)
	assert.ErrorIs(t, wrapped, core.ErrWirelessDenied)

	copied := &core.Absence{Code: core.AbsenceWirelessDenied}
	assert.ErrorIs(t, copied, core.ErrWirelessDenied, "the same code is the same absence")
	assert.NotErrorIs(t, &core.Absence{Code: core.AbsencePermission}, core.ErrWirelessDenied)
	assert.NotErrorIs(t, &core.Absence{}, &core.Absence{}, "an absence with no code matches nothing")
}

// The error's text is the sentence a person reads, and the cause underneath
// stays reachable.
func TestAnAbsenceReadsAsItsDetailAndKeepsItsCause(t *testing.T) {
	cause := errors.New("connection refused")
	a := &core.Absence{Code: core.AbsenceLHMServerOff, Detail: "the web server is off", Err: cause}

	assert.Equal(t, "the web server is off", a.Error())
	assert.ErrorIs(t, a, cause)
}

// A refusal to open is the one absence a reader can act on; any other failure
// is not one at all.
func TestARefusedOpenIsAnActionableAbsence(t *testing.T) {
	refused := &os.PathError{Op: "open", Path: "device", Err: syscall.EACCES}

	a, ok := core.PermissionAbsence(refused)
	require.True(t, ok)
	assert.Equal(t, core.AbsencePermission, a.Code)
	assert.True(t, a.Actionable)
	assert.Equal(t, core.PermissionDetail, a.Detail)

	_, ok = core.PermissionAbsence(errors.New("timeout"))
	assert.False(t, ok)
}

// Wi-Fi details withheld carries its own verdict and the setting that would
// change it, where the panel used to write both.
func TestWiFiWithheldSaysWhatAndWhere(t *testing.T) {
	a, ok := core.AbsenceOf(core.ErrWirelessDenied)
	require.True(t, ok)
	assert.Equal(t, "details withheld", a.Text)
	assert.Contains(t, a.Detail, "ms-settings:privacy-location")
}
