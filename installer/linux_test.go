package installer

import (
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/fynstall/extension"
)

// standInEnv makes this test program a process that only waits, so a test
// has a real one to stop: a copy of it named hayami is a panel as far as the
// kernel's name and the file's place say.
const standInEnv = "HAYAMI_TEST_STAND_IN"

func TestMain(m *testing.M) {
	if os.Getenv(standInEnv) != "" {
		time.Sleep(time.Minute)
		return
	}
	os.Exit(m.Run())
}

// A rule with invented contents: what the tests compare is whether a file is
// this one, not what it grants.
const rule = "# the installer's copy\nKERNEL==\"hidraw*\", ATTRS{idVendor}==\"aaaa\", TAG+=\"uaccess\"\n"

// installedAt is the context of an install whose directory carries the rule,
// with udev's directory a test's own. It returns the context, the directory
// and what was logged.
func installedAt(t *testing.T, upgrade bool) (c *extension.Context, udev string, logged *[]string) {
	t.Helper()
	root := t.TempDir()
	udev = t.TempDir()
	t.Setenv(UdevDirEnv, udev)
	src := filepath.Join(root, filepath.FromSlash(rulePayload))
	require.NoError(t, os.MkdirAll(filepath.Dir(src), 0o755))
	require.NoError(t, os.WriteFile(src, []byte(rule), 0o644))
	logged = &[]string{}
	c = extension.NewContext(extension.Host{
		App:      extension.App{ID: "io.ushineko.hayami", Name: "hayami", Version: "0.0.0"},
		Scope:    func() string { return "user" },
		Root:     func() string { return root },
		Params:   map[string]string{ParamSensors: No, ParamNetwork: No},
		Declared: []string{ParamSensors, ParamNetwork},
		Log:      func(_ extension.Level, text string) { *logged = append(*logged, text) },
		Upgrade:  upgrade,
	})
	return c, udev, logged
}

// readOnly makes dir one a person may not write to, as /etc/udev/rules.d is.
// Root may write anywhere, and a test of being refused has nothing to say
// there.
func readOnly(t *testing.T, dir string) {
	t.Helper()
	if os.Geteuid() == 0 {
		t.Skip("root is refused nothing")
	}
	require.NoError(t, os.Chmod(dir, 0o555))
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
}

// Spec 063 R3. Where udev's directory can be written, the install puts the
// rule there and says to replug.
func TestTheRuleIsInstalledWhereItMayBe(t *testing.T) {
	c, udev, logged := installedAt(t, false)
	require.NoError(t, afterInstall(context.Background(), c))

	got, err := os.ReadFile(filepath.Join(udev, ruleName))
	require.NoError(t, err, "the rule is where udev reads it")
	assert.Equal(t, rule, string(got))
	said := strings.Join(*logged, "\n")
	assert.Contains(t, said, "Installed the device rule to "+filepath.Join(udev, ruleName))
	assert.Contains(t, said, "Replug")
	assert.NotContains(t, said, "sudo", "nothing is asked of a person where nothing is needed")
}

// Where it cannot, the install succeeds, writes nothing, and says the two
// commands that install the rule, with the paths of this install.
func TestTheRuleIsExplainedWhereItMayNotBeWritten(t *testing.T) {
	c, udev, logged := installedAt(t, false)
	readOnly(t, udev)
	require.NoError(t, afterInstall(context.Background(), c), "an install does not fail over the rule")

	assert.NoFileExists(t, filepath.Join(udev, ruleName))
	entries, err := os.ReadDir(udev)
	require.NoError(t, err)
	assert.Empty(t, entries, "nothing is left half written")
	said := strings.Join(*logged, "\n")
	assert.Contains(t, said, "sudo install -m644 "+filepath.Join(c.Root(), "share", ruleName)+" "+filepath.Join(udev, ruleName))
	assert.Contains(t, said, "sudo udevadm control --reload")
	assert.Contains(t, said, "not permitted")
}

// A rule that is there already and says the same is left as it is, whatever
// its comments: sanshoku's own copy has another header.
func TestARuleAlreadyInPlaceIsLeftAlone(t *testing.T) {
	c, udev, logged := installedAt(t, false)
	theirs := strings.Replace(rule, "# the installer's copy", "# another program's copy\n", 1)
	dest := filepath.Join(udev, ruleName)
	require.NoError(t, os.WriteFile(dest, []byte(theirs), 0o644))
	readOnly(t, udev)
	require.NoError(t, afterInstall(context.Background(), c))

	got, err := os.ReadFile(dest)
	require.NoError(t, err)
	assert.Equal(t, theirs, string(got))
	assert.NotContains(t, strings.Join(*logged, "\n"), "sudo", "a rule that is in place is not asked for")
}

// An uninstall takes the rule it wrote. An upgrade leaves it, and so does an
// uninstall that finds another program's file under the same name.
func TestAnUninstallRemovesOnlyTheRuleItWrote(t *testing.T) {
	t.Run("its own", func(t *testing.T) {
		c, udev, logged := installedAt(t, false)
		dest := filepath.Join(udev, ruleName)
		require.NoError(t, os.WriteFile(dest, []byte(rule), 0o644))
		require.NoError(t, beforeUninstall(context.Background(), c))
		assert.NoFileExists(t, dest)
		assert.Contains(t, strings.Join(*logged, "\n"), "Removed the device rule")
	})
	t.Run("an upgrade", func(t *testing.T) {
		c, udev, _ := installedAt(t, true)
		dest := filepath.Join(udev, ruleName)
		require.NoError(t, os.WriteFile(dest, []byte(rule), 0o644))
		require.NoError(t, beforeUninstall(context.Background(), c))
		assert.FileExists(t, dest)
	})
	t.Run("another program's", func(t *testing.T) {
		c, udev, logged := installedAt(t, false)
		dest := filepath.Join(udev, ruleName)
		require.NoError(t, os.WriteFile(dest, []byte(rule+"# and more\n"), 0o644))
		require.NoError(t, beforeUninstall(context.Background(), c))
		assert.FileExists(t, dest)
		assert.Contains(t, strings.Join(*logged, "\n"), "Left "+dest+" alone")
	})
	t.Run("root's to remove", func(t *testing.T) {
		c, udev, logged := installedAt(t, false)
		dest := filepath.Join(udev, ruleName)
		require.NoError(t, os.WriteFile(dest, []byte(rule), 0o644))
		readOnly(t, udev)
		require.NoError(t, beforeUninstall(context.Background(), c), "an uninstall does not fail over the rule")
		assert.FileExists(t, dest)
		assert.Contains(t, strings.Join(*logged, "\n"), "sudo rm "+dest)
	})
}

// standIn starts a copy of this test program at path, as a process that only
// waits, and returns a channel that closes when it has ended.
func standIn(t *testing.T, path string) (pid int, ended <-chan struct{}) {
	t.Helper()
	self, err := os.Executable()
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	in, err := os.Open(self) //nolint:gosec // this test program
	require.NoError(t, err)
	defer func() { _ = in.Close() }()
	out, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755) //nolint:gosec // a program, in the test's directory
	require.NoError(t, err)
	_, err = io.Copy(out, in)
	require.NoError(t, err)
	require.NoError(t, out.Close())

	cmd := exec.CommandContext(t.Context(), path) //nolint:gosec // the copy made above
	cmd.Env = append(os.Environ(), standInEnv+"=1")
	require.NoError(t, cmd.Start())
	done := make(chan struct{})
	go func() { _ = cmd.Wait(); close(done) }()
	t.Cleanup(func() { _ = cmd.Process.Kill(); <-done })

	// The kernel's name for it is the file's once it has started the copy.
	require.Eventually(t, func() bool {
		comm, _ := os.ReadFile(filepath.Join("/proc", strconv.Itoa(cmd.Process.Pid), "comm"))
		return strings.TrimSpace(string(comm)) == filepath.Base(path)
	}, 5*time.Second, 10*time.Millisecond)
	return cmd.Process.Pid, done
}

/*
Spec 063 R4, against real processes. A panel that runs from the install
directory, and one from where install.sh put it, are stopped, and the hook
has waited for them; a panel from anywhere else, and a terminal panel from
the install directory, run on.
*/
func TestAnInstallStopsThePanelItReplacesAndNoOther(t *testing.T) {
	c, _, logged := installedAt(t, false)
	home := t.TempDir()
	t.Setenv("HOME", home)

	_, installedEnded := standIn(t, filepath.Join(c.Root(), "bin", "hayami"))
	_, scriptEnded := standIn(t, filepath.Join(home, ".local", "bin", "hayami"))
	elsewhere, elsewhereEnded := standIn(t, filepath.Join(t.TempDir(), "hayami"))
	pane, paneEnded := standIn(t, filepath.Join(c.Root(), "bin", "hayami-tui"))

	require.NoError(t, afterInstall(context.Background(), c))

	for name, ended := range map[string]<-chan struct{}{"the installer's": installedEnded, "install.sh's": scriptEnded} {
		select {
		case <-ended:
		case <-time.After(2 * time.Second):
			t.Errorf("%s panel still runs after the hook returned", name)
		}
	}
	for name, p := range map[string]struct {
		pid   int
		ended <-chan struct{}
	}{"a panel from elsewhere": {elsewhere, elsewhereEnded}, "a terminal panel": {pane, paneEnded}} {
		select {
		case <-p.ended:
			t.Errorf("%s was stopped", name)
		default:
			assert.NoError(t, syscall.Kill(p.pid, 0), "%s runs on", name)
		}
	}
	var stopped int
	for _, line := range *logged {
		if strings.HasPrefix(line, "Stopped hayami, which was running") {
			stopped++
		}
	}
	assert.Equal(t, 2, stopped, "each panel stopped is said")
}

// A panel whose file was replaced under it, which is what an install over
// install.sh's leaves, is still found: the kernel marks the path deleted.
func TestAPanelWhoseFileWasReplacedIsStillStopped(t *testing.T) {
	c, _, _ := installedAt(t, false)
	home := t.TempDir()
	t.Setenv("HOME", home)
	path := filepath.Join(home, ".local", "bin", "hayami")
	_, ended := standIn(t, path)
	require.NoError(t, os.Remove(path))
	require.NoError(t, os.Symlink(filepath.Join(c.Root(), "bin", "hayami"), path))

	require.NoError(t, afterInstall(context.Background(), c))
	select {
	case <-ended:
	case <-time.After(2 * time.Second):
		t.Error("the panel still runs")
	}
}
