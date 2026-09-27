package usage

import (
	"encoding/json"
	"time"
)

// Result is what a caller gets back: the payload, when it was obtained, and
// whether this call went upstream.
type Result struct {
	// Data is the last successful payload, which may be one another process
	// fetched. It is nil when nothing has ever succeeded.
	Data json.RawMessage

	// FetchedAt is when Data was obtained, so every pane shows the same age
	// for the same reading rather than the age of its own last look.
	FetchedAt time.Time

	// Fresh reports whether Data came from a request this call made.
	Fresh bool
}

// Fetch is a request upstream. It returns the payload, or an error and a
// retry-after the server asked for.
//
// It is injected rather than imported because this package is the protocol:
// the provider that knows how to ask is spec 003's, and this one must be
// testable without a network.
type Fetch func() (data json.RawMessage, retryAfter time.Duration, err error)

// Cached returns the usage payload, going upstream at most once per gate.
//
// The order is the Python's, and each step is there for a reason a running
// system taught somebody:
//
//  1. Read the cache. Inside the gate, return it and make no request. This is
//     what makes five panes cost one API call.
//  2. Take the lock. If another process has it, that process is fetching right
//     now: read what is there and draw it rather than queueing behind them.
//  3. Read again under the lock. The holder may have finished between step one
//     and step two, and fetching on top of their fresh answer is the stampede
//     the lock exists to prevent.
//  4. Fetch. On success, write the payload and set the gate ttl ahead.
//  5. On failure, keep the last good payload and its timestamp, and push the
//     gate by the longer of the ttl and any retry-after. A failed fetch never
//     clobbers good data: an outage should not make every pane flap to an
//     error.
//
// force skips the gate. It still takes the lock and still writes, so a manual
// refresh never stampedes and every other reader gets the benefit.
func Cached(now time.Time, ttl time.Duration, account, provider string, force bool, fetch Fetch) (Result, error) {
	cached, err := Read(account, provider)
	if err != nil {
		return Result{}, err
	}
	if !force && cached != nil && !cached.Open(now) {
		return served(*cached), nil
	}

	var out Result
	err = withLock(account, provider, func(locked bool) error {
		if !locked {
			// Someone else is fetching. What they have is better than what we
			// would wait for.
			again, err := Read(account, provider)
			if err != nil {
				return err
			}
			if again != nil {
				out = served(*again)
			}
			return nil
		}

		again, err := Read(account, provider)
		if err != nil {
			return err
		}
		if !force && again != nil && !again.Open(now) {
			out = served(*again)
			return nil
		}
		if again != nil {
			cached = again
		}

		data, retry, ferr := fetch()
		if ferr == nil {
			at := seconds(now)
			entry := Entry{
				NextAttemptAt: at + ttl.Seconds(),
				FetchedAt:     &at,
				Data:          data,
			}
			if err := Write(account, provider, entry); err != nil {
				return err
			}
			out = Result{Data: data, FetchedAt: now, Fresh: true}
			return nil
		}

		// Keep the last good reading and move the gate.
		back := ttl
		if retry > back {
			back = retry
		}
		entry := Entry{NextAttemptAt: seconds(now) + back.Seconds()}
		if cached != nil {
			entry.FetchedAt = cached.FetchedAt
			entry.Data = cached.Data
		}
		if err := Write(account, provider, entry); err != nil {
			return err
		}
		if cached != nil {
			out = served(*cached)
		}
		return ferr
	})
	return out, err
}

// Served is a cached entry as a result, for a caller that read one itself.
func Served(e Entry) Result { return served(e) }

// served is a cached entry as a result.
func served(e Entry) Result {
	r := Result{Data: e.Data}
	if at, ok := e.Fetched(); ok {
		r.FetchedAt = at
	}
	return r
}
