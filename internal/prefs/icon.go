package prefs

import (
	"fyne.io/fyne/v2"

	"github.com/ushineko/hayami/internal/gui/assets"
)

// appIcon is the application icon, the same file the desktop entry installs.
// Duplicated from internal/gui rather than imported: prefs is built by the
// command line as well, which does not link the panel.
func appIcon() fyne.Resource { return fyne.NewStaticResource("hayami.svg", assets.IconSVG()) }
