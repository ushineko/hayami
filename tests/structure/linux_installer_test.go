package structure_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// InstallersEnv names the directory `make installer` wrote to. The test below
// installs for real and needs the built programs, so it runs where they were
// built and is skipped where they were not.
const InstallersEnv = "HAYAMI_INSTALLERS"

// standIn is a program that only waits, built as a panel install.sh left
// running: a real process for the installer to find.
const standIn = `package main

import "time"

func main() { time.Sleep(time.Minute) }
`

/*
Spec 063, against the installer a release carries. In a home of the test's
own, over a panel that install.sh left and that is running, the command-line
installer installs both panels where the desktop finds them, puts the device
rule where udev reads it, and stops the panel it replaced. The uninstaller
then takes all of it away, puts back what the install replaced, and leaves
the settings.

Nothing here is the desk's: every path is under t.TempDir().
*/
func TestTheLinuxInstallerInstallsOverAPanelAndUninstalls(t *testing.T) {
	linuxOnly(t)
	dist := os.Getenv(InstallersEnv)
	if dist == "" {
		t.Skipf("%s is not set: build the installers with `make installer` and point it at dist/", InstallersEnv)
	}
	version := strings.TrimSpace(readFile(t, filepath.Join(repoRoot(t), "VERSION")))
	installer := filepath.Join(dist, "hayami-"+version+"-linux-amd64-cli-installer")
	require.FileExists(t, installer)

	home, udev := t.TempDir(), t.TempDir()
	env := []string{"HOME=" + home, "HAYAMI_UDEV_DIR=" + udev, "PATH=" + os.Getenv("PATH")}
	run := func(name string, args ...string) string {
		t.Helper()
		cmd := exec.CommandContext(t.Context(), name, args...) //nolint:gosec // the installer under test
		cmd.Env = env
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "%s %v: %s", filepath.Base(name), args, out)
		return string(out)
	}

	// The panel install.sh left: a file in ~/.local/bin, running.
	link := filepath.Join(home, ".local", "bin", "hayami")
	require.NoError(t, os.MkdirAll(filepath.Dir(link), 0o755))
	src := filepath.Join(t.TempDir(), "main.go")
	require.NoError(t, os.WriteFile(src, []byte(standIn), 0o644))
	build := exec.CommandContext(t.Context(), "go", "build", "-o", link, src) //nolint:gosec // the stand-in above
	build.Env = append(os.Environ(), "CGO_ENABLED=0")
	out, err := build.CombinedOutput()
	require.NoError(t, err, "building the stand-in: %s", out)
	old := exec.CommandContext(t.Context(), link) //nolint:gosec // the stand-in above
	require.NoError(t, old.Start())
	ended := make(chan struct{})
	go func() { _ = old.Wait(); close(ended) }()
	t.Cleanup(func() { _ = old.Process.Kill(); <-ended })
	require.Eventually(t, func() bool {
		comm, _ := os.ReadFile(filepath.Join("/proc", strconv.Itoa(old.Process.Pid), "comm"))
		return strings.TrimSpace(string(comm)) == "hayami"
	}, 5*time.Second, 10*time.Millisecond)
	settings := filepath.Join(home, ".config", "hayami", "settings.yaml")
	require.NoError(t, os.MkdirAll(filepath.Dir(settings), 0o755))
	require.NoError(t, os.WriteFile(settings, []byte("opacity: 0.9\n"), 0o644))

	said := run(installer, "--yes", "--launch=false")

	root := filepath.Join(home, ".local", "share", "hayami")
	// This version, plain on a release and with its commit off one (Makefile,
	// BUILD_VERSION); the release job holds a release to the plain form.
	assert.Regexp(t, `^hayami-tui version `+regexp.QuoteMeta(version)+`(-[0-9a-f]+-dev)?$`,
		strings.TrimSpace(run(filepath.Join(home, ".local", "bin", "hayami-tui"), "--version")),
		"the terminal panel runs from the link on PATH")
	target, err := os.Readlink(link)
	require.NoError(t, err, "the panel on PATH is a link into the install")
	assert.Equal(t, filepath.Join(root, "bin", "hayami"), target)
	for _, entry := range []string{
		filepath.Join(home, ".local", "share", "applications", "io.ushineko.hayami.desktop"),
		filepath.Join(home, ".config", "autostart", "io.ushineko.hayami.desktop"),
	} {
		assert.Contains(t, readFile(t, entry), "Exec="+filepath.Join(root, "bin", "hayami")+"\n", entry)
	}
	assert.FileExists(t, filepath.Join(home, ".local", "share", "icons", "hicolor", "256x256", "apps", "io.ushineko.hayami.png"))

	assert.Equal(t, readFile(t, filepath.Join(repoRoot(t), "packaging", "60-sanshoku.rules")),
		readFile(t, filepath.Join(udev, "60-sanshoku.rules")), "the device rule is where udev reads it")
	assert.Contains(t, said, "Installed the device rule")

	select {
	case <-ended:
	case <-time.After(2 * time.Second):
		t.Error("the panel install.sh left still runs beside the new one")
	}
	assert.Contains(t, said, "Stopped hayami, which was running")

	run(filepath.Join(root, "uninstall"), "--cli")

	assert.NoDirExists(t, root)
	assert.NoFileExists(t, filepath.Join(udev, "60-sanshoku.rules"))
	assert.NoFileExists(t, filepath.Join(home, ".config", "autostart", "io.ushineko.hayami.desktop"))
	assert.NoFileExists(t, filepath.Join(home, ".local", "bin", "hayami-tui"))
	info, err := os.Lstat(link)
	require.NoError(t, err, "what the install replaced is put back")
	assert.True(t, info.Mode().IsRegular(), "and is the file it was, not a link")
	assert.Equal(t, "opacity: 0.9\n", readFile(t, settings), "the settings are the person's")
}

// readFile is a file's contents, or a failed test.
func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path) //nolint:gosec // a path the test made
	require.NoError(t, err)
	return string(b)
}
