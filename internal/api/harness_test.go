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
	"github.com/ushineko/hayami/internal/api/client"
	"github.com/ushineko/hayami/internal/api/tcpkey"
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

	// The TCP port's, when options.tcp (spec 115): its address, its key,
	// and a client that dials it and adds nothing.
	tcpAddr   string
	key       tcpkey.Key
	tcpClient *http.Client
}

// options adjust a harness before it serves.
type options struct {
	fixedClock bool
	settings   config.Config
	tune       func(*api.Hub, *api.Server)
	// tcp serves the TCP port beside the socket, with a key of the test's.
	tcp bool
}

func start(t *testing.T, srcs []panel.Source, o options) *harness {
	t.Helper()
	// Short: a socket's path is capped at 104 bytes on macOS and t.TempDir
	// is long there.
	sock := testAddress(t)
	var err error

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
	limiter := api.NewLimiter(api.MaxConns)
	go func() { _ = hs.Serve(limiter.Wrap(l)) }()
	h := &harness{hub: hub, srv: srv, sock: sock, client: serviceClient(sock)}
	var ht *http.Server
	if o.tcp {
		var lc net.ListenConfig
		tl, err := lc.Listen(context.Background(), "tcp4", "127.0.0.1:0")
		require.NoError(t, err)
		port := tl.Addr().(*net.TCPAddr).Port
		h.key, err = tcpkey.Make(filepath.Join(t.TempDir(), tcpkey.FileName), port, false)
		require.NoError(t, err)
		h.tcpAddr = tl.Addr().String()
		h.tcpClient = &http.Client{Timeout: 30 * time.Second}
		ht = srv.HTTP()
		ht.Handler = srv.TCPHandler(h.key, port)
		go func() { _ = ht.Serve(limiter.Wrap(tl)) }()
	}
	t.Cleanup(func() {
		cancel()
		_ = hs.Close()
		if ht != nil {
			_ = ht.Close()
		}
	})
	return h
}

// tcpGet is a GET of path on the TCP port, with headers as given (Host
// among them): its status, its body, and its headers.
func (h *harness) tcpGet(t testing.TB, path string, headers map[string]string) (int, []byte, http.Header) {
	t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://"+h.tcpAddr+path, http.NoBody)
	require.NoError(t, err)
	for k, v := range headers {
		if k == "Host" {
			req.Host = v
			continue
		}
		req.Header.Set(k, v)
	}
	resp, err := h.tcpClient.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return resp.StatusCode, body, resp.Header
}

// bearer is the header that carries the harness's token.
func (h *harness) bearer() map[string]string {
	return map[string]string{"Authorization": "Bearer " + h.key.Token}
}

// serviceClient is an HTTP client whose every request goes to the service at sock.
func serviceClient(sock string) *http.Client {
	return &http.Client{Transport: &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return client.Dial(ctx, sock)
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
	return client.Dial(context.Background(), sock) //nolint:wrapcheck // a test's
}
