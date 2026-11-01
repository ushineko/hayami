package panel

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Spec 076. The client follows no redirect: a 307 from the token endpoint
// would otherwise send the refresh request's body, refresh token and all, to
// wherever it pointed. The redirect comes back as the response.
func TestTheClientFollowsNoRedirect(t *testing.T) {
	var reached atomic.Int32
	elsewhere := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { reached.Add(1) }))
	t.Cleanup(elsewhere.Close)
	endpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, elsewhere.URL+"/token", http.StatusTemporaryRedirect)
	}))
	t.Cleanup(endpoint.Close)

	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, endpoint.URL+"/token",
		strings.NewReader(`{"refresh_token":"invented-refresh-token"}`))
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer invented-access-token")
	resp, err := httpClient().Do(req)
	require.NoError(t, err)
	_ = resp.Body.Close()

	assert.Equal(t, http.StatusTemporaryRedirect, resp.StatusCode)
	assert.Zero(t, reached.Load(), "the redirect's target was sent the request")
}
