package cli_test

import (
	"context"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

/*
B6. What `api get` and `api check` print from the socket is made printable
first, whoever answered on it: a terminal's escape sequence in a body, or in
an error that names it, does not reach the terminal. A well-formed answer is
printed as it came.
*/
func TestTheAPICommandsPrintOnlyPrintableText(t *testing.T) {
	const hostile = "\x1b]0;owned\x07\u202ename"
	sock := answer(t, map[string]string{
		"/v1/health":  `{"ok":true}`,
		"/v1/hostile": hostile,
		"/v1":         `{"service":"hayami","version":"` + hostile + `","api":"9.0","sections":[]}`,
	})

	out, err := run(t, "api", "--address", sock, "get", "health")
	require.NoError(t, err)
	assert.Contains(t, out, `"ok": true`)

	out, _ = run(t, "api", "--address", sock, "get", "/v1/hostile")
	assert.NotContains(t, out, "\x1b")
	assert.NotContains(t, out, "\u202e")
	assert.Contains(t, out, "name")

	out, err = run(t, "api", "--address", sock, "check")
	require.Error(t, err)
	assert.NotContains(t, out+err.Error(), "\x1b")
	assert.NotContains(t, out+err.Error(), "\u202e")

	// status prints the service's own words, and an error can carry a key
	// the service chose: both are cleaned on their way out.
	const jsonHostile = `\u001b]0;owned\u0007\u202ename`
	sock = answer(t, map[string]string{
		"/v1":          `{"service":"hayami","version":"` + jsonHostile + `","api":"1.0","sections":["clock"]}`,
		"/v1/sections": `{"sections":[{"key":"` + jsonHostile + `","title":"t","default":true,"shown":true,"interval_s":1}]}`,
		"/v1/health":   `{"ok":true,"` + jsonHostile + `":1}`,
	})
	out, err = run(t, "api", "--address", sock, "status")
	require.NoError(t, err)
	assert.Contains(t, out, "name")
	assert.NotContains(t, out, "\x1b")
	assert.NotContains(t, out, "\u202e")
	_, err = run(t, "api", "--address", sock, "check")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "a field the contract lacks")
	assert.NotContains(t, err.Error(), "\x1b")
	assert.NotContains(t, err.Error(), "\u202e")
}

// --timeout ends a command whose service never answers.
func TestAnAPICommandGivesUpAtItsTimeout(t *testing.T) {
	dir, err := os.MkdirTemp("", "hy")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	sock := filepath.Join(dir, "api.sock")
	var lc net.ListenConfig
	l, err := lc.Listen(context.Background(), "unix", sock)
	require.NoError(t, err)
	t.Cleanup(func() { _ = l.Close() })
	go func() {
		// Accepts, and says nothing.
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			t.Cleanup(func() { _ = c.Close() })
		}
	}()
	began := time.Now()
	_, err = run(t, "api", "--address", sock, "--timeout", "300ms", "get", "health")
	require.Error(t, err)
	assert.Less(t, time.Since(began), 5*time.Second)
}

// `api address` prints the default address, or the one given.
func TestAPIAddressIsTheContractsRule(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", "/run/user/1000")
	out, err := run(t, "api", "address")
	if err != nil {
		t.Skipf("no default address on this system: %v", err)
	}
	assert.Equal(t, filepath.Join("/run/user/1000", "hayami", "api.sock")+"\n", out)
	out, err = run(t, "api", "--address", "/elsewhere.sock", "address")
	require.NoError(t, err)
	assert.Equal(t, "/elsewhere.sock\n", out)
}

// answer serves each path's body as JSON on a socket, and returns it.
func answer(t *testing.T, bodies map[string]string) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "hy")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	sock := filepath.Join(dir, "api.sock")
	srv := &http.Server{ReadHeaderTimeout: time.Second, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, ok := bodies[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(strings.TrimSpace(body)))
	})}
	var lc net.ListenConfig
	l, err := lc.Listen(context.Background(), "unix", sock)
	require.NoError(t, err)
	go func() { _ = srv.Serve(l) }()
	t.Cleanup(func() { _ = srv.Close() })
	return sock
}
