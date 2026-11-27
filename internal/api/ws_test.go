package api_test

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/hayami/internal/api"
	"github.com/ushineko/hayami/internal/api/client"
	"github.com/ushineko/hayami/internal/api/contract"
	"github.com/ushineko/hayami/internal/api/tcpkey"
	"github.com/ushineko/hayami/internal/config"
	"github.com/ushineko/hayami/internal/panel"
)

// rawWS is a WebSocket on the harness's TCP port with nothing done for the
// test: the challenge and everything after are its to answer.
func rawWS(t *testing.T, h *harness, host string) (*websocket.Conn, int, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	o := &websocket.DialOptions{Host: host}
	c, resp, err := websocket.Dial(ctx, "ws://"+h.tcpAddr+"/v1/ws", o)
	status := 0
	if resp != nil {
		status = resp.StatusCode
		if resp.Body != nil {
			_ = resp.Body.Close()
		}
	}
	if c != nil {
		t.Cleanup(func() { _ = c.CloseNow() })
	}
	return c, status, err
}

// readMsg reads one message from c and decodes it into v, after checking its
// type is want; it returns the raw message for a conformance check.
func readMsg(t *testing.T, c *websocket.Conn, want string, v any) []byte {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	typ, data, err := c.Read(ctx)
	require.NoError(t, err)
	require.Equal(t, websocket.MessageText, typ)
	var m contract.WsMessage
	require.NoError(t, json.Unmarshal(data, &m))
	require.Equal(t, want, m.Type, "message: %s", data)
	require.NoError(t, json.Unmarshal(data, v))
	return data
}

func send(t *testing.T, c *websocket.Conn, v any) {
	t.Helper()
	data, err := json.Marshal(v)
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.NoError(t, c.Write(ctx, websocket.MessageText, data))
}

// closedWith reads until the connection ends and returns how.
func closedWith(t *testing.T, c *websocket.Conn) websocket.StatusCode {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	for {
		_, data, err := c.Read(ctx)
		if err != nil {
			return websocket.CloseStatus(err)
		}
		var m contract.WsMessage
		_ = json.Unmarshal(data, &m)
		require.NotContains(t, []string{contract.WsTypeHello, contract.WsTypeSection, contract.WsTypeDetail}, m.Type,
			"a client not let in was sent %s", m.Type)
	}
}

func nonce() string { return strings.Repeat("A", 43) }

// refusedWith is the status an upgrade that must be refused was answered.
func refusedWith(ctx context.Context, t *testing.T, url string, o *websocket.DialOptions) int {
	t.Helper()
	c, resp, err := websocket.Dial(ctx, url, o)
	if c != nil {
		_ = c.CloseNow()
	}
	require.Error(t, err, "the upgrade was not refused")
	require.NotNil(t, resp)
	_ = resp.Body.Close()
	return resp.StatusCode
}

/*
Spec 115, review F1. The TCP port's connection begins with the mutual
challenge-response: the client's proof over the service's nonce lets it in,
and hello carries the service's proof over the client's, which only a holder
of the token could make. The token itself is never sent.
*/
func TestThePortsChallengeLetsTheTokenHolderIn(t *testing.T) {
	h := start(t, sources(fixtures()), options{tcp: true})
	c, _, err := rawWS(t, h, "")
	require.NoError(t, err)

	var ch contract.WsChallenge
	conforms(t, "WsChallenge", readMsg(t, c, contract.WsTypeChallenge, &ch))
	ours := "B" + strings.Repeat("C", 42)
	proof, err := contract.WsProof(h.key.Token, contract.WsClientLabel, ch.Nonce, ours)
	require.NoError(t, err)
	auth := contract.WsAuth{Type: contract.WsTypeAuth, Nonce: ours, Proof: proof}
	sent, _ := json.Marshal(auth)
	assert.NotContains(t, string(sent), h.key.Token, "the token crossed the connection")
	send(t, c, auth)

	var hello contract.WsHello
	conforms(t, "WsHello", readMsg(t, c, contract.WsTypeHello, &hello))
	require.NotNil(t, hello.Proof, "no proof from the service on the port")
	want, err := contract.WsProof(h.key.Token, contract.WsServerLabel, ours, ch.Nonce)
	require.NoError(t, err)
	assert.Equal(t, want, *hello.Proof, "the service's proof is not the token holder's")
	assert.NotEqual(t, proof, *hello.Proof, "the service's proof is the client's")

	send(t, c, contract.WsSubscribe{Type: contract.WsTypeSubscribe, Sections: []string{"cooler"}, Details: []string{}})
	var sec contract.WsSection
	conforms(t, "WsSection", readMsg(t, c, contract.WsTypeSection, &sec))
	assert.Equal(t, "cooler", sec.Data.Key)
}

/*
Spec 115, review F1. A client that cannot prove it holds the token is closed
with 4401 and sent nothing past the challenge: a wrong proof, no answer in
time, the service's own nonce sent back, a malformed nonce, a binary answer,
another message first, or the token itself offered as a proof.
*/
func TestAClientThatCannotProveItIsClosed(t *testing.T) {
	h := start(t, sources(fixtures()), options{tcp: true})
	was := api.AuthWait
	api.AuthWait = 300 * time.Millisecond
	t.Cleanup(func() { api.AuthWait = was })
	other := mustToken(t)

	cases := map[string]func(c *websocket.Conn, theirs string){
		"a wrong token's proof": func(c *websocket.Conn, theirs string) {
			p, _ := contract.WsProof(other, contract.WsClientLabel, theirs, nonce())
			send(t, c, contract.WsAuth{Type: contract.WsTypeAuth, Nonce: nonce(), Proof: p})
		},
		"the token as the proof": func(c *websocket.Conn, _ string) {
			send(t, c, contract.WsAuth{Type: contract.WsTypeAuth, Nonce: nonce(), Proof: h.key.Token})
		},
		"the service's proof sent back": func(c *websocket.Conn, theirs string) {
			p, _ := contract.WsProof(h.key.Token, contract.WsServerLabel, theirs, nonce())
			send(t, c, contract.WsAuth{Type: contract.WsTypeAuth, Nonce: nonce(), Proof: p})
		},
		"the service's nonce sent back": func(c *websocket.Conn, theirs string) {
			p, _ := contract.WsProof(h.key.Token, contract.WsClientLabel, theirs, theirs)
			send(t, c, contract.WsAuth{Type: contract.WsTypeAuth, Nonce: theirs, Proof: p})
		},
		"a short nonce": func(c *websocket.Conn, theirs string) {
			p, _ := contract.WsProof(h.key.Token, contract.WsClientLabel, theirs, "AAAA")
			send(t, c, contract.WsAuth{Type: contract.WsTypeAuth, Nonce: "AAAA", Proof: p})
		},
		"no answer": func(*websocket.Conn, string) {},
		"a subscription first": func(c *websocket.Conn, _ string) {
			send(t, c, contract.WsSubscribe{Type: contract.WsTypeSubscribe, Sections: []string{"cooler"}, Details: []string{}})
		},
		"a binary answer": func(c *websocket.Conn, _ string) {
			_ = c.Write(context.Background(), websocket.MessageBinary, []byte(`{"type":"auth"}`))
		},
	}
	for name, answer := range cases {
		t.Run(name, func(t *testing.T) {
			c, _, err := rawWS(t, h, "")
			require.NoError(t, err)
			var ch contract.WsChallenge
			readMsg(t, c, contract.WsTypeChallenge, &ch)
			answer(c, ch.Nonce)
			assert.Equal(t, api.StatusUnauthorized, closedWith(t, c))
		})
	}
}

// Spec 115, review F1. The reference client refuses a server on the port
// that cannot prove it holds the token, having sent it nothing it can use.
func TestTheClientRefusesAnImpostor(t *testing.T) {
	impostorToken := mustToken(t)
	victimToken := mustToken(t)
	var got []string
	srv := &http.Server{ReadHeaderTimeout: time.Second, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer func() { _ = c.CloseNow() }()
		ctx := r.Context()
		ns := nonce()
		b, _ := json.Marshal(contract.WsChallenge{Type: contract.WsTypeChallenge, Nonce: ns})
		_ = c.Write(ctx, websocket.MessageText, b)
		_, data, err := c.Read(ctx)
		if err != nil {
			return
		}
		got = append(got, string(data))
		var a contract.WsAuth
		_ = json.Unmarshal(data, &a)
		p, _ := contract.WsProof(impostorToken, contract.WsServerLabel, a.Nonce, ns)
		b, _ = json.Marshal(contract.WsHello{Type: contract.WsTypeHello, Service: "hayami", Version: "x", API: contract.Version, Sections: []string{}, Proof: &p})
		_ = c.Write(ctx, websocket.MessageText, b)
		time.Sleep(time.Second)
	})}
	var lc net.ListenConfig
	l, err := lc.Listen(context.Background(), "tcp4", "127.0.0.1:0")
	require.NoError(t, err)
	go func() { _ = srv.Serve(l) }()
	t.Cleanup(func() { _ = srv.Close() })

	_, err = client.DialTCPWS(context.Background(), l.Addr().String(), victimToken)
	require.ErrorIs(t, err, client.ErrImpostor)
	require.Len(t, got, 1)
	assert.NotContains(t, got[0], victimToken, "the client sent the impostor its token")
}

// Spec 115 R9. The port answers only a request naming it as its host; the
// WebSocket's upgrade is refused before anything else for another name, and
// for a browser's request, on the port and on the socket.
func TestTheWebSocketWantsItsOwnNameAndNoBrowser(t *testing.T) {
	h := start(t, sources(fixtures()), options{tcp: true})
	_, port, err := net.SplitHostPort(h.tcpAddr)
	require.NoError(t, err)
	for _, host := range []string{"evil.example:" + port, "127.0.0.1", "localhost:1", "[::1]:" + port} {
		_, status, err := rawWS(t, h, host)
		require.Error(t, err, "host %q", host)
		assert.Equal(t, http.StatusForbidden, status, "host %q", host)
	}
	c, _, err := rawWS(t, h, "localhost:"+port)
	require.NoError(t, err, "localhost:<port> was refused")
	var ch contract.WsChallenge
	readMsg(t, c, contract.WsTypeChallenge, &ch)

	ctx := context.Background()
	hdr := http.Header{"Origin": {"http://127.0.0.1:" + port}}
	assert.Equal(t, http.StatusForbidden, refusedWith(ctx, t, "ws://"+h.tcpAddr+"/v1/ws", &websocket.DialOptions{HTTPHeader: hdr}),
		"a browser's upgrade on the port")
	assert.Equal(t, http.StatusForbidden, refusedWith(ctx, t, "ws://hayami/v1/ws", &websocket.DialOptions{HTTPClient: serviceClient(h.sock), HTTPHeader: hdr}),
		"a browser's upgrade on the socket")
}

/*
Spec 116 R4–R6. On the socket hello comes first, with no proof. A
subscription brings the latest readings at once; another replaces it without
a new connection, letting go of what it dropped; one naming what is not there
is an error that lists what is, and the one before stands; closing lets go of
everything.
*/
func TestTheSocketsWebSocketFollowsSubscriptions(t *testing.T) {
	f, clk := coolerFixture(), clockFixture()
	f.interval, clk.interval = 50*time.Millisecond, 50*time.Millisecond
	h := start(t, []panel.Source{f, clk}, options{settings: config.Config{Sections: []string{"cooler", "clock"}},
		tune: func(hub *api.Hub, _ *api.Server) { hub.MinLease = 60 * time.Millisecond }})
	ctx := context.Background()
	c, resp, err := websocket.Dial(ctx, "ws://hayami/v1/ws", &websocket.DialOptions{HTTPClient: serviceClient(h.sock)})
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
	require.NoError(t, err)
	defer func() { _ = c.CloseNow() }()
	var hello contract.WsHello
	conforms(t, "WsHello", readMsg(t, c, contract.WsTypeHello, &hello))
	assert.Nil(t, hello.Proof, "a proof on the socket, where there is no challenge")

	send(t, c, contract.WsSubscribe{Type: contract.WsTypeSubscribe, Sections: []string{"cooler"}, Details: []string{}})
	var sec contract.WsSection
	readMsg(t, c, contract.WsTypeSection, &sec)
	assert.Equal(t, "cooler", sec.Data.Key)

	send(t, c, contract.WsSubscribe{Type: contract.WsTypeSubscribe, Sections: []string{"clock"}, Details: []string{}})
	waitFor(t, "the cooler to be let go", func() bool {
		ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
		defer cancel()
		_, data, err := c.Read(ctx)
		return err == nil && strings.Contains(string(data), `"key":"clock"`)
	})

	send(t, c, contract.WsSubscribe{Type: contract.WsTypeSubscribe, Sections: []string{"nope"}, Details: []string{}})
	var e contract.WsError
	for {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		_, data, err := c.Read(ctx)
		cancel()
		require.NoError(t, err)
		if strings.Contains(string(data), `"type":"error"`) {
			conforms(t, "WsError", data)
			require.NoError(t, json.Unmarshal(data, &e))
			break
		}
	}
	assert.Equal(t, contract.ErrNotFound, e.Data.Code)
	assert.Contains(t, e.Data.Known, "clock")

	_ = c.Close(websocket.StatusNormalClosure, "")
	waitFor(t, "everything to be let go", func() bool { s, d := h.hub.Active(); return s == 0 && d == 0 })
}

// Spec 116 R2, R3. A WebSocket counts as a stream; a message too large, a
// binary one or one that is not a message here closes the connection.
func TestTheWebSocketKeepsItsLimits(t *testing.T) {
	h := start(t, sources(fixtures()), options{tcp: true})
	ctx := context.Background()
	open := func() *websocket.Conn {
		c, resp, err := websocket.Dial(ctx, "ws://hayami/v1/ws", &websocket.DialOptions{HTTPClient: serviceClient(h.sock)})
		if resp != nil && resp.Body != nil {
			_ = resp.Body.Close()
		}
		require.NoError(t, err)
		var hello contract.WsHello
		readMsg(t, c, contract.WsTypeHello, &hello)
		return c
	}
	for name, msg := range map[string]func(c *websocket.Conn){
		"too large": func(c *websocket.Conn) {
			_ = c.Write(ctx, websocket.MessageText, []byte(`{"type":"subscribe","sections":["`+strings.Repeat("x", api.MaxWsMessage)+`"],"details":[]}`))
		},
		"binary":  func(c *websocket.Conn) { _ = c.Write(ctx, websocket.MessageBinary, []byte(`{}`)) },
		"unknown": func(c *websocket.Conn) { send(t, c, map[string]string{"type": "get"}) },
		"a field the contract lacks": func(c *websocket.Conn) {
			send(t, c, map[string]any{"type": "subscribe", "sections": []string{}, "details": []string{}, "extra": 1})
		},
	} {
		t.Run(name, func(t *testing.T) {
			c := open()
			defer func() { _ = c.CloseNow() }()
			msg(c)
			code := closedWith(t, c)
			assert.Contains(t, []websocket.StatusCode{websocket.StatusMessageTooBig, websocket.StatusUnsupportedData}, code)
		})
	}

	held := make([]*websocket.Conn, 0, api.MaxStreams)
	for range api.MaxStreams {
		held = append(held, open())
	}
	defer func() {
		for _, c := range held {
			_ = c.CloseNow()
		}
	}()
	assert.Equal(t, http.StatusServiceUnavailable, refusedWith(ctx, t, "ws://hayami/v1/ws", &websocket.DialOptions{HTTPClient: serviceClient(h.sock)}),
		"a WebSocket past the streams' limit opened")
	_, status := openStream(t, h, "sections=clock")
	assert.Equal(t, http.StatusServiceUnavailable, status, "a stream past the limit WebSockets share opened")
}

// Review F2. The TCP port has its own limit: connections to it from anyone
// cannot take the socket's places, and one it refuses is closed.
func TestThePortsConnectionsDoNotTakeTheSockets(t *testing.T) {
	h := start(t, sources(fixtures()), options{tcp: true})
	var held []net.Conn
	defer func() {
		for _, c := range held {
			_ = c.Close()
		}
	}()
	// More than the whole socket's limit: were the port's connections counted
	// with the socket's, these alone would take every place.
	var d net.Dialer
	for range api.MaxConns + 4 {
		c, err := d.DialContext(context.Background(), "tcp", h.tcpAddr)
		require.NoError(t, err)
		held = append(held, c)
	}
	status, body, _ := h.get(t, "/v1/health")
	assert.Equal(t, http.StatusOK, status, "the port's connections took the socket's places: %s", body)

	resp, err := http.Get("http://" + h.tcpAddr + "/v1/health") //nolint:noctx // a test's
	if err == nil {
		_ = resp.Body.Close()
		assert.True(t, resp.Close || resp.StatusCode == http.StatusServiceUnavailable, "a refused connection was kept open")
	}
}

// Review F4. A key replaced ends the sessions the old one let in, and lets
// in only the new one's holder.
func TestANewKeyEndsTheOldSessions(t *testing.T) {
	h := start(t, sources(fixtures()), options{tcp: true})
	ctx := context.Background()
	w, err := client.DialTCPWS(ctx, h.tcpAddr, h.key.Token)
	require.NoError(t, err)
	next := tcpkey.Key{Address: h.key.Address, Token: mustToken(t)}
	h.auth.Set(&next)
	waitCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	_, err = w.Next(waitCtx)
	require.Error(t, err, "the old session went on")
	require.NoError(t, waitCtx.Err(), "the old session was not ended; the read waited out its deadline")
	_, err = client.DialTCPWS(ctx, h.tcpAddr, h.key.Token)
	require.Error(t, err, "the old token was let in")
	w2, err := client.DialTCPWS(ctx, h.tcpAddr, next.Token)
	require.NoError(t, err)
	_ = w2.Close()
	h.auth.Set(nil)
	_, err = client.DialTCPWS(ctx, h.tcpAddr, next.Token)
	require.Error(t, err, "a port without a key let someone in")
}
