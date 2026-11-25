//go:build !windows

package client_test

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// listen is a stand-in service's socket, in a directory of the test's, and
// its address.
func listen(t *testing.T) (net.Listener, string) {
	t.Helper()
	// Short: a socket's path is capped at 104 bytes on macOS and t.TempDir
	// is long there.
	dir, err := os.MkdirTemp("", "hy")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	sock := filepath.Join(dir, "api.sock")
	var lc net.ListenConfig
	l, err := lc.Listen(context.Background(), "unix", sock)
	require.NoError(t, err)
	return l, sock
}
