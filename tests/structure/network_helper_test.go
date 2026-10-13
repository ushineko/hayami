package structure_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

/*
Spec 061. -WithNetwork is the one way the checkout's installer touches the
network helper, and a dry run of it says what the elevated step would do and
runs nothing. Without the switch the helper is not mentioned as a step, only
offered.
*/
func TestTheWindowsInstallerOffersTheNetworkHelperOnlyWhenAsked(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("install_windows.ps1 installs for Windows")
	}
	powershell, err := exec.LookPath("powershell.exe")
	if err != nil {
		t.Skip("no Windows PowerShell")
	}
	run := func(extra ...string) string {
		dir := t.TempDir()
		args := append([]string{"-NoProfile", "-ExecutionPolicy", "Bypass",
			"-File", filepath.Join(repoRoot(t), "scripts", "install_windows.ps1"),
			"-DryRun", "-Destination", filepath.Join(dir, "prog"),
			"-StartMenuDir", filepath.Join(dir, "menu"), "-StartupDir", filepath.Join(dir, "startup")}, extra...)
		out, err := exec.CommandContext(t.Context(), powershell, args...).CombinedOutput()
		require.NoError(t, err, "%s", out)
		entries, err := os.ReadDir(dir)
		require.NoError(t, err)
		assert.Empty(t, entries, "a dry run wrote something")
		return string(out)
	}

	asked := run("-WithNetwork")
	assert.Contains(t, asked, "The network helper, for the bandwidth card's list of programs")
	assert.Regexp(t, `would run: .*hayami-tui\.exe network-helper install`, asked)
	assert.Contains(t, asked, "Local Service")
	assert.Contains(t, asked, "UAC")

	plain := run()
	assert.NotContains(t, plain, "would run:", "the helper is installed only when asked for")
	assert.Contains(t, plain, "-WithNetwork", "the switch is offered")
}

// Spec 061. The uninstaller removes the helper through the program that
// installed it, before that program is removed, and only where the service
// is registered.
func TestTheWindowsUninstallerRemovesTheNetworkHelper(t *testing.T) {
	body, err := os.ReadFile(filepath.Join(repoRoot(t), "scripts", "uninstall_windows.ps1"))
	require.NoError(t, err)
	text := string(body)
	helper := strings.Index(text, "Get-Service hayami-network")
	files := strings.Index(text, "Remove-Item -Force $path")
	require.GreaterOrEqual(t, helper, 0, "the uninstaller does not look for the helper")
	require.GreaterOrEqual(t, files, 0)
	assert.Less(t, helper, files, "the helper is removed while hayami-tui.exe is still there to remove it")
	assert.Contains(t, text, "network-helper remove")
	assert.Contains(t, text, "sc.exe delete hayami-network", "what to do by hand is said")
}
