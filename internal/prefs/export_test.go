package prefs

import (
	"net/url"

	"github.com/ushineko/fynedesygn/markdown"
)

// SectionPrefKeys are the sections that have preferences of their own.
func SectionPrefKeys() []string {
	out := make([]string, 0, len(sectionPrefs))
	for key := range sectionPrefs {
		out = append(out, key)
	}
	return out
}

// Readme is the About page's document pane while About is on screen.
func (w *Window) Readme() *markdown.Pane { return w.readme }

// OpenURLWith makes f what a tapped web link calls, until the test ends.
func OpenURLWith(t interface{ Cleanup(func()) }, f func(*url.URL) error) {
	was := openURL
	openURL = f
	t.Cleanup(func() { openURL = was })
}
