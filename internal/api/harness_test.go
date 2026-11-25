package api_test

import (
	"context"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/ushineko/hayami/internal/api"
	"github.com/ushineko/hayami/internal/config"
	"github.com/ushineko/hayami/internal/panel"
)

func TestMain(m *testing.M) {
	// The clock section names the machine's zone; a test's is UTC wherever
	// it runs, so the golden files are the same on every machine.
	time.Local = time.UTC
	os.Exit(m.Run())
}

// harness is the real server on a real socket in a directory of the test's,
// with a client that dials it.
type harness struct {
	hub    *api.Hub
	srv    *api.Server
	sock   string
	client *http.Client
}

// options adjust a harness before it serves.
type options struct {
	fixedClock bool
	settings   config.Config
	tune       func(*api.Hub, *api.Server)
}

func start(t *testing.T, srcs []panel.Source, o options) *harness {
	t.Helper()
	// Short: a socket's path is capped at 104 bytes on macOS and t.TempDir
	// is long there.
	dir, err := os.MkdirTemp("", "hy")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	sock := filepath.Join(dir, "run", "api.sock")

	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	var l net.Listener
	if api.Supported() {
		l, err = api.Listen(sock, log)
	} else {
		require.NoError(t, os.MkdirAll(filepath.Dir(sock), 0o700))
		var lc net.ListenConfig
		l, err = lc.Listen(context.Background(), "unix", sock)
	}
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	settings := o.settings
	if settings.Sections == nil {
		settings.Sections = []string{"clock", "usage"}
	}
	hub := api.NewHub(ctx, srcs, settings, log)
	if o.fixedClock {
		hub.SetClock(func() time.Time { return polledAt })
	}
	srv := api.NewServer(hub, "1.2.3", log)
	if o.tune != nil {
		o.tune(hub, srv)
	}
	hs := srv.HTTP()
	go func() { _ = hs.Serve(api.Limit(l, api.MaxConns)) }()
	t.Cleanup(func() {
		cancel()
		_ = hs.Close()
	})
	return &harness{hub: hub, srv: srv, sock: sock, client: unixClient(sock)}
}

// unixClient is an HTTP client whose every request goes to sock.
func unixClient(sock string) *http.Client {
	return &http.Client{Transport: &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, "unix", sock)
		},
		MaxIdleConnsPerHost: 4,
	}, Timeout: 30 * time.Second}
}

// get is a GET of path: its status, its body, and its headers.
func (h *harness) get(t testing.TB, path string) (int, []byte, http.Header) {
	t.Helper()
	resp, err := fetch(context.Background(), h.client, path)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return resp.StatusCode, body, resp.Header
}

// fetch is a GET of path with client; the caller closes the body.
func fetch(ctx context.Context, client *http.Client, path string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://hayami"+path, http.NoBody)
	if err != nil {
		return nil, err
	}
	return client.Do(req)
}

// dial is a raw connection to sock.
func dial(t testing.TB, sock string) (net.Conn, error) {
	t.Helper()
	var d net.Dialer
	return d.DialContext(context.Background(), "unix", sock)
}
