package cooler_test

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/hayami/internal/cooler"
)

/*
fakeLiquidctl puts a liquidctl on PATH that says what the test tells it to.

A script rather than a stubbed function, because the thing under test is how a
*subprocess* fails: the exit status and the stderr, which is exactly the pair
that was being thrown away. A fake that returned an error value would assert
the classification against a shape that never reaches production.
*/
func fakeLiquidctl(t *testing.T, stdout, stderr string, code int) {
	t.Helper()
	dir := t.TempDir()
	script := "#!/bin/sh\n" +
		"printf '%s' " + quote(stdout) + "\n" +
		"printf '%s' " + quote(stderr) + " >&2\n" +
		"exit " + itoa(code) + "\n"
	path := filepath.Join(dir, "liquidctl")
	require.NoError(t, os.WriteFile(path, []byte(script), 0o700)) //nolint:gosec // a test's own script
	t.Setenv("PATH", dir)
}

// quote wraps a string for the shell, which is safe here because every caller
// passes a literal from this file.
func quote(s string) string { return "'" + s + "'" }

/*
A machine with no cooler is not a machine where liquidctl failed.

This is the whole bug. `liquidctl --match kraken status` on a box with no
Kraken prints its complaint and **exits 1**, and every non-zero exit looked the
same from here -- so the error was neither ErrNoCooler nor ErrNoLiquidctl, the
cooler source treated it as a fault, and the section was dropped along with the
processor temperature that was being read perfectly well (issue #54).
*/
func TestNoDeviceMatchesIsNoCoolerAndNotAFailure(t *testing.T) {
	fakeLiquidctl(t, "", "ERROR: no device matches available drivers and selection criteria\n", 1)

	_, err := cooler.Cooling(context.Background())

	require.Error(t, err)
	assert.ErrorIs(t, err, cooler.ErrNoCooler,
		"a machine with no cooler was reported as liquidctl failing")
}

// Any other non-zero exit stays the failure it is, and carries enough to act
// on: the exit status and what liquidctl actually said.
func TestAnyOtherFailureKeepsItsExitStatusAndItsMessage(t *testing.T) {
	fakeLiquidctl(t, "", "Traceback (most recent call last):\nOSError: [Errno 13] Permission denied\n", 1)

	_, err := cooler.Cooling(context.Background())

	require.Error(t, err)
	assert.NotErrorIs(t, err, cooler.ErrNoCooler)
	assert.NotErrorIs(t, err, cooler.ErrNoLiquidctl)
	assert.Contains(t, err.Error(), "exit status 1")
	assert.Contains(t, err.Error(), "Traceback",
		"the first line of liquidctl's complaint is what makes the error actionable")

	var exit *exec.ExitError
	assert.True(t, errors.As(err, &exit), "the exit status must survive for a caller that wants it")
}

// A liquidctl that succeeds and reports no liquid temperature is still
// ErrNoCooler: this program looks at machines that may not have one, and the
// two routes to "no cooler" must not be distinguishable to a caller.
func TestASuccessfulRunWithNoLiquidDeviceIsAlsoNoCooler(t *testing.T) {
	fakeLiquidctl(t, `[{"description":"Corsair HX1000i","status":[{"key":"VRM temperature","value":40.0,"unit":"C"}]}]`, "", 0)

	_, err := cooler.Cooling(context.Background())

	assert.ErrorIs(t, err, cooler.ErrNoCooler)
}

// An absent liquidctl is its own answer, and not a failure either.
func TestAnAbsentLiquidctlIsNotAFailure(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	_, err := cooler.Cooling(context.Background())

	assert.ErrorIs(t, err, cooler.ErrNoLiquidctl)
}

// A complaint long enough to be a stack trace is bounded: it ends up in a
// tooltip and in doctor's output, neither of which wants a page of Python.
func TestAVeryLongComplaintIsBounded(t *testing.T) {
	long := make([]byte, 4096)
	for i := range long {
		long[i] = 'x'
	}
	fakeLiquidctl(t, "", string(long), 1)

	_, err := cooler.Cooling(context.Background())

	require.Error(t, err)
	assert.Less(t, len(err.Error()), 400, "a subprocess's whole stderr reached the error")
}
