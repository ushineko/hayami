package client

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

	"github.com/coder/websocket"

	"github.com/ushineko/hayami/internal/api/contract"
)

// ErrImpostor is a TCP port whose server could not prove it holds the
// token: not the service, whatever it says (spec 115). Nothing it sent is
// trusted, and the client has told it nothing it can use.
var ErrImpostor = errors.New("whatever answered on the TCP port could not prove it is the service")

// maxWsMessage bounds what the client reads of one message, as MaxBody does
// a response.
const maxWsMessage = MaxBody

/*
WS is a WebSocket on the service (specs 115, 116): Hello as the service
sent it, then the messages for what Subscribe asked for.
*/
type WS struct {
	c     *websocket.Conn
	Hello contract.WsHello
}

/*
DialWS opens the WebSocket on the service's socket at address. There is no
challenge on the socket: hello comes first, with a null proof.
*/
func DialWS(ctx context.Context, address string) (*WS, error) {
	hc := &http.Client{Transport: &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) { return Dial(ctx, address) },
	}}
	c, resp, err := websocket.Dial(ctx, "ws://hayami/v1/ws", &websocket.DialOptions{HTTPClient: hc})
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
	if err != nil {
		return nil, fmt.Errorf("opening the WebSocket: %w", err)
	}
	c.SetReadLimit(maxWsMessage)
	w := &WS{c: c}
	if err := w.readHello(ctx, nil); err != nil {
		_ = c.CloseNow()
		return nil, err
	}
	return w, nil
}

/*
DialTCPWS opens the WebSocket on the TCP port at address ("127.0.0.1:<port>")
with token (spec 115): it answers the service's challenge with a proof and a
challenge of its own, and checks the service's proof before it trusts
anything. The token never crosses the connection, so a program that took the
port first learns nothing it can use, and is refused with ErrImpostor.
*/
func DialTCPWS(ctx context.Context, address, token string) (*WS, error) {
	hc := &http.Client{Transport: &http.Transport{Proxy: nil}, CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	c, resp, err := websocket.Dial(ctx, "ws://"+address+"/v1/ws", &websocket.DialOptions{HTTPClient: hc})
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
	if err != nil {
		return nil, fmt.Errorf("opening the WebSocket: %w", err)
	}
	c.SetReadLimit(maxWsMessage)
	w := &WS{c: c}
	fail := func(err error) (*WS, error) {
		_ = c.CloseNow()
		return nil, err
	}
	var ch contract.WsChallenge
	if err := w.readAs(ctx, contract.WsTypeChallenge, &ch); err != nil {
		return fail(err)
	}
	theirs, err := base64.RawURLEncoding.DecodeString(ch.Nonce)
	if err != nil || len(theirs) != 32 {
		return fail(ErrImpostor)
	}
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	ours := base64.RawURLEncoding.EncodeToString(b)
	proof, err := contract.WsProof(token, contract.WsClientLabel, ch.Nonce, ours)
	if err != nil {
		return fail(err)
	}
	if err := w.send(ctx, contract.WsAuth{Type: contract.WsTypeAuth, Nonce: ours, Proof: proof}); err != nil {
		return fail(err)
	}
	want, err := contract.WsProof(token, contract.WsServerLabel, ours, ch.Nonce)
	if err != nil {
		return fail(err)
	}
	if err := w.readHello(ctx, &want); err != nil {
		return fail(err)
	}
	return w, nil
}

// readHello reads hello and checks its proof: want's on the TCP port, null
// on the socket.
func (w *WS) readHello(ctx context.Context, want *string) error {
	var h contract.WsHello
	if err := w.readAs(ctx, contract.WsTypeHello, &h); err != nil {
		if want != nil && websocket.CloseStatus(err) == 4401 {
			return errors.New("the service refused the token: is it the one in api-tcp.json?") //nolint:err113 // said plainly
		}
		return err
	}
	switch {
	case want == nil && h.Proof != nil:
		return errors.New("the service sent a proof on the socket, where there is no challenge") //nolint:err113 // said plainly
	case want != nil && (h.Proof == nil || !contract.WsProofEqual(*h.Proof, *want)):
		return ErrImpostor
	}
	w.Hello = h
	return nil
}

// ResubscribeEvery is how often a client that holds a WebSocket resends its
// subscription, well within the service's 90 s for a client that sends
// nothing (spec 118).
const ResubscribeEvery = 30 * time.Second

// Subscribe replaces what the connection follows.
func (w *WS) Subscribe(ctx context.Context, sections, details []string) error {
	if sections == nil {
		sections = []string{}
	}
	if details == nil {
		details = []string{}
	}
	return w.send(ctx, contract.WsSubscribe{Type: contract.WsTypeSubscribe, Sections: sections, Details: details})
}

/*
Next is the next message: a contract.WsSection, a contract.WsDetail or a
contract.WsError, each held to the contract, or an error for anything else.
*/
func (w *WS) Next(ctx context.Context) (any, error) {
	typ, data, err := w.c.Read(ctx)
	if err != nil {
		return nil, fmt.Errorf("reading the WebSocket: %w", err)
	}
	if typ != websocket.MessageText {
		return nil, errors.New("the service sent a binary message") //nolint:err113 // said plainly
	}
	var m contract.WsMessage
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, errors.New("the service sent a message that is not JSON") //nolint:err113 // said plainly
	}
	var v any
	switch m.Type {
	case contract.WsTypeSection:
		v = &contract.WsSection{}
	case contract.WsTypeDetail:
		v = &contract.WsDetail{}
	case contract.WsTypeError:
		v = &contract.WsError{}
	default:
		return nil, &ContractError{Path: "type", Reason: fmt.Sprintf("%q is not a message the service sends here", m.Type)}
	}
	if err := Decode(data, v); err != nil {
		return nil, within(m.Type, err)
	}
	switch x := v.(type) {
	case *contract.WsSection:
		return *x, nil
	case *contract.WsDetail:
		return *x, nil
	default:
		return *(v.(*contract.WsError)), nil
	}
}

// Close ends the WebSocket.
func (w *WS) Close() error {
	return w.c.Close(websocket.StatusNormalClosure, "") //nolint:wrapcheck // the connection's own error
}

func (w *WS) send(ctx context.Context, v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("encoding a message: %w", err)
	}
	if err := w.c.Write(ctx, websocket.MessageText, data); err != nil {
		return fmt.Errorf("writing to the WebSocket: %w", err)
	}
	return nil
}

// readAs reads the next message, which must be want's type, into v, held to
// the contract.
func (w *WS) readAs(ctx context.Context, want string, v any) error {
	typ, data, err := w.c.Read(ctx)
	if err != nil {
		return fmt.Errorf("reading the WebSocket: %w", err)
	}
	if typ != websocket.MessageText {
		return errors.New("the service sent a binary message") //nolint:err113 // said plainly
	}
	var m contract.WsMessage
	if err := json.Unmarshal(data, &m); err != nil || m.Type != want {
		return &ContractError{Path: "type", Reason: fmt.Sprintf("the first message is not %s", want)}
	}
	if err := Decode(data, v); err != nil {
		return within(want, err)
	}
	return nil
}

// WSCheckReport is what CheckWS read.
type WSCheckReport struct {
	Sections, Details int
}

/*
CheckWS holds the WebSocket to the contract (spec 116 R10): hello as it came,
a section message for every section hello lists, each to the full-value
rules as `check` holds them; the details those sections offer, up to the
most a subscription names; and a subscription naming a section that is not
there, answered by an error that lists those that are. Every message is
decoded strictly by Next.
*/
func CheckWS(ctx context.Context, w *WS, out io.Writer) (WSCheckReport, error) {
	var r WSCheckReport
	say := func(format string, args ...any) {
		if out != nil {
			_, _ = fmt.Fprintf(out, format+"\n", args...)
		}
	}
	if len(w.Hello.Sections) == 0 {
		return r, errors.New("hello lists no sections") //nolint:err113 // said plainly
	}
	say("hello: %s %s, contract %s, %d sections", w.Hello.Service, w.Hello.Version, w.Hello.API, len(w.Hello.Sections))
	if err := w.Subscribe(ctx, w.Hello.Sections, nil); err != nil {
		return r, err
	}
	var details []string
	seen := map[string]bool{}
	for len(seen) < len(w.Hello.Sections) {
		m, err := w.Next(ctx)
		if err != nil {
			return r, err
		}
		s, ok := m.(contract.WsSection)
		if !ok {
			return r, fmt.Errorf("asked for every section, the service sent %T", m) //nolint:err113 // said plainly
		}
		if err := sectionRules("ws section "+s.Data.Key, s.Data.Key, s.Data); err != nil {
			return r, err
		}
		if !seen[s.Data.Key] {
			seen[s.Data.Key] = true
			say("section %s", s.Data.Key)
			for _, d := range s.Data.Details {
				if len(details) < maxCheckDetails {
					details = append(details, s.Data.Key+"/"+d.ID)
				}
			}
		}
	}
	r.Sections = len(seen)
	if len(details) > 0 {
		if err := w.Subscribe(ctx, nil, details); err != nil {
			return r, err
		}
		got := map[string]bool{}
		for len(got) < len(details) {
			m, err := w.Next(ctx)
			if err != nil {
				return r, err
			}
			switch d := m.(type) {
			case contract.WsDetail:
				if !got[d.Data.Section+"/"+d.Data.ID] {
					got[d.Data.Section+"/"+d.Data.ID] = true
					say("detail %s/%s", d.Data.Section, d.Data.ID)
				}
			case contract.WsSection:
				// A section message still in flight from the subscription before.
			default:
				return r, fmt.Errorf("asked for details, the service sent %T", m) //nolint:err113 // said plainly
			}
		}
		r.Details = len(got)
	}
	if err := w.Subscribe(ctx, []string{"no-such-section"}, nil); err != nil {
		return r, err
	}
	for {
		m, err := w.Next(ctx)
		if err != nil {
			return r, err
		}
		e, ok := m.(contract.WsError)
		if !ok {
			continue // readings of what is still followed
		}
		if e.Data.Code != contract.ErrNotFound || len(e.Data.Known) == 0 {
			return r, fmt.Errorf("a section that is not there was answered %q, knowing %v", e.Data.Code, e.Data.Known) //nolint:err113 // said plainly
		}
		say("a section that is not there: refused, %d known", len(e.Data.Known))
		return r, nil
	}
}

// maxCheckDetails is the most details CheckWS subscribes to: the most one
// subscription may name.
const maxCheckDetails = 16
