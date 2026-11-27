package api_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/ushineko/hayami/internal/api"
	"github.com/ushineko/hayami/internal/api/contract"
	"github.com/ushineko/hayami/internal/config"
)

/*
FuzzRequest is the router and the ID decoding (threat model B4): any path
and query a client sends is answered with a status the contract documents
and the error shape, never a panic, and never a 500.
*/
func FuzzRequest(f *testing.F) {
	for _, seed := range []string{
		"/v1", "/v1/sections/cooler/details/cpu%2Fa%2Fb", "/v1/sections/cooler/details/cpu/" + longProcess + "/41",
		"/v1/readings?sections=,,clock,,nope", "/v1/sections/%00", "/v1/sections/usage/details/accounts/github/2026-11-24/x",
		"/v1/sections/bandwidth/details/peers/eth0/x/@::1", "/v1/../etc/passwd", "/v1/sections/cooler/details/" + strings.Repeat("a/", 500),
	} {
		f.Add(seed)
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	ctx, cancel := context.WithCancel(context.Background())
	f.Cleanup(cancel)
	hub := api.NewHub(ctx, sources(fixtures()), config.Config{}, log)
	hub.FirstWait = 50 * time.Millisecond
	handler := api.NewServer(hub, "fuzz", log).Handler()

	f.Fuzz(func(t *testing.T, target string) {
		if !strings.HasPrefix(target, "/") || strings.Contains(target, "/v1/events") || strings.Contains(target, "/v1/ws") {
			return
		}
		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://hayami"+target, http.NoBody)
		if err != nil {
			return
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		switch w.Code {
		case http.StatusOK, http.StatusNotFound, http.StatusBadRequest, http.StatusMovedPermanently,
			http.StatusTemporaryRedirect, http.StatusPermanentRedirect:
		default:
			t.Fatalf("%q answered %d: %s", target, w.Code, w.Body.String())
		}
	})
}

/*
FuzzWsMessage is what a client sends on the WebSocket (spec 116, B4): on the
socket, any message is answered with messages the contract has or a close
with a code it documents, never a panic, and a subscription never leases past
its bounds; on the TCP port, anything sent for the challenge but a proof made
with the token is closed with 4401 and sent no reading (spec 115).
*/
func FuzzWsMessage(f *testing.F) {
	for _, seed := range []string{
		`{"type":"subscribe","sections":["cooler"],"details":[]}`,
		`{"type":"subscribe","sections":[],"details":["cooler/cpu","cooler/cpu","cooler/gpu"]}`,
		`{"type":"subscribe","sections":["nope"],"details":["x/y/z"]}`,
		`{"type":"auth","nonce":"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA","proof":"x"}`,
		`{"type":"subscribe"}`, `{"type":1}`, `[]`, `null`, `{"type":"hello"}`,
	} {
		f.Add(seed)
	}
	h := start(f, sources(fixtures()), options{tcp: true})
	was := api.AuthWait
	api.AuthWait = 200 * time.Millisecond
	f.Cleanup(func() { api.AuthWait = was })
	allowed := map[websocket.StatusCode]bool{
		-1: true, websocket.StatusNormalClosure: true, websocket.StatusUnsupportedData: true,
		websocket.StatusMessageTooBig: true, websocket.StatusPolicyViolation: true,
	}

	f.Fuzz(func(t *testing.T, msg string) {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()

		c, resp, err := websocket.Dial(ctx, "ws://hayami/v1/ws", &websocket.DialOptions{HTTPClient: serviceClient(h.sock)})
		if resp != nil && resp.Body != nil {
			_ = resp.Body.Close()
		}
		if err != nil {
			return // over the streams' limit while earlier inputs close
		}
		_, _, _ = c.Read(ctx) // hello
		_ = c.Write(ctx, websocket.MessageText, []byte(msg))
		rctx, rcancel := context.WithTimeout(ctx, 300*time.Millisecond)
		for {
			_, data, err := c.Read(rctx)
			if err != nil {
				if code := websocket.CloseStatus(err); rctx.Err() == nil && !allowed[code] {
					t.Fatalf("%q closed with %d", msg, code)
				}
				break
			}
			var m contract.WsMessage
			if json.Unmarshal(data, &m) != nil || (m.Type != contract.WsTypeSection && m.Type != contract.WsTypeDetail && m.Type != contract.WsTypeError) {
				t.Fatalf("%q was answered %s", msg, data)
			}
		}
		rcancel()
		_ = c.CloseNow()
		if _, d := h.hub.Active(); d > api.MaxDetails {
			t.Fatalf("%q leased %d details", msg, d)
		}

		tc, resp, err := websocket.Dial(ctx, "ws://"+h.tcpAddr+"/v1/ws", nil)
		if resp != nil && resp.Body != nil {
			_ = resp.Body.Close()
		}
		if err != nil {
			return
		}
		defer func() { _ = tc.CloseNow() }()
		_, _, _ = tc.Read(ctx) // challenge
		_ = tc.Write(ctx, websocket.MessageText, []byte(msg))
		for {
			_, data, err := tc.Read(ctx)
			if err != nil {
				if code := websocket.CloseStatus(err); code != api.StatusUnauthorized && code != websocket.StatusMessageTooBig {
					t.Fatalf("%q on the port closed with %d, not 4401", msg, code)
				}
				return
			}
			t.Fatalf("%q on the port was answered %s", msg, data)
		}
	})
}
