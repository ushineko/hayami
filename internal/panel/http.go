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
var httpClient = sync.OnceValue(func() *http.Client {
	return &http.Client{Timeout: claude.Timeout}
})
