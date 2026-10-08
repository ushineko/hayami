package structure_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

/*
The PowerShell scripts carry CRLF and a UTF-8 BOM.

Windows PowerShell 5.1 -- the one an unprepared Windows machine has -- reads a
script without a BOM as the system code page and mis-parses an LF script's
here-strings, and it fails somewhere unrelated to either. .gitattributes pins
CRLF on checkout; the BOM is in the file. Checked on every platform, because a
Linux editor is where either would be lost.
*/
func TestThePowerShellScriptsCarryCRLFAndABOM(t *testing.T) {
	scripts, err := filepath.Glob(filepath.Join(repoRoot(t), "scripts", "*.ps1"))
	require.NoError(t, err)
	require.NotEmpty(t, scripts, "no PowerShell scripts found")

	for _, path := range scripts {
		body, err := os.ReadFile(path)
		require.NoError(t, err)
		name := filepath.Base(path)
		assert.True(t, bytes.HasPrefix(body, []byte{0xEF, 0xBB, 0xBF}), "%s has no UTF-8 BOM", name)
		lf := bytes.Count(body, []byte("\n"))
		crlf := bytes.Count(body, []byte("\r\n"))
		assert.Equal(t, lf, crlf, "%s has a line ending that is not CRLF", name)
	}
}

// A dry run of the Windows installer names what it would write and writes
// none of it, and so needs neither Go nor gcc. It is run under Windows
// PowerShell 5.1 for the reason above.
func TestTheWindowsInstallerDryRunWritesNothing(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("install_windows.ps1 installs for Windows")
	}
	powershell, err := exec.LookPath("powershell.exe")
	if err != nil {
		t.Skip("no Windows PowerShell")
	}
	dir := t.TempDir()
	dest := filepath.Join(dir, "prog")
	menu := filepath.Join(dir, "menu")
	startup := filepath.Join(dir, "startup")

	cmd := exec.CommandContext(t.Context(), powershell, "-NoProfile", "-ExecutionPolicy", "Bypass",
		"-File", filepath.Join(repoRoot(t), "scripts", "install_windows.ps1"),
		"-DryRun", "-Autostart", "-Destination", dest, "-StartMenuDir", menu, "-StartupDir", startup)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "%s", out)

	for _, want := range []string{
		filepath.Join(dest, "hayami.exe"),
		filepath.Join(dest, "hayami-tui.exe"),
		filepath.Join(menu, "hayami.lnk"),
		filepath.Join(startup, "hayami.lnk"),
	} {
		assert.Contains(t, string(out), want)
	}
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Empty(t, entries, "a dry run wrote something")
	assert.NotContains(t, string(out), "LibreHardwareMonitor, for the processor",
		"LibreHardwareMonitor is installed only when asked for")
	assert.Contains(t, string(out), "-WithSensors", "the switch is offered")
}

/*
Spec 036. -WithSensors is the one way the installer touches
LibreHardwareMonitor, and a dry run of it says each step -- install, settings,
an elevated start -- and the firewall point, and changes nothing: no winget
run, no settings written, no process started. On a machine that already has
it the steps say so instead; either way the output names it.
*/
func TestTheWindowsInstallerOffersLibreHardwareMonitorOnlyWhenAsked(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("install_windows.ps1 installs for Windows")
	}
	powershell, err := exec.LookPath("powershell.exe")
	if err != nil {
		t.Skip("no Windows PowerShell")
	}
	dir := t.TempDir()

	cmd := exec.CommandContext(t.Context(), powershell, "-NoProfile", "-ExecutionPolicy", "Bypass",
		"-File", filepath.Join(repoRoot(t), "scripts", "install_windows.ps1"),
		"-DryRun", "-WithSensors", "-Destination", filepath.Join(dir, "prog"),
		"-StartMenuDir", filepath.Join(dir, "menu"), "-StartupDir", filepath.Join(dir, "startup"))
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "%s", out)
	text := string(out)

	assert.Contains(t, text, "LibreHardwareMonitor, for the processor's temperature")
	assert.Regexp(t, `would run: winget install --id LibreHardwareMonitor\.LibreHardwareMonitor --exact|already installed: `, text)
	assert.Regexp(t, `would write, if it has no settings yet|its settings are left as they are`, text)
	assert.Contains(t, text, "would start it as administrator")
	assert.Contains(t, text, "Windows Firewall's default (block inbound) is what keeps port 8085 off your network")

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Empty(t, entries, "a dry run wrote something")
}

// Spec 036. Uninstalling hayami leaves LibreHardwareMonitor, which may have
// uses of its own, and says how to remove it.
func TestTheWindowsUninstallerLeavesLibreHardwareMonitor(t *testing.T) {
	body, err := os.ReadFile(filepath.Join(repoRoot(t), "scripts", "uninstall_windows.ps1"))
	require.NoError(t, err)
	assert.Contains(t, string(body), "LibreHardwareMonitor and PawnIO")
	assert.NotContains(t, string(body), "winget uninstall --id LibreHardwareMonitor.LibreHardwareMonitor\r\n", "it is named, never run")
	assert.NotRegexp(t, `(?m)^\s*&\s*winget`, string(body), "the uninstaller runs no winget")
}
