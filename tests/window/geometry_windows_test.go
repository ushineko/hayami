package window_test

import (
	"image"
	"image/color"
)

// inkLevel is the brightness above which a pixel is a value's or a label's
// text. The cards are dark, their text near white; the heading and the reasons
// are dimmed, and the plot's line is coloured, and both stay under it.
const inkLevel = 170

// span is a run of rows or columns, inclusive.
type span struct{ From, To int }

// ink says whether the pixel at x, y is text that finds a line: neutral or a
// status colour, and bright.
//
// Bright is not enough to find a line: the trend plot's light-blue line is as
// bright as text where it is anti-aliased, and a plot that slopes across
// fifteen rows read as a line of text, a line the card does not have (spec
// 041). The plot's colours are blue-led -- purple, light blue, teal -- and
// every line of text holds some ink that is not: a label, an arrow.
func ink(img *image.NRGBA, x, y int) bool {
	c := img.At(x, y)
	return luminance(c) > inkLevel && !plotted(c)
}

// glyph says whether the pixel at x, y is part of a word on a line already
// found: bright, whatever its colour.
//
// Within a line the plot is not there to confuse, and a word may be in the
// plot's own colours: a busy rate is drawn in the accent, (96, 205, 255),
// which no colour rule tells from the plot's light blue, (96, 192, 240).
// Counted with ink, a rate that crossed the notable threshold vanished from
// its row and the Wi-Fi test failed whenever the desk's traffic was busy
// (#149).
func glyph(img *image.NRGBA, x, y int) bool {
	return luminance(img.At(x, y)) > inkLevel
}

// plotted says whether c is one of the plot's blue-led colours: blue well
// above red and not below green. Measured on the window: (176,144,240),
// (96,192,240), (64,144,176) are the plot; (96,192,80) is a battery's green.
func plotted(c color.Color) bool {
	r, g, b, _ := c.RGBA()
	r, g, b = r>>8, g>>8, b>>8
	return b > r+48 && b >= g
}

// lines are the horizontal bands of the picture that carry text, top to
// bottom: one per row of the card.
func lines(img *image.NRGBA) []span {
	b := img.Bounds()
	var out []span
	in := false
	for y := b.Min.Y; y < b.Max.Y; y++ {
		has := false
		for x := b.Min.X; x < b.Max.X && !has; x++ {
			has = ink(img, x, y)
		}
		switch {
		case has && !in:
			out = append(out, span{y, y})
			in = true
		case has:
			out[len(out)-1].To = y
		default:
			in = false
		}
	}
	return out
}

// words are the runs of text along one line, left to right, where a gap wider
// than gap columns separates two of them.
func words(img *image.NRGBA, line span, gap int) []span {
	b := img.Bounds()
	var out []span
	last := -1 << 30
	for x := b.Min.X; x < b.Max.X; x++ {
		has := false
		for y := line.From; y <= line.To && !has; y++ {
			has = glyph(img, x, y)
		}
		if !has {
			continue
		}
		if x-last > gap {
			out = append(out, span{x, x})
		} else {
			out[len(out)-1].To = x
		}
		last = x
	}
	return out
}
