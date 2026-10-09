package window_test

import (
	"image"
	"regexp"
	"strings"
	"testing"
	"time"
	"unsafe"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	hayami "github.com/ushineko/hayami"
)

/*
A Contents entry clicked in About brings its heading to the top of the page
(spec 055), read from pictures of the running window.

The click is posted to the window, as the reorder test's are, so the person's
pointer stays where it is. The page is wheeled down, by posted wheel messages,
until the Contents list is on screen; the list is found by the links' colour.
A heading is told from any other line by its size and by its width: a heading
is its Contents entry's words at a larger size, so heading width over entry
width is the same ratio for every heading of a level, and a page stopped at
the wrong place shows a first line that breaks it.
*/
func TestAContentsLinkClickedInAboutScrollsToItsHeading(t *testing.T) {
	hwnd := start(t, panelSettings{Sections: []string{"bandwidth"}, Settle: time.Second, Preferences: "about"})
	prefs := preferencesOf(t, hwnd)
	time.Sleep(2 * time.Second)

	top := shoot(t, prefs, "about-top.png")
	t.Logf("About on opening: %s", top.path)
	names := contentsEntries(t)

	list, at := toContents(t, prefs, len(names))
	t.Logf("Contents in view: %s", at.path)

	// The page's first line of text sits this far below the scroller's top:
	// the About header's name on opening, a heading after a jump.
	left := list[0].x0 - 80
	firstTop := firstLine(top.img, left).From

	var ratios []float64
	for _, name := range []string{"Platform notes", "Changelog", "Installing"} {
		i := indexOf(names, name)
		require.GreaterOrEqual(t, i, 0, "the Contents has no %q", name)
		entry := list[i]
		click(t, prefs, entry.x0+(entry.x1-entry.x0)/2, (entry.y0+entry.y1)/2)
		time.Sleep(700 * time.Millisecond)
		after := shoot(t, prefs, "after-"+strings.ReplaceAll(strings.ToLower(name), " ", "-")+".png")
		t.Logf("after clicking %q: %s", name, after.path)

		heading := firstLine(after.img, left)
		width := inkWidth(after.img, heading, left)
		ratio := float64(width) / float64(entry.x1-entry.x0+1)
		t.Logf("%q: first line at y %d-%d (page's first line on opening: %d), width %d, entry width %d, ratio %.3f",
			name, heading.From, heading.To, firstTop, width, entry.x1-entry.x0+1, ratio)
		assert.InDelta(t, firstTop, heading.From, 24, "%q: the first line is not at the top of the page", name)
		assert.Greater(t, heading.To-heading.From, entry.y1-entry.y0, "%q: the first line is not heading-sized", name)
		ratios = append(ratios, ratio)

		// Back to the list for the next click.
		if name != "Installing" {
			list, _ = toContents(t, prefs, len(names))
		}
	}
	for _, r := range ratios {
		assert.Greater(t, r, 1.1, "a heading is drawn larger than its entry")
		assert.InDelta(t, ratios[0], r, ratios[0]*0.06,
			"heading width over entry width differs between headings of one level: %v", ratios)
	}
}

/*
toContents wheels the page to its top and then down a notch at a time until
the Contents list's n entries are all in view, and returns them and the
picture they were found in.
*/
func toContents(t *testing.T, prefs uintptr, n int) ([]linkLine, *picture) {
	t.Helper()
	at := shoot(t, prefs, "contents.png")
	b := at.img.Bounds()
	for range 30 { // the changelog is a long way down
		wheel(t, prefs, b.Max.X*2/3, b.Max.Y/2, 120)
	}
	time.Sleep(300 * time.Millisecond)
	for notch := 0; notch < 30; notch++ {
		wheel(t, prefs, b.Max.X*2/3, b.Max.Y/2, -1)
		time.Sleep(300 * time.Millisecond)
		at = shoot(t, prefs, "contents.png")
		if list := contentsList(at.img, n); len(list) == n {
			return list, at
		}
	}
	require.FailNow(t, "the Contents list never came into view", "last picture %s", at.path)
	return nil, nil
}

// contentsEntries are the README's Contents entries, in order.
func contentsEntries(t *testing.T) []string {
	t.Helper()
	doc := hayami.README()
	start := strings.Index(doc, "## Contents\n")
	require.GreaterOrEqual(t, start, 0)
	section := doc[start:]
	if end := strings.Index(section[3:], "\n## "); end >= 0 {
		section = section[:end+3]
	}
	var out []string
	for _, m := range regexp.MustCompile(`(?m)^- \[([^\]]+)\]\(#`).FindAllStringSubmatch(section, -1) {
		out = append(out, m[1])
	}
	require.NotEmpty(t, out)
	return out
}

func indexOf(xs []string, x string) int {
	for i, v := range xs {
		if v == x {
			return i
		}
	}
	return -1
}

// linkLine is one line of link-coloured text: its ink's box.
type linkLine struct{ x0, y0, x1, y1 int }

// linked is a link's ink: the accent's blue, (96, 205, 255) at its core,
// down to its anti-aliased edge, which the plain text's greys never are.
func linked(img *image.NRGBA, x, y int) bool {
	r, g, b, _ := img.At(x, y).RGBA()
	r, g, b = r>>8, g>>8, b>>8
	return b > r+40 && b+8 >= g && b > 90
}

/*
contentsList is the Contents list in the picture: n lines of link-coloured
text in a run, each the next line down, starting at the same x. Nil when no
such run is in view.
*/
func contentsList(img *image.NRGBA, n int) []linkLine {
	b := img.Bounds()
	var all []linkLine
	in := false
	for y := b.Min.Y; y < b.Max.Y; y++ {
		x0, x1 := -1, -1
		for x := b.Max.X / 6; x < b.Max.X-40; x++ {
			if linked(img, x, y) {
				if x0 < 0 {
					x0 = x
				}
				x1 = x
			}
		}
		switch {
		case x0 >= 0 && !in:
			all = append(all, linkLine{x0, y, x1, y})
			in = true
		case x0 >= 0:
			l := &all[len(all)-1]
			l.x0, l.x1, l.y1 = min(l.x0, x0), max(l.x1, x1), y
		default:
			in = false
		}
	}
	// A run of n with one left edge and an even pitch.
	for i := 0; i+n <= len(all); i++ {
		run := all[i : i+n]
		ok := true
		pitch := run[1].y0 - run[0].y0
		for j := 1; j < n && ok; j++ {
			ok = abs(run[j].x0-run[0].x0) <= 3 && abs(run[j].y0-run[j-1].y0-pitch) <= 6
		}
		if ok && run[0].y0 > b.Max.Y/12 && run[n-1].y1 < b.Max.Y-b.Max.Y/12 {
			return run
		}
	}
	return nil
}

// firstLine is the first band of text right of x, below the window's own
// header (the top tenth of the picture).
func firstLine(img *image.NRGBA, left int) span {
	b := img.Bounds()
	from := -1
	for y := b.Max.Y / 10; y < b.Max.Y; y++ {
		has := false
		for x := left; x < b.Max.X-40 && !has; x++ {
			has = glyph(img, x, y)
		}
		switch {
		case has && from < 0:
			from = y
		case !has && from >= 0:
			return span{from, y - 1}
		}
	}
	return span{from, from}
}

// inkWidth is how wide the text on line is, right of left.
func inkWidth(img *image.NRGBA, line span, left int) int {
	x0, x1 := -1, -1
	for y := line.From; y <= line.To; y++ {
		for x := left; x < img.Bounds().Max.X-40; x++ {
			if glyph(img, x, y) {
				if x0 < 0 || x < x0 {
					x0 = x
				}
				x1 = max(x1, x)
			}
		}
	}
	return x1 - x0 + 1
}

const wmMouseWheel = 0x020A

/*
wheel turns the wheel over the window at x, y in its picture's pixels, by
notches (negative is down), without moving the pointer: a posted move tells
GLFW where the pointer is, and the wheel message scrolls whatever is there.
*/
func wheel(t *testing.T, hwnd uintptr, x, y, notches int) {
	t.Helper()
	var r rect
	_, _, _ = getWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&r)))
	var origin struct{ X, Y int32 }
	_, _, _ = clientToScreen.Call(hwnd, uintptr(unsafe.Pointer(&origin)))
	cx, cy := x-int(origin.X-r.Left), y-int(origin.Y-r.Top)
	ok, _, err := postMessageW.Call(hwnd, wmMouseMove, 0, uintptr(uint32(cy)<<16|uint32(cx)&0xffff))
	require.NotZero(t, ok, "posting a mouse move: %v", err)
	sx, sy := int(r.Left)+x, int(r.Top)+y
	delta := uint32(int16(notches*120)) & 0xffff
	ok, _, err = postMessageW.Call(hwnd, wmMouseWheel, uintptr(delta<<16), uintptr(uint32(sy)<<16|uint32(sx)&0xffff))
	require.NotZero(t, ok, "posting a wheel turn: %v", err)
	time.Sleep(30 * time.Millisecond)
}
