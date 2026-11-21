package cli

import (
	"bytes"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Specs 061 and 072. The terminal program carries the network helper's
// command, and asking after the helper changes nothing: it prints what is
// there. Off Windows and Linux that is that none is needed.
func TestTheHelpersCommandSaysWhatIsThere(t *testing.T) {
	root := TUI("1.2.3")
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetArgs([]string{"network-helper", "status"})
	require.NoError(t, root.Execute())
	if runtime.GOOS == "windows" || runtime.GOOS == "linux" {
		assert.Regexp(t, `^(not installed|installed: )`, out.String())
	} else {
		assert.Contains(t, out.String(), "not needed")
	}
}

// What the service runs is not in the help, and refuses to run without the
// account it serves: a pipe open to nobody in particular is not one this
// program makes.
func TestTheServicesOwnCommandNeedsItsAccount(t *testing.T) {
	root := TUI("1.2.3")
	run, _, err := root.Find([]string{"network-helper", "run"})
	require.NoError(t, err)
	assert.True(t, run.Hidden)

	root.SetArgs([]string{"network-helper", "run"})
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	err = root.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "allow")
}

// Spec 103. The sensor helper's command says what is there, and its run
// needs the account it serves, as the network helper's does.
func TestTheSensorHelpersCommand(t *testing.T) {
	root := TUI("1.2.3")
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetArgs([]string{"sensor-helper", "status"})
	require.NoError(t, root.Execute())
	if runtime.GOOS == "windows" {
		assert.Regexp(t, `^(not installed|installed: )`, out.String())
	} else {
		assert.Contains(t, out.String(), "not needed")
	}

	root = TUI("1.2.3")
	run, _, err := root.Find([]string{"sensor-helper", "run"})
	require.NoError(t, err)
	assert.True(t, run.Hidden)
	root.SetArgs([]string{"sensor-helper", "run"})
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	require.ErrorContains(t, root.Execute(), "allow")
}
