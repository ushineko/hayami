package structure_test

import (
	"encoding/xml"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/hayami/internal/core"
)

// A LibreHardwareMonitor settings file as it writes one: keys hayami does not
// need, one it needs with the wrong value, one it needs already right, and
// most of what it needs missing. Invented values throughout.
const lhmSettings = "\xef\xbb\xbf<?xml version=\"1.0\" encoding=\"utf-8\"?>\r\n" +
	"<configuration>\r\n" +
	"  <appSettings>\r\n" +
	"    <add key=\"plotMenuItem\" value=\"false\" />\r\n" +
	"    <add key=\"runWebServerMenuItem\" value=\"false\" />\r\n" +
	"    <add key=\"listenerIp\" value=\"+\" />\r\n" +
	"    <add key=\"listenerPort\" value=\"8085\" />\r\n" +
	"    <add key=\"mainForm.Width\" value=\"470\" />\r\n" +
	"  </appSettings>\r\n" +
	"</configuration>\r\n"

// lhmKeys reads a settings file's keys and values.
func lhmKeys(t *testing.T, path string) map[string]string {
	t.Helper()
	body, err := os.ReadFile(path)
	require.NoError(t, err)
	var doc struct {
		Adds []struct {
			Key   string `xml:"key,attr"`
			Value string `xml:"value,attr"`
		} `xml:"appSettings>add"`
	}
	require.NoError(t, xml.Unmarshal([]byte(strings.TrimPrefix(string(body), "\xef\xbb\xbf")), &doc), "%s is not XML", path)
	out := map[string]string{}
	for _, a := range doc.Adds {
		out[a.Key] = a.Value
	}
	return out
}

// runLHMSettings runs scripts/lhm_settings.ps1 under Windows PowerShell 5.1.
func runLHMSettings(t *testing.T, args ...string) string {
	t.Helper()
	powershell, err := exec.LookPath("powershell.exe")
	if err != nil {
		t.Skip("no Windows PowerShell")
	}
	full := append([]string{"-NoProfile", "-ExecutionPolicy", "Bypass",
		"-File", filepath.Join(repoRoot(t), "scripts", "lhm_settings.ps1")}, args...)
	out, err := exec.CommandContext(t.Context(), powershell, full...).CombinedOutput()
	require.NoError(t, err, "%s", out)
	return string(out)
}

/*
Spec 042. The settings LibreHardwareMonitor needs for hayami are set, and
nothing else is touched: a key hayami does not need keeps its value, a needed
key with the wrong value is corrected, a missing one is added, and the file
as it was is kept once beside it. Run again, it has nothing to do and writes
nothing, so the installer can be run as often as anyone likes.
*/
func TestLibreHardwareMonitorSettingsAreSetOnceAndOnlyWhatHayamiNeeds(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("LibreHardwareMonitor's settings are set on Windows")
	}
	path := filepath.Join(t.TempDir(), "LibreHardwareMonitor.config")
	require.NoError(t, os.WriteFile(path, []byte(lhmSettings), 0o600))

	out := runLHMSettings(t, "-Path", path, "-Port", "9001")
	assert.Contains(t, out, "changes: 6")

	got := lhmKeys(t, path)
	assert.Equal(t, map[string]string{
		"plotMenuItem": "false", "listenerIp": "+", "mainForm.Width": "470", // untouched
		"runWebServerMenuItem":  "true", // corrected
		"listenerPort":          "9001", // the port hayami asks
		"authenticationEnabled": "false",
		"startMinMenuItem":      "true",
		"minCloseMenuItem":      "true",
		"minTrayMenuItem":       "true",
	}, got)

	backup, err := os.ReadFile(path + ".bak-hayami")
	require.NoError(t, err, "the file as it was is kept")
	assert.Equal(t, lhmSettings, string(backup))

	written, err := os.ReadFile(path)
	require.NoError(t, err)
	again := runLHMSettings(t, "-Path", path, "-Port", "9001")
	assert.Contains(t, again, "unchanged")
	assert.Contains(t, again, "changes: 0")
	after, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, written, after, "a second run wrote the file")
	backupAfter, err := os.ReadFile(path + ".bak-hayami")
	require.NoError(t, err)
	assert.Equal(t, lhmSettings, string(backupAfter), "the backup was overwritten")

	// A later change (another port) changes the file again and keeps the
	// first backup: it is the file before hayami ever touched it.
	runLHMSettings(t, "-Path", path, "-Port", "9002")
	assert.Equal(t, "9002", lhmKeys(t, path)["listenerPort"])
	backupLater, err := os.ReadFile(path + ".bak-hayami")
	require.NoError(t, err)
	assert.Equal(t, lhmSettings, string(backupLater), "a later change overwrote the first backup")
}

// Spec 042. A dry run says each key it would set and writes nothing; a
// machine with no settings file yet gets one with the keys, and no backup of
// a file that was never there.
func TestLibreHardwareMonitorSettingsDryRunWritesNothingAndAMissingFileIsMade(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("LibreHardwareMonitor's settings are set on Windows")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "LibreHardwareMonitor.config")
	require.NoError(t, os.WriteFile(path, []byte(lhmSettings), 0o600))

	out := runLHMSettings(t, "-Path", path, "-DryRun")
	assert.Contains(t, out, "would set runWebServerMenuItem=true")
	assert.Contains(t, out, "would set minCloseMenuItem=true")
	assert.NotContains(t, out, "listenerPort", "a key already right is not mentioned")
	body, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, lhmSettings, string(body), "a dry run wrote the file")
	assert.NoFileExists(t, path+".bak-hayami")

	fresh := filepath.Join(dir, "fresh", "LibreHardwareMonitor.config")
	require.NoError(t, os.MkdirAll(filepath.Dir(fresh), 0o750))
	runLHMSettings(t, "-Path", fresh)
	assert.Equal(t, "true", lhmKeys(t, fresh)["startMinMenuItem"])
	assert.Equal(t, "8085", lhmKeys(t, fresh)["listenerPort"])
	assert.NoFileExists(t, fresh+".bak-hayami")
}

/*
Spec 042. On a machine without LibreHardwareMonitor, a dry run says every
step -- install, settings, the startup task, the start, the one UAC prompt,
the check -- and does none of them. The per-user directories are an empty
throwaway, so this machine's own install is not found.
*/
func TestADryRunOnAFreshMachineSaysEverySensorsStep(t *testing.T) {
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
	cmd.Env = append(os.Environ(), "LOCALAPPDATA="+filepath.Join(dir, "local"), "APPDATA="+filepath.Join(dir, "roaming"))
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "%s", out)
	text := string(out)

	for _, step := range []string{
		"would run: winget install --id LibreHardwareMonitor.LibreHardwareMonitor --exact",
		"would set its settings (stopped first): web server on, port 8085, no password,",
		"closing the window hides it in the notification area",
		"would register the startup task 'LibreHardwareMonitor': at logon, highest privileges",
		"would start it through the task: one UAC prompt for all of this",
	} {
		assert.Contains(t, text, step)
	}
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Empty(t, entries, "a dry run wrote something")
}

// Spec 042. The installer's check reads the processor temperature by the
// labels the panel reads it by, in the same order. Held here, because a
// label added to one and not the other would pass the installer's check on a
// machine the panel then cannot read.
func TestTheInstallersCheckReadsThePanelsLabels(t *testing.T) {
	body, err := os.ReadFile(filepath.Join(repoRoot(t), "scripts", "install_windows.ps1"))
	require.NoError(t, err)
	line := regexp.MustCompile(`(?m)^\$SensorsCPULabels = @\((.*)\)\s*$`).FindStringSubmatch(string(body))
	require.Len(t, line, 2, "install_windows.ps1 has no $SensorsCPULabels")
	var labels []string
	for _, m := range regexp.MustCompile(`"([^"]*)"`).FindAllStringSubmatch(line[1], -1) {
		labels = append(labels, m[1])
	}
	assert.Equal(t, core.LHMCPULabels, labels)
}
