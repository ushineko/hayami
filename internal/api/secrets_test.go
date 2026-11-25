package api_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/hayami/internal/api/contract"
	"github.com/ushineko/hayami/internal/claude"
	"github.com/ushineko/hayami/internal/config"
	"github.com/ushineko/hayami/internal/panel"
	"github.com/ushineko/hayami/internal/testenv"
)

// Invented credentials, each a string nothing else would produce.
const (
	fakeAccess  = "sk-ant-oat01-HAYAMI-TEST-ACCESS-TOKEN-0f3c"
	fakeRefresh = "sk-ant-ort01-HAYAMI-TEST-REFRESH-TOKEN-9a1d"
	fakeForge   = "ghp_HAYAMITESTFORGEKEY0123456789abcdef"
	fakeAPIKey  = "sk-HAYAMI-TEST-OPENAI-KEY-77e2"
)

/*
R24. Nothing secret leaves. The real sources, over a home of this test's
with a credential file and keys in the environment, served whole: every
section, every detail, and the details below them. No response carries a
token, a key, the credential file's name or the home it is in.

No request leaves the machine: the proxy is an address nothing answers on,
so a fetch the usage source tries fails at once.
*/
func TestNothingSecretLeaves(t *testing.T) {
	home := t.TempDir()
	testenv.Home(t, home)
	testenv.Cache(t, t.TempDir())
	testenv.NoDevices(t)
	t.Setenv("PATH", t.TempDir())
	t.Setenv("DBUS_SESSION_BUS_ADDRESS", "unix:path=/nonexistent/hayami-test")
	t.Setenv("DBUS_SYSTEM_BUS_ADDRESS", "unix:path=/nonexistent/hayami-test")
	for _, v := range []string{"HTTPS_PROXY", "HTTP_PROXY", "https_proxy", "http_proxy"} {
		t.Setenv(v, "http://127.0.0.1:9")
	}
	t.Setenv("NO_PROXY", "")
	t.Setenv("GITHUB_TOKEN", fakeForge)
	t.Setenv("GH_TOKEN", fakeForge)
	t.Setenv("OPENAI_API_KEY", fakeAPIKey)

	dir := filepath.Join(home, ".claude")
	require.NoError(t, os.MkdirAll(dir, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(dir, claude.CredentialsFile), []byte(`{"claudeAiOauth": {
		"accessToken": "`+fakeAccess+`", "refreshToken": "`+fakeRefresh+`",
		"expiresAt": 4102444800000, "scopes": ["user:inference"], "subscriptionType": "max"}}`), 0o600))

	srcs := panel.Sources(panel.Keys(), panel.Env{Settings: config.Config{}})
	h := start(t, srcs, options{})

	var bodies []string
	get := func(path string) []byte {
		_, body, _ := h.get(t, path)
		bodies = append(bodies, string(body))
		return body
	}
	_, index, _ := h.get(t, "/v1")
	var idx contract.Index
	require.NoError(t, json.Unmarshal(index, &idx))
	get("/v1/readings")
	for _, key := range idx.Sections {
		var s contract.Section
		require.NoError(t, json.Unmarshal(get("/v1/sections/"+key), &s))
		for _, ref := range s.Details {
			walk(t, get, key, ref.ID, 2)
		}
	}
	require.NotEmpty(t, bodies)
	for _, body := range bodies {
		for _, secret := range []string{fakeAccess, fakeRefresh, fakeForge, fakeAPIKey, claude.CredentialsFile, home} {
			assert.NotContains(t, body, secret)
		}
	}
}

// walk reads a detail and, depth steps down, a few of its children.
func walk(t *testing.T, get func(string) []byte, key, id string, depth int) {
	var d contract.Detail
	if json.Unmarshal(get("/v1/sections/"+key+"/details/"+escape(id)), &d) != nil || depth == 0 {
		return
	}
	for i, child := range d.Children {
		if i == 4 || strings.Contains(child, "\x00") {
			break
		}
		walk(t, get, key, child, depth-1)
	}
}
