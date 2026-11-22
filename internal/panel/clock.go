package panel

import (
	"context"
	"sync"
	"time"

	"github.com/ushineko/hayami/internal/view"
)

/*
Paced is a source that says when it next wants polling, where a fixed interval
would not do (spec 106): the clock, which changes at the turn of a second and
would run up to a second late on a ticker started at any other moment.
*/
type Paced interface {
	// Next is how long from now until the source wants polling again.
	Next(now time.Time) time.Duration
}

// Wait is how long a shell waits before polling a source again: when it
// says, for a Paced source, and its interval otherwise.
func Wait(s Source, now time.Time) time.Duration {
	if p, ok := s.(Paced); ok {
		return p.Next(now)
	}
	return s.Interval()
}

// Clock is the clock section's source (spec 106): the time, read at each
// turn of a second. It reads nothing else and makes no request.
type Clock struct {
	now func() time.Time

	mu sync.Mutex
	at time.Time
}

// NewClock is a clock reading now; nil is the system's clock.
func NewClock(now func() time.Time) *Clock {
	if now == nil {
		now = time.Now
	}
	return &Clock{now: now}
}

// Key is the clock's.
func (c *Clock) Key() string { return view.ClockInfo.Key }

// Interval is a second, for a shell that asks it rather than Next.
func (c *Clock) Interval() time.Duration { return time.Second }

// clockLate is how far past the turn of a second the clock is read, so a
// timer that fires a little early does not read the second before.
const clockLate = 5 * time.Millisecond

// Next is the time to the next second's turn.
func (c *Clock) Next(now time.Time) time.Duration {
	return now.Truncate(time.Second).Add(time.Second + clockLate).Sub(now)
}

// Poll reads the time. A clock always has something to say.
func (c *Clock) Poll(context.Context) (bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.at = c.now()
	return true, nil
}

// Section is the time last read.
func (c *Clock) Section() view.Section {
	c.mu.Lock()
	defer c.mu.Unlock()
	at := c.at
	if at.IsZero() {
		at = c.now()
	}
	return view.Clock(at)
}

// clockData is the clock as the command line prints it.
type clockData struct {
	Time string `json:"time"`
}

// Data is the time last read, as RFC 3339.
func (c *Clock) Data() any {
	c.mu.Lock()
	defer c.mu.Unlock()
	return clockData{Time: c.at.Format(time.RFC3339)}
}
