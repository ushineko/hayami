package structure_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// sleeper is a program that does nothing for a minute: something to run from
// the install directory so Windows holds its file the way it holds a running
// panel or terminal pane.
const sleeper = "package main\n\nimport \"time\"\n\nfunc main() { time.Sleep(time.Minute) }\n"

/*
The installer replaces a program that is running. Windows will not overwrite
a running executable but will rename it, so the running copy moves aside to
<name>.old and the new file takes its name; the next install removes the old
one once nothing holds it. Before this the install failed whenever the
terminal pane was open, which on a desk that keeps it open is every time.

A real running program, not a file held open by the test: what Windows allows
of a mapped executable (rename, not overwrite or delete) is the behaviour
under test.
*/
func TestTheInstallerReplacesARunningProgram(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("install_binary.ps1 installs for Windows")
	}
	powershell, err := exec.LookPath("powershell.exe")
	if err != nil {
		t.Skip("no Windows PowerShell")
	}
	dir := t.TempDir()
	src := filepath.Join(dir, "sleeper.go")
	require.NoError(t, os.WriteFile(src, []byte(sleeper), 0o600))
	dest := filepath.Join(dir, "prog")
	require.NoError(t, os.MkdirAll(dest, 0o700))
	target := filepath.Join(dest, "hayami-tui.exe")
	build := exec.CommandContext(t.Context(), "go", "build", "-o", target, src)
	build.Env = append(os.Environ(), "CGO_ENABLED=0")
	out, err := build.CombinedOutput()
	require.NoError(t, err, "%s", out)

	running := exec.CommandContext(t.Context(), target)
	require.NoError(t, running.Start())
	stopped := false
	stop := func() {
		if !stopped {
			_ = running.Process.Kill()
			_ = running.Wait()
			stopped = true
		}
	}
	t.Cleanup(stop)
	// The image is mapped once the process exists; a moment makes sure.
	time.Sleep(200 * time.Millisecond)

	fresh := filepath.Join(dir, "new-build.exe")
	require.NoError(t, os.WriteFile(fresh, []byte("the new build"), 0o600))
	install := func() string {
		t.Helper()
		script := ". '" + filepath.Join(repoRoot(t), "scripts", "install_binary.ps1") + "'; " +
			"Install-Binary '" + fresh + "' '" + target + "'"
		cmd := exec.CommandContext(t.Context(), powershell, "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", script)
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "%s", out)
		return string(out)
	}

	said := install()
	got, err := os.ReadFile(target)
	require.NoError(t, err)
	assert.Equal(t, "the new build", string(got), "the new build did not take the program's name")
	assert.FileExists(t, target+".old", "the running copy was not moved aside")
	assert.Contains(t, said, "moved aside", "the installer did not say the running copy was moved aside")

	// Once nothing holds it, the next install removes the old copy.
	stop()
	said = install()
	assert.NoFileExists(t, target+".old", "the old copy outlived the program that held it")
	assert.NotContains(t, said, "moved aside", "nothing was running, and the installer said otherwise")
}
