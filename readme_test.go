package hayami_test

import (
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"unicode"

	"fyne.io/fyne/v2"
	fynetheme "fyne.io/fyne/v2/theme"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/image/font/sfnt"

	"github.com/ushineko/fynedesygn/markdown"

	hayami "github.com/ushineko/hayami"
)

// The embedded README is the file on disk, so About cannot show a stale copy.
func TestTheEmbeddedReadmeIsTheFileOnDisk(t *testing.T) {
	onDisk, err := os.ReadFile("README.md")
	require.NoError(t, err)
	assert.Equal(t, string(onDisk), hayami.README())
}

// It is the document and not a fragment: About is the only place a user reads
// it inside the program, so a truncated embed would be invisible until
// somebody scrolled.
func TestTheEmbeddedReadmeIsWhole(t *testing.T) {
	doc := hayami.README()
	assert.True(t, strings.HasPrefix(doc, "# hayami"), "the README no longer starts with its title")
	assert.Contains(t, doc, "## Changelog", "the changelog is missing, so the embed is truncated")
}

// readmeImages are the images the README displays, by path, with their alt
// text.
func readmeImages(t *testing.T) map[string]string {
	t.Helper()
	body, err := os.ReadFile("README.md")
	require.NoError(t, err)
	out := map[string]string{}
	for _, m := range regexp.MustCompile(`!\[([^\]]*)\]\(([^)\s]+)\)`).FindAllStringSubmatch(string(body), -1) {
		out[m[2]] = m[1]
	}
	return out
}

// Every picture in the gallery is on the page. tools/screenshot.sh writes
// docs/img/gallery-*.png, and a shot added there and not to the README is a
// picture nobody sees. The photographs the specs take are theirs, linked from
// the specs, and exempt by their prefix.
func TestEveryGalleryImageIsInTheReadme(t *testing.T) {
	shots, err := filepath.Glob("docs/img/gallery-*.png")
	require.NoError(t, err)
	require.NotEmpty(t, shots, "the gallery is empty: run make screenshots")

	shown := readmeImages(t)
	for _, shot := range shots {
		shot = filepath.ToSlash(shot) // the README's paths, whatever the host's
		alt, ok := shown[shot]
		if assert.True(t, ok, "%s is not displayed by the README", shot) {
			assert.NotEmpty(t, strings.TrimSpace(alt),
				"%s has no alt text, which is all a screen reader says of it", shot)
		}
	}
}

// Every picture the README displays is there to display. A renamed or
// removed shot otherwise shows as a broken image on the project page and as
// its alt text, dimmed, in About.
func TestEveryImageTheReadmeShowsExists(t *testing.T) {
	for path := range readmeImages(t) {
		_, err := os.Stat(path)
		assert.NoError(t, err, "the README displays %s, which is not there", path)
	}
}

/*
Every link in the README works in About as well as on GitHub (spec 055).

The About page scrolls to a #heading and opens an absolute web address; it
draws any other link as plain text, because it has nothing to resolve a path
against. So a link to another document is written as its full address, and an
anchor names a heading that is there: a renamed heading otherwise leaves a
Contents entry that does nothing, on both.
*/
func TestEveryReadmeLinkIsAnAnchorOrAWebAddress(t *testing.T) {
	doc := hayami.README()
	anchors := markdown.Anchors(markdown.Blocks(doc))
	links := regexp.MustCompile(`(!?)\[[^\]]*\]\(([^)\s]+)\)`).FindAllStringSubmatch(doc, -1)
	require.NotEmpty(t, links)
	for _, m := range links {
		image, dest := m[1] == "!", m[2]
		if image {
			continue // TestEveryImageTheReadmeShowsExists
		}
		u, err := url.Parse(dest)
		if !assert.NoError(t, err, dest) {
			continue
		}
		switch {
		case markdown.IsAnchor(u):
			_, ok := anchors[u.Fragment]
			assert.True(t, ok, "%s names no heading in the README", dest)
		case markdown.IsWebURL(u):
		default:
			assert.Fail(t, "a link About cannot follow",
				"%s is neither a #heading nor an absolute http(s) address; write it as "+
					"https://github.com/ushineko/hayami/blob/main/%s", dest, dest)
		}
	}
}

// The images About shows are the ones embedded: an image the README displays
// and the embed lacks is alt text in About and a picture on GitHub.
func TestEveryReadmeImageIsEmbedded(t *testing.T) {
	for path := range readmeImages(t) {
		_, err := fs.Stat(hayami.Images(), path)
		assert.NoError(t, err, "%s is shown by the README and not embedded", path)
	}
}

/*
Every character the README shows is one the window can draw in the face it is
drawn in (fynedesygn's docs/fyne-quirks.md, 19).

A character outside Fyne's bundled fonts is drawn from a system face, and Fyne
marks the end of that run as a missing glyph: "Options → Remote Web Server"
read "Options →� Remote Web Server" in About. Code is drawn in the monospace
face and prose in the text faces, and the two do not have the same characters
(the monospace one has the arrow), so each is checked against its own. The CJK
of the name is the exception, and deliberate: it is what the program is
called, and it has no ASCII spelling.
*/
func TestEveryReadmeCharacterIsInItsBundledFont(t *testing.T) {
	prose := bundled(t, fynetheme.DefaultTextFont(), fynetheme.DefaultTextBoldFont(),
		fynetheme.DefaultTextItalicFont(), fynetheme.DefaultTextBoldItalicFont())
	mono := bundled(t, fynetheme.DefaultTextMonospaceFont())

	code := regexp.MustCompile("(?s)```.*?```|`[^`\\n]*`")
	doc := hayami.README()
	check := func(text, where string, has func(rune) bool) {
		seen := map[rune]bool{}
		for _, r := range text {
			if r < 0x80 || seen[r] || unicode.Is(unicode.Han, r) {
				continue
			}
			seen[r] = true
			assert.True(t, has(r), "the README's %s uses %q (U+%04X), which its bundled font lacks; "+
				"About draws it with a missing-glyph mark beside it", where, r, r)
		}
	}
	check(code.ReplaceAllString(doc, ""), "prose", prose)
	check(strings.Join(code.FindAllString(doc, -1), ""), "code", mono)
}

// bundled reports whether any of some font resources has a glyph for a rune.
func bundled(t *testing.T, fonts ...fyne.Resource) func(rune) bool {
	t.Helper()
	faces := make([]*sfnt.Font, 0, len(fonts))
	for _, res := range fonts {
		face, err := sfnt.Parse(res.Content())
		require.NoError(t, err, res.Name())
		faces = append(faces, face)
	}
	var buf sfnt.Buffer
	return func(r rune) bool {
		for _, face := range faces {
			if i, err := face.GlyphIndex(&buf, r); err == nil && i != 0 {
				return true
			}
		}
		return false
	}
}
