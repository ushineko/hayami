//go:build !windows

package desktop

import (
	"sync"
	"time"
)

// SettleWait is how long the panel's place has to stand still before it is
// reported. A drawer closing is four changes of geometry in a few
// milliseconds, two of them places the window only passes through.
const SettleWait = 150 * time.Millisecond

/*
settler turns the compositor's reports of the panel's geometry into the one
place it came to rest at.

**The reports do not arrive in the order they were sent.** The bus library
runs each incoming call on a goroutine of its own, so two reports a
millisecond apart reach their handler in either order, and the place written
last was not always the place the window was in last: a panel whose drawer
had just closed was saved a drawer's width to the left of where it stood.
Every message carries its sender's serial number, which only goes up, so a
report older than one already taken is dropped.

**And the place a window passes through is not a place it was put.** A window
that shrinks and is then moved by the compositor reports both. The newest
report is kept and handed on once nothing newer has come for wait.
*/
type settler struct {
	wait time.Duration
	out  func(x, y, width int)

	mu      sync.Mutex
	sender  string
	serial  uint32
	heard   bool
	x, y, w int
	timer   *time.Timer
	gen     int
}

// take is one report: who sent it, its serial number, and the geometry.
func (s *settler) take(sender string, serial uint32, x, y, width int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	// A serial is compared within one sender's: a compositor that was
	// restarted counts from the start again. One that is ahead by less than
	// half the counter's range is newer, so a counter that wraps is still
	// newer.
	if ahead := serial - s.serial; s.heard && sender == s.sender && (ahead == 0 || ahead > 1<<31) {
		return
	}
	s.sender, s.serial, s.heard = sender, serial, true
	s.x, s.y, s.w = x, y, width

	s.gen++
	gen := s.gen
	if s.wait <= 0 {
		s.out(x, y, width)
		return
	}
	if s.timer != nil {
		s.timer.Stop()
	}
	s.timer = time.AfterFunc(s.wait, func() { s.settled(gen) })
}

// settled hands on the newest report, unless a newer one came while the
// timer was firing.
func (s *settler) settled(gen int) {
	s.mu.Lock()
	if gen != s.gen {
		s.mu.Unlock()
		return
	}
	x, y, w := s.x, s.y, s.w
	s.mu.Unlock()
	s.out(x, y, w)
}

// stop drops a report that has not been handed on.
func (s *settler) stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.gen++
	if s.timer != nil {
		s.timer.Stop()
	}
}
