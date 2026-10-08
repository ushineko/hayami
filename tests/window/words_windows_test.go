package window_test

import (
	"image"
	"image/color"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

/*
A word in the plot's own colours is still a word, and the plot is still not a
line (#149).

A busy rate is drawn in the accent, which is the plot's light blue to within a
few levels. The picture here is the case that failed: a row with a white label
and an accent rate, and under it a plot line in the same light blue sloping
across more rows than a line of text is tall. The line finder must see one
line, not two; the word finder must see both words on it. It needs no window,
so it runs whether or not HAYAMI_WINDOW_TEST is set.
*/
func TestAWordInThePlotsColoursIsAWordAndThePlotIsNotALine(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 200, 80))
	fill := func(x0, y0, x1, y1 int, c color.NRGBA) {
		for y := y0; y <= y1; y++ {
			for x := x0; x <= x1; x++ {
				img.SetNRGBA(x, y, c)
			}
		}
	}
	fill(0, 0, 199, 79, color.NRGBA{R: 30, G: 30, B: 34, A: 255})     // the card
	fill(10, 10, 40, 22, color.NRGBA{R: 235, G: 235, B: 235, A: 255}) // a label
	fill(90, 10, 140, 22, color.NRGBA{R: 96, G: 205, B: 255, A: 255}) // a busy rate, in the accent
	// The plot, sloping across twenty rows, in the bright light blue its
	// anti-aliased line reaches: above the text threshold, as the line once
	// read as text was.
	for x := 0; x < 200; x++ {
		y := 70 - x/10
		img.SetNRGBA(x, y, color.NRGBA{R: 110, G: 210, B: 255, A: 255})
	}

	text := textLines(img)
	require.Len(t, text, 1, "the plot was read as a line of text, or the row was lost: %v", text)
	got := words(img, text[0], 12)
	assert.Equal(t, []span{{10, 40}, {90, 140}}, got, "the accent rate is not a word on its row")
}
