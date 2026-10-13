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

/*
Detailer is a source that has more to say than its card has room for
(spec 060): the processes behind a rate, the limits behind a meter. A source
without it has no details and its card does nothing on a click.

A detail is asked for only while a shell is showing it. Nothing here runs on
the section's own poll, so a panel nobody clicks reads nothing new.
*/
type Detailer interface {
	// Details are the details the source offers for its current reading, in
	// the order a list of them reads.
	Details() []view.DetailInfo

	// PollDetail reads one detail. A detail that cannot be read is returned
	// with the reason in it, and the error is for a log.
	PollDetail(ctx context.Context, id string) (view.Detail, error)

	// DetailInterval is how often a shown detail wants reading.
	DetailInterval(id string) time.Duration

	// CloseDetail says a detail is no longer shown, so whatever was held
	// open to read it can be let go.
	CloseDetail(id string)
}

// ProcessInterval is how often a list of processes is read while it is
// shown: the bandwidth section's two seconds, whichever section the list
// belongs to. A list a person is watching is watched for what changes, and
// five seconds between changes reads as a list that has stopped.
const ProcessInterval = 2 * time.Second

// Offers reports whether a source offers the detail id.
func Offers(s Source, id string) bool {
	d, ok := s.(Detailer)
	if !ok {
		return false
	}
	for _, info := range d.Details() {
		if info.ID == id {
			return true
		}
	}
	return false
}

// unread is a detail that could not be read: its heading, and the reason.
func unread(section, id, title string, err error) view.Detail {
	return view.Detail{Section: section, ID: id, Title: title, Reasons: []view.Reason{
		reason(view.Reason{Text: "not available", Status: view.Info}, err),
	}}
}
