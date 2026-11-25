package unit_test

import (
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/hayami/internal/api"
	"github.com/ushineko/hayami/internal/api/client"
	"github.com/ushineko/hayami/internal/api/unit"
)

// R3, R5. The socket systemd makes is where the service and every client
// look for it: the unit's %t is the runtime directory.
func TestTheSocketIsAtTheContractsAddress(t *testing.T) {
	const run = "/run/user/1000"
	t.Setenv("XDG_RUNTIME_DIR", run)
	listen := regexp.MustCompile(`(?m)^ListenStream=(.*)$`).FindStringSubmatch(unit.SocketText())
	require.Len(t, listen, 2)
	path := strings.ReplaceAll(listen[1], "%t", run)

	want, err := api.Address()
	require.NoError(t, err)
	assert.Equal(t, want, path)
	got, err := client.Address()
	require.NoError(t, err)
	assert.Equal(t, got, path)
}
