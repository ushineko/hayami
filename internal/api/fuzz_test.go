package api_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ushineko/hayami/internal/api"
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
		if !strings.HasPrefix(target, "/") || strings.Contains(target, "/v1/events") {
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
