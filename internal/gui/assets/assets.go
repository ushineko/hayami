/*
Package assets holds the files the desktop front end embeds.

The icon is a copy of packaging/hayami.svg. It is duplicated rather than
referenced because go:embed cannot reach outside the package directory, and
the packaging copy has to stay on disk for the desktop entry and the icon
theme to install. Change one and change the other; a test in this package
says so if they drift.
*/
package assets

import _ "embed"

//go:embed hayami.svg
var iconSVG []byte

// IconSVG is the application icon: the window's, and the taskbar's where the
// desktop resolves it from the window rather than from the desktop entry.
func IconSVG() []byte { return iconSVG }
