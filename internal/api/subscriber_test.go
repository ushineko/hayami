package api

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// R31. A slow client cannot make a stream's queue grow: it holds the latest
// event for each thing it names and nothing older.
func TestAStreamsQueueKeepsOnlyTheLatest(t *testing.T) {
	s := newSubscriber(time.Now)
	for i := range 1000 {
		s.offer("section/clock", "section", i)
	}
	s.offer("detail/cooler/cpu", "detail", "x")
	s.offer("section/clock", "section", 1000)

	got := s.take()
	require.Len(t, got, 2)
	assert.Equal(t, streamEvent{name: "section", data: 1000}, got[0], "the first queued, with its latest reading")
	assert.Equal(t, streamEvent{name: "detail", data: "x"}, got[1])
	assert.Empty(t, s.take())
}

// R31. A stream that leaves its queue unread past the limit is ended.
func TestAStreamLeftUnreadIsEnded(t *testing.T) {
	now := time.Unix(0, 0)
	s := newSubscriber(func() time.Time { return now })
	s.offer("section/clock", "section", 1)
	now = now.Add(StallLimit / 2)
	s.offer("section/clock", "section", 2)
	select {
	case <-s.Dead():
		t.Fatal("ended before the limit")
	default:
	}
	now = now.Add(StallLimit)
	s.offer("section/clock", "section", 3)
	select {
	case <-s.Dead():
	default:
		t.Fatal("a stream unread past the limit was not ended")
	}
	s.offer("section/clock", "section", 4)
}

// R3. A path the system would cut short is refused, naming the limit.
func TestAPathTooLongIsRefused(t *testing.T) {
	assert.NoError(t, checkPath("/run/user/1000/hayami/api.sock", 108))
	err := checkPath("/"+string(make([]byte, 120)), 108)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "fewer than 108")
}
