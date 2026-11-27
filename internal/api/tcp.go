package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"

	"github.com/ushineko/hayami/internal/api/contract"
	"github.com/ushineko/hayami/internal/api/tcpkey"
)

/*
TCPHandler is the routes as the loopback TCP port serves them (spec 115):
behind two checks the socket does not need, since every account and every
local process can reach a loopback port. A request must name the port as its
host, 127.0.0.1:<port> or localhost:<port>, which a page that rebound a name
of its own to 127.0.0.1 cannot (R9); then it must carry the token (R8). Only
then does it reach the routes and the checks every request passes.
*/
func (s *Server) TCPHandler(key tcpkey.Key, port int) http.Handler {
	next := s.Handler()
	hosts := map[string]bool{
		net.JoinHostPort(tcpkey.Host, strconv.Itoa(port)): true,
		net.JoinHostPort("localhost", strconv.Itoa(port)): true,
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !hosts[strings.ToLower(r.Host)] {
			writeError(w, http.StatusForbidden, contract.ErrForbidden,
				"a request on the port names 127.0.0.1:"+strconv.Itoa(port)+" or localhost:"+strconv.Itoa(port)+" as its host", nil)
			return
		}
		token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		if !ok || !key.Equal(token) {
			// The same answer whatever was wrong: no token, another scheme,
			// a wrong one.
			w.Header().Set("WWW-Authenticate", `Bearer realm="hayami"`)
			writeError(w, http.StatusUnauthorized, contract.ErrUnauthorized,
				"a request on the port carries the token from "+tcpkey.FileName+" as Authorization: Bearer", nil)
			return
		}
		next.ServeHTTP(w, r)
	})
}

/*
tcpListener is the port, or nil when it is not set up (R1, R2, R11): the
listener systemd or launchd handed over, or one made on the key's address.
Without a key the service may serve with, there is no port, and one handed
over is closed: a port is never served without its token. The port is on
127.0.0.1 or it is refused.
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
		var lc net.ListenConfig
		l, err = lc.Listen(context.Background(), "tcp4", key.Address)
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
	return l, key
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
