package unit

import (
	"context"
	"encoding/xml"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/ushineko/hayami/internal/api/client"
)

// plistValues reads a plist's strings, keys and booleans in order, by XML,
// which is how launchd reads it.
func plistValues(t *testing.T, text string) []string {
	t.Helper()
	d := xml.NewDecoder(strings.NewReader(text))
	d.Strict = true
	var out []string
	var in string
	for {
		tok, err := d.Token()
		if err != nil {
			break
		}
		switch v := tok.(type) {
		case xml.StartElement:
			in = v.Name.Local
			if in == "true" || in == "false" {
				out = append(out, in)
			}
		case xml.CharData:
			if in == "string" || in == "key" || in == "integer" {
				out = append(out, in+"="+string(v))
			}
		case xml.EndElement:
			in = ""
		}
	}
	return out
}

// R7. The agent runs the program with `serve`, launchd holds the socket at
// 0600, it is restarted after a failure and not after a clean exit, and its
// booleans are the form launchd reads: `<false/>`, never `<false></false>`,
// which plutil accepts and launchd refuses with "Input/output error".
func TestTheAgentIsWhatLaunchdLoads(t *testing.T) {
	text, err := PlistText("/Users/a/.local/bin/hayami-tui", "/var/folders/x/T/hayami/api.sock")
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(text, `<?xml version="1.0" encoding="UTF-8"?>`))
	assert.Contains(t, text, "<!-- "+mark+" -->")
	assert.Contains(t, text, "<false/>")
	assert.NotContains(t, text, "></false>")
	assert.NotContains(t, text, "></true>")
	assert.Equal(t, []string{
		"key=Label", "string=" + Label,
		"key=ProgramArguments", "string=/Users/a/.local/bin/hayami-tui", "string=serve",
		"key=Sockets", "key=" + sockets, "key=SockPathName", "string=/var/folders/x/T/hayami/api.sock",
		"key=SockPathMode", "integer=384",
		"key=KeepAlive", "key=SuccessfulExit", "false",
		"key=ThrottleInterval", "integer=5",
		"key=ProcessType", "string=Background",
	}, plistValues(t, text))
}

// B2. A path is a value, never markup: one written to close its string and
// add a key is still one string, the whole path.
func TestAPathCannotAddAKey(t *testing.T) {
	hostile := `/Users/a/x</string><key>RunAtLoad</key><true/><string>/bin/sh`
	text, err := PlistText(hostile, "/var/folders/x/T/hayami/api.sock")
	require.NoError(t, err)
	values := plistValues(t, text)
	assert.Contains(t, values, "string="+hostile)
	assert.NotContains(t, values, "key=RunAtLoad")
	assert.NotContains(t, values, "true")

	for _, bad := range []string{"relative/hayami-tui", "/Users/a/x\nRunAtLoad", "/Users/a/\x00x"} {
		_, err := PlistText(bad, "/var/folders/x/T/hayami/api.sock")
		require.ErrorIs(t, err, ErrBadProgram, "%q", bad)
	}
}

// program is a program file of the test's, the account's own and writable by
// it alone.
func program(t *testing.T) string {
	t.Helper()
	d := t.TempDir()
	require.NoError(t, os.Chmod(d, 0o755))
	p := filepath.Join(d, "hayami-tui")
	require.NoError(t, os.WriteFile(p, []byte("#!/bin/sh\n"), 0o755)) //nolint:gosec // a program
	return p
}

// B2, the adversarial review (5a). launchd runs the agent's program as this
// account with nobody watching, so a program another account could change,
// or one in a directory another account could change, is refused.
func TestAProgramOthersCanChangeIsRefused(t *testing.T) {
	fakeLaunchd(t)
	ctx := context.Background()

	p := program(t)
	require.NoError(t, os.Chmod(p, 0o775))
	err := Install(ctx, p)
	require.ErrorIs(t, err, ErrBadProgram)
	assert.Contains(t, err.Error(), "can be changed by other accounts")

	p = program(t)
	require.NoError(t, os.Chmod(filepath.Dir(p), 0o777))
	require.ErrorIs(t, Install(ctx, p), ErrBadProgram)

	link := filepath.Join(t.TempDir(), "hayami-tui")
	p = program(t)
	require.NoError(t, os.Chmod(p, 0o757))
	require.NoError(t, os.Symlink(p, link))
	require.ErrorIs(t, Install(ctx, link), ErrBadProgram, "the file the link leads to is what runs")

	assert.False(t, Installed())
	require.NoError(t, Install(ctx, program(t)))
}

// fakeLaunchd stands in for launchctl and the LaunchAgents directory.
func fakeLaunchd(t *testing.T) (*[][]string, string) {
	t.Helper()
	agents := t.TempDir()
	wasCtl, wasDir := launchctl, dir
	t.Cleanup(func() { launchctl, dir = wasCtl, wasDir })
	var calls [][]string
	loaded := false
	launchctl = func(_ context.Context, args ...string) (string, error) {
		calls = append(calls, args)
		switch args[0] {
		case "bootstrap":
			loaded = true
		case "bootout":
			if !loaded {
				return "", os.ErrNotExist
			}
			loaded = false
		case "print":
			if !loaded {
				return "", os.ErrNotExist
			}
			return "\tstate = running\n", nil
		}
		return "", nil
	}
	dir = func() (string, error) { return agents, nil }
	wasWait := waitAnswer
	t.Cleanup(func() { waitAnswer = wasWait })
	waitAnswer = func(context.Context, string) error { return nil }
	// Short: a socket's path is capped at 104 bytes here, and t.TempDir is
	// longer than that leaves room for.
	tmp, err := os.MkdirTemp("", "hy")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(tmp) })
	t.Setenv("TMPDIR", tmp)
	return &calls, agents
}

// R7, R25. Installing writes the agent and loads it into the account's gui
// domain; again, it is taken out and loaded on the new program; removing takes
// it out, and its file and socket go.
func TestTheAgentIsInstalledAgainAndRemoved(t *testing.T) {
	calls, agents := fakeLaunchd(t)
	ctx := context.Background()
	path := filepath.Join(agents, Label+".plist")

	require.NoError(t, Install(ctx, program(t)))
	assert.True(t, Installed())
	assert.Equal(t, State{Installed: true, Listening: true, Running: true}, Status(ctx))
	assert.Contains(t, *calls, []string{"bootstrap", domain(), path})

	*calls = nil
	require.NoError(t, Install(ctx, program(t)))
	assert.Equal(t, [][]string{{"bootout", domain() + "/" + Label}, {"print", domain() + "/" + Label}, {"bootstrap", domain(), path}}, *calls,
		"an upgrade takes the agent out, waits until launchd has let it go, and loads it on the new program")

	require.NoError(t, Remove(ctx))
	assert.False(t, Installed())
	_, err := os.Stat(path)
	require.ErrorIs(t, err, os.ErrNotExist)
	require.NoError(t, Remove(ctx), "removing what is not there is not an error")
}

// R4, R7. launchd makes the socket but not its directory, so the install
// makes it, this account's alone; a link at its place is refused, not
// followed. Without the directory the agent fails at every start (found on
// the Mac).
func TestTheSocketsDirectoryIsMadeAndALinkRefused(t *testing.T) {
	fakeLaunchd(t)
	ctx := context.Background()
	socket, err := client.Address()
	require.NoError(t, err)

	require.NoError(t, Install(ctx, program(t)))
	info, err := os.Lstat(filepath.Dir(socket))
	require.NoError(t, err)
	assert.True(t, info.IsDir())
	assert.Equal(t, os.FileMode(0o700), info.Mode().Perm())
	require.NoError(t, Remove(ctx))

	// The fake launchd made no socket, so the removal left the directory.
	require.NoError(t, os.Remove(filepath.Dir(socket)))
	require.NoError(t, os.Symlink(t.TempDir(), filepath.Dir(socket)))
	err = Install(ctx, program(t))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not a directory but a link")
}

// R25. A file at the agent's path that hayami did not write is neither
// replaced nor removed.
func TestAnAgentHayamiDidNotWriteIsLeftAlone(t *testing.T) {
	calls, agents := fakeLaunchd(t)
	path := filepath.Join(agents, Label+".plist")
	theirs := "<?xml version=\"1.0\"?><plist version=\"1.0\"><dict/></plist>\n"
	require.NoError(t, os.WriteFile(path, []byte(theirs), 0o600))

	require.ErrorIs(t, Install(context.Background(), program(t)), ErrNotOurs)
	require.ErrorIs(t, Remove(context.Background()), ErrNotOurs)
	body, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, theirs, string(body))
	assert.Empty(t, *calls, "launchd is not asked about an agent that is not hayami's")
	assert.False(t, Installed())
}
