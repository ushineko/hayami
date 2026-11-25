package api

import (
	"sync"
	"time"
)

/*
subscriber is one stream's queue (spec 113 R31): the latest event for each
section and detail it names, and nothing older. A slow client cannot make
the hub wait and cannot make the queue grow: an event replaces the one
before it for the same thing, and the hub never blocks offering one.

A queue that has held something undrained for StallLimit is a client that
is not reading; the stream is ended (Dead).
*/
type subscriber struct {
	now func() time.Time

	mu      sync.Mutex
	pending map[string]streamEvent
	order   []string
	since   time.Time
	ended   bool

	signal chan struct{}
	dead   chan struct{}
}

// streamEvent is one event's name and payload.
type streamEvent struct {
	name string
	data any
}

// StallLimit is how long a stream may leave an event unread (R31).
var StallLimit = 30 * time.Second

func newSubscriber(now func() time.Time) *subscriber {
	return &subscriber{
		now: now, pending: map[string]streamEvent{},
		signal: make(chan struct{}, 1), dead: make(chan struct{}),
	}
}

// offer queues an event for key, replacing any not yet sent.
func (s *subscriber) offer(key, name string, data any) {
	s.mu.Lock()
	if s.ended {
		s.mu.Unlock()
		return
	}
	now := s.now()
	if len(s.pending) == 0 {
		s.since = now
	} else if now.Sub(s.since) > StallLimit {
		s.ended = true
		close(s.dead)
		s.mu.Unlock()
		return
	}
	if _, ok := s.pending[key]; !ok {
		s.order = append(s.order, key)
	}
	s.pending[key] = streamEvent{name: name, data: data}
	s.mu.Unlock()
	select {
	case s.signal <- struct{}{}:
	default:
	}
}

// take is every queued event, in the order first queued, and empties the
// queue.
func (s *subscriber) take() []streamEvent {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]streamEvent, 0, len(s.order))
	for _, k := range s.order {
		out = append(out, s.pending[k])
	}
	s.pending, s.order = map[string]streamEvent{}, nil
	return out
}

// Ready is signalled when there is something to take.
func (s *subscriber) Ready() <-chan struct{} { return s.signal }

// Dead is closed when the stream stalled.
func (s *subscriber) Dead() <-chan struct{} { return s.dead }
