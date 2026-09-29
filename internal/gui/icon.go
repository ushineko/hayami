package gui

import (
	"fyne.io/fyne/v2"

	"github.com/ushineko/hayami/internal/gui/assets"
)

// appIcon is the application icon, embedded at build time from
// internal/gui/assets; packaging/hayami.svg is the same file, installed into
// the icon theme for the desktop entry to find.
func appIcon() fyne.Resource { return fyne.NewStaticResource("hayami.svg", assets.IconSVG()) }
