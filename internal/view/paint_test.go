package view_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/hayami/internal/view"
)

// A painter sees a string and a status and nothing else. That is the whole of
// the hole this package opens for a shell, and it is what stops a shell
// deciding what a section says.
func TestAPainterIsToldWhatEachPieceIs(t *testing.T) {
	seen := map[view.Status][]string{}
	painter := func(text string, s view.Status) string {
		seen[s] = append(seen[s], text)
		return text
	}
	s := view.Section{Key: "usage", Title: "Usage", Meters: []view.Meter{
		{Label: "work E spend", Caption: "spend: 85 %", Reset: "1 Oct",
			Fraction: 0.85, Status: view.Warn},
	}}

	view.RenderWith([]view.Section{s}, view.ArrangeRow, 80, painter)

	require.NotEmpty(t, seen[view.Warn], "the bar and its figures carry the verdict")
	require.NotEmpty(t, seen[view.Dim], "a label and a track are not readings")
	assert.Contains(t, strings.Join(seen[view.Warn], ""), "85 %")
	assert.Contains(t, strings.Join(seen[view.Dim], ""), "work E spend")
}

// Nothing is painted without a painter. Every other test in this package is
// the assertion, and so is every pipe and every redirect to a file.
func TestWithoutAPainterNothingIsChanged(t *testing.T) {
	s := view.Section{Key: "usage", Title: "Usage", Meters: []view.Meter{
		{Label: "max M 5h", Caption: "5h: 4 %", Reset: "in 2h", Fraction: 0.04, Status: view.Good},
	}}

	plain := view.Render([]view.Section{s}, view.ArrangeRow, 80)
	nilled := view.RenderWith([]view.Section{s}, view.ArrangeRow, 80, nil)

	assert.Equal(t, plain, nilled)
	for _, line := range plain {
		assert.NotContains(t, line, "\x1b", "an escape code reached a pane nobody painted")
	}
}

// The eye finds a number by where it is. A reset that landed in a different
// place on every line would have to be read for rather than glanced at.
func TestInARowTheColumnsLineUpAcrossTheLines(t *testing.T) {
	s := view.Section{Key: "usage", Title: "Usage", Meters: []view.Meter{
		{Label: "max M 5h", Caption: "5h: 4 %  7d: 20 %", Reset: "in   2h 53m", Fraction: 0.2},
		{Label: "work E spend", Caption: "spend: 85 %", Reset: "      1 Oct", Fraction: 0.85},
	}}

	lines := view.Render([]view.Section{s}, view.ArrangeRow, 100)

	require.Len(t, lines, 2)
	assert.Equal(t, len([]rune(lines[0])), len([]rune(lines[1])),
		"two lines of one pane are not the same length")
	assert.Equal(t, strings.Index(lines[0], "5h: 4 %"),
		strings.Index(lines[1], "spend: 85 %"),
		"the figures begin at a different column on each line")
	assert.True(t, strings.HasSuffix(lines[0], "53m"))
	assert.True(t, strings.HasSuffix(lines[1], "Oct"),
		"the reset should end at the right edge")
}

// A pane too narrow for a bar keeps the numbers: the caption is the reading
// and the bar is the impression.
func TestAPaneTooNarrowForABarStillCarriesItsFigures(t *testing.T) {
	s := view.Section{Key: "usage", Title: "Usage", Meters: []view.Meter{
		{Label: "max M 5h", Caption: "5h: 4 %", Reset: "in 2h", Fraction: 0.04},
	}}

	line := view.Render([]view.Section{s}, view.ArrangeRow, 30)[0]

	assert.Contains(t, line, "4 %")
}

// The bar has to say something without colour. A pane is read over ssh, in a
// pipe and by people who cannot tell red from green, and a bar drawn in one
// glyph and two colours — which is what rich does — says nothing to any of
// them.
func TestTheBarIsReadableWithoutColour(t *testing.T) {
	s := view.Section{Key: "usage", Title: "Usage", Meters: []view.Meter{
		{Label: "max", Caption: "5h: 40 %", Reset: "in 2h", Fraction: 0.4},
	}}

	line := view.Render([]view.Section{s}, view.ArrangeRow, 80)[0]

	assert.Contains(t, line, string(view.BarFull))
	assert.Contains(t, line, string(view.BarEmpty))
	assert.NotEqual(t, view.BarFull, view.BarEmpty,
		"one glyph in two colours is a bar that vanishes in a pipe")
}
