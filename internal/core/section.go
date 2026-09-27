package core

import (
	"context"
	"time"
)

// Section is one source of readings: it knows its key, it polls, and it says
// whether it has anything to report.
//
// The interface is here rather than in the view because a section is a source
// first and a card second. Both shells hold the same set of these and neither
// knows how any of them reads its numbers.
type Section interface {
	// Key names the section in the settings and on the command line.
	Key() string

	// Title is what the section is called on screen.
	Title() string

	// Poll takes one reading. It returns false when the source has nothing to
	// say, which is different from an error: a machine with no cooler is not a
	// broken machine.
	Poll(ctx context.Context) (bool, error)

	// Interval is how often this section wants polling. A byte counter and a
	// thermal probe share a window and nothing else.
	Interval() time.Duration
}
