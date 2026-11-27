package unit

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/hayami/internal/api/tcpkey"
)

// manager is a stand-in for the person's service manager: what it was asked,
// in order, and what it says.
type manager struct {
	calls  []string
	fail   error
	active map[string]bool
}

// fake puts a stand-in manager and a units directory of the test's own in
// place for the length of a test, and says the system has a manager.
func fake(t *testing.T) (*manager, string) {
	t.Helper()
	m := &manager{active: map[string]bool{}}
	units := t.TempDir()
	wasCtl, wasDir, wasOffered, wasKey := systemctl, dir, Offered, keyPath
	t.Cleanup(func() { systemctl, dir, Offered, keyPath = wasCtl, wasDir, wasOffered, wasKey })
	key := filepath.Join(t.TempDir(), tcpkey.FileName)
	keyPath = func() (string, error) { return key, nil }
	systemctl = func(_ context.Context, args ...string) (string, error) {
		m.calls = append(m.calls, strings.Join(args, " "))
		if args[0] == "is-active" {
			if m.active[args[1]] {
				return "active", nil
			}
			return "inactive", errors.New("exit status 3")
		}
		return "", m.fail
	}
	dir = func() (string, error) { return units, nil }
	Offered = func() bool { return true }
	return m, units
}

/*
R5. Install writes the socket and the service, each marked as hayami's, the
service naming the program as systemd reads it, then listens on the socket
and restarts a running service on the new program.
*/
func TestInstallWritesTheUnitsAndListens(t *testing.T) {
	m, units := fake(t)
	program := "/home/someone/My 100% programs/bin/hayami-tui"
	require.NoError(t, Install(context.Background(), program))

	socket, err := os.ReadFile(filepath.Join(units, SocketName))
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(string(socket), mark+"\n"))
	for _, line := range []string{"ListenStream=%t/hayami/api.sock", "SocketMode=0600", "DirectoryMode=0700", "RemoveOnStop=yes", "WantedBy=sockets.target"} {
		assert.Contains(t, string(socket), "\n"+line+"\n")
	}
	service, err := os.ReadFile(filepath.Join(units, ServiceName))
	require.NoError(t, err)
	assert.Contains(t, string(service), "\nExecStart=\"/home/someone/My 100%% programs/bin/hayami-tui\" serve\n")
	assert.Contains(t, string(service), "\nRequires="+SocketName+"\n")
	assert.Contains(t, string(service), "\nRestart=on-failure\n")
	assert.NotContains(t, string(service), "[Install]", "the socket starts the service; nothing else does")

	assert.Equal(t, []string{"daemon-reload", "enable --now " + SocketName, "try-restart " + ServiceName}, m.calls)
	assert.True(t, Installed())

	// Again, on a new program: the units are replaced, and the same asked.
	m.calls = nil
	require.NoError(t, Install(context.Background(), "/opt/hayami/bin/hayami-tui"))
	service, err = os.ReadFile(filepath.Join(units, ServiceName))
	require.NoError(t, err)
	assert.Contains(t, string(service), `ExecStart="/opt/hayami/bin/hayami-tui" serve`)
	assert.Equal(t, []string{"daemon-reload", "enable --now " + SocketName, "try-restart " + ServiceName}, m.calls)
}

// A unit at hayami's path that hayami did not write is the person's, and is
// neither replaced nor removed.
func TestAUnitHayamiDidNotWriteIsLeftAlone(t *testing.T) {
	m, units := fake(t)
	theirs := "[Socket]\nListenStream=%t/elsewhere.sock\n"
	require.NoError(t, os.WriteFile(filepath.Join(units, SocketName), []byte(theirs), 0o600))

	require.ErrorIs(t, Install(context.Background(), "/opt/hayami/bin/hayami-tui"), ErrNotOurs)
	require.ErrorIs(t, Remove(context.Background()), ErrNotOurs)
	body, err := os.ReadFile(filepath.Join(units, SocketName))
	require.NoError(t, err)
	assert.Equal(t, theirs, string(body))
	assert.Empty(t, m.calls, "nothing was asked of the manager")
	assert.False(t, Installed())
}

// A program no unit can name is refused before anything is written.
func TestAProgramAUnitCannotNameIsRefused(t *testing.T) {
	_, units := fake(t)
	for _, p := range []string{"hayami-tui", `/opt/a"b/hayami-tui`, "/opt/a\\b/hayami-tui", "/opt/a\nExecStartPre=/bin/x/hayami-tui"} {
		require.ErrorIs(t, Install(context.Background(), p), ErrBadProgram, p)
	}
	entries, err := os.ReadDir(units)
	require.NoError(t, err)
	assert.Empty(t, entries)
}

/*
R25. Remove stops the socket and the service and takes both units away;
having nothing to remove is not a failure, and a manager that cannot be
asked does not leave the files behind.
*/
func TestRemoveTakesTheUnitsAway(t *testing.T) {
	m, units := fake(t)
	require.NoError(t, Remove(context.Background()), "nothing to remove")
	assert.Empty(t, m.calls)

	require.NoError(t, Install(context.Background(), "/opt/hayami/bin/hayami-tui"))
	m.calls = nil
	require.NoError(t, Remove(context.Background()))
	assert.Equal(t, []string{"disable --now " + SocketName, "stop " + ServiceName, "daemon-reload"}, m.calls)
	entries, err := os.ReadDir(units)
	require.NoError(t, err)
	assert.Empty(t, entries)

	require.NoError(t, Install(context.Background(), "/opt/hayami/bin/hayami-tui"))
	// The link enable makes, which a manager that cannot be asked does not
	// take away.
	wants := filepath.Join(units, "sockets.target.wants")
	require.NoError(t, os.MkdirAll(wants, 0o700))
	require.NoError(t, os.Symlink(filepath.Join(units, SocketName), filepath.Join(wants, SocketName)))
	m.fail = errors.New("no manager")
	err = Remove(context.Background())
	require.Error(t, err, "what the manager said is reported")
	assert.Contains(t, err.Error(), "until you sign out")
	entries, err = os.ReadDir(units)
	require.NoError(t, err)
	require.Len(t, entries, 1, "the files go anyway")
	links, err := os.ReadDir(wants)
	require.NoError(t, err)
	assert.Empty(t, links, "and so does the link to the socket")
}

// A link in sockets.target.wants of the same name that points elsewhere is
// not hayami's to remove.
func TestALinkToAnotherUnitIsLeft(t *testing.T) {
	_, units := fake(t)
	require.NoError(t, Install(context.Background(), "/opt/hayami/bin/hayami-tui"))
	wants := filepath.Join(units, "sockets.target.wants")
	require.NoError(t, os.MkdirAll(wants, 0o700))
	elsewhere := filepath.Join(t.TempDir(), SocketName)
	require.NoError(t, os.Symlink(elsewhere, filepath.Join(wants, SocketName)))
	require.NoError(t, Remove(context.Background()))
	target, err := os.Readlink(filepath.Join(wants, SocketName))
	require.NoError(t, err)
	assert.Equal(t, elsewhere, target)
}

// Install refuses where there is no manager to write for.
func TestInstallRefusesWithoutAManager(t *testing.T) {
	_, units := fake(t)
	Offered = func() bool { return false }
	require.ErrorIs(t, Install(context.Background(), "/opt/hayami/bin/hayami-tui"), ErrNotHere)
	entries, err := os.ReadDir(units)
	require.NoError(t, err)
	assert.Empty(t, entries)
}

// Status says what the manager says, and asks nothing when nothing is
// installed.
func TestStatusIsWhatTheManagerSays(t *testing.T) {
	m, _ := fake(t)
	assert.Equal(t, State{}, Status(context.Background()))
	assert.Empty(t, m.calls)

	require.NoError(t, Install(context.Background(), "/opt/hayami/bin/hayami-tui"))
	m.active[SocketName] = true
	assert.Equal(t, State{Installed: true, Listening: true}, Status(context.Background()))
	m.active[ServiceName] = true
	assert.Equal(t, State{Installed: true, Listening: true, Running: true}, Status(context.Background()))
}

/*
The units are ones systemd loads: systemd-analyze verifies each, where the
machine has it. This is the manager's own parser, not a reading of the text.
*/
func TestSystemdVerifiesTheUnits(t *testing.T) {
	analyze, err := exec.LookPath("systemd-analyze")
	if err != nil {
		t.Skip("systemd-analyze is not on this machine")
	}
	units := t.TempDir()
	// The program must exist for verify to pass ExecStart.
	program := filepath.Join(units, "hayami-tui")
	require.NoError(t, os.WriteFile(program, []byte("#!/bin/sh\n"), 0o700))
	service, err := ServiceText(program)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(units, SocketName), []byte(SocketText()), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(units, ServiceName), []byte(service), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(units, TCPSocketName), []byte(TCPSocketText(tcpkey.DefaultPort)), 0o600))

	cmd := exec.CommandContext(context.Background(), analyze, "verify", "--man=no", //nolint:gosec // the test's own files
		filepath.Join(units, SocketName), filepath.Join(units, ServiceName), filepath.Join(units, TCPSocketName))
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "systemd-analyze verify: %s", out)
	// verify loads the units ours depend on and reports on those too: a CI
	// runner's own xfs_scrub_all.service drew a warning. Only what it says
	// about the units in this directory is ours to answer for.
	var ours []string
	for line := range strings.Lines(string(out)) {
		if strings.Contains(line, units) {
			ours = append(ours, strings.TrimSpace(line))
		}
	}
	assert.Empty(t, ours, "verify had something to say about hayami's units:\n%s", out)
}

// keyAt is the test's key file.
func keyAt(t *testing.T) string {
	t.Helper()
	p, err := keyPath()
	require.NoError(t, err)
	return p
}

/*
Spec 115 R2, R13. InstallTCP writes the port's key and its socket unit, on
127.0.0.1 with its descriptor named for the service, and listens; an upgrade
keeps the port and its token, and renewing replaces the token.
*/
func TestInstallTCPWritesThePortsUnitAndItsKey(t *testing.T) {
	m, units := fake(t)
	const program = "/opt/hayami/bin/hayami-tui"
	require.NoError(t, InstallTCP(context.Background(), program, 47931, false))

	key, err := tcpkey.Read(keyAt(t))
	require.NoError(t, err)
	assert.Equal(t, 47931, key.Port())
	body, err := os.ReadFile(filepath.Join(units, TCPSocketName))
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(string(body), mark+"\n"))
	for _, line := range []string{"ListenStream=127.0.0.1:47931", "FileDescriptorName=tcp", "Service=" + ServiceName, "WantedBy=sockets.target"} {
		assert.Contains(t, string(body), "\n"+line+"\n")
	}
	assert.NotContains(t, string(body), key.Token, "the unit carried the token")
	assert.Equal(t, []string{"daemon-reload", "enable --now " + SocketName, "enable " + TCPSocketName,
		"restart " + TCPSocketName, "try-restart " + ServiceName}, m.calls)

	// An upgrade: the port and its token stay.
	require.NoError(t, Install(context.Background(), program))
	again, err := tcpkey.Read(keyAt(t))
	require.NoError(t, err)
	assert.Equal(t, key, again)
	assert.FileExists(t, filepath.Join(units, TCPSocketName))

	require.NoError(t, InstallTCP(context.Background(), program, 47931, true))
	renewed, err := tcpkey.Read(keyAt(t))
	require.NoError(t, err)
	assert.NotEqual(t, key.Token, renewed.Token)
}

// Spec 115 R13. RemoveTCP takes the port's unit and key and leaves the
// socket and the service; and the port follows its key, so an install with
// the key gone takes the unit away.
func TestThePortGoesWithItsKey(t *testing.T) {
	m, units := fake(t)
	const program = "/opt/hayami/bin/hayami-tui"
	require.NoError(t, InstallTCP(context.Background(), program, tcpkey.DefaultPort, false))

	m.calls = nil
	require.NoError(t, RemoveTCP(context.Background(), program))
	assert.Equal(t, []string{"disable --now " + TCPSocketName, "daemon-reload", "try-restart " + ServiceName}, m.calls)
	assert.NoFileExists(t, filepath.Join(units, TCPSocketName))
	assert.NoFileExists(t, keyAt(t))
	assert.FileExists(t, filepath.Join(units, SocketName))
	assert.FileExists(t, filepath.Join(units, ServiceName))

	require.NoError(t, InstallTCP(context.Background(), program, tcpkey.DefaultPort, false))
	require.NoError(t, os.Remove(keyAt(t)))
	require.NoError(t, Install(context.Background(), program))
	assert.NoFileExists(t, filepath.Join(units, TCPSocketName), "a port without its key was kept")

	require.NoError(t, InstallTCP(context.Background(), program, tcpkey.DefaultPort, false))
	require.NoError(t, Remove(context.Background()))
	entries, err := os.ReadDir(units)
	require.NoError(t, err)
	assert.Empty(t, entries, "removing the service left the port")
	assert.NoFileExists(t, keyAt(t))
}

// Spec 115 R15. Status says whether the port is set up and listening.
func TestStatusSaysThePort(t *testing.T) {
	m, _ := fake(t)
	require.NoError(t, InstallTCP(context.Background(), "/opt/hayami/bin/hayami-tui", tcpkey.DefaultPort, false))
	m.active[SocketName] = true
	assert.Equal(t, State{Installed: true, Listening: true, TCP: true}, Status(context.Background()))
	m.active[TCPSocketName] = true
	assert.Equal(t, State{Installed: true, Listening: true, TCP: true, TCPListening: true}, Status(context.Background()))
}

// Spec 115 R13. The installed service's program is read back as it was
// written, a percent and a space included; nothing installed is ErrNotHere.
func TestTheInstalledProgramIsReadBack(t *testing.T) {
	_, _ = fake(t)
	_, err := InstalledProgram()
	require.ErrorIs(t, err, ErrNotHere)
	const program = "/home/someone/My 100% programs/bin/hayami-tui"
	require.NoError(t, Install(context.Background(), program))
	got, err := InstalledProgram()
	require.NoError(t, err)
	assert.Equal(t, program, got)
}
