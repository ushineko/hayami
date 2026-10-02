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
}
