/*
Package hayami is the repository's root: the README, embedded so that the
preferences window's About section shows the document itself rather than a
second, less careful copy that would drift from it.

Following ototo, which does the same for the same reason. There are no
diagrams to resolve: hayami's README carries none, so the pane is given the
document alone.
*/
package hayami

import _ "embed"

//go:embed README.md
var readme string

// README is the document the About section shows.
func README() string { return readme }
