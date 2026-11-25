package api_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/hayami/internal/api"
)

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
