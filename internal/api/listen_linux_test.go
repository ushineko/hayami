package api_test

import (
	"io"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/hayami/internal/api"
)

func shortDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "hy")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}

var quiet = slog.New(slog.NewTextHandler(io.Discard, nil))

// R4. The socket is 0600 in a 0700 directory, and a connection from this
// account is served.
func TestTheSocketIsThisAccountsAlone(t *testing.T) {
	dir := shortDir(t)
	sock := filepath.Join(dir, "run", "api.sock")
	l, err := api.Listen(sock, quiet)
	require.NoError(t, err)
	defer func() { _ = l.Close() }()

	info, err := os.Stat(filepath.Dir(sock))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o700), info.Mode().Perm())
	info, err = os.Stat(sock)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())

	go func() {
		c, err := l.Accept()
		if err == nil {
			_, _ = c.Write([]byte("ok"))
			_ = c.Close()
		}
	}()
	c, err := dial(t, sock)
	require.NoError(t, err)
	defer func() { _ = c.Close() }()
	b, _ := io.ReadAll(c)
	assert.Equal(t, "ok", string(b), "the peer check lets this account in")
}

// R4. A socket that answers is another service: a second refuses to start.
// One left by a service that ended is replaced.
func TestASecondServiceRefusesAndAStaleSocketIsReplaced(t *testing.T) {
	dir := shortDir(t)
	sock := filepath.Join(dir, "api.sock")
	first, err := api.Listen(sock, quiet)
	require.NoError(t, err)
	go func() {
		for {
			c, err := first.Accept()
			if err != nil {
				return
			}
			_ = c.Close()
		}
	}()

	_, err = api.Listen(sock, quiet)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already listening")

	// A socket file with nobody behind it: what a killed service leaves.
	require.NoError(t, first.Close())
	var lc net.ListenConfig
	stale, err := lc.Listen(t.Context(), "unix", sock)
	require.NoError(t, err)
	stale.(*net.UnixListener).SetUnlinkOnClose(false)
	require.NoError(t, stale.Close())
	_, err = os.Stat(sock)
	require.NoError(t, err, "the stale socket is there")

	l, err := api.Listen(sock, quiet)
	require.NoError(t, err, "a stale socket is replaced")
	_ = l.Close()
}

// R3. The default address is the runtime directory's.
func TestTheAddressIsInTheRuntimeDirectory(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", "/run/user/4242")
	addr, err := api.Address()
	require.NoError(t, err)
	assert.Equal(t, "/run/user/4242/hayami/api.sock", addr)

	t.Setenv("XDG_RUNTIME_DIR", "")
	_, err = api.Address()
	assert.Error(t, err, "no runtime directory is no address, not a guess")
}

// The adversarial review's finding 3. A socket's directory that is a link is
// refused, and the directory it points to is left as it was: a link put in a
// shared directory would otherwise have this account's directory made 0700,
// or the socket made where someone else chose.
func TestASocketDirectoryThatIsALinkIsRefused(t *testing.T) {
	dir := shortDir(t)
	target := filepath.Join(dir, "shared")
	require.NoError(t, os.Mkdir(target, 0o750))
	link := filepath.Join(dir, "h")
	require.NoError(t, os.Symlink(target, link))

	_, err := api.Listen(filepath.Join(link, "api.sock"), quiet)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "link")
	info, err := os.Stat(target)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o750), info.Mode().Perm(), "the linked directory is not touched")
}
