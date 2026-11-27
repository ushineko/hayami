package api_test

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/hayami/internal/api"
	"github.com/ushineko/hayami/internal/api/tcpkey"
)

// Spec 115 R8. A request on the port without the token, with another scheme,
// with a wrong token or a cut one is refused, each with the same answer; with
// the token it reaches the routes.
func TestThePortWantsTheToken(t *testing.T) {
	h := start(t, sources(fixtures()), options{tcp: true})
	host := map[string]string{}

	refusals := map[string]map[string]string{
		"no token":       host,
		"basic":          {"Authorization": "Basic " + h.key.Token},
		"no scheme":      {"Authorization": h.key.Token},
		"wrong token":    {"Authorization": "Bearer " + strings.Repeat("A", len(h.key.Token))},
		"cut token":      {"Authorization": "Bearer " + h.key.Token[:20]},
		"token and more": {"Authorization": "Bearer " + h.key.Token + "x"},
		"lower scheme":   {"Authorization": "bearer " + h.key.Token},
	}
	var first []byte
	for name, hdr := range refusals {
		status, body, header := h.tcpGet(t, "/v1/health", hdr)
		assert.Equal(t, http.StatusUnauthorized, status, name)
		assert.Equal(t, `Bearer realm="hayami"`, header.Get("WWW-Authenticate"), name)
		conforms(t, "Error", body)
		assert.Contains(t, string(body), `"unauthorized"`, name)
		if first == nil {
			first = body
		}
		assert.Equal(t, string(first), string(body), "%s was answered differently", name)
	}

	status, body, _ := h.tcpGet(t, "/v1/health", h.bearer())
	assert.Equal(t, http.StatusOK, status, string(body))
	status, body, _ = h.tcpGet(t, "/v1/sections/cooler", h.bearer())
	assert.Equal(t, http.StatusOK, status, string(body))
	conforms(t, "Section", body)

	// A path that is not there is not told to a client without the token.
	status, _, _ = h.tcpGet(t, "/nowhere", nil)
	assert.Equal(t, http.StatusUnauthorized, status)
}

// Spec 115 R9, R5. A request on the port must name the port as its host,
// whatever token it carries; a browser's is refused there as on the socket.
func TestThePortWantsItsOwnName(t *testing.T) {
	h := start(t, sources(fixtures()), options{tcp: true})
	_, port, err := net.SplitHostPort(h.tcpAddr)
	require.NoError(t, err)

	for _, host := range []string{"evil.example:" + port, "evil.example", "127.0.0.1", "127.0.0.1:1", "localhost:1", "[::1]:" + port, "hayami"} {
		hdr := h.bearer()
		hdr["Host"] = host
		status, body, _ := h.tcpGet(t, "/v1/health", hdr)
		assert.Equal(t, http.StatusForbidden, status, "host %q", host)
		conforms(t, "Error", body)
	}
	// The host is looked at before the token: a rebound page learns nothing
	// of whether its guess was right.
	status, _, _ := h.tcpGet(t, "/v1/health", map[string]string{"Host": "evil.example:" + port})
	assert.Equal(t, http.StatusForbidden, status)

	for _, host := range []string{"127.0.0.1:" + port, "localhost:" + port, "LOCALHOST:" + port} {
		hdr := h.bearer()
		hdr["Host"] = host
		status, body, _ := h.tcpGet(t, "/v1/health", hdr)
		assert.Equal(t, http.StatusOK, status, "host %q: %s", host, body)
	}

	hdr := h.bearer()
	hdr["Origin"] = "http://127.0.0.1:" + port
	status, body, _ := h.tcpGet(t, "/v1/health", hdr)
	assert.Equal(t, http.StatusForbidden, status, "a browser's request with the token was served")
	conforms(t, "Error", body)
}

// Spec 115 R5. The socket and the port share the limits: event streams open
// on one count against the other.
func TestThePortAndTheSocketShareTheStreams(t *testing.T) {
	h := start(t, sources(fixtures()), options{tcp: true})
	for range api.MaxStreams {
		_, status := openStream(t, h, "sections=clock")
		require.Equal(t, http.StatusOK, status)
	}
	status, body, header := h.tcpGet(t, "/v1/events?sections=clock", h.bearer())
	assert.Equal(t, http.StatusServiceUnavailable, status, "a stream past the limit opened on the port")
	assert.NotEmpty(t, header.Get("Retry-After"))
	conforms(t, "Error", body)
}

// Spec 115 R5. One limiter counts the connections of every listener it
// wraps: past its limit, a connection to the other listener is answered 503.
func TestTheListenersShareTheConnections(t *testing.T) {
	var lc net.ListenConfig
	a, err := lc.Listen(context.Background(), "tcp4", "127.0.0.1:0")
	require.NoError(t, err)
	b, err := lc.Listen(context.Background(), "tcp4", "127.0.0.1:0")
	require.NoError(t, err)
	lm := api.NewLimiter(2)
	held := make(chan net.Conn, 4)
	for _, l := range []net.Listener{lm.Wrap(a), lm.Wrap(b)} {
		go func() {
			for {
				c, err := l.Accept()
				if err != nil {
					return
				}
				held <- c
			}
		}()
	}
	t.Cleanup(func() {
		_ = a.Close()
		_ = b.Close()
		close(held)
		for c := range held {
			_ = c.Close()
		}
	})
	var d net.Dialer
	for range 2 {
		c, err := d.DialContext(context.Background(), "tcp", a.Addr().String())
		require.NoError(t, err)
		t.Cleanup(func() { _ = c.Close() })
	}
	waitFor(t, "two connections held", func() bool { return len(held) == 2 })

	c, err := d.DialContext(context.Background(), "tcp", b.Addr().String())
	require.NoError(t, err)
	defer func() { _ = c.Close() }()
	_, _ = fmt.Fprint(c, "GET /v1/health HTTP/1.1\r\nHost: x\r\n\r\n")
	_ = c.SetReadDeadline(time.Now().Add(5 * time.Second))
	buf := make([]byte, 64)
	n, _ := c.Read(buf)
	assert.True(t, strings.HasPrefix(string(buf[:n]), "HTTP/1.1 503"), "the other listener's connection got %q", buf[:n])
}

// syncBuffer is a log both the service and the test touch.
type syncBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p) //nolint:wrapcheck // a buffer's
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

// runService is `serve` as Run starts it, on a socket of the test's and a
// key at keyPath, with its log captured. It is stopped when the test ends.
func runService(t *testing.T, keyPath string) (sock string, log *syncBuffer) {
	t.Helper()
	sock = testAddress(t)
	log = &syncBuffer{}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- api.Run(ctx, api.Options{
			Listen: sock, KeyPath: keyPath, Version: "1.2.3",
			SettingsPath: filepath.Join(t.TempDir(), "settings.yaml"),
			Log:          slog.New(slog.NewTextHandler(log, nil)),
		})
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			t.Error("the service did not stop")
		}
	})
	waitFor(t, "the service to listen", func() bool {
		c, err := dial(t, sock)
		if err != nil {
			return false
		}
		_ = c.Close()
		return true
	})
	return sock, log
}

// freePort is a port nothing listens on, a moment ago.
func freePort(t *testing.T) int {
	t.Helper()
	var lc net.ListenConfig
	l, err := lc.Listen(context.Background(), "tcp4", "127.0.0.1:0")
	require.NoError(t, err)
	port := l.Addr().(*net.TCPAddr).Port
	require.NoError(t, l.Close())
	return port
}

// Spec 115 R1, R10, R11. The service listens on the key's port, on
// 127.0.0.1 alone, and its log never carries the token, even when it refuses
// requests and keys.
func TestTheServiceListensOnItsKeysPortOnLoopbackAlone(t *testing.T) {
	if !api.Supported() {
		t.Skip("the service does not run here")
	}
	port := freePort(t)
	keyPath := filepath.Join(t.TempDir(), tcpkey.FileName)
	key, err := tcpkey.Make(keyPath, port, false)
	require.NoError(t, err)
	sock, log := runService(t, keyPath)
	_ = sock

	addr := tcpkey.Address(port)
	waitFor(t, "the port to listen", func() bool {
		var d net.Dialer
		c, err := d.DialContext(context.Background(), "tcp", addr)
		if err != nil {
			return false
		}
		_ = c.Close()
		return true
	})
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://"+addr+"/v1/health", http.NoBody)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+key.Token)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	_ = resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	req.Header.Set("Authorization", "Bearer wrong")
	resp, err = http.DefaultClient.Do(req)
	require.NoError(t, err)
	_ = resp.Body.Close()
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)

	// The port is on loopback alone: the machine's other addresses refuse.
	for _, ip := range otherAddresses(t) {
		var d net.Dialer
		d.Timeout = time.Second
		c, err := d.DialContext(context.Background(), "tcp", net.JoinHostPort(ip, fmt.Sprint(port)))
		if err == nil {
			_ = c.Close()
			t.Errorf("the port answered on %s", ip)
		}
	}
	assert.NotContains(t, log.String(), key.Token, "the log carried the token")
}

// otherAddresses are this machine's IPv4 addresses that are not loopback.
func otherAddresses(t *testing.T) []string {
	t.Helper()
	addrs, err := net.InterfaceAddrs()
	require.NoError(t, err)
	var out []string
	for _, a := range addrs {
		if n, ok := a.(*net.IPNet); ok && n.IP.To4() != nil && !n.IP.IsLoopback() {
			out = append(out, n.IP.String())
		}
	}
	return out
}

// Spec 115 R11. A missing key means no port and a refused one says why in
// the log, without the token; either way the socket serves.
func TestWithoutAGoodKeyThereIsNoPortAndTheSocketServes(t *testing.T) {
	if !api.Supported() {
		t.Skip("the service does not run here")
	}
	dir := t.TempDir()
	tok, err := tcpkey.NewToken()
	require.NoError(t, err)
	cases := map[string]string{
		"missing":   filepath.Join(dir, "missing.json"),
		"malformed": filepath.Join(dir, "malformed.json"),
		"wide":      filepath.Join(dir, "wide.json"),
	}
	require.NoError(t, os.WriteFile(cases["malformed"], []byte(`{"address":`+tok), 0o600))
	require.NoError(t, os.WriteFile(cases["wide"], []byte(`{"address":"0.0.0.0:`+fmt.Sprint(freePort(t))+`","token":"`+tok+`"}`), 0o600))
	for name, path := range cases {
		t.Run(name, func(t *testing.T) {
			sock, log := runService(t, path)
			c := serviceClient(sock)
			resp, err := fetch(context.Background(), c, "/v1/health")
			require.NoError(t, err)
			_ = resp.Body.Close()
			assert.Equal(t, http.StatusOK, resp.StatusCode, "the socket did not serve")
			assert.Contains(t, log.String(), "no TCP port")
			assert.NotContains(t, log.String(), tok, "the log carried the token")
		})
	}
}
