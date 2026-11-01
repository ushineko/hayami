package nethelper

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// installInto points the helper's files at a directory of the test's and
// records what systemd is asked, for the length of a test.
func installInto(t *testing.T) (string, *[]string) {
	t.Helper()
	root := t.TempDir()
	var asked []string
	wasRoot, wasCtl := sysRoot, systemctl
	sysRoot = root
	systemctl = func(_ context.Context, args ...string) error {
		asked = append(asked, strings.Join(args, " "))
		return nil
	}
	t.Cleanup(func() { sysRoot, systemctl = wasRoot, wasCtl })
	return root, &asked
}

// Spec 072. Installing copies the program where only root can change it,
// writes both units for the account named, and starts the socket; the
// version it wrote is read back. Removing takes all three.
func TestInstallingWritesTheUnitsAndRemovingTakesThem(t *testing.T) {
	root, asked := installInto(t)

	require.NoError(t, Install("1234", "1.2.3"))
	program, err := os.Stat(filepath.Join(root, "usr/lib/hayami/hayami-network"))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o755), program.Mode().Perm(), "run by everyone, written by its owner only")

	socket, err := os.ReadFile(filepath.Join(root, "etc/systemd/system/hayami-network.socket"))
	require.NoError(t, err)
	assert.Contains(t, string(socket), "SocketUser=1234\n")
	assert.Contains(t, string(socket), "SocketMode=0600\n")
	assert.Contains(t, string(socket), "ListenStream=/run/hayami-network.sock\n")

	service, err := os.ReadFile(filepath.Join(root, "etc/systemd/system/hayami-network.service"))
	require.NoError(t, err)
	assert.Contains(t, string(service), "ExecStart=/usr/lib/hayami/hayami-network network-helper run --allow 1234\n")
	assert.Contains(t, string(service), "DynamicUser=yes\n")
	assert.Contains(t, string(service), "CapabilityBoundingSet=CAP_BPF CAP_NET_ADMIN CAP_PERFMON\n")
	assert.NotContains(t, string(service), "[Install]", "the service is started by its socket and by nothing else")

	assert.Equal(t, []string{
		"daemon-reload", "stop hayami-network.service", "enable hayami-network.socket", "restart hayami-network.socket",
	}, *asked)

	state := Probe()
	assert.True(t, state.Applies)
	assert.False(t, state.Offered, "hayami's installer does not become root to install it")
	assert.True(t, state.Installed)
	assert.Equal(t, "1.2.3", state.Version)
	assert.Equal(t, "1234", state.For)
	assert.Equal(t, "/usr/lib/hayami/hayami-network", state.Path)

	require.NoError(t, Remove())
	for _, p := range []string{
		"usr/lib/hayami/hayami-network", "usr/lib/hayami",
		"etc/systemd/system/hayami-network.socket", "etc/systemd/system/hayami-network.service",
	} {
		assert.NoFileExists(t, filepath.Join(root, p))
	}
	assert.False(t, Probe().Installed)
}

// An account is named by user name or number, and one that does not exist
// is refused before anything is written.
func TestTheAccountIsNamedOrNumbered(t *testing.T) {
	uid, err := uidOf("1234")
	require.NoError(t, err)
	assert.Equal(t, uint32(1234), uid)

	uid, err = uidOf("root")
	require.NoError(t, err)
	assert.Equal(t, uint32(0), uid)

	root, _ := installInto(t)
	require.Error(t, Install("no-such-account-here", "1.2.3"))
	assert.NoFileExists(t, filepath.Join(root, "etc/systemd/system/hayami-network.service"))
}

// Without root, installing is refused with the command to run, and nothing
// asks for a password.
func TestInstallingWithoutRootSaysWhatToRun(t *testing.T) {
	err := RunElevated(t.Context(), "/opt/hayami/bin/hayami-tui", "network-helper", "install", "--allow", "1000")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "`sudo /opt/hayami/bin/hayami-tui network-helper install --allow 1000`")
}
