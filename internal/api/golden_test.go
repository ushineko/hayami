package api_test

import (
	"bytes"
	"encoding/json"
	"flag"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var update = flag.Bool("update", false, "write the golden files from what the server answers")

// escape is a drill's ID as a path: each step percent-encoded.
func escape(id string) string {
	var out []string
	for _, step := range splitSteps(id) {
		out = append(out, url.PathEscape(step))
	}
	return joinSteps(out)
}

func splitSteps(id string) []string {
	var out []string
	start := 0
	for i := range len(id) {
		if id[i] == '/' {
			out = append(out, id[start:i])
			start = i + 1
		}
	}
	return append(out, id[start:])
}

func joinSteps(steps []string) string {
	out := ""
	for i, s := range steps {
		if i > 0 {
			out += "/"
		}
		out += s
	}
	return out
}

/*
R28. Every endpoint over the fixtures: each response keeps its schema, and
its golden file pins its values. A change the schema forbids fails here; a
change it allows and the golden file does not is a change to review, made
with -update.
*/
func TestEveryEndpointKeepsTheContract(t *testing.T) {
	h := start(t, sources(fixtures()), options{fixedClock: true})
	cases := []struct {
		name, path, schema string
		status             int
	}{
		{"index", "/v1", "Index", 200},
		{"health", "/v1/health", "Health", 200},
		{"sections", "/v1/sections", "SectionList", 200},
		{"section-clock", "/v1/sections/clock", "Section", 200},
		{"section-bandwidth", "/v1/sections/bandwidth", "Section", 200},
		{"section-usage", "/v1/sections/usage", "Section", 200},
		{"section-cooler", "/v1/sections/cooler", "Section", 200},
		{"section-peripherals", "/v1/sections/peripherals", "Section", 200},
		{"readings", "/v1/readings?sections=clock,peripherals", "Readings", 200},
		{"detail-cpu", "/v1/sections/cooler/details/cpu", "Detail", 200},
		{"detail-cpu-name", "/v1/sections/cooler/details/" + escape("cpu/"+longProcess), "Detail", 200},
		{"detail-cpu-process", "/v1/sections/cooler/details/" + escape("cpu/"+longProcess+"/41"), "Detail", 200},
		{"detail-peers", "/v1/sections/bandwidth/details/peers", "Detail", 200},
		{"detail-peers-name", "/v1/sections/bandwidth/details/" + escape("peers/eth0/"+longProcess), "Detail", 200},
		{"detail-vpn", "/v1/sections/bandwidth/details/vpn", "Detail", 200},
		{"detail-overview", "/v1/sections/usage/details/accounts", "Detail", 200},
		{"detail-account", "/v1/sections/usage/details/" + escape(longAccount), "Detail", 200},
		{"detail-day", "/v1/sections/usage/details/accounts/github/2026-11-24", "Detail", 200},
		{"detail-change", "/v1/sections/usage/details/" + escape("accounts/github/2026-11-24/pr:someone/project#12"), "Detail", 200},
		{"error-section", "/v1/sections/nope", "Error", 404},
		{"error-detail", "/v1/sections/cooler/details/nope", "Error", 404},
		{"error-path", "/v2", "Error", 404},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			status, body, header := h.get(t, c.path)
			assert.Equal(t, c.status, status, "%s", body)
			assert.Equal(t, "application/json; charset=utf-8", header.Get("Content-Type"))
			conforms(t, c.schema, body)
			golden(t, c.name, body)
		})
	}
}

// golden compares body, indented, with its file, or writes it with -update.
func golden(t *testing.T, name string, body []byte) {
	t.Helper()
	var pretty bytes.Buffer
	require.NoError(t, json.Indent(&pretty, body, "", "  "))
	path := filepath.Join("testdata", "golden", name+".json")
	if *update {
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, pretty.Bytes(), 0o600))
		return
	}
	want, err := os.ReadFile(path)
	require.NoError(t, err, "no golden file; run with -update")
	assert.JSONEq(t, string(want), pretty.String(), "%s differs from its golden file", name)
}

// The method and header refusals keep the error shape too (R23).
func TestARefusalKeepsTheErrorShape(t *testing.T) {
	h := start(t, sources(fixtures()), options{})
	for _, c := range []struct {
		method string
		header string
		body   string
		status int
	}{
		{http.MethodPost, "", "", http.StatusMethodNotAllowed},
		{http.MethodGet, "", "x", http.StatusBadRequest},
		{http.MethodGet, "http://example.com", "", http.StatusForbidden},
	} {
		req, err := http.NewRequestWithContext(t.Context(), c.method, "http://hayami/v1/health", bytes.NewBufferString(c.body))
		require.NoError(t, err)
		if c.body == "" {
			req.Body, req.ContentLength = http.NoBody, 0
		}
		if c.header != "" {
			req.Header.Set("Origin", c.header)
		}
		resp, err := h.client.Do(req)
		require.NoError(t, err)
		var buf bytes.Buffer
		_, _ = buf.ReadFrom(resp.Body)
		_ = resp.Body.Close()
		assert.Equal(t, c.status, resp.StatusCode, "%s %q", c.method, c.header)
		conforms(t, "Error", buf.Bytes())
		if c.method != http.MethodGet {
			assert.Equal(t, http.MethodGet, resp.Header.Get("Allow"))
		}
	}
}
