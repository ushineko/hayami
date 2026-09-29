package tui_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/hayami/internal/panel"
	"github.com/ushineko/hayami/internal/tui"
	"github.com/ushineko/hayami/internal/view"
)

// reasonSource reports nothing and says why, which is what a source on a
// machine without the hardware does.
type reasonSource struct {
	key     string
	reasons []view.Reason
	err     error
}

func (r *reasonSource) Key() string             { return r.key }
func (r *reasonSource) Interval() time.Duration { return time.Minute }
func (r *reasonSource) Poll(context.Context) (bool, error) {
	return false, r.err
}
func (r *reasonSource) Section() view.Section {
	return view.Section{Key: r.key, Title: r.key, Reasons: r.reasons}
}
func (r *reasonSource) Data() any { return nil }

/*
A pane draws a section that has only a reason.

The terminal half of the same fault: the poll's error was discarded into
`drawn = false` and the block simply was not there, so `hayami-tui` on a
machine with no cooler looked exactly like `hayami-tui` on a machine where the
cooler code was broken (issue #54).
*/
func TestAPaneDrawsASectionThatHasOnlyAReason(t *testing.T) {
	src := &reasonSource{key: "cooler", reasons: []view.Reason{
		{Label: "Coolant", Text: "no cooler", Status: view.Info},
	}}
	m := tui.New(tui.Options{Sources: []panel.Source{src}, Arrangement: view.ArrangeStack, Once: true})

	got := run(t, m, time.Second)

	require.Len(t, got.Sections(), 1)
	assert.Contains(t, got.View(), "no cooler")
}

// A source whose poll failed outright is drawn too, as long as it said why.
// The error deciding visibility is what made a failure invisible.
func TestAPollThatFailedIsStillDrawnWhenItSaidWhy(t *testing.T) {
	src := &reasonSource{
		key:     "cooler",
		err:     assertAnError,
		reasons: []view.Reason{{Text: "liquidctl failed", Status: view.Warn}},
	}
	m := tui.New(tui.Options{Sources: []panel.Source{src}, Arrangement: view.ArrangeStack, Once: true})

	got := run(t, m, time.Second)

	require.Len(t, got.Sections(), 1)
	assert.Contains(t, got.View(), "liquidctl failed")
}

// A source with nothing at all still draws nothing: an unconfigured section is
// not a fault and should not take a line saying so.
func TestAPaneLeavesOutASectionWithNothingAtAll(t *testing.T) {
	src := &reasonSource{key: "cooler"}
	m := tui.New(tui.Options{Sources: []panel.Source{src}, Arrangement: view.ArrangeStack, Once: true})

	got := run(t, m, time.Second)

	assert.Empty(t, got.Sections())
}

// Every arrangement draws the reason, not only the stack: a pane in row form
// is the one somebody runs in a status line, and a reading missing from it
// with no explanation is the same silence in a smaller space.
func TestEveryArrangementDrawsAReason(t *testing.T) {
	for _, a := range []view.Arrangement{view.ArrangeStack, view.ArrangeGrid, view.ArrangeRow} {
		src := &reasonSource{key: "cooler", reasons: []view.Reason{
			{Label: "Coolant", Text: "no cooler", Status: view.Info},
		}}
		m := tui.New(tui.Options{Sources: []panel.Source{src}, Arrangement: a, Once: true})

		got := run(t, m, time.Second)

		assert.True(t, strings.Contains(got.View(), "no cooler"),
			"%s drew no reason: %q", a, got.View())
	}
}

// assertAnError is a poll that failed, whatever the reason.
var assertAnError = errAnError{}

type errAnError struct{}

func (errAnError) Error() string { return "asking liquidctl for the cooler: exit status 2" }
