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

	require.NoError(t, Install(ctx, "/Users/a/.local/bin/hayami-tui"))
	assert.True(t, Installed())
	assert.Equal(t, State{Installed: true, Listening: true, Running: true}, Status(ctx))
	assert.Contains(t, *calls, []string{"bootstrap", domain(), path})

	*calls = nil
	require.NoError(t, Install(ctx, "/Users/a/.local/bin/hayami-tui"))
	assert.Equal(t, [][]string{{"bootout", domain() + "/" + Label}, {"bootstrap", domain(), path}}, *calls,
		"an upgrade takes the agent out and loads it on the new program")

	require.NoError(t, Remove(ctx))
	assert.False(t, Installed())
	_, err := os.Stat(path)
	require.ErrorIs(t, err, os.ErrNotExist)
	require.NoError(t, Remove(ctx), "removing what is not there is not an error")
}

// R25. A file at the agent's path that hayami did not write is neither
// replaced nor removed.
func TestAnAgentHayamiDidNotWriteIsLeftAlone(t *testing.T) {
	calls, agents := fakeLaunchd(t)
	path := filepath.Join(agents, Label+".plist")
	theirs := "<?xml version=\"1.0\"?><plist version=\"1.0\"><dict/></plist>\n"
	require.NoError(t, os.WriteFile(path, []byte(theirs), 0o600))

	require.ErrorIs(t, Install(context.Background(), "/Users/a/.local/bin/hayami-tui"), ErrNotOurs)
	require.ErrorIs(t, Remove(context.Background()), ErrNotOurs)
	body, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, theirs, string(body))
	assert.Empty(t, *calls, "launchd is not asked about an agent that is not hayami's")
	assert.False(t, Installed())
}
