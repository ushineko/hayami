package forge

import (
	"errors"
	"io"
	"net/http"
	"time"
)

// Timeout bounds one request to a forge, connection to last byte.
const Timeout = 15 * time.Second

// MaxBody is the most of one response a reader takes. A fortnight's
// contributions are tens of kilobytes; a page of events about the same.
const MaxBody = 4 << 20

// ErrNotHTTPS is a request that was not going over HTTPS: a token is never
// sent in the clear (spec 102 R11).
var ErrNotHTTPS = errors.New("forge: refusing a request that is not https")

// ErrTooLarge is a response longer than MaxBody.
var ErrTooLarge = errors.New("forge: response too large")

/*
NewClient is the HTTP client every forge request goes through (spec 102 R11):
HTTPS only, no redirect followed, each response read through MaxBody, the
whole request bounded by Timeout.

A redirect is not followed because the token goes with the request: a forge
that answers somewhere else gets a refusal, not the key. base is the
transport underneath, nil for the default; a test gives its own.
*/
func NewClient(base http.RoundTripper) *http.Client {
	if base == nil {
		base = http.DefaultTransport
	}
	return &http.Client{
		Timeout:   Timeout,
		Transport: guarded{base: base},
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

type guarded struct{ base http.RoundTripper }

func (g guarded) RoundTrip(r *http.Request) (*http.Response, error) {
	if r.URL.Scheme != "https" {
		return nil, ErrNotHTTPS
	}
	resp, err := g.base.RoundTrip(r)
	if err != nil {
		return nil, err //nolint:wrapcheck // a transport passes its base's failure on as it was
	}
	resp.Body = &limited{rc: resp.Body, left: MaxBody}
	return resp, nil
}

// limited reads at most left bytes, and fails rather than ending quietly
// when there is more: a cut JSON document would decode as a different one.
type limited struct {
	rc   io.ReadCloser
	left int64
}

func (l *limited) Read(p []byte) (int, error) {
	if l.left <= 0 {
		var one [1]byte
		if n, _ := l.rc.Read(one[:]); n > 0 {
			return 0, ErrTooLarge
		}
		return 0, io.EOF
	}
	if int64(len(p)) > l.left {
		p = p[:l.left]
	}
	n, err := l.rc.Read(p)
	l.left -= int64(n)
	return n, err //nolint:wrapcheck // a reader's io.EOF must stay io.EOF
}

func (l *limited) Close() error { return l.rc.Close() } //nolint:wrapcheck // the body's own close
