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

// repoRoot is the checkout this test runs against.
func repoRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs("../..")
	require.NoError(t, err)
	return root
}

/*
The launcher entry names the binary by its full path.

A bare `Exec=hayami` resolves against the *session's* PATH, which comes from
the display manager and environment.d rather than from a shell's rc files. On
one machine `~/.local/bin` is on it and on the next it is not, so the entry
launched the panel on the machine it was written on and did nothing at all —
no window, no error — on the other (issue #60).

The packaged file keeps its bare Exec, which is right for a system install
where /usr/bin is always on the session PATH; the installer is what has to know
where it put things.
*/
func TestTheInstalledEntryNamesTheBinaryAbsolutely(t *testing.T) {
	linuxOnly(t)
	root := repoRoot(t)
	home := t.TempDir()

	cmd := exec.CommandContext(t.Context(), "bash",
		filepath.Join(root, "install.sh"), "--dry-run", "--autostart")
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "HOME="+home)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "install.sh --dry-run failed: %s", out)

	want := "Exec=" + filepath.Join(home, ".local", "bin", "hayami")

	// Counted, not merely present. Both entries are written -- the launcher's
	// and the autostart copy -- and they fail independently: a first version
	// of this test asserted the string appeared *somewhere*, and passed with
	// the launcher entry reverted to its bare Exec because the autostart one
	// still carried the absolute path. The autostart half fails silently until
	// a login, which is the worse of the two to miss.
	assert.Equal(t, 2, strings.Count(string(out), want),
		"both the launcher entry and the autostart copy must name the binary absolutely:\n%s", out)

	for _, dest := range []string{
		filepath.Join(home, ".local", "share", "applications"),
		filepath.Join(home, ".config", "autostart"),
	} {
		assert.Contains(t, string(out), dest, "no entry would be installed into %s", dest)
	}
}

/*
The packaged entry keeps its bare Exec and its load-bearing name.

Spec 014 records why the basename must stay io.ushineko.hayami.desktop: Fyne
sets the Wayland app_id from gui.AppID, and a compositor matches that against a
desktop file of the same name to find the window's icon. This is here so a
tidy-up cannot rename it without a test saying no.
*/
func TestThePackagedEntryIsSystemShapedAndKeepsItsName(t *testing.T) {
	root := repoRoot(t)
	path := filepath.Join(root, "packaging", "io.ushineko.hayami.desktop")

	body, err := os.ReadFile(path) //nolint:gosec // a path inside this checkout
	require.NoError(t, err, "the launcher entry's basename is load-bearing; see spec 014")

	lines := strings.Split(string(body), "\n")
	assert.Contains(t, lines, "Exec=hayami",
		"the packaged entry is for a system install, where the bare name resolves")
	assert.Contains(t, lines, "StartupWMClass=io.ushineko.hayami",
		"X11 and XWayland match the window on WM_CLASS")
}

// installer runs install.sh (or uninstall.sh) as a dry run against a udev
// directory the test made, and returns what it printed. A dry run, so a
// checkout without its binaries built is enough and nothing is written.
func installer(t *testing.T, script, udevDir string) string {
	t.Helper()
	linuxOnly(t)
	root := repoRoot(t)
	cmd := exec.CommandContext(t.Context(), "bash", filepath.Join(root, script), "--dry-run")
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "HOME="+t.TempDir(), "HAYAMI_UDEV_DIR="+udevDir)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "%s must not fail over the udev rule: %s", script, out)
	return string(out)
}

// readOnlyDir is a directory the installer may not write to, which is what
// /etc/udev/rules.d is to a user.
func readOnlyDir(t *testing.T, rule []byte) string {
	t.Helper()
	if os.Geteuid() == 0 {
		t.Skip("root may write anywhere, so there is no privilege to lack")
	}
	dir := t.TempDir()
	if rule != nil {
		require.NoError(t, os.WriteFile(filepath.Join(dir, "60-sanshoku.rules"), rule, 0o600))
	}
	require.NoError(t, os.Chmod(dir, 0o555))
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
	return dir
}

func packagedRule(t *testing.T) []byte {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(repoRoot(t), "packaging", "60-sanshoku.rules"))
	require.NoError(t, err)
	return body
}

/*
R5.1. Without the rule and without the privilege to install it, the installer
prints the two commands that do, and succeeds.

liquidctl's and OpenRazer's packages used to put a rule like this in place;
reading the devices directly, nothing does, and a device that is found and
may not be opened otherwise looks like no device at all. The installer does
not become root to fix it.
*/
func TestTheInstallerPrintsTheUdevCommandsWhenItMayNotInstall(t *testing.T) {
	dir := readOnlyDir(t, nil)

	out := installer(t, "install.sh", dir)

	root := repoRoot(t)
	assert.Contains(t, out, "sudo install -m644 "+filepath.Join(root, "packaging", "60-sanshoku.rules")+" "+
		filepath.Join(dir, "60-sanshoku.rules"))
	assert.Contains(t, out, "sudo udevadm control --reload")
	assert.NotContains(t, out, "would run: sudo", "the installer tried to become root")
}

// R5.1. With the privilege, it installs the rule and reloads udev itself.
func TestTheInstallerInstallsTheUdevRuleWhenItMay(t *testing.T) {
	dir := t.TempDir()

	out := installer(t, "install.sh", dir)

	assert.Contains(t, out, "would run: install -m644 ")
	assert.Contains(t, out, filepath.Join(dir, "60-sanshoku.rules"))
	assert.NotContains(t, out, "sudo ")
}

// R5.1. With the rule in place, the installer says nothing about it -- whether
// it is this copy or sanshoku's own, which differs only in its comments.
func TestTheInstallerSaysNothingWhenTheRuleIsThere(t *testing.T) {
	ours := packagedRule(t)
	var rulesOnly []byte
	for line := range strings.SplitSeq(string(ours), "\n") {
		if !strings.HasPrefix(line, "#") {
			rulesOnly = append(rulesOnly, line+"\n"...)
		}
	}

	for name, body := range map[string][]byte{"this copy": ours, "the same rules, other comments": rulesOnly} {
		out := installer(t, "install.sh", readOnlyDir(t, body))
		assert.NotContains(t, out, "60-sanshoku", name)
		assert.NotContains(t, out, "udevadm", name)
	}
}

// R5.1. The uninstaller mirrors it: this copy of the rule, where it may not be
// removed, gets the two commands that remove it; a file of the same name that
// install.sh did not write is left alone.
func TestTheUninstallerMirrorsTheUdevRule(t *testing.T) {
	out := installer(t, "uninstall.sh", readOnlyDir(t, packagedRule(t)))
	assert.Contains(t, out, "sudo rm ")
	assert.Contains(t, out, "sudo udevadm control --reload")

	out = installer(t, "uninstall.sh", readOnlyDir(t, []byte("# somebody else's\n")))
	assert.Contains(t, out, "Left ")
	assert.NotContains(t, out, "sudo rm ")
}

// linuxOnly skips a test of install.sh or uninstall.sh anywhere else: they
// install for a Linux desktop, and Windows has scripts/install_windows.ps1.
func linuxOnly(t *testing.T) {
	t.Helper()
	if runtime.GOOS != "linux" {
		t.Skip("install.sh installs for a Linux desktop")
	}
}
