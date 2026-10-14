package window_test

import (
	"fmt"
	"image"
	"os"
	"path/filepath"
	"testing"
	"time"
	"unsafe"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	wmMouseLeave = 0x02A3
	wmChar       = 0x0102
)

// typed posts a character to the panel's window, as a key pressed while it
// holds the keyboard arrives.
func typed(t *testing.T, hwnd uintptr, r rune) {
	t.Helper()
	ok, _, err := postMessageW.Call(hwnd, wmChar, uintptr(r), 0)
	require.NotZero(t, ok, "posting a character: %v", err)
}

/*
tap is click, and then the pointer leaves the window. A click's posted move
leaves GLFW believing the pointer rests on the row, and a row with a tip
shows it a moment later, over the card and the drawer in the picture. A
person's pointer would be there too; the picture is of the drawer.
*/
func tap(t *testing.T, hwnd uintptr, x, y int) {
	t.Helper()
	click(t, hwnd, x, y)
	ok, _, err := postMessageW.Call(hwnd, wmMouseLeave, 0, 0)
	require.NotZero(t, ok, "posting the pointer leaving: %v", err)
}

// place is the window's rectangle on the screen.
func place(t *testing.T, hwnd uintptr) rect {
	t.Helper()
	var r rect
	ok, _, err := getWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&r)))
	require.NotZero(t, ok, "reading the window's rectangle: %v", err)
	return r
}

// drawerSettle is how long the window is given to change shape after a
// click, and listSettle how long a list is given to be read and drawn.
const (
	drawerSettle = 700 * time.Millisecond
	listSettle   = 3 * time.Second
)

/*
opened asserts the window is closed's with a drawer beside it: wider by a
drawer's width, the same height, and with the cards where they were on the
screen, so one of its sides has not moved. It returns how far the cards have
moved in the picture, which is the drawer's width when it opened on the left.
*/
func opened(t *testing.T, closed, open rect, what string) (shift int) {
	t.Helper()
	grew := int(open.Right-open.Left) - int(closed.Right-closed.Left)
	require.Greater(t, grew, 100, "%s: the window did not grow a drawer: %+v then %+v", what, closed, open)
	assert.Equal(t, closed.Bottom-closed.Top, open.Bottom-open.Top, "%s: the drawer is the cards' height", what)
	assert.Equal(t, closed.Top, open.Top, what)
	if open.Left == closed.Left {
		return 0
	}
	assert.Equal(t, closed.Right, open.Right, "%s: neither side of the window stayed where it was", what)
	return int(closed.Left - open.Left)
}

// inkIn counts the lines of text in the picture's columns from x0 to x1.
func inkIn(p *picture, x0, x1 int) int {
	n := 0
	for _, line := range p.text {
		for _, w := range p.words(line) {
			if w.From >= x0 && w.To <= x1 {
				n++
				break
			}
		}
	}
	return n
}

/*
Spec 060, spec 062 R1. On the real window a click on the processor's row, the
graphics card's and the memory's each opens the cooler's drawer at the cards'
height; a second click closes it and the window's rectangle comes back; and
while the list is open, a value changing moves nothing. Which row is clicked
no longer chooses the list: the card is one target, and the list is the one
last shown.

Read off the window's rectangle and its picture. The rows are found by their
labels in the closed picture (the harness's card). Open, the window is wider
by the drawer and no taller, one of its sides has not moved, and the drawer's
columns hold lines of text. The clicks are posted to the panel's own window,
so the pointer on the desk is not touched.
*/
func TestAClickOnAnyCoolerRowOpensItsDrawerAndASecondClosesIt(t *testing.T) {
	cpuLabel, gpuLabel := processorRows(t)
	s := panelSettings{Sections: []string{"cooler"}, LHM: "http://127.0.0.1:9/data.json", Settle: coolerSettle}
	hwnd := start(t, s)
	c := cardOf(t, "cooler", s)
	p := shoot(t, hwnd, "drawer-closed.png")
	closed := place(t, hwnd)
	width := int(closed.Right - closed.Left)

	for _, label := range []string{cpuLabel, gpuLabel, "Memory"} {
		line := p.row(t, c, label)
		y := (line.From + line.To) / 2

		tap(t, hwnd, width/2, y)
		time.Sleep(drawerSettle)
		open := place(t, hwnd)
		shift := opened(t, closed, open, label)

		time.Sleep(listSettle)
		shot := shoot(t, hwnd, fmt.Sprintf("drawer-%s.png", label))
		drawerFrom, drawerTo := width, int(open.Right-open.Left)
		if shift > 0 {
			drawerFrom, drawerTo = 0, shift
		}
		assert.GreaterOrEqual(t, inkIn(shot, drawerFrom, drawerTo), 3,
			"%s: the drawer holds no list (picture %s)", label, shot.path)

		// Another reading has arrived by now: nothing moved.
		assert.Equal(t, open, place(t, hwnd), "%s: a new reading moved the window", label)

		tap(t, hwnd, shift+width/2, y)
		time.Sleep(drawerSettle)
		assert.Equal(t, closed, place(t, hwnd), "%s: the window did not come back", label)
	}
}

/*
Spec 062 R4, R6. With the cooler's drawer open, a choice's key changes what
it shows and leaves it open: the window is the same rectangle before and
after, and the picture is another. A click on another row of the card then
closes it, because the card is one target.

The key is posted to the panel's own window as a character. Not run on the
desk this was written at, which is not a Windows one.
*/
func TestAChoiceChangesTheListAndLeavesTheDrawerOpen(t *testing.T) {
	cpuLabel, _ := processorRows(t)
	s := panelSettings{Sections: []string{"cooler"}, LHM: "http://127.0.0.1:9/data.json", Settle: coolerSettle}
	hwnd := start(t, s)
	c := cardOf(t, "cooler", s)
	p := shoot(t, hwnd, "switch-closed.png")
	closed := place(t, hwnd)
	width := int(closed.Right - closed.Left)
	mid := func(label string) int { l := p.row(t, c, label); return (l.From + l.To) / 2 }

	tap(t, hwnd, width/2, mid(cpuLabel))
	time.Sleep(drawerSettle)
	open := place(t, hwnd)
	shift := opened(t, closed, open, "the cooler's")
	time.Sleep(listSettle)
	first := shoot(t, hwnd, "switch-cpu.png")

	typed(t, hwnd, 'm')
	time.Sleep(listSettle)
	assert.Equal(t, open, place(t, hwnd), "changing what is shown moved the window")
	second := shoot(t, hwnd, "switch-memory.png")
	assert.NotEqual(t, first.img.Pix, second.img.Pix, "the drawer shows what it showed before")

	tap(t, hwnd, shift+width/2, mid("Memory"))
	time.Sleep(drawerSettle)
	assert.Equal(t, closed, place(t, hwnd), "a click on another row of the card did not close the drawer")
}

// barPixels counts the pixels in a region that are a meter's fill in the
// good verdict's colour: green well clear of its red and its blue, which no
// text on the panel is.
func barPixels(img *image.NRGBA, x0, x1, y0, y1 int) int {
	n := 0
	b := img.Bounds()
	for y := max(y0, b.Min.Y); y < min(y1, b.Max.Y); y++ {
		for x := max(x0, b.Min.X); x < min(x1, b.Max.X); x++ {
			c := img.NRGBAAt(x, y)
			if int(c.G) > int(c.R)+50 && int(c.G) > int(c.B)+50 {
				n++
			}
		}
	}
	return n
}

// writeUsageBreakdown is one invented Claude account with limits and a week
// by use, behind a gate an hour away.
func writeUsageBreakdown(t *testing.T, dir string, now time.Time) {
	t.Helper()
	require.NoError(t, os.MkdirAll(dir, 0o700))
	iso := func(d time.Duration) string { return now.Add(d).UTC().Format(time.RFC3339) }
	data := fmt.Sprintf(`{
	  "five_hour": {"utilization": 12, "resets_at": %[1]q},
	  "seven_day": {"utilization": 40, "resets_at": %[2]q},
	  "limits": [
	    {"kind": "session", "percent": 12, "severity": "normal", "resets_at": %[1]q},
	    {"kind": "weekly_all", "percent": 40, "severity": "normal", "resets_at": %[2]q}
	  ],
	  "seven_day_breakdown": {"rows": [
	    {"key": "claude_code", "display_name": "Claude Code", "percent": 80},
	    {"key": "chat", "display_name": "Chats", "percent": 20}
	  ]},
	  "extra_usage": {"is_enabled": false}
	}`, iso(2*time.Hour), iso(4*24*time.Hour))
	entry := fmt.Sprintf(`{"next_attempt_at": %f, "fetched_at": %f, "data": %s}`,
		float64(now.Add(time.Hour).Unix()), float64(now.Unix()), data)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "usage-max.json"), []byte(entry), 0o600))
}

/*
Spec 060 R13, spec 062 R9. A click on a usage meter opens every account's
breakdown at the cards' height, here one account's, and a second click closes
it. The cooler is drawn beside it so the drawer has a panel's height to show
the breakdown in. A breakdown taller than the cards is held headless
(internal/gui) and has no test here.
*/
func TestAClickOnAMeterOpensTheBreakdownAndASecondClosesIt(t *testing.T) {
	now := time.Now()
	s := panelSettings{Sections: []string{"usage", "cooler"}, LHM: "http://127.0.0.1:9/data.json",
		Settle: coolerSettle, Cache: func(dir string) { writeUsageBreakdown(t, filepath.Join(dir, "claude-usage-widget"), now) }}
	hwnd := start(t, s)
	p := shoot(t, hwnd, "meter-closed.png")
	closed := place(t, hwnd)
	width := int(closed.Right - closed.Left)
	// The usage card is first, and its meter's label is the first line of
	// text the picture has in the body's size.
	require.NotEmpty(t, p.text, "picture %s", p.path)
	y := (p.text[0].From + p.text[0].To) / 2
	require.Less(t, y, int(closed.Bottom-closed.Top)/3, "the first line is not in the first card (picture %s)", p.path)
	require.Zero(t, barPixels(p.img, 0, width, y+30, int(closed.Bottom-closed.Top)),
		"the cards below the meter hold something bar-coloured, so the drawer's bars could not be told from it")

	tap(t, hwnd, width/2, y)
	time.Sleep(drawerSettle)
	open := place(t, hwnd)
	shift := opened(t, closed, open, "the meter")
	time.Sleep(time.Second)
	shot := shoot(t, hwnd, "meter-open.png")
	drawerFrom, drawerTo := width, int(open.Right-open.Left)
	if shift > 0 {
		drawerFrom, drawerTo = 0, shift
	}
	assert.GreaterOrEqual(t, inkIn(shot, drawerFrom, drawerTo), 4,
		"the drawer holds no breakdown (picture %s)", shot.path)
	// A breakdown's limits are meters, and a list of processes has none: the
	// drawer's columns hold bars in a verdict's colour.
	assert.Greater(t, barPixels(shot.img, drawerFrom, drawerTo, 0, shot.img.Bounds().Dy()), 50,
		"the drawer holds no meter (picture %s)", shot.path)

	tap(t, hwnd, shift+width/2, y)
	time.Sleep(drawerSettle)
	assert.Equal(t, closed, place(t, hwnd))
}

/*
Spec 060 R11. A click on the bandwidth card opens the network by process at
the cards' height, list or typed absence, and a second closes it. Which of
the two it shows depends on whether this run may start a trace session, and
either is a drawer with something in it.
*/
func TestAClickOnTheBandwidthCardOpensItsDrawer(t *testing.T) {
	s := panelSettings{Sections: []string{"bandwidth", "cooler"}, LHM: "http://127.0.0.1:9/data.json", Settle: coolerSettle}
	hwnd := start(t, s)
	p := shoot(t, hwnd, "bandwidth-closed.png")
	closed := place(t, hwnd)
	width := int(closed.Right - closed.Left)
	require.NotEmpty(t, p.text, "picture %s", p.path)
	y := (p.text[0].From + p.text[0].To) / 2 // the card's heading

	tap(t, hwnd, width/2, y)
	time.Sleep(drawerSettle)
	open := place(t, hwnd)
	shift := opened(t, closed, open, "the bandwidth card")
	time.Sleep(2 * listSettle)
	shot := shoot(t, hwnd, "bandwidth-open.png")
	drawerFrom, drawerTo := width, int(open.Right-open.Left)
	if shift > 0 {
		drawerFrom, drawerTo = 0, shift
	}
	assert.GreaterOrEqual(t, inkIn(shot, drawerFrom, drawerTo), 2,
		"the drawer holds neither a list nor a reason (picture %s)", shot.path)
	assert.Equal(t, open, place(t, hwnd))

	tap(t, hwnd, shift+width/2, y)
	time.Sleep(drawerSettle)
	assert.Equal(t, closed, place(t, hwnd))
}
