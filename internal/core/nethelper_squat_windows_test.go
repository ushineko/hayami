package core

import (
	"errors"
	"fmt"
	"os"
	"runtime"
	"testing"
	"time"
	"unsafe"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/windows"
)

// squat makes a pipe of the given name, open to everyone, the way a program
// that took the helper's name while it was stopped would.
func squat(t *testing.T, name string) windows.Handle {
	t.Helper()
	sd, err := windows.SecurityDescriptorFromString("D:(A;;GA;;;WD)")
	require.NoError(t, err)
	sa := &windows.SecurityAttributes{SecurityDescriptor: sd}
	sa.Length = uint32(unsafe.Sizeof(*sa))
	path, err := windows.UTF16PtrFromString(name)
	require.NoError(t, err)
	h, err := windows.CreateNamedPipe(path, windows.PIPE_ACCESS_DUPLEX,
		windows.PIPE_TYPE_BYTE|windows.PIPE_WAIT, 1, 4096, 4096, 0, sa)
	require.NoError(t, err)
	t.Cleanup(func() { _ = windows.CloseHandle(h) })
	return h
}

func testPipe(t *testing.T) string {
	t.Helper()
	return fmt.Sprintf(`\\.\pipe\hayami-squat-%d-%d`, os.Getpid(), time.Now().UnixNano())
}

// GHSA-94c3-mf55-vv8v. A pipe of the helper's name served by any process but
// the service's is refused before a byte of it is read, and says why.
func TestAPipeHeldByAnotherProgramIsRefused(t *testing.T) {
	pipe := testPipe(t)
	squat(t, pipe)

	r := &helperNet{pipe: pipe, start: func() error { return errNoNetHelper }, now: time.Now,
		// The service manager says the helper is not running: process 0.
		verify: func(server uint32) error { return serverMatches(server, 0) }}
	defer r.Close()
	_, err := r.Read(t.Context())

	var a *Absence
	require.ErrorAs(t, err, &a)
	assert.Equal(t, "helper's pipe is taken", a.Text)
	assert.Contains(t, a.Detail, fmt.Sprint(windows.GetCurrentProcessId()), "it names the process holding the pipe")
	assert.Nil(t, r.stream, "nothing is read from it")
}

/*
A server gets no token for the panel's user from the pipe the panel opens.
Two things hold that: the panel opens the pipe anonymously, and it never
writes to it, while Windows lets a server impersonate only after reading
from the pipe. The control case is a client that writes and asks for nothing,
which a server does impersonate, so the test can tell the difference.
*/
func TestTheServerCannotImpersonateThePanel(t *testing.T) {
	t.Run("the panel", func(t *testing.T) {
		got := serverToken(t, func(pipe string) (windows.Handle, error) { return openPipe(pipe) }, false)
		assert.False(t, got, "a server holds a token for the panel's user")
	})
	t.Run("a client that writes and asks for nothing", func(t *testing.T) {
		got := serverToken(t, func(pipe string) (windows.Handle, error) {
			path, err := windows.UTF16PtrFromString(pipe)
			if err != nil {
				return 0, err
			}
			return windows.CreateFile(path, windows.GENERIC_READ|windows.GENERIC_WRITE, 0, nil, windows.OPEN_EXISTING, 0, 0)
		}, true)
		assert.True(t, got, "the control case: this server should hold the client's token")
	})
}

// serverToken connects open's client to a server of its own and reports
// whether the server, impersonating it, holds a token for a user other than
// the anonymous one.
func serverToken(t *testing.T, open func(string) (windows.Handle, error), write bool) bool {
	t.Helper()
	pipe := testPipe(t)
	server := squat(t, pipe)
	client, err := open(pipe)
	require.NoError(t, err)
	defer func() { _ = windows.CloseHandle(client) }()
	err = windows.ConnectNamedPipe(server, nil)
	if err != nil && !errors.Is(err, windows.ERROR_PIPE_CONNECTED) {
		require.NoError(t, err)
	}
	if write {
		var n uint32
		require.NoError(t, windows.WriteFile(client, []byte("x"), &n, nil))
		buf := make([]byte, 1)
		require.NoError(t, windows.ReadFile(server, buf, &n, nil))
	}

	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := impersonate(server); err != nil {
		return false // refused: no token at all
	}
	defer func() { _ = windows.RevertToSelf() }()
	var token windows.Token
	if err := windows.OpenThreadToken(windows.CurrentThread(), windows.TOKEN_QUERY, true, &token); err != nil {
		return false // an anonymous token cannot be opened
	}
	defer func() { _ = token.Close() }()
	user, err := token.GetTokenUser()
	require.NoError(t, err)
	return !user.User.Sid.IsWellKnown(windows.WinAnonymousSid)
}

var procImpersonateNamedPipeClient = windows.NewLazySystemDLL("advapi32.dll").NewProc("ImpersonateNamedPipeClient")

// impersonate is what a hostile server would do with the panel's handle.
func impersonate(h windows.Handle) error {
	if r, _, err := procImpersonateNamedPipeClient.Call(uintptr(h)); r == 0 {
		return err
	}
	return nil
}
