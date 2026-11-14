package panel

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/ushineko/hayami/internal/claude"
	"github.com/ushineko/hayami/internal/codex"
	"github.com/ushineko/hayami/internal/usage"
)

// UsageTTL is how long a reading is good for: the gate this program sets when
// a fetch succeeds.
//
// Two minutes, which is the monitor's own default. It is not this program's
// business alone — the gate is written into a file three programs read, and
// a hayami that set a shorter one would be deciding for the other two.
const UsageTTL = 2 * time.Minute

// fetchClaude asks for one account's usage, refreshing its token first when it
// has expired.
//
// The refresh is here rather than inside the fetch because it writes to a
// credential store, and the one call that can damage something outside this
// program should be visible at the place it is decided.
func fetchClaude(ctx context.Context, store claude.Store) usage.Fetch {
	fetch := func() (json.RawMessage, time.Duration, error) {
		creds, err := claude.Read(store)
		if err != nil {
			return nil, 0, err
		}

		if creds.Expired(time.Now()) {
			if err := claude.Refresh(ctx, httpClient(), store, creds); err != nil {
				return nil, waited(err), err
			}
		}

		body, err := claude.Usage(ctx, httpClient(), creds.AccessToken)
		if err == nil {
			return body, 0, nil
		}

		// A server that said how long to wait is obeyed; the cache's gate does
		// the waiting for every reader, not only this one.
		if after := waited(err); after > 0 {
			return nil, after, err
		}

		// A rejected token is a person having to log in. It is still a failure
		// and still moves the gate, because asking again in thirty seconds
		// would neither fix it nor be polite.
		return nil, 0, err
	}
	return func() (json.RawMessage, time.Duration, error) {
		body, wait, err := fetch()
		if errors.Is(err, claude.ErrNeedsLogin) {
			err = loginRefused{err}
		}
		return body, wait, err
	}
}

// loginRefused is Claude's needs-a-login as the shared cache records it, so
// every viewer says so and not only the one that fetched (spec 089). Its
// text is the error's own.
type loginRefused struct{ error }

func (e loginRefused) Unwrap() error      { return e.error }
func (loginRefused) Is(target error) bool { return target == usage.ErrLoginRefused }

// waited is how long a server asked to be left alone, from either endpoint;
// nothing when it did not say.
func waited(err error) time.Duration {
	var retry *claude.Retryable
	if errors.As(err, &retry) {
		return retry.After
	}
	return 0
}

// fetchCodex asks the app-server, and reshapes the reply into what the cache
// holds.
func fetchCodex(ctx context.Context) usage.Fetch {
	return func() (json.RawMessage, time.Duration, error) {
		raw, err := codex.Usage(ctx)
		if err != nil {
			return nil, 0, err
		}
		body, err := codex.Normalise(raw)
		if err != nil {
			return nil, 0, err
		}
		return body, 0, nil
	}
}
