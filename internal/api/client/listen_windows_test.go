package client_test

import (
	"crypto/rand"
	"net"
	"testing"

	"github.com/Microsoft/go-winio"
	"github.com/stretchr/testify/require"
)

// listen is a stand-in service's pipe, of a name the test's own, and its
// address. Its access list is the default: this account's process serves it,
// which is what the client checks.
func listen(t *testing.T) (net.Listener, string) {
	t.Helper()
	addr := `\\.\pipe\hayami-test-` + rand.Text()
	l, err := winio.ListenPipe(addr, nil)
	require.NoError(t, err)
	return l, addr
}
