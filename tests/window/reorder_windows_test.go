package window_test

import (
	"image"
	"image/color"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"
	"unsafe"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// pidOf is the process that owns a window.
func pidOf(hwnd uintptr) int {
	var pid uint32
	_, _, _ = getWindowThreadProcessID.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
	return int(pid)
}

// preferencesOf waits for the preferences window of the panel's process. Its
// title carries the build's version, so it is matched by its start.
func preferencesOf(t *testing.T, panel uintptr) uintptr {
	t.Helper()
	pid := pidOf(panel)
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		var found uintptr
		cb := syscall.NewCallback(func(h, _ uintptr) uintptr {
			if pidOf(h) != pid {
				return 1
			}
			if v, _, _ := isWindowVisible.Call(h); v == 0 {
				return 1
			}
			buf := make([]uint16, 256)
			n, _, _ := getWindowTextW.Call(h, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
			if strings.HasPrefix(syscall.UTF16ToString(buf[:n]), "hayami preferences") {
				found = h
				return 0
			}
			return 1
		})
		_, _, _ = enumWindows.Call(cb, 0)
		if found != 0 {
			return found
		}
		time.Sleep(100 * time.Millisecond)
	}
	require.FailNow(t, "the preferences window did not appear")
	return 0
}

var (
	clientToScreen = user32.NewProc("ClientToScreen")
	postMessageW   = user32.NewProc("PostMessageW")
)

const (
	wmMouseMove   = 0x0200
	wmLButtonDown = 0x0201
	wmLButtonUp   = 0x0202
	mkLButton     = 0x0001
)

/*
click taps the window at x, y in its picture's pixels, by posting the mouse's
messages to it rather than moving the pointer: GLFW takes the cursor's place
from the move message, so the panel sees a tap where the person's pointer
never went, and a test that moved somebody's pointer would be worse than one
that does not. The picture is the whole window, frame and all; the messages
are in the client area's pixels.
*/
func click(t *testing.T, hwnd uintptr, x, y int) {
	t.Helper()
	var r rect
	_, _, _ = getWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&r)))
	var origin struct{ X, Y int32 }
	_, _, _ = clientToScreen.Call(hwnd, uintptr(unsafe.Pointer(&origin)))
	cx, cy := x-int(origin.X-r.Left), y-int(origin.Y-r.Top)
	at := uintptr(uint32(cy)<<16 | uint32(cx)&0xffff)
	for _, m := range []struct{ msg, keys uintptr }{
		{wmMouseMove, 0}, {wmLButtonDown, mkLButton}, {wmLButtonUp, 0},
	} {
		ok, _, err := postMessageW.Call(hwnd, m.msg, m.keys, at)
		require.NotZero(t, ok, "posting a mouse message: %v", err)
		time.Sleep(50 * time.Millisecond)
	}
}

// box is a rectangle in a picture.
type box struct{ X0, Y0, X1, Y1 int }

func (b box) centre() (int, int) { return (b.X0 + b.X1) / 2, (b.Y0 + b.Y1) / 2 }

/*
moveButtons are the preferences' up and down buttons, a pair to a section's
row, top to bottom, found in the picture.

A button is a region of the buttons' colour -- the commonest colour in the
right fifth of the window, after the page's own, in runs a button wide -- as
tall as it is wide; its
icon is a hole in the region, not a break in it. The menu's button at the top
right is one to its row and is not a pair.
*/
func moveButtons(img *image.NRGBA) [][2]box {
	b := img.Bounds()
	x0 := b.Max.X * 4 / 5
	page := img.NRGBAAt(b.Max.X/2, b.Max.Y-40)
	// Counted only in runs a button's width: the window's frame and its
	// margins are colours too, in runs much narrower or much wider.
	count := map[color.NRGBA]int{}
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := x0; x < b.Max.X; {
			c := img.NRGBAAt(x, y)
			end := x + 1
			for end < b.Max.X && img.NRGBAAt(end, y) == c {
				end++
			}
			if n := end - x; c != page && n >= 30 && n <= 120 {
				count[c] += n
			}
			x = end
		}
	}
	// An enabled button and a disabled one are two faces; anything with a
	// quarter of the commonest's count is one.
	most := 0
	for _, n := range count {
		most = max(most, n)
	}
	face := map[color.NRGBA]bool{}
	for c, n := range count {
		if n*4 >= most {
			face[c] = true
		}
	}

	seen := map[image.Point]bool{}
	var boxes []box
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := x0; x < b.Max.X; x++ {
			start := image.Pt(x, y)
			c := img.NRGBAAt(x, y)
			if seen[start] || !face[c] {
				continue
			}
			r := box{x, y, x, y}
			stack := []image.Point{start}
			seen[start] = true
			for len(stack) > 0 {
				q := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				r.X0, r.Y0 = min(r.X0, q.X), min(r.Y0, q.Y)
				r.X1, r.Y1 = max(r.X1, q.X), max(r.Y1, q.Y)
				for _, d := range []image.Point{{1, 0}, {-1, 0}, {0, 1}, {0, -1}} {
					n := q.Add(d)
					if n.X < x0 || n.X >= b.Max.X || n.Y < b.Min.Y || n.Y >= b.Max.Y || seen[n] {
						continue
					}
					if img.NRGBAAt(n.X, n.Y) == c {
						seen[n] = true
						stack = append(stack, n)
					}
				}
			}
			w, h := r.X1-r.X0+1, r.Y1-r.Y0+1
			if w >= 30 && w <= 120 && h*10 >= w*8 && h*10 <= w*12 {
				boxes = append(boxes, r)
			}
		}
	}

	// Two to a row, the left one up.
	var pairs [][2]box
	for i, a := range boxes {
		for _, c := range boxes[i+1:] {
			if abs(a.Y0-c.Y0) <= 2 && a.X0 != c.X0 {
				if c.X0 < a.X0 {
					a, c = c, a
				}
				pairs = append(pairs, [2]box{a, c})
			}
		}
	}
	slices.SortFunc(pairs, func(p, q [2]box) int { return p[0].Y0 - q[0].Y0 })
	return pairs
}

// labelWidth is the width of a line's first word: a row's label. A card's
// heading would do as well if it were ink, but it is set in the muted colour
// and only its icon is found.
func (p *picture) labelWidth(line int) int {
	w := p.words(p.text[line])
	if len(w) == 0 {
		return 0
	}
	return w[0].To - w[0].From
}

/*
Spec 052, issue #158. A section moved in the preferences moves on the panel
at once, with nothing restarted.

The panel shows the bandwidth and the cooler; the preferences, open on their
Sections page, are told to move the cooler up -- a tap on its row's up
button, found in a picture of that window. The panel is photographed before
and after, and the cooler's card is found by its first row's label, a
processor's name, whose width is the same in every picture: the lower card's
before, the top card's after. The window is the same size after: the same
cards in another order are the same height.
*/
func TestASectionMovedInThePreferencesMovesOnThePanel(t *testing.T) {
	s := panelSettings{Sections: []string{"bandwidth", "cooler"}, Interfaces: []string{"Wi-Fi"},
		LHM: "http://127.0.0.1:9/data.json", Settle: coolerSettle, Preferences: "sections"}
	hwnd := start(t, s)
	prefs := preferencesOf(t, hwnd)
	bandwidth, cooler := len(cardOf(t, "bandwidth", s)), len(cardOf(t, "cooler", s))

	before := shoot(t, hwnd, "before.png")
	t.Logf("before: %s", before.path)
	require.Len(t, before.text, bandwidth+cooler+2, "the panel is not the two cards (picture %s)", before.path)
	// The cooler's first label is a processor's name, the same in every
	// picture; the bandwidth's takes the signal's bars into its word or not,
	// as they light, so it is told apart only by not being the cooler's.
	cooled := before.labelWidth(bandwidth + 2)
	t.Logf("first labels before: top %d, lower (the cooler) %d", before.labelWidth(1), cooled)
	require.Greater(t, abs(before.labelWidth(1)-cooled), 10, "the two cards' labels are too alike to tell apart")

	page := shoot(t, prefs, "preferences.png")
	t.Logf("preferences: %s", page.path)
	pairs := moveButtons(page.img)
	t.Logf("move buttons: %v", pairs)
	require.GreaterOrEqual(t, len(pairs), 2, "the preferences show no rows of move buttons")
	x, y := pairs[1][0].centre() // the second row is the cooler; its up button
	click(t, prefs, x, y)

	time.Sleep(time.Second)
	after := shoot(t, hwnd, "after.png")
	t.Logf("after: %s", after.path)
	require.Len(t, after.text, bandwidth+cooler+2, "the panel is not the two cards (picture %s)", after.path)
	top, lower := after.labelWidth(1), after.labelWidth(cooler+2)
	t.Logf("first labels after: top %d, lower %d", top, lower)
	assert.InDelta(t, cooled, top, 2, "the cooler is not on top")
	assert.Greater(t, abs(lower-cooled), 10, "the lower card is still the cooler")
	assert.Equal(t, before.img.Bounds(), after.img.Bounds(), "the window changed size")
}
