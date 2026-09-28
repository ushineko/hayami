package gui

import (
	"fyne.io/fyne/v2"

	fdtheme "github.com/ushineko/fynedesygn/theme"

	"github.com/ushineko/hayami/internal/config"
)

// ThemeWith is themeWith, for a test in the black-box package.
//
// The hook is the whole of how the panel and the preferences window share one
// application theme without wearing each other's face, and it is not worth
// widening the package's API for everybody else to be able to say so. The
// queue is named because the real one is fyne.Do, which needs an event loop a
// test does not have.
// WithCardOpacity is withCardOpacity, so a test can say which theme the fade
// belongs to and which it does not.
func WithCardOpacity(base fyne.Theme, percent int) fyne.Theme {
	return withCardOpacity(base, percent)
}

func ThemeWith(
	store *config.Store,
	notify func(config.Config),
	queue func(func()),
) func(fdtheme.Appearance) fyne.Theme {
	return themeWith(store, notify, queue)
}
