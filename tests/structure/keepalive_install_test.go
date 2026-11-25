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

// Spec 113 R25. The same for the service other programs read: the uninstaller takes
// its socket and service, hayami's own only, and the installer restarts it on the new
// program and sets none up.
func TestTheShellScriptsLookAfterTheAPIService(t *testing.T) {
	linuxOnly(t)
	run := func(script string, units map[string]string) string {
		t.Helper()
		config := t.TempDir()
		dir := filepath.Join(config, "systemd", "user")
		require.NoError(t, os.MkdirAll(dir, 0o700))
		for name, body := range units {
			require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600))
		}
		root := repoRoot(t)
		cmd := exec.CommandContext(t.Context(), "bash", filepath.Join(root, script), "--dry-run")
		cmd.Dir = root
		cmd.Env = append(os.Environ(), "HOME="+t.TempDir(), "HAYAMI_UDEV_DIR="+t.TempDir(), "XDG_CONFIG_HOME="+config)
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "%s: %s", script, out)
		return string(out)
	}
	const ours = "# Written by hayami's installer (spec 113). Running the installer again, or uninstalling hayami, removes it.\n[Unit]\n"
	const theirs = "[Socket]\nListenStream=%t/their.sock\n"
	both := map[string]string{"hayami-api.socket": ours, "hayami-api.service": ours}

	assert.Contains(t, run("uninstall.sh", both), "would stop and remove hayami-api.socket hayami-api.service")
	mixed := run("uninstall.sh", map[string]string{"hayami-api.socket": theirs, "hayami-api.service": ours})
	assert.Contains(t, mixed, "hayami-api.socket alone: hayami did not write it")
	assert.Contains(t, mixed, "would stop and remove hayami-api.service")
	assert.NotContains(t, run("uninstall.sh", nil), "hayami-api")

	assert.Contains(t, run("install.sh", both), "would run: systemctl --user try-restart hayami-api.service")
	assert.NotContains(t, run("install.sh", nil), "hayami-api", "set up only by the installer's page")
}
