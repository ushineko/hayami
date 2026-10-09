/*
Package hayami is the repository's root: the README, embedded so that the
preferences window's About section shows the document itself rather than a
second, less careful copy that would drift from it.

Following ototo, which does the same for the same reason. There are no
diagrams to resolve: hayami's README carries none. Its screenshots are
embedded beside it (spec 055), the gallery and nothing else: the specs'
photographs are linked from the specs, not shown by the README, and would be
weight the program carries for nobody.
*/
package hayami

import (
	"embed"
	"io/fs"
)

//go:embed README.md
var readme string

//go:embed docs/img/gallery-*.png
var images embed.FS

// README is the document the About section shows.
func README() string { return readme }

// Images is the file system the README's relative image paths resolve in:
// docs/img/gallery-*.png, at the paths the README names them by.
func Images() fs.FS { return images }
