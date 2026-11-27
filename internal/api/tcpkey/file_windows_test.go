//go:build windows

package tcpkey_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/hayami/internal/api/tcpkey"
)

// Spec 115 R6, R11, on Windows. A key Make wrote is read; the same content
// in a file that takes its folder's permissions is refused.
func TestAKeyFileOthersCanReadIsRefused(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, tcpkey.FileName)
	k, err := tcpkey.Make(path, tcpkey.DefaultPort, false)
	require.NoError(t, err)
	_, err = tcpkey.Read(path)
	require.NoError(t, err, "the key Make wrote was refused")

	body, err := os.ReadFile(path)
	require.NoError(t, err)
	plain := filepath.Join(dir, "plain.json")
	require.NoError(t, os.WriteFile(plain, body, 0o600))
	_, err = tcpkey.Read(plain)
	require.Error(t, err, "a key with its folder's permissions was read")
	assert.NotContains(t, err.Error(), k.Token)
}
