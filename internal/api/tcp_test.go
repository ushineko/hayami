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
	"github.com/ushineko/hayami/internal/api/client"
	"github.com/ushineko/hayami/internal/api/tcpkey"
)

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
	ctx := context.Background()
	w, err := client.DialTCPWS(ctx, addr, key.Token)
	require.NoError(t, err, "the challenge-response with the key's token failed")
	assert.Equal(t, "1.2.3", w.Hello.Version)

	_, err = client.DialTCPWS(ctx, addr, mustToken(t))
	require.Error(t, err, "a wrong token was let in")

	// The port serves nothing else, with the token as a header or without.
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+addr+"/v1/health", http.NoBody)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+key.Token)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	_ = resp.Body.Close()
	assert.Equal(t, http.StatusNotFound, resp.StatusCode, "the port answered something but the WebSocket")

	// Review F4: a new token ends the session the old one let in, and the old
	// one is refused from then on, with nothing restarting the service.
	renewed, err := tcpkey.Make(keyPath, port, true)
	require.NoError(t, err)
	readEnd := make(chan error, 1)
	go func() {
		for {
			if _, err := w.Next(ctx); err != nil {
				readEnd <- err
				return
			}
		}
	}()
	select {
	case <-readEnd:
	case <-time.After(10 * time.Second):
		t.Fatal("the session the old token let in was not ended")
	}
	_, err = client.DialTCPWS(ctx, addr, key.Token)
	require.Error(t, err, "the old token was let in after it was replaced")
	w2, err := client.DialTCPWS(ctx, addr, renewed.Token)
	require.NoError(t, err, "the new token was refused")
	_ = w2.Close()

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
	assert.NotContains(t, log.String(), renewed.Token, "the log carried the token")
}

// mustToken is a token nobody holds.
func mustToken(t *testing.T) string {
	t.Helper()
	tok, err := tcpkey.NewToken()
	require.NoError(t, err)
	return tok
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
			// The port is looked at once the socket serves, so its line can
			// come after the socket answers (seen on the Windows VM).
			waitFor(t, "the log to say why there is no port", func() bool { return strings.Contains(log.String(), "no TCP port") })
			assert.NotContains(t, log.String(), tok, "the log carried the token")
		})
	}
}

// Spec 117 (#423). A service nothing handed its sockets to starts without a
// key and serves the port once one is written, with no restart: the Windows
// instance starts before the installer writes the key.
func TestAKeyWrittenAfterTheStartIsServed(t *testing.T) {
	if !api.Supported() {
		t.Skip("the service does not run here")
	}
	port := freePort(t)
	keyPath := filepath.Join(t.TempDir(), tcpkey.FileName)
	_, log := runService(t, keyPath)
	waitFor(t, "the log to say there is no port", func() bool { return strings.Contains(log.String(), "no TCP port") })

	key, err := tcpkey.Make(keyPath, port, false)
	require.NoError(t, err)
	ctx := context.Background()
	var w *client.WS
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if w, err = client.DialTCPWS(ctx, tcpkey.Address(port), key.Token); err == nil {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	require.NoError(t, err, "the port was not served after its key was written")
	_ = w.Close()
	assert.Contains(t, log.String(), "the TCP port's key appeared")
	assert.NotContains(t, log.String(), key.Token)
}
