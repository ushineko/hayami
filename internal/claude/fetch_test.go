package claude_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/hayami/internal/claude"
)

// The fetch and the refresh are tested against a server of this test's own.
// Nothing here talks to Anthropic: a test that needed the real endpoint would
// be a test that needed a real token.
func serve(t *testing.T, h http.HandlerFunc) *http.Client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return &http.Client{
		Transport: rewrite{to: srv.URL, base: srv.Client().Transport},
		Timeout:   5 * time.Second,
	}
}

// rewrite sends every request to the test server, whatever it was addressed
// to, so the real URLs stay in the code where they are read.
type rewrite struct {
	to   string
	base http.RoundTripper
}

func (r rewrite) RoundTrip(req *http.Request) (*http.Response, error) {
	to, err := http.NewRequestWithContext(req.Context(), req.Method, r.to+req.URL.Path, req.Body)
	if err != nil {
		return nil, err
	}
	to.Header = req.Header
	base := r.base
	if base == nil {
		base = http.DefaultTransport
	}
	return base.RoundTrip(to) //nolint:wrapcheck // a transport returns what the base returned
}

func TestTheUsageRequestCarriesTheTokenAndTheBetaHeader(t *testing.T) {
	var gotAuth, gotBeta string
	client := serve(t, func(w http.ResponseWriter, r *http.Request) {
		gotAuth, gotBeta = r.Header.Get("Authorization"), r.Header.Get("anthropic-beta")
		_, _ = w.Write([]byte(`{"five_hour":{"utilization":1.0,"resets_at":""}}`))
	})

	body, err := claude.Usage(t.Context(), client, fakeAccess)

	require.NoError(t, err)
	assert.Equal(t, "Bearer "+fakeAccess, gotAuth)
	assert.Equal(t, claude.BetaHeader, gotBeta)
	assert.True(t, json.Valid(body))
}

// A 429 with a Retry-After is the server saying how long to wait, and the
// cache's gate is what does the waiting.
func TestATooManyRequestsCarriesItsRetryAfter(t *testing.T) {
	client := serve(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Retry-After", "120")
		w.WriteHeader(http.StatusTooManyRequests)
	})

	_, err := claude.Usage(t.Context(), client, fakeAccess)

	var retry *claude.Retryable
	require.ErrorAs(t, err, &retry)
	assert.Equal(t, 2*time.Minute, retry.After)
}

// A 401 is a person having to log in. Asking again in thirty seconds would
// neither fix it nor be polite.
func TestARejectedTokenSaysTheAccountNeedsALogin(t *testing.T) {
	client := serve(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	})

	_, err := claude.Usage(t.Context(), client, fakeAccess)

	assert.ErrorIs(t, err, claude.ErrNeedsLogin)
}

// The refresh writes the new token back, because every other reader expects
// the store to hold a usable one.
func TestARefreshWritesTheNewTokenBack(t *testing.T) {
	root := profiles(t)
	path := storeFile(t, filepath.Join(root, "work"), "enterprise")
	store := claude.Store{Name: "work", Path: path}
	creds, err := claude.Read(store)
	require.NoError(t, err)

	client := serve(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"access_token":"a-new-token","refresh_token":"a-new-refresh","expires_in":3600}`))
	})

	require.NoError(t, claude.Refresh(t.Context(), client, store, creds))

	again, err := claude.Read(store)
	require.NoError(t, err)
	assert.Equal(t, "a-new-token", again.AccessToken)
	assert.Equal(t, "a-new-refresh", again.RefreshToken)
	assert.Greater(t, again.ExpiresAt, time.Now().UnixMilli())
}

// The file is Claude Code's. A write that dropped a key it had would break a
// program somebody needs, and the keys to worry about are the ones this build
// has never heard of.
func TestARefreshKeepsEveryKeyTheFileHad(t *testing.T) {
	root := profiles(t)
	path := storeFile(t, filepath.Join(root, "work"), "enterprise")
	store := claude.Store{Name: "work", Path: path}
	creds, err := claude.Read(store)
	require.NoError(t, err)

	client := serve(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"access_token":"a-new-token","expires_in":3600}`))
	})
	require.NoError(t, claude.Refresh(t.Context(), client, store, creds))

	body, err := os.ReadFile(path)
	require.NoError(t, err)
	var got map[string]any
	require.NoError(t, json.Unmarshal(body, &got))

	assert.Contains(t, got, "anotherTopLevelKey", "a key beside the oauth object was dropped")
	oauth, ok := got["claudeAiOauth"].(map[string]any)
	require.True(t, ok)
	assert.Contains(t, oauth, "scopes")
	assert.Contains(t, oauth, "rateLimitTier")
	assert.Contains(t, oauth, "something_this_build_has_never_heard_of",
		"a key this build does not know was dropped from a file it does not own")
	assert.Equal(t, "enterprise", oauth["subscriptionType"])
	assert.Equal(t, "a-new-token", oauth["accessToken"])
	assert.Equal(t, fakeRefresh, oauth["refreshToken"],
		"a reply with no refresh token must leave the one that worked")
}

// A refresh that fails leaves the store exactly as it was. A stale credential
// file is a problem; a damaged one is somebody's afternoon.
func TestARefreshThatFailsLeavesTheStoreByteForByte(t *testing.T) {
	root := profiles(t)
	path := storeFile(t, filepath.Join(root, "work"), "enterprise")
	store := claude.Store{Name: "work", Path: path}
	creds, err := claude.Read(store)
	require.NoError(t, err)
	before, err := os.ReadFile(path)
	require.NoError(t, err)

	for _, reply := range []http.HandlerFunc{
		func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusInternalServerError) },
		func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusUnauthorized) },
		func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(`{}`)) },
		func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(`not json`)) },
	} {
		err := claude.Refresh(t.Context(), serve(t, reply), store, creds)
		require.Error(t, err)

		after, err := os.ReadFile(path)
		require.NoError(t, err)
		assert.Equal(t, before, after, "a failed refresh changed the credential store")
	}
}

// Nothing this program writes carries a token. Asserted rather than assumed,
// because the way a token escapes is always somewhere nobody looked.
func TestNoTokenReachesAnErrorOrTheDisk(t *testing.T) {
	root := profiles(t)
	path := storeFile(t, filepath.Join(root, "work"), "enterprise")
	store := claude.Store{Name: "work", Path: path}
	creds, err := claude.Read(store)
	require.NoError(t, err)

	client := serve(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	errs := []error{}
	_, e := claude.Usage(t.Context(), client, creds.AccessToken)
	errs = append(errs, e)
	errs = append(errs, claude.Refresh(t.Context(), client, store, creds))

	for _, e := range errs {
		require.Error(t, e)
		assert.NotContains(t, e.Error(), fakeAccess)
		assert.NotContains(t, e.Error(), fakeRefresh)
	}
}

func TestATokenPastItsExpiryIsRefreshedBeforeItIsUsed(t *testing.T) {
	now := time.Now()
	expired := &claude.Credentials{ExpiresAt: now.Add(30 * time.Second).UnixMilli()}
	good := &claude.Credentials{ExpiresAt: now.Add(time.Hour).UnixMilli()}
	unknown := &claude.Credentials{}

	assert.True(t, expired.Expired(now), "a token expiring inside the margin is expired")
	assert.False(t, good.Expired(now))
	assert.False(t, unknown.Expired(now), "a store that states no expiry is not guessed at")
}
