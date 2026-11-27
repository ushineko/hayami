package tcpkey_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/hayami/internal/api/tcpkey"
)

// Spec 115 R6, R7. A key made is read back; the token is kept across a
// rewrite, and replaced when renewed.
func TestAKeyIsKeptAndRenewed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hayami", tcpkey.FileName)

	_, err := tcpkey.Read(path)
	require.ErrorIs(t, err, tcpkey.ErrNoFile)

	k, err := tcpkey.Make(path, tcpkey.DefaultPort, false)
	require.NoError(t, err)
	assert.Equal(t, "127.0.0.1:47930", k.Address)
	assert.Equal(t, tcpkey.DefaultPort, k.Port())
	assert.Len(t, k.Token, 43, "32 bytes in base64url without padding")

	got, err := tcpkey.Read(path)
	require.NoError(t, err)
	assert.Equal(t, k, got)

	moved, err := tcpkey.Make(path, 47931, false)
	require.NoError(t, err)
	assert.Equal(t, k.Token, moved.Token, "moving the port changed the token")
	assert.Equal(t, 47931, moved.Port())

	renewed, err := tcpkey.Make(path, 47931, true)
	require.NoError(t, err)
	assert.NotEqual(t, k.Token, renewed.Token, "renewing kept the token")
	assert.True(t, renewed.Equal(renewed.Token))
	assert.False(t, renewed.Equal(k.Token), "the old token still matched")

	require.NoError(t, tcpkey.Remove(path))
	require.NoError(t, tcpkey.Remove(path), "removing what is not there failed")
	_, err = tcpkey.Read(path)
	require.ErrorIs(t, err, tcpkey.ErrNoFile)
}

// Spec 115 R6. The file is made readable by this account alone.
func TestAKeyIsThisAccountsAlone(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the access list is checked by TestAKeyFileOthersCanReadIsRefused on Windows")
	}
	path := filepath.Join(t.TempDir(), tcpkey.FileName)
	_, err := tcpkey.Make(path, tcpkey.DefaultPort, false)
	require.NoError(t, err)
	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
}

// Spec 115 R11. A file another account can read, a link, and a file whose
// content is not a key the service may serve with are each refused, and no
// refusal carries the token.
func TestAKeyThatIsNotPrivateOrNotAKeyIsRefused(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, tcpkey.FileName)
	k, err := tcpkey.Make(path, tcpkey.DefaultPort, false)
	require.NoError(t, err)

	refused := func(t *testing.T, p, why string) {
		t.Helper()
		_, err := tcpkey.Read(p)
		require.Error(t, err, why)
		assert.NotErrorIs(t, err, tcpkey.ErrNoFile)
		assert.NotContains(t, err.Error(), k.Token, "an error carried the token")
	}

	if runtime.GOOS != "windows" {
		require.NoError(t, os.Chmod(path, 0o640))
		refused(t, path, "a key the group can read")
		require.NoError(t, os.Chmod(path, 0o600))

		link := filepath.Join(dir, "link.json")
		require.NoError(t, os.Symlink(path, link))
		refused(t, link, "a link to a key")
	}

	write := func(name, body string) string {
		p := filepath.Join(dir, name)
		require.NoError(t, os.WriteFile(p, []byte(body), 0o600))
		return p
	}
	refused(t, write("notjson.json", "not json "+k.Token), "not JSON")
	refused(t, write("wide.json", `{"address":"0.0.0.0:47930","token":"`+k.Token+`"}`), "an address on every interface")
	refused(t, write("lan.json", `{"address":"192.0.2.1:47930","token":"`+k.Token+`"}`), "an address not loopback")
	refused(t, write("noport.json", `{"address":"127.0.0.1:0","token":"`+k.Token+`"}`), "port 0")
	refused(t, write("bigport.json", `{"address":"127.0.0.1:70000","token":"`+k.Token+`"}`), "a port past 65535")
	refused(t, write("short.json", `{"address":"127.0.0.1:47930","token":"`+k.Token[:20]+`"}`), "a short token")
	refused(t, write("empty.json", `{"address":"127.0.0.1:47930","token":""}`), "no token")
	refused(t, write("big.json", strings.Repeat(" ", 5000)+`{}`), "a file larger than a key")
}
