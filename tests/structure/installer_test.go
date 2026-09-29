package structure_test

import (
	"os"
	"os/exec"
	"path/filepath"
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
