package cli_test

import (
	"crypto/rand"
	"net"
	"testing"

	"github.com/Microsoft/go-winio"
	"github.com/stretchr/testify/require"
)

// listenAPI is a stand-in service's pipe, of a name the test's own, and its
// address.
func listenAPI(t *testing.T) (net.Listener, string) {
	t.Helper()
	addr := `\\.\pipe\hayami-test-` + rand.Text()
	l, err := winio.ListenPipe(addr, nil)
	require.NoError(t, err)
	return l, addr
}
