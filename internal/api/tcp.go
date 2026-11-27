package api

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ushineko/hayami/internal/api/contract"
	"github.com/ushineko/hayami/internal/api/tcpkey"
)

// MaxTCPConns is the most connections the TCP port serves at once (spec 115
// R5, review F2): its own count, apart from the socket's, so connections
// from another account to the port can never take the socket's places.
const MaxTCPConns = 16

// keyLook is how often the service looks at its key for a change (review F4).
const keyLook = 2 * time.Second

/*
TCPAuth is the key the TCP port's sessions are let in with, and the sessions
it let in. A new key replaces it and ends them, so a token replaced or a key
removed lets nobody in from then on (review F4); nil lets nobody in.
*/
type TCPAuth struct {
	key atomic.Pointer[tcpkey.Key]

	mu       sync.Mutex
	sessions map[*context.CancelFunc]struct{}
}

func newTCPAuth(key tcpkey.Key) *TCPAuth {
	a := &TCPAuth{sessions: map[*context.CancelFunc]struct{}{}}
	a.key.Store(&key)
	return a
}

// current is the key, or nil.
func (a *TCPAuth) current() *tcpkey.Key { return a.key.Load() }

// Set replaces the key, and ends every session the old one let in.
func (a *TCPAuth) Set(key *tcpkey.Key) {
	a.key.Store(key)
	a.mu.Lock()
	defer a.mu.Unlock()
	for end := range a.sessions {
		(*end)()
	}
}

// track keeps a session's end until the returned func lets it go.
func (a *TCPAuth) track(end context.CancelFunc) func() {
	a.mu.Lock()
	defer a.mu.Unlock()
	p := &end
	a.sessions[p] = struct{}{}
	return func() {
		a.mu.Lock()
		defer a.mu.Unlock()
		delete(a.sessions, p)
	}
}

/*
TCPHandler is the routes as the loopback TCP port serves them (spec 115):
/v1/ws alone, behind its challenge-response, since every account and every
local process can reach a loopback port. A request must first name the port
as its host, 127.0.0.1:<port> or localhost:<port>, which a page that rebound
a name of its own to 127.0.0.1 cannot (R9); then it passes the checks every
request passes (GET, no body, no Origin). Anything refused closes its
connection, so an unauthenticated client holds no place (review F2).
*/
func (s *Server) TCPHandler(auth *TCPAuth, port int) http.Handler {
	hosts := map[string]bool{
		net.JoinHostPort(tcpkey.Host, strconv.Itoa(port)): true,
		net.JoinHostPort("localhost", strconv.Itoa(port)): true,
	}
	routes := s.guard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/ws" {
			w.Header().Set("Connection", "close")
			writeError(w, http.StatusNotFound, contract.ErrNotFound, "the TCP port serves /v1/ws alone; the socket serves the rest", nil)
			return
		}
		s.ws(w, r, auth)
	}))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !hosts[strings.ToLower(r.Host)] {
			w.Header().Set("Connection", "close")
			writeError(w, http.StatusForbidden, contract.ErrForbidden,
				"a request on the port names 127.0.0.1:"+strconv.Itoa(port)+" or localhost:"+strconv.Itoa(port)+" as its host", nil)
			return
		}
		routes.ServeHTTP(w, r)
	})
}

/*
NewTCPAuth is the key a TCP port serves with, for a server built by hand
(the tests' harness); Run makes its own.
*/
func NewTCPAuth(key tcpkey.Key) *TCPAuth { return newTCPAuth(key) }

/*
tcpListener is the port, or nil when it is not set up (R1, R2, R11): the
listener systemd or launchd handed over, or one made on the key's address.
Without a key the service may serve with, there is no port, and one handed
over is closed: a port is never served without its key. The port is on
127.0.0.1 and on the key's port, or it is refused (review F5).
*/
func tcpListener(keyPath string, log *slog.Logger) (net.Listener, tcpkey.Key) {
	handed, _, herr := ActivatedTCP()
	if herr != nil {
		log.Warn("the TCP port handed over could not be taken", "error", herr)
	}
	drop := func() {
		if handed != nil {
			_ = handed.Close()
		}
	}
	key, err := tcpkey.Read(keyPath)
	switch {
	case errors.Is(err, tcpkey.ErrNoFile):
		drop()
		log.Info("no TCP port: it is not set up", "key", keyPath)
		return nil, tcpkey.Key{}
	case err != nil:
		drop()
		log.Warn("no TCP port: its key is refused", "error", err)
		return nil, tcpkey.Key{}
	}
	l := handed
	if l == nil {
		l, err = listenTCP(key.Address)
		if err != nil {
			log.Warn("no TCP port: it could not be listened on", "address", key.Address, "error", err)
			return nil, tcpkey.Key{}
		}
	}
	if err := loopbackOnly(l); err != nil {
		_ = l.Close()
		log.Warn("no TCP port", "error", err)
		return nil, tcpkey.Key{}
	}
	if p := tcpPort(l); p != key.Port() {
		_ = l.Close()
		log.Warn("no TCP port: the port handed over is not the key's; set the port up again",
			"handed", p, "key", key.Port())
		return nil, tcpkey.Key{}
	}
	return l, key
}

// listenTCP listens on address, exclusively where the system has a way to
// say so (review F6).
func listenTCP(address string) (net.Listener, error) {
	lc := net.ListenConfig{Control: exclusiveAddress}
	return lc.Listen(context.Background(), "tcp4", address) //nolint:wrapcheck // names the address
}

// loopbackOnly refuses a listener on any address but 127.0.0.1 (R1).
func loopbackOnly(l net.Listener) error {
	a, ok := l.Addr().(*net.TCPAddr)
	if !ok || !a.IP.Equal(net.IPv4(127, 0, 0, 1)) {
		return fmt.Errorf("the port is on %s, not 127.0.0.1, and is not served", l.Addr())
	}
	return nil
}

// tcpPort is a listener's port.
func tcpPort(l net.Listener) int {
	if a, ok := l.Addr().(*net.TCPAddr); ok {
		return a.Port
	}
	return 0
}

/*
followKey looks at the key every keyLook and, when the file changes, lets
sessions in with the new one and ends those the old one let in (review F4):
a token replaced is refused from then on on every system, Windows included,
where nothing restarts the service. A key removed or refused lets nobody in.
A port moved is served from the next start, and says so.
*/
func followKey(ctx context.Context, path string, port int, auth *TCPAuth, log *slog.Logger) {
	stamp := func() (time.Time, int64) {
		info, err := os.Stat(path)
		if err != nil {
			return time.Time{}, -1
		}
		return info.ModTime(), info.Size()
	}
	seenAt, seenSize := stamp()
	t := time.NewTicker(keyLook)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		at, size := stamp()
		if at.Equal(seenAt) && size == seenSize {
			continue
		}
		seenAt, seenSize = at, size
		key, err := tcpkey.Read(path)
		switch {
		case err != nil:
			auth.Set(nil)
			log.Warn("the TCP port lets nobody in: its key is gone or refused", "error", err)
		case key.Port() != port:
			auth.Set(nil)
			log.Warn("the TCP port lets nobody in: its key names another port, served from the next start", "key", key.Port(), "port", port)
		default:
			auth.Set(&key)
			log.Info("the TCP port's key changed; sessions let in with the old one are ended")
		}
	}
}

func bytesReader(b []byte) *bytes.Reader { return bytes.NewReader(b) }
