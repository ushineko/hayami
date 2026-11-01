package view_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/ushineko/hayami/internal/view"
)

// osc52 writes "hi" to the terminal's clipboard; a process can name itself
// with it, and a provider's reply could carry it.
const osc52 = "\x1b]52;c;aGk=\x07"

// Spec 076. Text from outside hayami reaches the terminal as words: a
// section drawn with no painter, or with the terminal's, carries no escape
// sequence it did not add itself, and the painter's own colours survive.
func TestASectionCarriesNoEscapeItWasGiven(t *testing.T) {
	sec := view.Section{Title: "Cooler" + osc52, Rows: []view.Row{{Label: "evil" + osc52, Value: "1" + osc52}},
		Reasons: []view.Reason{{Label: "x", Text: "t" + osc52, Detail: "d" + osc52}}}

	plain := strings.Join(view.RenderWith([]view.Section{sec}, view.ArrangeStack, 60, nil), "\n")
	assert.NotContains(t, plain, "\x1b")
	assert.NotContains(t, plain, "\x07")

	red := view.Painter(func(text string, _ view.Status) string { return "\x1b[31m" + text + "\x1b[0m" })
	painted := strings.Join(view.RenderWith([]view.Section{sec}, view.ArrangeStack, 60, red), "\n")
	assert.NotContains(t, painted, "\x1b]", "the given sequence is gone")
	assert.Contains(t, painted, "\x1b[31m", "the painter's own colour is kept")
}

// A detail printed for the command line is the same: a process's name, a
// peer's, a reason's detail.
func TestADetailCarriesNoEscapeItWasGiven(t *testing.T) {
	d := view.Detail{Title: "Bandwidth by peer", Note: "n" + osc52,
		Lines:   []view.DetailLine{{Name: "proc" + osc52, Value: "1 B/s" + osc52}},
		Reasons: []view.Reason{{Text: "t", Detail: "d" + osc52}},
		Parts:   []view.Detail{{Title: "eth0" + osc52}}}
	text := d.Text()
	assert.NotContains(t, text, "\x1b")
	assert.NotContains(t, text, "\x07")
	assert.Contains(t, text, "proc]52;c;aGk=")
}
