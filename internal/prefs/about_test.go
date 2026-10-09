package prefs_test

import (
	"net/url"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/test"
	fynetheme "fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/fynedesygn/fynetest"
	"github.com/ushineko/fynedesygn/markdown"

	"github.com/ushineko/hayami/internal/prefs"
)

// aboutOpen is the preferences window on About, at a size with somewhere to
// scroll to, and the README pane in it.
func aboutOpen(t *testing.T) (*prefs.Window, *markdown.Pane) {
	t.Helper()
	a := test.NewApp()
	t.Cleanup(a.Quit)
	w := prefs.New(a, prefs.Options{Store: store(t, ""), Version: "1.2.3", Page: "about"})
	w.Shell().Window.Resize(fyne.NewSize(900, 700))
	p := w.Readme()
	require.NotNil(t, p, "About has no README pane")
	// The resize is measured now rather than by the pane's settle timer, whose
	// goroutine would otherwise shape text while the test does (#125).
	p.Settle()
	return w, p
}

// link is the drawn hyperlink saying text, in whichever of the README's
// blocks draws it first.
func link(t *testing.T, p *markdown.Pane, text string) *widget.Hyperlink {
	t.Helper()
	for i := range p.Blocks() {
		var found *widget.Hyperlink
		fynetest.WalkRendered(p.Visual(i), func(o fyne.CanvasObject) bool {
			if h, ok := o.(*widget.Hyperlink); ok && h.Text == text {
				found = h
				return true
			}
			return false
		})
		if found != nil {
			return found
		}
	}
	require.FailNow(t, "the README draws no link "+text)
	return nil
}

// tap taps a hyperlink over its text, which is at its left: Fyne ignores a
// tap beside it.
func tap(h *widget.Hyperlink) {
	w := fyne.MeasureText(h.Text, fynetheme.TextSize(), fyne.TextStyle{}).Width
	test.TapAt(h, fyne.NewPos(fynetheme.InnerPadding()+w/2, h.Size().Height/2))
}

// Spec 055. A Contents entry, tapped, brings its heading to the top of the
// page: a near one and the last one, the changelog, a long way down.
func TestAContentsLinkScrollsAboutToItsHeading(t *testing.T) {
	w, p := aboutOpen(t)
	sc := w.Shell().Scroller()
	d := fyne.CurrentApp().Driver()
	for _, c := range []struct{ text, slug string }{
		{"Changelog", "changelog"}, {"Platform notes", "platform-notes"}, {"Installing", "installing"},
	} {
		tap(link(t, p, c.text))
		want, ok := p.AnchorY(c.slug)
		require.True(t, ok, "the README has no heading #%s", c.slug)
		assert.InDelta(t, want, sc.Offset.Y, 0.5, "%s: the page is not at its heading", c.text)
		heading := p.Visual(p.Anchors()[c.slug])
		at := d.AbsolutePositionForObject(heading).Y - d.AbsolutePositionForObject(sc).Y
		assert.InDelta(t, 0, at, 0.5, "%s: the heading is %v below the top of the page", c.text, at)
	}
}

// A link to a web address goes to the browser; there are no links to other
// documents by path to go nowhere (readme_test.go holds the README to that).
func TestAWebLinkInAboutGoesToTheBrowser(t *testing.T) {
	var opened []string
	prefs.OpenURLWith(t, func(u *url.URL) error { opened = append(opened, u.String()); return nil })
	_, p := aboutOpen(t)
	var web *widget.Hyperlink
	for i := range p.Blocks() {
		fynetest.WalkRendered(p.Visual(i), func(o fyne.CanvasObject) bool {
			if h, ok := o.(*widget.Hyperlink); ok && h.URL != nil && h.URL.Scheme == "https" {
				web = h
				return true
			}
			return false
		})
		if web != nil {
			break
		}
	}
	require.NotNil(t, web, "the README has no web link to tap")
	tap(web)
	assert.Equal(t, []string{web.URL.String()}, opened)
}

// The screenshots are pictures, not their alt text: the pane is given the
// embedded images.
func TestAboutShowsTheReadmesScreenshots(t *testing.T) {
	_, p := aboutOpen(t)
	images := 0
	for i := range p.Blocks() {
		fynetest.WalkRendered(p.Visual(i), func(o fyne.CanvasObject) bool {
			if _, ok := o.(*canvas.Image); ok {
				images++
			}
			if l, ok := o.(*widget.Label); ok && strings.HasPrefix(l.Text, "[") && strings.Contains(l.Text, "](") {
				t.Errorf("an image drew as its alt text: %.60s", l.Text)
			}
			return false
		})
	}
	assert.GreaterOrEqual(t, images, 7, "the gallery's seven screenshots are not drawn")
}
