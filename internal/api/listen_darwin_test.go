package api_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/hayami/internal/api"
	"github.com/ushineko/hayami/internal/api/client"
)

// R3. On macOS the address is in the per-user temporary directory, and the
// client finds it there; one past sun_path's 104 bytes is refused, naming the
// limit, never cut short.
func TestTheAddressIsInThePerUserTemporaryDirectory(t *testing.T) {
	t.Setenv("TMPDIR", "/var/folders/ab/cdef/T/")
	addr, err := api.Address()
	require.NoError(t, err)
	assert.Equal(t, "/var/folders/ab/cdef/T/hayami/api.sock", addr)
	theirs, err := client.Address()
	require.NoError(t, err)
	assert.Equal(t, addr, theirs)

	t.Setenv("TMPDIR", "/"+strings.Repeat("d", 100))
	_, err = api.Address()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "fewer than 104")
	_, err = client.Address()
	require.Error(t, err)

	// Unset, it is the system's answer for this account, as the client's is.
	t.Setenv("TMPDIR", "")
	addr, err = api.Address()
	require.NoError(t, err)
	assert.True(t, strings.HasSuffix(addr, "/hayami/api.sock"), addr)
	theirs, err = client.Address()
	require.NoError(t, err)
	assert.Equal(t, addr, theirs)
}
