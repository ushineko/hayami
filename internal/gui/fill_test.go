package gui_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/fynedesygn/glance"
	"github.com/ushineko/hayami/internal/gui"
)

// Spec 090: the drawers fill the room.

// setRoom changes the room the compositor reports.
func (d *compositor) setRoom(left, right float32) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.left, d.right = left, right
}

// R1. At the cap, where the compositor says another drawer would not fit,
// the outermost offers Fill; with room for another it does not.
func TestTheFillButtonAtTheCap(t *testing.T) {
	gui.SettleRoomAtOnce(t)
	d := &compositor{left: 10, right: 5000, answers: true}
	p, await := deskPanel(t, d, coolerDetails())

	gui.Tap(p, "cooler", "", -1)
	await("the drawer", func() bool { return gui.DrawerOpen(p) })
	// Watched on this goroutine and not with assert.Never, whose condition
	// runs on one of its own: a tick still running the queued draws when
	// Never returned raced the draws below.
	for end := time.Now().Add(100 * time.Millisecond); time.Now().Before(end); time.Sleep(5 * time.Millisecond) {
		await("draws", func() bool { return true })
		require.False(t, gui.FillOffered(p), "Fill offered with room for another drawer")
	}

	// No room for another: the Shift-click puts the next detail in the
	// outermost, and the row is at its cap.
	d.setRoom(10, 10)
	gui.ShiftTapSection(p, "cooler")
	await("the outermost replaced", func() bool { return assert.ObjectsAreEqual([]string{"cooler/gpu"}, gui.OpenDrawers(p)) })
	await("Fill at the cap", func() bool { return gui.FillOffered(p) })
}

// R1. Where nothing reports the room there is no edge to fill to.
func TestNoFillWithoutTheRoom(t *testing.T) {
	gui.SettleRoomAtOnce(t)
	p, await := drawerPanel(t, coolerDetails())
	for range glance.DrawersWithoutRoom + 1 {
		gui.ShiftTapSection(p, "cooler")
	}
	await("the row at its cap", func() bool { return len(gui.OpenDrawers(p)) == glance.DrawersWithoutRoom })
	assert.False(t, gui.FillOffered(p))
}

// R2. A press fills, a second unfills; the cards keep their width.
func TestFillAndUnfill(t *testing.T) {
	gui.SettleRoomAtOnce(t)
	d := &compositor{left: 10, right: 40, answers: true}
	p, await := deskPanel(t, d, coolerDetails())
	gui.Tap(p, "cooler", "", -1)
	await("Fill", func() bool { return gui.FillOffered(p) })
	cards, reach := gui.CardsWidth(p), gui.DrawerReach(p)

	gui.PressFill(p)
	await("filled", func() bool { return gui.Filled(p) })
	assert.InDelta(t, reach+40, gui.DrawersWidth(p), 1, "the drawer did not take the room")
	assert.InDelta(t, cards, gui.CardsWidth(p), 0.01)
	assert.True(t, gui.FillOffered(p), "a filled row stopped offering the way back")

	gui.PressFill(p)
	await("unfilled", func() bool { return !gui.Filled(p) })
	assert.InDelta(t, reach, gui.DrawersWidth(p), 0.01)
}

// R3. At the cap, a Shift-click replaces the outermost and the row stays
// filled; a plain click closing all ends it.
func TestReplacingKeepsItFilledAndClosingEndsIt(t *testing.T) {
	gui.SettleRoomAtOnce(t)
	d := &compositor{left: 10, right: 40, answers: true}
	p, await := deskPanel(t, d, coolerDetails(), netDetails())
	gui.Tap(p, "cooler", "", -1)
	await("Fill", func() bool { return gui.FillOffered(p) })
	gui.PressFill(p)
	await("filled", func() bool { return gui.Filled(p) })

	d.setRoom(10, 0)
	gui.ShiftTapSection(p, "net")
	await("net in the outermost", func() bool { return assert.ObjectsAreEqual([]string{"net/quotas"}, gui.OpenDrawers(p)) })
	assert.True(t, gui.Filled(p), "replacing the outermost ended filling")

	gui.Tap(p, "net", "", -1)
	await("closed", func() bool { return len(gui.OpenDrawers(p)) == 0 })
	assert.False(t, gui.Filled(p))
	assert.False(t, gui.FillOffered(p))
}

// R3. A plain click with several out closes them all and Fill goes with
// them.
func TestClosingSeveralEndsFilling(t *testing.T) {
	gui.SettleRoomAtOnce(t)
	d := &compositor{left: 10, right: 5000, answers: true}
	p, await := deskPanel(t, d, coolerDetails(), netDetails())
	gui.Tap(p, "cooler", "", -1)
	await("one", func() bool { return gui.DrawerOpen(p) })
	gui.ShiftTapSection(p, "net")
	await("two", func() bool { return len(gui.OpenDrawers(p)) == 2 })
	// Now no room for a third: the next Shift-click replaces the
	// outermost, and the row is at its cap.
	d.setRoom(10, 40)
	gui.ShiftTapSection(p, "cooler")
	await("the outermost replaced", func() bool {
		return assert.ObjectsAreEqual([]string{"cooler/cpu", "cooler/gpu"}, gui.OpenDrawers(p))
	})
	await("Fill", func() bool { return gui.FillOffered(p) })
	gui.PressFill(p)
	await("filled", func() bool { return gui.Filled(p) })

	gui.Tap(p, "cooler", "", -1)
	await("closed", func() bool { return len(gui.OpenDrawers(p)) == 0 })
	assert.False(t, gui.Filled(p))
	assert.False(t, gui.FillOffered(p), "Fill is still offered with no drawer out")
}

// R5. With a filled row on the left the place saved is the cards'.
func TestThePlaceIsTheCardsWithAFilledRow(t *testing.T) {
	gui.SettleRoomAtOnce(t)
	s := store(t)
	d := &compositor{left: 5000, right: 10, answers: true}
	p, await := deskPanel(t, d, coolerDetails())
	moved := func(x, y, width int) {
		gui.Moved(p, s, x, y, width)
		await("the move to be taken", func() bool { return true })
	}
	cards := int(gui.CardsWidth(p) + 0.5)
	moved(3000, 2037, cards)

	gui.Tap(p, "cooler", "", -1)
	await("one", func() bool { return gui.DrawerOpen(p) })
	require.Equal(t, glance.Left, gui.DrawerOnSide(p))
	d.setRoom(37, 10)
	gui.ShiftTapSection(p, "cooler")
	await("Fill", func() bool { return gui.FillOffered(p) })
	gui.PressFill(p)
	await("filled", func() bool { return gui.Filled(p) })

	extra := int(gui.DrawersWidth(p) + 0.5)
	moved(3000-extra, 2037, cards+extra)
	assert.Equal(t, [2]int{3000, 2037}, placed(s), "the filled row's corner was saved as the panel's place")
}
