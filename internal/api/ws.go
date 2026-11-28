package api

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"

	"github.com/ushineko/hayami/internal/api/contract"
)

// MaxWsMessage is the largest message a client may send (spec 116 R3): a
// subscription naming every section and the most details is far less.
const MaxWsMessage = 4096

// AuthWait is how long a client on the TCP port has to answer the challenge
// (spec 115).
var AuthWait = 5 * time.Second

/*
ClientIdle is how long a client may send nothing before its WebSocket is
closed (spec 118, #424). The service's pings do not show a client is alive:
Herdr answers them for a mod whose build was replaced, and that connection
would otherwise stay open, read by nobody, its sections leased. A client
resends its subscription well within it.
*/
const ClientIdle = 90 * time.Second

// StatusUnauthorized is the close code for a client that did not prove it
// holds the token, in time or at all.
const StatusUnauthorized websocket.StatusCode = 4401

// nonceBytes is a nonce's length before encoding.
const nonceBytes = 32

// newNonce is 32 random bytes in base64url.
func newNonce() string {
	b := make([]byte, nonceBytes)
	_, _ = rand.Read(b) // crypto/rand.Read does not fail; it aborts the program first
	return base64.RawURLEncoding.EncodeToString(b)
}

// goodNonce says a nonce is 32 bytes in base64url, as the contract has it.
func goodNonce(n string) bool {
	b, err := base64.RawURLEncoding.DecodeString(n)
	return err == nil && len(b) == nonceBytes
}

// wsSession is one WebSocket's connection and its context.
type wsSession struct {
	c   *websocket.Conn
	ctx context.Context
}

// write sends v as a text message, within WriteTimeout.
func (ws wsSession) write(v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("encoding a message: %w", err)
	}
	ctx, cancel := context.WithTimeout(ws.ctx, WriteTimeout)
	defer cancel()
	return ws.c.Write(ctx, websocket.MessageText, data) //nolint:wrapcheck // the connection's own error
}

// read is the next message as JSON into v, refused unless it is text and
// its type is want; within wait when wait is not zero.
func (ws wsSession) read(wait time.Duration) (contract.WsMessage, []byte, error) {
	ctx := ws.ctx
	if wait > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, wait)
		defer cancel()
	}
	typ, data, err := ws.c.Read(ctx)
	if err != nil {
		return contract.WsMessage{}, nil, err //nolint:wrapcheck // the connection's own error
	}
	if typ != websocket.MessageText {
		return contract.WsMessage{}, nil, errBinary
	}
	var m contract.WsMessage
	if err := json.Unmarshal(data, &m); err != nil {
		return contract.WsMessage{}, nil, errNotAMessage
	}
	return m, data, nil
}

var (
	errBinary      = errors.New("a message is text, not binary")
	errNotAMessage = errors.New("not a message here")
)

// strict decodes data into v, refusing a field v lacks.
func strict(data []byte, v any) error {
	d := json.NewDecoder(bytesReader(data))
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		return errNotAMessage
	}
	return nil
}

/*
ws is /v1/ws (specs 115, 116): the events /v1/events sends, as messages, for
what the client subscribes to. On the TCP port auth is the port's key, and
the connection begins with the mutual challenge-response: the client proves
it holds the token over the service's nonce, the service over the client's,
and the token itself is never sent. On the socket auth is nil and hello
comes first. A WebSocket counts as a stream.
*/
func (s *Server) ws(w http.ResponseWriter, r *http.Request, auth *TCPAuth) {
	if !s.openStream() {
		writeError(w, http.StatusServiceUnavailable, contract.ErrBusy, fmt.Sprintf("%d streams are open", MaxStreams), nil)
		return
	}
	defer s.closeStream()
	// The guard has refused any request carrying Origin, so the library's
	// own origin check, which passes a request without one, has nothing
	// left to decide.
	c, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	defer func() { _ = c.CloseNow() }()
	c.SetReadLimit(MaxWsMessage)
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	ws := wsSession{c: c, ctx: ctx}

	var proof *string
	if auth != nil {
		p, ok := s.challenge(ws, auth)
		if !ok {
			return
		}
		proof = &p
		// A new key ends the sessions the old one let in (spec 115, F4).
		defer auth.track(cancel)()
	}
	if ws.write(contract.WsHello{Type: contract.WsTypeHello, Service: "hayami", Version: s.version,
		API: contract.Version, Sections: s.hub.Keys(), Proof: proof}) != nil {
		return
	}
	s.wsServe(ws)
}

/*
challenge is the TCP port's mutual challenge-response: the service's nonce,
the client's answer within AuthWait, checked in constant time. It returns
the service's proof over the client's nonce for hello. A wrong, late or
malformed answer closes the connection with 4401, and says nothing of why.
*/
func (s *Server) challenge(ws wsSession, auth *TCPAuth) (string, bool) {
	key := auth.current()
	if key == nil {
		_ = ws.c.Close(StatusUnauthorized, "not authenticated")
		return "", false
	}
	ours := newNonce()
	if ws.write(contract.WsChallenge{Type: contract.WsTypeChallenge, Nonce: ours}) != nil {
		return "", false
	}
	refuse := func() (string, bool) {
		_ = ws.c.Close(StatusUnauthorized, "not authenticated")
		return "", false
	}
	// The deadline closes with 4401 while the read waits: a read whose own
	// context ended would close the connection without a code.
	late := time.AfterFunc(AuthWait, func() { _ = ws.c.Close(StatusUnauthorized, "not authenticated") })
	defer late.Stop()
	m, data, err := ws.read(0)
	if err != nil || m.Type != contract.WsTypeAuth {
		return refuse()
	}
	var a contract.WsAuth
	if strict(data, &a) != nil || !goodNonce(a.Nonce) || a.Nonce == ours {
		return refuse()
	}
	want, err := contract.WsProof(key.Token, contract.WsClientLabel, ours, a.Nonce)
	if err != nil || !contract.WsProofEqual(a.Proof, want) {
		return refuse()
	}
	mine, err := contract.WsProof(key.Token, contract.WsServerLabel, a.Nonce, ours)
	if err != nil {
		return refuse()
	}
	return mine, true
}

// wsServe follows the client's subscriptions until the connection ends: the
// latest reading of each thing at once, then one per new reading, a ping
// every KeepAlive, and an end for a client that stops reading.
func (s *Server) wsServe(ws wsSession) {
	subs := make(chan contract.WsSubscribe)
	readErr := make(chan error, 1)
	var heard atomic.Int64
	heard.Store(time.Now().UnixNano())
	go func() {
		for {
			m, data, err := ws.read(0)
			heard.Store(time.Now().UnixNano())
			if err == nil && m.Type != contract.WsTypeSubscribe {
				err = errNotAMessage
			}
			var sub contract.WsSubscribe
			if err == nil && strict(data, &sub) != nil {
				err = errNotAMessage
			}
			if err != nil {
				readErr <- err
				return
			}
			select {
			case subs <- sub:
			case <-ws.ctx.Done():
				return
			}
		}
	}()

	var sub *subscriber
	unsubscribe := func() {}
	defer func() { unsubscribe() }()
	var dead, ready <-chan struct{}
	tick := time.NewTicker(s.KeepAlive)
	defer tick.Stop()
	for {
		select {
		case <-ws.ctx.Done():
			return
		case err := <-readErr:
			switch {
			case errors.Is(err, errBinary), errors.Is(err, errNotAMessage):
				_ = ws.c.Close(websocket.StatusUnsupportedData, err.Error())
			}
			return
		case req := <-subs:
			if len(req.Details) > MaxStreamDetails {
				if ws.write(wsError(contract.ErrBadRequest, fmt.Sprintf("a subscription names at most %d details", MaxStreamDetails), nil)) != nil {
					return
				}
				continue
			}
			next, cancelNext, err := s.hub.subscribe(ws.ctx, req.Sections, req.Details)
			if err != nil {
				// The subscription before it stands.
				if ws.write(wsHubError(err)) != nil {
					return
				}
				continue
			}
			unsubscribe()
			sub, unsubscribe = next, cancelNext
			dead, ready = sub.Dead(), sub.Ready()
		case <-dead:
			_ = ws.c.Close(websocket.StatusPolicyViolation, "messages were left unread")
			return
		case <-ready:
			for _, ev := range sub.take() {
				if ws.write(struct {
					Type string `json:"type"`
					Data any    `json:"data"`
				}{ev.name, ev.data}) != nil {
					return
				}
			}
		case <-tick.C:
			if time.Since(time.Unix(0, heard.Load())) > s.ClientIdle {
				_ = ws.c.Close(websocket.StatusPolicyViolation, "the client sent nothing")
				return
			}
			go func() {
				ctx, cancel := context.WithTimeout(ws.ctx, StallLimit)
				defer cancel()
				if ws.c.Ping(ctx) != nil {
					_ = ws.c.Close(websocket.StatusPolicyViolation, "no answer to a ping")
				}
			}()
		}
	}
}

func wsError(code, message string, known []string) contract.WsError {
	if known == nil {
		known = []string{}
	}
	return contract.WsError{Type: contract.WsTypeError, Data: contract.ErrorBody{Code: code, Message: message, Known: known}}
}

// wsHubError is a subscription the hub refused, as writeHubError says it.
func wsHubError(err error) contract.WsError {
	var unknown *ErrUnknown
	var busy *ErrBusy
	switch {
	case errors.As(err, &unknown):
		return wsError(contract.ErrNotFound, unknown.Error(), unknown.Known)
	case errors.As(err, &busy):
		return wsError(contract.ErrBusy, busy.Error(), nil)
	default:
		return wsError(contract.ErrInternal, *errorText(err), nil)
	}
}
