package claude

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestEncodePreservesUnknownKeys locks the data-safety rule the whole package
// rests on: a rewrite changes the three token fields and keeps every other key,
// inside claudeAiOauth and beside it. No I/O, so it runs everywhere.
func TestEncodePreservesUnknownKeys(t *testing.T) {
	const body = `{
      "claudeAiOauth": {
        "accessToken": "old-access",
        "refreshToken": "old-refresh",
        "expiresAt": 1,
        "scopes": ["user:inference"],
        "subscriptionType": "max",
        "deep": {"a": [1, 2]}
      },
      "anotherTopLevelKey": {"kept": true}
    }`

	c, err := parseCredentials([]byte(body))
	require.NoError(t, err)

	c.AccessToken = "new-access"
	c.ExpiresAt = 999
	enc, err := encode(c)
	require.NoError(t, err)

	var got map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(enc, &got))
	assert.Contains(t, string(got["anotherTopLevelKey"]), "kept",
		"a top-level key this build does not know is kept")

	var inner map[string]any
	require.NoError(t, json.Unmarshal(got["claudeAiOauth"], &inner))
	assert.Equal(t, "new-access", inner["accessToken"])
	assert.EqualValues(t, 999, inner["expiresAt"])
	assert.Equal(t, "old-refresh", inner["refreshToken"], "an untouched token is unchanged")
	assert.Equal(t, "max", inner["subscriptionType"])
	assert.Contains(t, inner, "scopes", "an unknown field inside claudeAiOauth is kept")
	assert.Contains(t, inner, "deep")
}

// keychainE2E skips unless a real macOS login Keychain is available and the test
// is opted into, so CI (where the keychain is locked or absent) does not run it.
// It talks to the real `security`; run it on macOS with HAYAMI_KEYCHAIN_E2E=1.
func keychainE2E(t *testing.T) {
	t.Helper()
	if runtime.GOOS != "darwin" || os.Getenv("HAYAMI_KEYCHAIN_E2E") == "" {
		t.Skip("set HAYAMI_KEYCHAIN_E2E=1 on macOS to run the real-Keychain test")
	}
}

func TestKeychainRoundTrip(t *testing.T) {
	keychainE2E(t)

	service := fmt.Sprintf("hayami-test-%d-%d", os.Getpid(), time.Now().UnixNano())
	account := "hayami-test"

	// Seed a throwaway item allowing any application (-A) so reading it back in
	// this test does not prompt. It is deleted at the end.
	seed := `{"claudeAiOauth":{"accessToken":"a1","refreshToken":"r1","expiresAt":1,"subscriptionType":"max","keep_me":true},"top":{"kept":true}}`
	require.NoError(t, exec.CommandContext(t.Context(), securityBin,
		"add-generic-password", "-U", "-A", "-s", service, "-a", account, "-w", seed).Run())
	t.Cleanup(func() {
		_ = exec.CommandContext(context.Background(), securityBin,
			"delete-generic-password", "-s", service, "-a", account).Run()
	})

	s := Store{Name: "max", Keychain: true, Service: service, Account: account}

	creds, err := Read(s)
	require.NoError(t, err)
	assert.Equal(t, "a1", creds.AccessToken)
	assert.Equal(t, "r1", creds.RefreshToken)
	assert.Equal(t, "max", creds.Plan)

	// Update through the package's own write and read it back.
	creds.AccessToken = "a2"
	require.NoError(t, write(s, creds))

	back, err := Read(s)
	require.NoError(t, err)
	assert.Equal(t, "a2", back.AccessToken, "the written token is read back")
	assert.Equal(t, "r1", back.RefreshToken)

	raw, err := keychainRead(service, account)
	require.NoError(t, err)
	assert.Contains(t, string(raw), "keep_me", "an unknown field survives the rewrite")
	assert.Contains(t, string(raw), `"top"`)
}

func TestKeychainNotFoundIsErrNotExist(t *testing.T) {
	keychainE2E(t)

	service := fmt.Sprintf("hayami-absent-%d", time.Now().UnixNano())
	_, err := Read(Store{Keychain: true, Service: service, Account: "none"})
	require.ErrorIs(t, err, fs.ErrNotExist,
		"a missing item reads as fs.ErrNotExist, so writeBack's removal branch fires")
}
