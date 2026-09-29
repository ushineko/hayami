package gui

import (
	"fyne.io/fyne/v2"
	fynetheme "fyne.io/fyne/v2/theme"

	"github.com/ushineko/hayami/internal/view"
)

/*
sectionIcon is the glyph a section asks for, as something Fyne can draw.

Fyne's own icons rather than drawings of our own. They are already in the
binary, they follow the theme's foreground colour, and a set drawn here would
be four more things to keep legible at every text size the panel offers. The
matches are the closest the standard set has, which is not always the obvious
word:

  - peripherals: a computer, because Fyne has no battery and the section is
    about the things plugged into one.
  - bandwidth: a download arrow. The section is both directions and the icon
    is one of them; it reads as traffic, which is the point.
  - cooler: a warning triangle would be wrong -- a cooler at temperature is
    not a fault -- so it is the media-record dot, a filled circle that reads
    as a gauge beside a temperature.
  - usage: a storage icon, a quota being filled.

A name this does not know draws nothing, which is what IconNone asks for and
what a settings file from a later version would arrive with.
*/
func sectionIcon(name view.IconName) fyne.Resource {
	switch name {
	case view.IconPeripherals:
		return fynetheme.ComputerIcon()
	case view.IconBandwidth:
		return fynetheme.DownloadIcon()
	case view.IconCooler:
		return fynetheme.MediaRecordIcon()
	case view.IconUsage:
		return fynetheme.StorageIcon()
	default:
		return nil
	}
}
