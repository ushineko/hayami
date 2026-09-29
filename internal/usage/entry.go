package usage

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Entry is one account's cache file.
//
// Data is held as raw JSON on purpose. This package is the protocol and knows
// nothing about a payload; a field it has never heard of must survive a write,
// because the program that wrote it is still running on the same machine.
type Entry struct {
	// NextAttemptAt is the gate, in seconds since the epoch. Before it,
	// callers read Data and make no request. It carries both freshness and
	// backoff, which is why there is one number and not two.
	NextAttemptAt float64 `json:"next_attempt_at"`

	// FetchedAt is when Data was obtained. It is null, not zero, when nothing
	// has ever succeeded: a pane shows the age of a reading, and an age of
	// fifty-six years is worse than no age.
	FetchedAt *float64 `json:"fetched_at"`

	// Data is the last successful payload. A failed fetch never replaces it.
	Data json.RawMessage `json:"data"`
}

// Fetched is FetchedAt as a time, and whether there is one.
func (e Entry) Fetched() (time.Time, bool) {
	if e.FetchedAt == nil {
		return time.Time{}, false
	}
	return epoch(*e.FetchedAt), true
}

// NextAttempt is the gate as a time: when a caller may fetch again. A panel
// that is waiting says so, and says until when, rather than drawing a blank
// for however long the backoff runs.
func (e Entry) NextAttempt() time.Time { return epoch(e.NextAttemptAt) }

// Open reports whether the gate has passed: whether a caller may fetch.
func (e Entry) Open(now time.Time) bool {
	return seconds(now) >= e.NextAttemptAt
}

// Read returns the cached entry, or nil when there is none.
//
// A file that does not parse is treated as no file rather than as an error.
// It is a cache: the worst a corrupt one can cost is one extra request, and a
// panel that refused to draw because a cache file was truncated would be
// trading a cheap problem for an expensive one.
func Read(account, provider string) (*Entry, error) {
	path, err := Path(account, provider)
	if err != nil {
		return nil, err
	}
	b, err := os.ReadFile(path) //nolint:gosec // the path this package computes
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading the usage cache: %w", err)
	}
	var e Entry
	if err := json.Unmarshal(b, &e); err != nil {
		// A cache is a cache. A corrupt one costs one extra request; a panel
		// that refused to draw over it would cost the whole reading.
		return nil, nil //nolint:nilerr // deliberate: unreadable cache means no cache
	}
	e.Data = payload(e.Data)
	return &e, nil
}

/*
payload normalises a `data` that is present but says nothing.

A fetch that has never succeeded leaves `"data": null` in the file, and
json.RawMessage keeps that as the four bytes `null` rather than as nil. Those
four bytes then decode into zero usage windows and **no error**, so an account
that has never fetched looked exactly like an account that is fine and has
nothing to report -- which is how a machine sat with an empty Usage section for
thirty-five minutes with nothing anywhere saying why (issue #54).

Nothing is a reading, so nothing is what this returns.

Read is the only place this is needed. A nil payload and the literal `null`
serialise to the same four bytes, so a write cannot put anything into the file
that a read does not take back out.
*/
func payload(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 || string(bytes.TrimSpace(raw)) == "null" {
		return nil
	}
	return raw
}

// Write replaces the entry atomically: a temporary file named for this
// process, then a rename. A reader never sees half a file, and two writers
// never interleave into one.
func Write(account, provider string, e Entry) error {
	path, err := Path(account, provider)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil { //nolint:gosec // a cache directory others read
		return fmt.Errorf("making the cache directory: %w", err)
	}

	b, err := json.Marshal(e)
	if err != nil {
		return fmt.Errorf("encoding the usage cache: %w", err)
	}

	tmp := fmt.Sprintf("%s.%d.tmp", path, os.Getpid())
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return fmt.Errorf("writing the usage cache: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("replacing the usage cache: %w", err)
	}
	return nil
}

// seconds is a time as the epoch float the cache file carries.
func seconds(t time.Time) float64 { return float64(t.UnixNano()) / float64(time.Second) }

// epoch is the inverse.
func epoch(f float64) time.Time {
	return time.Unix(0, int64(f*float64(time.Second)))
}
