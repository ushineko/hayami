package claude

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"
)

// The endpoints and the client this program presents itself with.
//
// Transcribed from the program that uses them today. They belong to somebody
// else, and a change to them is something this repository finds out about the
// way any client does.
const (
	UsageURL = "https://api.anthropic.com/api/oauth/usage"

	// TokenURL is where a refresh is asked for. It is an address, not a
	// secret: the client id below is Claude Code's own and is published in
	// every copy of it.
	TokenURL = "https://console.anthropic.com/api/oauth/token" //nolint:gosec // an endpoint, not a credential

	// ClientID is Claude Code's own OAuth client. The refresh is the one
	// Claude Code would make; presenting a different client would be asking
	// for a token this store is not for.
	ClientID = "9d1c250a-e61b-44d9-88ed-5944d1962f5e"

	// BetaHeader is what the usage endpoint requires.
	BetaHeader = "oauth-2025-04-20"
)

// Timeout is how long a request may take. Short: a panel polls on a timer and
// a request that outlives its interval would pile up behind itself.
const Timeout = 10 * time.Second

// ErrNeedsLogin is a store whose tokens the server has rejected outright. It
// is not retried on the next poll: a 401 means a person has to log in, and
// asking again every thirty seconds would neither fix it nor be polite.
var ErrNeedsLogin = errors.New("this account needs a login")

// Retryable is a failure the server asked us to wait out, carrying how long.
type Retryable struct {
	After time.Duration
	Err   error
}

func (r *Retryable) Error() string { return r.Err.Error() }
func (r *Retryable) Unwrap() error { return r.Err }

// Usage fetches one account's usage with the token it is given.
//
// The token goes into one header on one request and nowhere else. It is not
// logged on success or on failure: an error here carries a status code and a
// message, and a token in a log file is a token in a backup.
func Usage(ctx context.Context, client *http.Client, accessToken string) (json.RawMessage, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, UsageURL, nil)
	if err != nil {
		return nil, fmt.Errorf("building the usage request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("anthropic-beta", BetaHeader)
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("asking for the usage: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	switch {
	case resp.StatusCode == http.StatusUnauthorized, resp.StatusCode == http.StatusForbidden:
		return nil, ErrNeedsLogin
	case resp.StatusCode == http.StatusTooManyRequests:
		return nil, &Retryable{
			After: retryAfter(resp.Header.Get("Retry-After")),
			Err:   fmt.Errorf("the usage endpoint answered %d", resp.StatusCode),
		}
	case resp.StatusCode != http.StatusOK:
		return nil, fmt.Errorf("the usage endpoint answered %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return nil, fmt.Errorf("reading the usage: %w", err)
	}
	if !json.Valid(body) {
		return nil, errors.New("the usage endpoint answered with something that is not JSON")
	}
	return body, nil
}

// maxBody is a ceiling on a reply this program does not control. A cache entry
// is a few kilobytes; a megabyte is a generous ceiling and a bounded one.
const maxBody = 1 << 20

// retryAfter reads the header, in seconds. A header that is a date rather than
// a number, or missing, yields nothing and the caller's own interval applies.
func retryAfter(v string) time.Duration {
	if v == "" {
		return 0
	}
	if n, err := strconv.Atoi(v); err == nil && n > 0 {
		return time.Duration(n) * time.Second
	}
	if t, err := http.ParseTime(v); err == nil {
		if d := time.Until(t); d > 0 {
			return d
		}
	}
	return 0
}
