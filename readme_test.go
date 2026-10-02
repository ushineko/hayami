package hayami_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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
