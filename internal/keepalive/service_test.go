package keepalive_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/hayami/internal/keepalive"
)

// serviceBench is a configuration directory of the test's and a service manager that
// says what it was asked, and fails when told to.
func serviceBench(t *testing.T) (path string, asked *[]string, fail *error) {
	t.Helper()
	if runtime.GOOS != "linux" {
		t.Skip("the helper is a systemd service, on Linux")
	}
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	asked, fail = &[]string{}, new(error)
	keepalive.AnswerSystemctl(t, func(_ context.Context, args ...string) error {
		*asked = append(*asked, strings.Join(args, " "))
		return *fail
	})
	path, err := keepalive.UnitPath()
	require.NoError(t, err)
	return path, asked, fail
}

func TestTheUnitRunsTheHelperAndComesBackFromACrash(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("the helper is a systemd service, on Linux")
	}
	text, err := keepalive.UnitText("/opt/hayami/bin/hayami-tui")
	require.NoError(t, err)
	assert.Contains(t, text, `ExecStart="/opt/hayami/bin/hayami-tui" vpn-keepalive run`+"\n")
	assert.Contains(t, text, "Restart=on-failure\n", "and a clean stop stays stopped")
	assert.Contains(t, text, "WantedBy=graphical-session.target\n")
	assert.Contains(t, text, "PartOf=graphical-session.target\n")
	assert.NotContains(t, text, "User=", "as the person, with nothing added")
	assert.NotContains(t, text, "AmbientCapabilities")
}

// A home directory's name is that name in the unit and nothing else: a percent is a
// specifier to systemd, and a line break another line of the unit. Each of these was
// also started by a real manager (spec 085).
func TestAProgramsPathIsOneArgumentWhateverItIsCalled(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("the helper is a systemd service, on Linux")
	}
	for path, want := range map[string]string{
		"/home/a b/hayami-tui":   `"/home/a b/hayami-tui"`,
		"/home/100%/hayami-tui":  `"/home/100%%/hayami-tui"`,
		"/home/$HOME/hayami-tui": `"/home/$HOME/hayami-tui"`,
	} {
		text, err := keepalive.UnitText(path)
		require.NoError(t, err, path)
		assert.Contains(t, text, "ExecStart="+want+" vpn-keepalive run\n")
	}
	for _, path := range []string{"hayami-tui", "./hayami-tui", "/a\n[Service]\nExecStartPre=/bin/evil/hayami-tui", "/a\tb",
		`/home/a"b/hayami-tui`, `/home/a'b/hayami-tui`, `/home/a\b/hayami-tui`} {
		_, err := keepalive.UnitText(path)
		require.ErrorIs(t, err, keepalive.ErrBadProgram, path)
	}
}

func TestInstallingWritesTheUnitAndStartsIt(t *testing.T) {
	path, asked, _ := serviceBench(t)
	assert.False(t, keepalive.Installed())

	require.NoError(t, keepalive.Install(context.Background(), "/opt/hayami-tui"))

	assert.True(t, keepalive.Installed())
	assert.Equal(t, []string{"daemon-reload", "enable " + keepalive.UnitName, "restart " + keepalive.UnitName}, *asked)
	body, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(body), "/opt/hayami-tui")

	// Again, with another program: what an upgrade does.
	require.NoError(t, keepalive.Install(context.Background(), "/other/hayami-tui"))
	body, err = os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(body), "/other/hayami-tui")
	assert.NotContains(t, string(body), "/opt/hayami-tui")
	entries, err := os.ReadDir(filepath.Dir(path))
	require.NoError(t, err)
	assert.Len(t, entries, 1, "no temporary file left")
}

// A manager's start returns before the program has run, so a program that cannot run
// has to be looked for, or the install says it worked.
func TestAHelperThatDidNotStartIsSaid(t *testing.T) {
	serviceBench(t)
	keepalive.AnswerRunning(t, keepalive.ErrDidNotStart)
	err := keepalive.Install(context.Background(), "/opt/hayami-tui")
	require.ErrorIs(t, err, keepalive.ErrDidNotStart)
	assert.ErrorContains(t, err, "journalctl --user -u "+keepalive.UnitName)
	assert.True(t, keepalive.Installed(), "there to be read, and removed")
}

// A person's own service of that name is theirs.
func TestAServiceHayamiDidNotWriteIsLeftAlone(t *testing.T) {
	path, asked, _ := serviceBench(t)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
	const theirs = "[Service]\nExecStart=/their/own\n"
	require.NoError(t, os.WriteFile(path, []byte(theirs), 0o600))

	require.ErrorIs(t, keepalive.Install(context.Background(), "/opt/hayami-tui"), keepalive.ErrNotOurs)
	require.ErrorIs(t, keepalive.Remove(context.Background()), keepalive.ErrNotOurs)

	assert.False(t, keepalive.Installed())
	assert.Empty(t, *asked, "the manager is not asked to touch it")
	body, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, theirs, string(body))
}

// A link at the path is not written through: what it points at is unchanged.
func TestInstallingDoesNotWriteThroughALink(t *testing.T) {
	path, _, _ := serviceBench(t)
	require.NoError(t, keepalive.Install(context.Background(), "/opt/hayami-tui"))
	body, err := os.ReadFile(path)
	require.NoError(t, err)
	target := filepath.Join(t.TempDir(), "elsewhere")
	require.NoError(t, os.WriteFile(target, body, 0o600))
	require.NoError(t, os.Remove(path))
	require.NoError(t, os.Symlink(target, path))

	require.NoError(t, keepalive.Install(context.Background(), "/other/hayami-tui"))

	unchanged, err := os.ReadFile(target)
	require.NoError(t, err)
	assert.Equal(t, string(body), string(unchanged))
	info, err := os.Lstat(path)
	require.NoError(t, err)
	assert.Zero(t, info.Mode()&os.ModeSymlink, "the link was replaced")
}

func TestRemovingStopsTheServiceAndTakesItAway(t *testing.T) {
	path, asked, _ := serviceBench(t)
	require.NoError(t, keepalive.Install(context.Background(), "/opt/hayami-tui"))
	*asked = nil

	require.NoError(t, keepalive.Remove(context.Background()))

	assert.Equal(t, []string{"disable " + keepalive.UnitName, "stop " + keepalive.UnitName, "daemon-reload"}, *asked)
	assert.NoFileExists(t, path)
	assert.False(t, keepalive.Installed())

	*asked = nil
	require.NoError(t, keepalive.Remove(context.Background()), "nothing to remove is no failure")
	assert.Empty(t, *asked)
}

// A session with no manager to ask does not leave the unit behind for the next one, and
// what the manager said is not swallowed.
func TestAManagerThatWillNotAnswerStillLosesTheUnit(t *testing.T) {
	path, _, fail := serviceBench(t)
	require.NoError(t, keepalive.Install(context.Background(), "/opt/hayami-tui"))
	*fail = errors.New("no manager here") //nolint:err113 // a test's failure

	require.ErrorContains(t, keepalive.Remove(context.Background()), "no manager here")
	assert.NoFileExists(t, path)
}

func TestWhatIsOfferedSaysWhatIsBeingAgreedTo(t *testing.T) {
	said := keepalive.OfferWhy + keepalive.OfferCaution + strings.Join(keepalive.OfferSteps(), " ")
	for _, want := range []string{"re-dials", "Three failed checks", "bounces", "outside the tunnel", "no administrator", keepalive.UnitName, "uninstall"} {
		assert.Contains(t, said, want)
	}
}
