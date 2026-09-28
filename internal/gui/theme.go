package gui

import (
	"image/color"
	"math"

	"fyne.io/fyne/v2"
	fynetheme "fyne.io/fyne/v2/theme"
)

/*
withCardOpacity fades the cards without touching anything else.

The panel has two backgrounds and they want different things. The window's own
is the space *between* cards, and it is made fully transparent by the design
system so the desktop shows through the panel — that is what a glance window
is. The cards are what the readings sit on, and they are drawn with
ColorNameButton, which that transparency deliberately leaves alone: a card as
see-through as the gap around it is a card nobody can read a number off.

So the card gets its own alpha, and it is the one the user sets. Ninety-five
per cent is barely a fade and that is the point — the desktop behind is
suggested rather than shown, and the figures stay as legible as they are on an
opaque panel.

This is an alpha applied to the theme's own colour rather than a colour of its
own, so a card still follows whichever scheme the user picked in Appearance.
*/
func withCardOpacity(base fyne.Theme, percent int) fyne.Theme {
	if percent < 0 {
		percent = 0
	}
	if percent > 100 {
		percent = 100
	}
	return cardOpacity{Theme: base, percent: percent}
}

type cardOpacity struct {
	fyne.Theme
	percent int
}

// Color fades the card's fill and its border and leaves every other colour as
// the scheme drew it.
//
// The border as well as the fill, because a hairline that stayed opaque around
// a faded card reads as a seam rather than as an edge.
func (t cardOpacity) Color(n fyne.ThemeColorName, v fyne.ThemeVariant) color.Color {
	switch n {
	case fynetheme.ColorNameButton, fynetheme.ColorNameSeparator:
		return fade(t.Theme.Color(n, v), t.percent)
	default:
		return t.Theme.Color(n, v)
	}
}

// fade multiplies a colour's alpha by a percentage.
//
// Straight (non-premultiplied) alpha, because that is what Fyne's canvas takes
// and what the compositor blends: scaling the colour channels as well would
// darken the card as it faded rather than letting the desktop through it.
//
// The percentage is clamped by the caller, so the product of a byte and
// 0..100, divided by 100, is a byte. It is checked here anyway rather than
// asserted, because the arithmetic is unforgiving: a percentage that slipped
// through would wrap into a colour nobody asked for rather than failing.
func fade(c color.Color, percent int) color.Color {
	n := color.NRGBAModel.Convert(c).(color.NRGBA)

	alpha := int(n.A) * percent / 100
	if alpha < 0 {
		alpha = 0
	}
	if alpha > math.MaxUint8 {
		alpha = math.MaxUint8
	}
	return color.NRGBA{R: n.R, G: n.G, B: n.B, A: uint8(alpha)}
}
