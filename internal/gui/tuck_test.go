package gui_test

import (
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/ushineko/hayami/internal/gui"
)

// Spec 091: the panel tucks into a corner.

// tucked tucks a panel over a compositor and waits for it.
func tucked(t *testing.T, p *gui.Panel, await func(string, func() bool)) {
	t.Helper()
	gui.AskTuck(p)
	await("the panel to tuck", func() bool { return gui.Tucked(p) })
}

// R1, R3. A click on the glyph tucks into the corner the room gives; while
// tucked neither the cards' sections nor the drawers' details are read.
func TestTuckingStopsEveryPoll(t *testing.T) {
	gui.SettleRoomAtOnce(t)
	d := &compositor{left: 5000, right: 10, answers: true}
	cooler := coolerDetails()
	p, await := deskPanel(t, d, cooler)
	gui.Tap(p, "cooler", "", -1)
	await("the drawer", func() bool { return gui.DrawerOpen(p) })
	assert.True(t, polling(cooler, "cpu"))
	assert.True(t, gui.Shown(p, "cooler"))

	tucked(t, p, await)
	d.mu.Lock()
	corners := append([]bool(nil), d.corners...)
	d.mu.Unlock()
	// The drawer is on the left, so the corner is the right one.
	assert.Equal(t, []bool{true}, corners, "the corner held is not the one away from the drawers")
	assert.False(t, polling(cooler, "cpu"), "a tucked panel's drawer went on being read")
	assert.False(t, gui.Shown(p, "cooler"), "a tucked panel's section went on being read")
}

// R2, R3, R4. A click on the square brings the panel back: every shown
// section is woken to read at once, and the drawers are back and read.
func TestUntuckingReadsAtOnceAndTheDrawersComeBack(t *testing.T) {
	gui.SettleRoomAtOnce(t)
	d := &compositor{left: 10, right: 5000, answers: true}
	cooler, net := coolerDetails(), netDetails()
	p, await := deskPanel(t, d, cooler, net)
	gui.Tap(p, "cooler", "", -1)
	await("one", func() bool { return gui.DrawerOpen(p) })
	gui.ShiftTapSection(p, "net")
	await("two", func() bool { return len(gui.OpenDrawers(p)) == 2 })
	before := gui.OpenDrawers(p)

	tucked(t, p, await)
	for _, key := range []string{"cooler", "net"} {
		gui.Woken(p, key) // what the openings left
	}
	gui.Untuck(p)
	await("back", func() bool { return !gui.Tucked(p) })
	assert.Equal(t, before, gui.OpenDrawers(p), "the drawers did not come back as they were")
	assert.True(t, gui.Shown(p, "cooler"))
	assert.True(t, gui.Woken(p, "cooler"), "the section was not woken to read at once")
	assert.True(t, gui.Woken(p, "net"))
	assert.True(t, polling(cooler, "cpu"), "the drawer was not read again")
	assert.True(t, polling(net, "quotas"))
}

// R5. A place reported while tucked is the square's and is not saved.
func TestATuckedPlaceIsNotSaved(t *testing.T) {
	gui.SettleRoomAtOnce(t)
	s := store(t)
	d := &compositor{left: 10, right: 5000, answers: true}
	p, await := deskPanel(t, d, coolerDetails())
	gui.Moved(p, s, 1200, 800, int(gui.CardsWidth(p)+0.5))
	await("the move", func() bool { return true })

	tucked(t, p, await)
	gui.Moved(p, s, 1400, 1100, 42)
	await("the move", func() bool { return true })
	assert.Equal(t, [2]int{1200, 800}, placed(s), "the square's place was saved as the panel's")
}

// R6. Where nothing can hold the corner the glyph is not offered: a panel
// without a compositor to ask, on a desktop whose window does not move
// itself.
func TestNoGlyphWithoutAHeldCorner(t *testing.T) {
	p, _ := drawerPanel(t, coolerDetails())
	assert.Equal(t, runtime.GOOS == "windows", gui.TuckOffered(p))
}
