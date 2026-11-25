//go:build !windows

package api_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// testAddress is a socket's path in a directory of the test's.
func testAddress(t *testing.T) string {
	t.Helper()
	// Short: a socket's path is capped at 104 bytes on macOS and t.TempDir
	// is long there.
	dir, err := os.MkdirTemp("", "hy")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return filepath.Join(dir, "run", "api.sock")
}
