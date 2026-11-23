package view_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/hayami/internal/view"
)

// Spec 112 R1. A prose line flows into lines that fit, its words whole and
// in order, a word longer than a line broken; other lines are untouched.
func TestProseFlowsToTheWidth(t *testing.T) {
	text := "Content keeps the existing release_status: hidden tag (e1d825ec-7305-42fe-965f-bf7c8d38e647, not ready)"
	lines := []view.DetailLine{
		{Name: "Opened", Value: "Sun 22 Nov"},
		{Name: text, Wide: true, Prose: true, Status: view.Info, Indent: 1},
	}
	for _, width := range []int{20, 36, 60, 200} {
		out := view.Reflow(lines, view.FitsColumns(width))
		require.Equal(t, lines[0], out[0], "a line that is not prose changed")
		var words []string
		for _, l := range out[1:] {
			assert.False(t, l.Prose)
			assert.True(t, l.Wide)
			assert.Equal(t, 1, l.Indent)
			assert.LessOrEqual(t, len([]rune(l.Name))+2, width, "%q is wider than %d", l.Name, width)
			words = append(words, l.Name)
		}
		joined := strings.Join(words, " ")
		assert.Equal(t, strings.ReplaceAll(text, " ", ""), strings.ReplaceAll(joined, " ", ""), "a character was lost at %d", width)
		if width == 200 {
			assert.Len(t, out, 2, "a paragraph that fits is one line")
		}
		if width == 36 {
			assert.Greater(t, len(out), 3)
		}
	}
}

// Spec 112 R2. The terminal's drawer flows a change's description to its
// columns: a wider drawer is fewer, longer lines.
func TestTheTerminalFlowsADescriptionToItsWidth(t *testing.T) {
	c := &view.DevChange{Kind: "pr", Ref: "#1", Title: "a change", State: "open",
		Body: strings.Repeat("words that wrap at the drawer's width and no other ", 8)}
	page := view.DevChangePage("GitHub", "x", view.DevOpenPR, "octo/widget", "#1", "", c, false, nil)
	count := func(width int) int {
		n := 0
		for _, l := range view.RenderDetail(page, width, 200, view.DrawerShape{Marked: -1}) {
			if strings.Contains(l, "words") {
				n++
				assert.LessOrEqual(t, len([]rune(strings.TrimRight(l, " "))), width, l)
			}
		}
		return n
	}
	narrow, wide := count(50), count(100)
	assert.Greater(t, narrow, wide, "the description did not flow to the drawer's width")
	assert.Greater(t, wide, 1)
}
