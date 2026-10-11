package installer

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// sleeper is the environment variable that makes this test program wait
// instead of running its tests: the stand-in for a running panel.
const sleeper = "HAYAMI_TEST_SLEEPER"

func TestMain(m *testing.M) {
	if os.Getenv(sleeper) != "" {
		time.Sleep(time.Minute)
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// running copies this test program to path, starts it as a program that
// only waits, and returns it. It is killed when the test ends.
func running(t *testing.T, path string) *exec.Cmd {
	t.Helper()
	self, err := os.Executable()
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o750))
	in, err := os.Open(self)
	require.NoError(t, err)
	out, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0o700)
	require.NoError(t, err)
	_, err = io.Copy(out, in)
	require.NoError(t, err)
	require.NoError(t, in.Close())
	require.NoError(t, out.Close())

	cmd := exec.Command(path)
	cmd.Env = append(os.Environ(), sleeper+"=1")
	require.NoError(t, cmd.Start())
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	})
	return cmd
}

// exited is whether cmd ends within a second.
func exited(cmd *exec.Cmd) bool {
	done := make(chan struct{})
	go func() {
		_, _ = cmd.Process.Wait()
		close(done)
	}()
	select {
	case <-done:
		return true
	case <-time.After(time.Second):
		return false
	}
}

/*
Spec 059, R2.5, and the fault its first desk check found. The uninstall hook
stops the panel that runs from the install directory, which is in bin there,
so that its file can be removed; and leaves alone a program of the same name
that runs from somewhere else.

Real processes: copies of this test program that only wait. The first
version of stopPrograms looked beside the uninstaller, found nothing in bin,
and an uninstall with the panel running left hayami.exe behind.
*/
func TestTheUninstallHookStopsThePanelInBinAndNoOther(t *testing.T) {
	root, elsewhere := t.TempDir(), t.TempDir()
	panel := running(t, filepath.Join(root, "bin", "hayami.exe"))
	pane := running(t, filepath.Join(root, "bin", "hayami-tui.exe"))
	other := running(t, filepath.Join(elsewhere, "hayami.exe"))

	stopped := stopPrograms(root)

	assert.ElementsMatch(t, []string{"hayami.exe", "hayami-tui.exe"}, stopped)
	assert.True(t, exited(panel), "the panel in the install directory is still running")
	assert.True(t, exited(pane), "the terminal panel in the install directory is still running")
	assert.False(t, exited(other), "a hayami.exe from somewhere else was stopped")
	require.NoError(t, os.Remove(filepath.Join(root, "bin", "hayami.exe")), "Windows still holds the file of the stopped panel")
}

// A file is inside a directory at any depth, without regard to case, and
// the directory is not inside itself.
func TestInsideIsAnyDepthAndAnyCase(t *testing.T) {
	assert.True(t, inside(`C:\Programs\hayami`, `C:\Programs\hayami\bin\hayami.exe`))
	assert.True(t, inside(`C:\Programs\hayami`, `c:\programs\HAYAMI\hayami.exe`))
	assert.False(t, inside(`C:\Programs\hayami`, `C:\Programs\hayami-old\bin\hayami.exe`))
	assert.False(t, inside(`C:\Programs\hayami`, `C:\git\hayami\hayami.exe`))
	assert.False(t, inside(`C:\Programs\hayami`, `C:\Programs\hayami`))
}
