package window_test

import "image"

// inkLevel is the brightness above which a pixel is a value's or a label's
// text. The cards are dark, their text near white; the heading and the reasons
// are dimmed, and the plot's line is coloured, and both stay under it.
const inkLevel = 170

// span is a run of rows or columns, inclusive.
type span struct{ From, To int }

// ink says whether the pixel at x, y is text.
func ink(img *image.NRGBA, x, y int) bool { return luminance(img.At(x, y)) > inkLevel }

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
			has = ink(img, x, y)
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
