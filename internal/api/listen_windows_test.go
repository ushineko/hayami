package api_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/windows"

	"github.com/ushineko/hayami/internal/api"
	"github.com/ushineko/hayami/internal/api/client"
)

func quiet() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

// thisAccount is the SID of the account the test runs as.
func thisAccount(t *testing.T) string {
	t.Helper()
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	require.NoError(t, err)
	return user.User.Sid.String()
}

// R3. The address is the pipe named for this account, and the client finds
// the one the service makes.
func TestTheAddressIsThisAccountsPipe(t *testing.T) {
	addr, err := api.Address()
	require.NoError(t, err)
	assert.Equal(t, `\\.\pipe\hayami-api-`+thisAccount(t), addr)
	theirs, err := client.Address()
	require.NoError(t, err)
	assert.Equal(t, addr, theirs)
}

// R4. The pipe's access list names this account and nothing else, and
// inherits nothing.
func TestThePipeIsThisAccountsAlone(t *testing.T) {
	addr := testAddress(t)
	l, err := api.Listen(addr, quiet())
	require.NoError(t, err)
	t.Cleanup(func() { _ = l.Close() })
	// Reading the access list opens the pipe as a client would, and a pipe
	// takes a client only once it is accepting.
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			_ = c.Close()
		}
	}()

	sd, err := windows.GetNamedSecurityInfo(addr, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	require.NoError(t, err)
	// Windows writes a well-known account by its alias (the built-in
	// Administrator is LA), so the access list is compared with the one
	// expected, written by Windows the same way.
	want, err := windows.SecurityDescriptorFromString("D:P(A;;FA;;;" + thisAccount(t) + ")")
	require.NoError(t, err)
	assert.Equal(t, want.String(), sd.String(), "one entry, this account's, full access, nothing inherited")
}

// R4. A pipe already at the name is a refusal to start, not a second
// listener.
func TestASecondServiceIsRefused(t *testing.T) {
	addr := testAddress(t)
	l, err := api.Listen(addr, quiet())
	require.NoError(t, err)
	t.Cleanup(func() { _ = l.Close() })

	second, err := api.Listen(addr, quiet())
	if second != nil {
		_ = second.Close()
	}
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already listening at "+addr)
}

// A name that is not a pipe's is refused before anything is made.
func TestAnAddressThatIsNotAPipeIsRefused(t *testing.T) {
	_, err := api.Listen(`C:\hayami\api.sock`, quiet())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not a pipe's name")
}

// The reference client reads the service over the pipe: its dial, its check
// of who serves it, and the overlapped handle under HTTP.
func TestTheClientReadsTheServiceOverThePipe(t *testing.T) {
	addr := testAddress(t)
	l, err := api.Listen(addr, quiet())
	require.NoError(t, err)
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	})}
	go func() { _ = srv.Serve(l) }()
	t.Cleanup(func() { _ = srv.Close() })

	c := client.New(addr)
	t.Cleanup(c.Close)
	h, err := c.Health(context.Background())
	require.NoError(t, err)
	assert.True(t, h.OK)
}
