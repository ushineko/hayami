package usage_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/hayami/internal/testenv"
	"github.com/ushineko/hayami/internal/usage"
)

// The slug is a contract with code this repository does not own, and getting
// it wrong is silent: hayami would write a file nothing else reads and both
// programs would poll on their own. These are the cases the Python's own
// rules produce; the next test checks them against the Python itself.
func TestTheSlugNamesTheSameFileThePythonNames(t *testing.T) {
	cases := []struct{ account, provider, want string }{
		{"", "claude", ""},
		{"max", "claude", "-max"},
		{"work", "claude", "-work"},
		{"", "codex", "-codex"},
		{"max", "codex", "-codex-max"},
		{"with space", "claude", "-with_space"},
		{"with.dot", "claude", "-with_dot"},
		{"with/slash", "claude", "-with_slash"},
		{"keeps-hyphen_and_underscore", "claude", "-keeps-hyphen_and_underscore"},
	}

	for _, c := range cases {
		assert.Equal(t, c.want, usage.Slug(c.account, c.provider),
			"account %q provider %q", c.account, c.provider)
	}
}

// The same cases, run through the Python that owns the format. It skips where
// that file is not checked out, so the suite still passes on a machine that
// has only this repository -- but on the machine doing the port, it is the
// thing that would notice the rule changing.
func TestTheSlugAgreesWithThePythonItself(t *testing.T) {
	src := pythonCache(t)
	python := pythonInterpreter(t)

	script := `
import sys, importlib.util
spec = importlib.util.spec_from_file_location("uc", sys.argv[1])
m = importlib.util.module_from_spec(spec)
sys.modules["uc"] = m
spec.loader.exec_module(m)
for line in sys.stdin.read().splitlines():
    account, provider = line.split("\t")
    print(m._slug(account or None, provider))
`
	input := strings.Join([]string{
		"\tclaude", "max\tclaude", "work\tclaude", "\tcodex", "max\tcodex",
		"with space\tclaude", "with.dot\tclaude", "with/slash\tclaude",
		"keeps-hyphen_and_underscore\tclaude",
	}, "\n")

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, python, "-c", script, src)
	cmd.Stdin = strings.NewReader(input)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "python said: %s", out)

	// Windows' Python ends its lines with CRLF.
	got := strings.Split(strings.TrimRight(strings.ReplaceAll(string(out), "\r\n", "\n"), "\n"), "\n")
	want := []string{"", "-max", "-work", "-codex", "-codex-max",
		"-with_space", "-with_dot", "-with_slash", "-keeps-hyphen_and_underscore"}
	require.Len(t, got, len(want))
	for i := range want {
		assert.Equal(t, want[i], got[i], "the Python and this package disagree on case %d", i)
	}
}

// pythonInterpreter finds a Python that runs, or skips.
//
// Found is not enough. Windows puts a python3.exe and a python.exe on the PATH
// that are Store installers, not interpreters: they exit 9009 and print where
// to get Python. So each candidate is asked to run something first.
func pythonInterpreter(t *testing.T) string {
	t.Helper()
	for _, name := range []string{"python3", "python"} {
		path, err := exec.LookPath(name)
		if err != nil {
			continue
		}
		ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
		err = exec.CommandContext(ctx, path, "-c", "import sys").Run()
		cancel()
		if err == nil {
			return path
		}
	}
	t.Skip("no python that runs; the cross-check needs the program that owns the format")
	return ""
}

// pythonCache finds the module that owns the format, or skips.
func pythonCache(t *testing.T) string {
	t.Helper()
	home, err := os.UserHomeDir()
	require.NoError(t, err)
	path := filepath.Join(home, "git", "ag-scripts", "peripheral-battery-monitor", "usage_cache.py")
	if _, err := os.Stat(path); err != nil {
		t.Skip("usage_cache.py is not checked out here; nothing to cross-check against")
	}
	return path
}

func TestTheCacheDirectoryIsTheWidgetsAndNotThisPrograms(t *testing.T) {
	testenv.Cache(t, "/tmp/somewhere")

	dir, err := usage.Dir()

	require.NoError(t, err)
	assert.Equal(t, filepath.Join("/tmp/somewhere", "claude-usage-widget"), dir,
		"renaming this directory would be leaving the cache, not joining it")
}

// On Windows the widget keeps its cache under LOCALAPPDATA, in a "cache"
// directory of its own, and that wins over everything else: it is the Windows
// widget's file hayami has to share, not one of its own beside it.
func TestLocalAppDataIsWhereTheWindowsWidgetKeepsTheCache(t *testing.T) {
	if runtime.GOOS == "darwin" {
		t.Skip("macOS has its own place and does not read LOCALAPPDATA")
	}
	testenv.Cache(t, "/tmp/somewhere")
	t.Setenv("LOCALAPPDATA", "/tmp/local")

	dir, err := usage.Dir()

	require.NoError(t, err)
	assert.Equal(t, filepath.Join("/tmp/local", "claude-usage-widget", "cache"), dir)
}

func TestThePathAndTheLockAreSiblings(t *testing.T) {
	testenv.Cache(t, t.TempDir())

	path, err := usage.Path("max", usage.ProviderClaude)
	require.NoError(t, err)
	lock, err := usage.LockPath("max", usage.ProviderClaude)
	require.NoError(t, err)

	assert.Equal(t, "usage-max.json", filepath.Base(path))
	assert.Equal(t, "usage-max.lock", filepath.Base(lock))
	assert.Equal(t, filepath.Dir(path), filepath.Dir(lock))
}
