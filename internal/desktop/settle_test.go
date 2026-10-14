//go:build !windows

package desktop

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type place struct{ x, y, w int }

// collect is a settler that hands on at once, and what it handed on.
func collect(wait time.Duration) (*settler, func() []place) {
	var mu sync.Mutex
	var got []place
	s := &settler{wait: wait, out: func(x, y, w int) {
		mu.Lock()
		defer mu.Unlock()
		got = append(got, place{x, y, w})
	}}
	return s, func() []place {
		mu.Lock()
		defer mu.Unlock()
		return append([]place(nil), got...)
	}
}

// The bus library runs each report on its own goroutine, so they arrive in
// any order. A report older than one already taken is dropped: the place
// kept is the one the compositor sent last, not the one that arrived last.
func TestAReportThatArrivesLateIsNotThePanelsPlace(t *testing.T) {
	s, got := collect(0)

	// A drawer closing on the left, as the compositor sent it: the window
	// shrinks at the open corner, then is moved to where its cards are.
	// The two arrive the other way round.
	s.take(":1.20", 41, 3696, 2037, 304)
	s.take(":1.20", 40, 3447, 2037, 304)

	assert.Equal(t, []place{{3696, 2037, 304}}, got())

	s.take(":1.20", 42, 3700, 2037, 304)
	assert.Equal(t, place{3700, 2037, 304}, got()[1], "a newer report was dropped")
	s.take(":1.20", 42, 1, 1, 1)
	assert.Len(t, got(), 2, "the same report twice is one report")
}

// A compositor that was restarted counts its serials from the start, under a
// new name: its first report is not older than the last of the one before.
// And a counter that wraps is still newer.
func TestSerialsAreComparedWithinOneSender(t *testing.T) {
	s, got := collect(0)
	s.take(":1.20", 9000, 10, 10, 304)
	s.take(":1.99", 3, 20, 20, 304)
	require.Len(t, got(), 2)
	assert.Equal(t, place{20, 20, 304}, got()[1])

	s.take(":1.77", 0xFFFFFFFE, 30, 30, 304)
	s.take(":1.77", 2, 40, 40, 304) // wrapped
	require.Len(t, got(), 4)
	assert.Equal(t, place{40, 40, 304}, got()[3])
}

// A place the window passes through is not handed on: only the one it has
// when nothing newer has come for the wait.
func TestOnlyThePlaceThePanelCameToRestAtIsHandedOn(t *testing.T) {
	s, got := collect(30 * time.Millisecond)

	s.take(":1.20", 1, 3447, 2037, 304) // shrunk, not yet moved
	s.take(":1.20", 2, 3696, 2037, 304) // moved to the cards
	s.take(":1.20", 3, 3696, 2037, 553) // put back to the open width
	s.take(":1.20", 4, 3447, 2037, 553) // and moved for it
	s.take(":1.20", 5, 3696, 2037, 304) // asked for its own width again
	assert.Empty(t, got(), "a place was handed on before the window stood still")

	require.Eventually(t, func() bool { return len(got()) == 1 }, time.Second, 2*time.Millisecond)
	time.Sleep(60 * time.Millisecond)
	assert.Equal(t, []place{{3696, 2037, 304}}, got())
}

// A watch that stops hands on nothing it was still waiting to.
func TestAStoppedWatchHandsNothingOn(t *testing.T) {
	s, got := collect(20 * time.Millisecond)
	s.take(":1.20", 1, 5, 5, 304)
	s.stop()
	time.Sleep(60 * time.Millisecond)
	assert.Empty(t, got())
}
