package gui

import (
	"image/color"
	"testing"

	"fyne.io/fyne/v2"
	fynetheme "fyne.io/fyne/v2/theme"
	"github.com/stretchr/testify/assert"

	fdtheme "github.com/ushineko/fynedesygn/theme"
)

// alphaOf reads a theme colour's alpha, at the eight-bit scale the theme is
// written in.
func alphaOf(t fyne.Theme, n fyne.ThemeColorName) int {
	_, _, _, a := t.Color(n, fynetheme.VariantDark).RGBA()
	return int(a >> 8)
}

// The card is faded by exactly the percentage asked for.
//
// This is the whole contract of the theme wrapper and it is asserted here
// because it cannot be read off a screenshot: the difference between a card at
// 95 % and one at 60 % over a dark desktop is a few units per channel, which
// is why the first three attempts to judge it by eye got it wrong.
func TestTheCardIsFadedByThePercentageAsked(t *testing.T) {
	base := fdtheme.New(fdtheme.BreezeDark, fdtheme.Options{})
	full := alphaOf(base, fynetheme.ColorNameButton)

	for _, percent := range []int{100, 95, 60, 0} {
		faded := withCardOpacity(base, percent)
		assert.Equal(t, full*percent/100, alphaOf(faded, fynetheme.ColorNameButton),
			"card at %d %%", percent)
	}
}

// The gap around the cards is not this wrapper's business.
//
// The window's own background is made fully transparent by the design system,
// and that is what makes the panel a glance window. Fading it here as well
// would be two things fighting over one colour.
func TestTheWindowsOwnBackgroundIsLeftAlone(t *testing.T) {
	base := fdtheme.New(fdtheme.BreezeDark, fdtheme.Options{})

	faded := withCardOpacity(base, 60)
	assert.Equal(t,
		alphaOf(base, fynetheme.ColorNameBackground),
		alphaOf(faded, fynetheme.ColorNameBackground))
}

// The border fades with the card. A hairline that stayed opaque around a faded
// card reads as a seam rather than as an edge.
func TestTheBorderFadesWithTheCard(t *testing.T) {
	base := fdtheme.New(fdtheme.BreezeDark, fdtheme.Options{})
	full := alphaOf(base, fynetheme.ColorNameSeparator)

	faded := withCardOpacity(base, 50)
	assert.Equal(t, full/2, alphaOf(faded, fynetheme.ColorNameSeparator))
}

// The text is never faded. A reading nobody can make out is not a reading, and
// the whole point of fading the card rather than the window is that the
// figures stay put.
func TestTheTextIsNeverFaded(t *testing.T) {
	base := fdtheme.New(fdtheme.BreezeDark, fdtheme.Options{})
	faded := withCardOpacity(base, 30)

	for _, n := range []fyne.ThemeColorName{
		fynetheme.ColorNameForeground,
		fynetheme.ColorNamePlaceHolder,
		fynetheme.ColorNameSuccess,
		fynetheme.ColorNameError,
	} {
		assert.Equal(t, alphaOf(base, n), alphaOf(faded, n), "%s was faded", n)
	}
}

// A percentage outside the range is clamped rather than wrapping into a colour
// nobody asked for. uint8 arithmetic is unforgiving.
func TestAnImpossiblePercentageIsClamped(t *testing.T) {
	base := fdtheme.New(fdtheme.BreezeDark, fdtheme.Options{})
	full := alphaOf(base, fynetheme.ColorNameButton)

	assert.Equal(t, full, alphaOf(withCardOpacity(base, 400), fynetheme.ColorNameButton))
	assert.Zero(t, alphaOf(withCardOpacity(base, -50), fynetheme.ColorNameButton))
}

// Fading an already-faded theme fades it once, not twice.
//
// The setting is applied whenever it changes, over whatever theme is current,
// and the theme that is current is usually one this package already faded.
func TestApplyingTheOpacityTwiceDoesNotFadeItTwice(t *testing.T) {
	base := fdtheme.New(fdtheme.BreezeDark, fdtheme.Options{})
	full := alphaOf(base, fynetheme.ColorNameButton)

	once := withCardOpacity(base, 50)
	twice := withCardOpacity(baseTheme(once), 50)

	assert.Equal(t, full/2, alphaOf(twice, fynetheme.ColorNameButton))
}

// The colour channels are untouched, so a faded card is the scheme's own
// colour seen through, not a darker colour.
func TestFadingChangesOnlyTheAlpha(t *testing.T) {
	c := color.NRGBA{R: 41, G: 44, B: 48, A: 255}
	out := color.NRGBAModel.Convert(fade(c, 50)).(color.NRGBA)

	assert.Equal(t, uint8(41), out.R)
	assert.Equal(t, uint8(44), out.G)
	assert.Equal(t, uint8(48), out.B)
	assert.Equal(t, uint8(127), out.A)
}
