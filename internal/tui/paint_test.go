package tui_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/hayami/internal/tui"
	"github.com/ushineko/hayami/internal/view"
)

// A pane is read in places a colour cannot reach. Every figure a colour
// emphasises is in the text as well, which is what makes turning it off cost
// nothing — and NO_COLOR is how a person says so.
func TestNoColorTurnsThePainterOff(t *testing.T) {
	t.Setenv("NO_COLOR", "1")

	assert.Nil(t, tui.Painter(), "a nil painter is what Render expects when nothing should be painted")
}

// What the painter does to each verdict cannot be asserted here, and finding
// that out is the point of writing this down.
//
// Under `go test` there is no terminal, so lipgloss degrades every style to
// plain text and a good reading paints identically to a bad one. A test that
// compared them would either fail against correct code or pass against code
// that painted nothing — which is how the colour came to be switched off in
// the first place, by a check on a colour profile that answers 0 in a terminal
// that plainly has colour.
//
// So this asserts what survives a terminal's absence: a painter exists, and
// the text goes through it unharmed. That the colours differ is a claim about
// what a pane looks like, and `tools/shot-tui.sh` is where that is settled.
func TestThePainterKeepsTheTextWhateverTheVerdict(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	p := tui.Painter()
	require.NotNil(t, p)

	for _, s := range []view.Status{view.Good, view.Warn, view.Bad, view.Dim, view.Info, view.Accent, view.Strong} {
		assert.Contains(t, p("48 %", s), "48 %", "the text must survive being painted")
	}
}
