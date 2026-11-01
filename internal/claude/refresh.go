package claude

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// ExpiryMargin is how long before a token's stated expiry it is treated as
// expired.
//
// A minute, because the expiry is a moment in the future and a request takes
// time to arrive. Refreshing a token that had thirty seconds left costs one
// call; using one that expires in flight costs a failed poll and a confusing
// error.
const ExpiryMargin = time.Minute

// Expired reports whether a token should be refreshed before it is used.
func (c *Credentials) Expired(now time.Time) bool {
	if c == nil || c.ExpiresAt == 0 {
		return false
	}
	return now.Add(ExpiryMargin).After(time.UnixMilli(c.ExpiresAt))
}

// refreshed is what the token endpoint answers with.
type refreshed struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int64  `json:"expires_in"`
}

// Refresh exchanges a refresh token for a new one and writes it back to the
// store it came from.
//
// **Writing back is deliberate.** The token belongs in Claude Code's store and
// every other reader expects to find a usable one there; a hayami that
// refreshed privately would leave the widget refreshing again a minute later,
// which is the stampede this whole arrangement exists to avoid.
//
// It is also the one operation in this program that can damage something
// outside it, so: the file is read whole, the new token is set into the object
// it came from, every other key is put back exactly as it was, and the write
// is a temporary file and a rename. A store that did not parse never reaches
// here, because Read refuses it.
//
// **And the file is read again just before it is written** (spec 077). The
// request takes up to ten seconds, and Claude Code writes the same file: it
// refreshes on expiry too, and keeps other keys there. Writing what was read
// before the request would put back what Claude Code wrote meanwhile.
func Refresh(ctx context.Context, client *http.Client, s Store, c *Credentials) error {
	used := c.RefreshToken
	body, err := json.Marshal(map[string]string{
		"grant_type":    "refresh_token",
		"refresh_token": c.RefreshToken,
		"client_id":     ClientID,
	})
	if err != nil {
		return fmt.Errorf("building the refresh request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, TokenURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("building the refresh request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("asking for a new token: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return ErrNeedsLogin
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("the token endpoint answered %d", resp.StatusCode)
	}

	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return fmt.Errorf("reading the new token: %w", err)
	}
	var got refreshed
	if err := json.Unmarshal(raw, &got); err != nil {
		return fmt.Errorf("reading the new token: %w", err)
	}
	if got.AccessToken == "" {
		// Nothing usable came back. The store is left exactly as it was: a
		// half-written credential file is worse than a stale one.
		return fmt.Errorf("the token endpoint answered without a token")
	}

	c.AccessToken = got.AccessToken
	if got.RefreshToken != "" {
		c.RefreshToken = got.RefreshToken
	}
	if got.ExpiresIn > 0 {
		c.ExpiresAt = time.Now().Add(time.Duration(got.ExpiresIn) * time.Second).UnixMilli()
	}
	return writeBack(s, c, used)
}

/*
writeBack puts refreshed tokens into the store as it is now, not as it was
when the refresh began.

A refresh token in the file other than the one this refresh used means
another program refreshed meanwhile: its tokens are in the file and in use,
so the file is left as it is and c takes them. Otherwise the three token
fields are set into what the file holds now, and every key another program
added in the meantime is kept. A store that is gone, or no longer parses,
is not written: a credential file hayami recreated would be one nobody
asked for.
*/
func writeBack(s Store, c *Credentials, used string) error {
	now, err := Read(s)
	if err != nil {
		return fmt.Errorf("reading the credential store again before writing it: %w", err)
	}
	if now.RefreshToken != used {
		*c = *now
		return nil
	}
	c.raw = now.raw
	return write(s, c)
}

// write puts the credentials back, keeping every key the file had.
//
// The object this package understands is re-encoded from the object that was
// read, not built fresh: a field inside claudeAiOauth that this build has
// never heard of — scopes, rateLimitTier, whatever is added next — survives,
// because it is decoded into a map and only the three keys that changed are
// replaced.
func write(s Store, c *Credentials) error {
	inner := map[string]json.RawMessage{}
	if body, ok := c.raw[oauthKey]; ok {
		if err := json.Unmarshal(body, &inner); err != nil {
			return fmt.Errorf("a credential store's %s does not parse: %w", oauthKey, err)
		}
	}
	for key, value := range map[string]any{
		"accessToken":  c.AccessToken,
		"refreshToken": c.RefreshToken,
		"expiresAt":    c.ExpiresAt,
	} {
		b, err := json.Marshal(value)
		if err != nil {
			return fmt.Errorf("encoding a credential: %w", err)
		}
		inner[key] = b
	}

	body, err := json.Marshal(inner)
	if err != nil {
		return fmt.Errorf("encoding the credentials: %w", err)
	}
	out := map[string]json.RawMessage{}
	for k, v := range c.raw {
		out[k] = v
	}
	out[oauthKey] = body

	encoded, err := json.Marshal(out)
	if err != nil {
		return fmt.Errorf("encoding the credential store: %w", err)
	}

	// A temporary file beside the real one, then a rename: a reader never sees
	// half a credential file, and a crash between the two leaves the original
	// untouched. The mode is the owner's alone, which is what the file already
	// is.
	tmp := fmt.Sprintf("%s.%d.tmp", s.Path, os.Getpid())
	if err := os.MkdirAll(filepath.Dir(s.Path), 0o700); err != nil {
		return fmt.Errorf("making the credential directory: %w", err)
	}
	if err := os.WriteFile(tmp, encoded, 0o600); err != nil {
		return fmt.Errorf("writing the credential store: %w", err)
	}
	if err := os.Rename(tmp, s.Path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("replacing the credential store: %w", err)
	}
	return nil
}
