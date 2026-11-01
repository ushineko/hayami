package panel

import (
	"net/http"
	"sync"

	"github.com/ushineko/hayami/internal/claude"
)

// httpClient is the one client this program makes requests with.
//
// One rather than one per call, so connections are reused across polls: a
// panel that opened a new connection every two minutes would be paying for a
// handshake it already had. The timeout is the package's, not the transport's
// default, because a request that outlives its poll piles up behind itself.
//
// It follows no redirect. Neither endpoint it calls redirects, and following
// one would carry what it sent to wherever the redirect pointed: on a 307 the
// refresh request's body, refresh token and all, and on a redirect to the
// same host over plain http the usage request's Authorization header
// (spec 076). A redirect is a response like any other, and an error.
var httpClient = sync.OnceValue(func() *http.Client {
	return &http.Client{Timeout: claude.Timeout, CheckRedirect: refuseRedirects}
})

// refuseRedirects hands a redirect back as the response.
func refuseRedirects(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
