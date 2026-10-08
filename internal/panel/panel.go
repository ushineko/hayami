/*
Package panel joins a reading to its drawing.

core polls and knows nothing about drawing; view describes a section and knows
nothing about polling. A Source is the one place the two meet, and both shells
hold the same set of them. A shell that built its own sections from core's
values would be a shell deciding what a section says, which is the thing the
parity test exists to catch.
*/
package panel

import (
	"context"
	"time"

	"github.com/ushineko/hayami/internal/view"
)

// Source is a section that polls itself and describes itself.
type Source interface {
	// Key names the section in the settings and on the command line.
	Key() string

	// Poll takes one reading, reporting whether the source has anything to
	// say. A source with nothing to say is not drawn.
	Poll(ctx context.Context) (bool, error)

	// Interval is how often this source wants polling.
	Interval() time.Duration

	// Section is the current reading, drawn.
	Section() view.Section

	// Data is the current reading as plain values, for the JSON the command
	// line prints. It is deliberately not what Section returns: that one is
	// formatted, and a column width is not a fact about a network interface.
	Data() any
}
