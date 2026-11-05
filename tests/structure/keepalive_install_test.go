package structure_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Spec 085. The shell scripts, in a dry run over a configuration directory the test
// made: the uninstaller takes hayami's own service and no other, and the installer
// restarts one that is there and starts none that is not.
func TestTheShellScriptsLookAfterTheKeepaliveService(t *testing.T) {
	linuxOnly(t)
	run := func(script, unit string) string {
		t.Helper()
		config := t.TempDir()
		if unit != "" {
			dir := filepath.Join(config, "systemd", "user")
			require.NoError(t, os.MkdirAll(dir, 0o700))
			require.NoError(t, os.WriteFile(filepath.Join(dir, "hayami-vpn-keepalive.service"), []byte(unit), 0o600))
		}
		root := repoRoot(t)
		cmd := exec.CommandContext(t.Context(), "bash", filepath.Join(root, script), "--dry-run")
		cmd.Dir = root
		cmd.Env = append(os.Environ(), "HOME="+t.TempDir(), "HAYAMI_UDEV_DIR="+t.TempDir(), "XDG_CONFIG_HOME="+config)
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "%s: %s", script, out)
		return string(out)
	}
	const ours = "# Written by hayami-tui vpn-keepalive install. hayami-tui vpn-keepalive remove takes it away.\n[Unit]\n"
	const theirs = "[Service]\nExecStart=/their/own\n"

	assert.Contains(t, run("uninstall.sh", ours), "would stop and remove")
	gone := run("uninstall.sh", theirs)
	assert.Contains(t, gone, "hayami did not write it")
	assert.NotContains(t, gone, "would stop and remove")
	assert.NotContains(t, run("uninstall.sh", ""), "keepalive")

	assert.Contains(t, run("install.sh", ours), "would run: systemctl --user try-restart hayami-vpn-keepalive.service")
	assert.NotContains(t, run("install.sh", ""), "keepalive", "installed only when asked")
}
