package claude_test

import (
	"net/http"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/hayami/internal/claude"
)

// Spec 089. A refresh token that is spent, expired or revoked is answered with
// a 400 and invalid_grant (RFC 6749 §5.2), and that is a person having to sign
// in, as a 401 is. Both shapes of the error are read: OAuth's string and the
// object Anthropic's APIs answer with.
func TestAnInvalidGrantSaysTheAccountNeedsALogin(t *testing.T) {
	for _, body := range []string{
		`{"error":"invalid_grant","error_description":"refresh token not found"}`,
		`{"type":"error","error":{"type":"invalid_grant","message":"refresh token not found"}}`,
	} {
		t.Cleanup(claude.ForgetRefusals)
		creds, store := expiredStore(t)
		client := serve(t, func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(body))
		})

		err := claude.Refresh(t.Context(), client, store, creds)
		assert.ErrorIs(t, err, claude.ErrNeedsLogin, "body: %s", body)
		claude.ForgetRefusals()
	}
}

// Any other 400 is our request at fault, not the person's login: it is a plain
// error naming the code, and the rest of the body stays out of it.
func TestAnotherBadRequestNamesItsCodeOnly(t *testing.T) {
	t.Cleanup(claude.ForgetRefusals)
	creds, store := expiredStore(t)
	client := serve(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid_client","error_description":"a long story the panel has no room for"}`))
	})

	err := claude.Refresh(t.Context(), client, store, creds)
	require.Error(t, err)
	assert.NotErrorIs(t, err, claude.ErrNeedsLogin)
	assert.Contains(t, err.Error(), "400 (invalid_client)")
	assert.NotContains(t, err.Error(), "long story")

	// A code that is not one, a line of control characters, is left out.
	client = serve(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"bad\u001b[2Jcode"}`))
	})
	err = claude.Refresh(t.Context(), client, store, creds)
	require.Error(t, err)
	assert.Equal(t, "the token endpoint answered 400", err.Error())
}

// A token the endpoint refused is not sent again: one request, then none, for
// as long as the store holds it. A sign-in writes a new refresh token, and that
// one is asked about.
func TestARefusedRefreshTokenIsNotSentAgain(t *testing.T) {
	t.Cleanup(claude.ForgetRefusals)
	_, store := expiredStore(t)
	var asked atomic.Int32
	client := serve(t, func(w http.ResponseWriter, _ *http.Request) {
		asked.Add(1)
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid_grant"}`))
	})

	for range 3 {
		again, err := claude.Read(store)
		require.NoError(t, err)
		assert.ErrorIs(t, claude.Refresh(t.Context(), client, store, again), claude.ErrNeedsLogin)
	}
	assert.Equal(t, int32(1), asked.Load(), "a refused refresh token was sent again")

	// The person signs in.
	rewriteStore(t, store.Path, func(_, oauth map[string]any) { oauth["refreshToken"] = "a-new-sign-in" })
	signedIn, err := claude.Read(store)
	require.NoError(t, err)
	_ = claude.Refresh(t.Context(), client, store, signedIn)
	assert.Equal(t, int32(2), asked.Load(), "a new sign-in was not asked about")
}

// A 429 from the token endpoint carries its Retry-After, as one from the usage
// endpoint does, and is not a login problem.
func TestATooManyRequestsFromTheTokenEndpointCarriesItsRetryAfter(t *testing.T) {
	t.Cleanup(claude.ForgetRefusals)
	creds, store := expiredStore(t)
	client := serve(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Retry-After", "600")
		w.WriteHeader(http.StatusTooManyRequests)
	})

	err := claude.Refresh(t.Context(), client, store, creds)
	var retry *claude.Retryable
	require.ErrorAs(t, err, &retry)
	assert.Equal(t, int64(600), int64(retry.After.Seconds()))
	assert.NotErrorIs(t, err, claude.ErrNeedsLogin)
}

// expiredStore is a store of this test's own and its credentials.
func expiredStore(t *testing.T) (*claude.Credentials, claude.Store) {
	t.Helper()
	root := profiles(t)
	path := storeFile(t, filepath.Join(root, "work"), "max")
	store := claude.Store{Name: "work", Path: path}
	creds, err := claude.Read(store)
	require.NoError(t, err)
	return creds, store
}
