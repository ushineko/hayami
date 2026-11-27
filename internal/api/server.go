/*
Package api is the service other programs read (spec 113): the readings,
their details and drills, as JSON over a socket only this account can open.

It is a shell beside the window, the terminal and the command line: it holds
the sources (Hub), polls them while a client reads them, and maps what they
hold to the contract (`api/openapi.yaml`, package contract). It decides
nothing about what a section reads, and serves nothing the view has cut or
padded: names whole, numbers in their units, times as times.
*/
package api

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ushineko/hayami/internal/api/contract"
	"github.com/ushineko/hayami/internal/safetext"
)

// The server's limits (R23, R31).
const (
	// MaxConns is the most connections open at once.
	MaxConns = 64
	// MaxStreams is the most event streams open at once.
	MaxStreams = 16
	// ReadHeaderTimeout closes a connection that sends no request.
	ReadHeaderTimeout = 5 * time.Second
	// MaxHeaderBytes caps a request's headers.
	MaxHeaderBytes = 16 << 10
	// WriteTimeout bounds every write to a client.
	WriteTimeout = 10 * time.Second
	// KeepAlive is how often an idle stream gets a comment.
	KeepAlive = 15 * time.Second
	// RetryAfter is what a client over a limit is told to wait, in seconds.
	RetryAfter = 2
)

// Server serves a hub over HTTP.
type Server struct {
	hub     *Hub
	version string
	log     *slog.Logger

	// KeepAlive is the comment interval on a stream; a test shortens it.
	KeepAlive time.Duration

	mu      sync.Mutex
	streams int
}

// NewServer serves hub, saying it is version.
func NewServer(hub *Hub, version string, log *slog.Logger) *Server {
	return &Server{hub: hub, version: version, log: log, KeepAlive: KeepAlive}
}

// HTTP is the server for a listener: its limits, its routes.
func (s *Server) HTTP() *http.Server {
	return &http.Server{
		Handler:           s.Handler(),
		ReadHeaderTimeout: ReadHeaderTimeout,
		MaxHeaderBytes:    MaxHeaderBytes,
		IdleTimeout:       60 * time.Second,
		ErrorLog:          slog.NewLogLogger(s.log.Handler(), slog.LevelWarn),
	}
}

// Handler is the routes, behind the checks every request passes.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1", s.index)
	mux.HandleFunc("/v1/health", s.health)
	mux.HandleFunc("/v1/sections", s.sectionList)
	mux.HandleFunc("/v1/sections/{key}", s.section)
	mux.HandleFunc("/v1/sections/{key}/details/{id...}", s.detail)
	mux.HandleFunc("/v1/readings", s.readings)
	mux.HandleFunc("/v1/events", s.events)
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		writeError(w, http.StatusNotFound, contract.ErrNotFound, "no such path; every path is under /v1", nil)
	})
	return s.guard(mux)
}

/*
guard is what every request passes before its route (R23, R31): only GET,
no body, no Origin -- nothing in a browser reaches this socket today, and a
bridge that let one would have to be a decision -- a deadline on the write,
and a panic made a 500 with the service still running.
*/
func (s *Server) guard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if p := recover(); p != nil {
				if p == http.ErrAbortHandler { //nolint:errorlint // the sentinel net/http panics with
					panic(p)
				}
				s.log.Error("a request panicked", "path", r.URL.Path, "panic", safetext.WithoutHome(fmt.Sprint(p)))
				writeError(w, http.StatusInternalServerError, contract.ErrInternal, "the request failed unexpectedly", nil)
			}
		}()
		switch {
		case r.Method != http.MethodGet:
			w.Header().Set("Allow", http.MethodGet)
			writeError(w, http.StatusMethodNotAllowed, contract.ErrMethodNotAllowed, "only GET is served", nil)
			return
		case r.ContentLength != 0 || len(r.TransferEncoding) > 0:
			writeError(w, http.StatusBadRequest, contract.ErrBadRequest, "a request has no body", nil)
			return
		case r.Header.Get("Origin") != "":
			writeError(w, http.StatusForbidden, contract.ErrForbidden, "requests from a browser are refused", nil)
			return
		}
		next.ServeHTTP(w, r)
	})
}

/*
writeJSON writes v as the response, with WriteTimeout to write it in. The
deadline is set here, once there is something to write, and not before the
handler: a first request may wait for a reading longer than a write may
take. v is encoded before anything is sent, so a value JSON cannot hold is a
500 in the error shape and not an empty 200.
*/
func writeJSON(w http.ResponseWriter, status int, v any) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		status, buf = http.StatusInternalServerError, bytes.Buffer{}
		_ = json.NewEncoder(&buf).Encode(contract.Error{Error: contract.ErrorBody{
			Code: contract.ErrInternal, Message: "the reading could not be written as JSON", Known: []string{},
		}})
	}
	_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(WriteTimeout))
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_, _ = w.Write(buf.Bytes())
}

// writeError writes the error shape, its text cleaned as a reading's is.
func writeError(w http.ResponseWriter, status int, code, message string, known []string) {
	if known == nil {
		known = []string{}
	}
	if status == http.StatusServiceUnavailable {
		w.Header().Set("Retry-After", strconv.Itoa(RetryAfter))
	}
	body := contract.Error{Error: contract.ErrorBody{Code: code, Message: message, Known: known}}
	cleanText(&body)
	writeJSON(w, status, body)
}

// writeHubError writes what the hub said was wrong.
func writeHubError(w http.ResponseWriter, err error) {
	var unknown *ErrUnknown
	var busy *ErrBusy
	switch {
	case errors.As(err, &unknown):
		writeError(w, http.StatusNotFound, contract.ErrNotFound, unknown.Error(), unknown.Known)
	case errors.As(err, &busy):
		writeError(w, http.StatusServiceUnavailable, contract.ErrBusy, busy.Error(), nil)
	default:
		writeError(w, http.StatusInternalServerError, contract.ErrInternal, *errorText(err), nil)
	}
}

func (s *Server) index(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, contract.Index{
		Service: "hayami", Version: s.version, API: contract.Version, Sections: s.hub.Keys(),
	})
}

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, contract.Health{OK: true})
}

func (s *Server) sectionList(w http.ResponseWriter, _ *http.Request) {
	settings := s.hub.Settings()
	out := contract.SectionList{Sections: []contract.SectionInfo{}}
	for _, info := range s.hub.Infos() {
		out.Sections = append(out.Sections, contract.SectionInfo{
			Key: info.Key, Title: info.Title, Default: info.Default, Shown: settings.Shows(info.Key),
			IntervalS: info.Interval.Seconds(),
		})
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) section(w http.ResponseWriter, r *http.Request) {
	snap, err := s.hub.Section(r.Context(), r.PathValue("key"))
	if err != nil {
		writeHubError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, snap)
}

func (s *Server) detail(w http.ResponseWriter, r *http.Request) {
	snap, err := s.hub.Detail(r.Context(), r.PathValue("key"), r.PathValue("id"))
	if err != nil {
		writeHubError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, snap)
}

// keyList is a comma-separated parameter's values.
func keyList(v string) []string {
	var out []string
	for _, k := range strings.Split(v, ",") {
		if k = strings.TrimSpace(k); k != "" {
			out = append(out, k)
		}
	}
	return out
}

func (s *Server) readings(w http.ResponseWriter, r *http.Request) {
	keys := s.hub.Keys()
	if r.URL.Query().Has("sections") {
		keys = keyList(r.URL.Query().Get("sections"))
	}
	// Each section is read at once: the first request for several unread
	// sections waits for the slowest, not their sum.
	snaps := make([]contract.Section, len(keys))
	errs := make([]error, len(keys))
	var wg sync.WaitGroup
	for i, key := range keys {
		wg.Go(func() { snaps[i], errs[i] = s.hub.Section(r.Context(), key) })
	}
	wg.Wait()
	out := contract.Readings{Readings: map[string]contract.Section{}}
	for i, key := range keys {
		if errs[i] != nil {
			writeHubError(w, errs[i])
			return
		}
		out.Readings[key] = snaps[i]
	}
	writeJSON(w, http.StatusOK, out)
}

// openStream counts a stream in, and reports false over the limit.
func (s *Server) openStream() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.streams >= MaxStreams {
		return false
	}
	s.streams++
	return true
}

func (s *Server) closeStream() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.streams--
}

/*
events is the stream (R21): an event for each section and detail named with
its latest reading, then one for each new one, and a comment every
KeepAlive so an idle stream is not taken for a dead one. A write that does
not finish in WriteTimeout, or a queue left unread for StallLimit, ends it.
*/
func (s *Server) events(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	keys, details := keyList(q.Get("sections")), keyList(q.Get("details"))
	if len(keys) == 0 && len(details) == 0 {
		writeError(w, http.StatusBadRequest, contract.ErrBadRequest, "name at least one section or detail", s.hub.Keys())
		return
	}
	if len(details) > MaxStreamDetails {
		writeError(w, http.StatusBadRequest, contract.ErrBadRequest, fmt.Sprintf("a stream names at most %d details", MaxStreamDetails), nil)
		return
	}
	if !s.openStream() {
		writeError(w, http.StatusServiceUnavailable, contract.ErrBusy, fmt.Sprintf("%d streams are open", MaxStreams), nil)
		return
	}
	defer s.closeStream()

	sub, cancel, err := s.hub.subscribe(r.Context(), keys, details)
	if err != nil {
		writeHubError(w, err)
		return
	}
	defer cancel()

	rc := http.NewResponseController(w)
	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	write := func(text string) bool {
		_ = rc.SetWriteDeadline(time.Now().Add(WriteTimeout))
		if _, err := w.Write([]byte(text)); err != nil {
			return false
		}
		return rc.Flush() == nil
	}
	if !write(": hayami " + contract.Version + "\n\n") {
		return
	}
	tick := time.NewTicker(s.KeepAlive)
	defer tick.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-sub.Dead():
			return
		case <-tick.C:
			if !write(": keepalive\n\n") {
				return
			}
		case <-sub.Ready():
			for _, ev := range sub.take() {
				data, err := json.Marshal(ev.data)
				if err != nil || !write("event: "+ev.name+"\ndata: "+string(data)+"\n\n") {
					return
				}
			}
		}
	}
}

/*
Limiter counts the connections open across every listener it wraps, and
answers one over its limit with a 503 and closes it, so a client over the
limit is told so rather than left waiting (spec 113 R31). The socket and the
TCP port share one (spec 115 R5).
*/
type Limiter struct {
	mu    sync.Mutex
	open  int
	limit int

	// refusing bounds the answers to connections over the limit: past it, a
	// flood's connections are closed without one.
	refusing chan struct{}
}

// NewLimiter is a Limiter of limit connections at once.
func NewLimiter(limit int) *Limiter {
	return &Limiter{limit: limit, refusing: make(chan struct{}, 16)}
}

// Wrap is l with its connections counted against the limiter.
func (lm *Limiter) Wrap(l net.Listener) net.Listener {
	return &limitListener{Listener: l, lm: lm}
}

// Limit wraps l so at most limit connections are served at once.
func Limit(l net.Listener, limit int) net.Listener { return NewLimiter(limit).Wrap(l) }

type limitListener struct {
	net.Listener
	lm *Limiter
}

func (l *limitListener) Accept() (net.Conn, error) {
	for {
		c, err := l.Listener.Accept()
		if err != nil {
			return nil, err //nolint:wrapcheck // the listener's own error, which http.Server inspects
		}
		lm := l.lm
		lm.mu.Lock()
		if lm.open >= lm.limit {
			lm.mu.Unlock()
			select {
			case lm.refusing <- struct{}{}:
				go func() {
					defer func() { <-lm.refusing }()
					refuse(c)
				}()
			default:
				_ = c.Close()
			}
			continue
		}
		lm.open++
		lm.mu.Unlock()
		return &countedConn{Conn: c, release: lm.release}, nil
	}
}

func (lm *Limiter) release() {
	lm.mu.Lock()
	defer lm.mu.Unlock()
	lm.open--
}

/*
refuse answers a connection over the limit and closes it. The request's
headers are read first, for a second at most: a client still sending them
when the answer came would see its write fail rather than read the 503.
*/
func refuse(c net.Conn) {
	defer func() { _ = c.Close() }()
	_ = c.SetReadDeadline(time.Now().Add(time.Second))
	if req, err := http.ReadRequest(bufio.NewReaderSize(io.LimitReader(c, MaxHeaderBytes), 4096)); err == nil {
		_ = req.Body.Close()
	}
	body := `{"error":{"code":"busy","message":"too many connections","known":[]}}` + "\n"
	_ = c.SetWriteDeadline(time.Now().Add(time.Second))
	_, _ = fmt.Fprintf(c, "HTTP/1.1 503 Service Unavailable\r\nContent-Type: application/json; charset=utf-8\r\n"+
		"Retry-After: %d\r\nContent-Length: %d\r\nConnection: close\r\n\r\n%s", RetryAfter, len(body), body)
}

// countedConn gives its place back once when it closes.
type countedConn struct {
	net.Conn
	once    sync.Once
	release func()
}

func (c *countedConn) Close() error {
	c.once.Do(c.release)
	return c.Conn.Close() //nolint:wrapcheck // the connection's own error, passed through as net/http expects
}
