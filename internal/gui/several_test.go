package gui_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/fynedesygn/glance"

	"github.com/ushineko/hayami/internal/config"
	"github.com/ushineko/hayami/internal/gui"
	"github.com/ushineko/hayami/internal/view"
)

// Spec 088: several drawers at once.

// netDetails is a section with one detail, which is no choice of a switch.
func netDetails() *detailSource {
	return newDetailSource("net", view.Section{Title: "Net", Opens: "quotas",
		Rows: []view.Row{{ID: "eth0", Label: "eth0", Value: "1"}}},
		map[string]view.Detail{"quotas": {Section: "net", ID: "quotas", Title: "Quotas",
			Lines: []view.DetailLine{{Name: "a", Value: "1"}, {Name: "b", Value: "2"}}}})
}

// polling reports whether a detail is still being read: its count moves
// over a few of its intervals.
func polling(src *detailSource, id string) bool {
	time.Sleep(20 * time.Millisecond)
	before, _ := src.count(id)
	time.Sleep(60 * time.Millisecond)
	after, _ := src.count(id)
	return after > before
}

// R3. A Shift-click on another card opens a second drawer; both are read.
func TestAShiftClickOpensASecondDrawer(t *testing.T) {
	cooler, net := coolerDetails(), netDetails()
	p, await := drawerPanel(t, cooler, net)

	gui.Tap(p, "cooler", "", -1)
	await("the first drawer", func() bool { return gui.DrawerOpen(p) })
	gui.ShiftTapSection(p, "net")
	await("the second drawer", func() bool { return len(gui.OpenDrawers(p)) == 2 })

	assert.Equal(t, []string{"cooler/cpu", "net/quotas"}, gui.OpenDrawers(p))
	assert.Equal(t, 2, gui.OpenedInLibrary(p))
	assert.True(t, polling(cooler, "cpu"), "the first drawer stopped being read")
	assert.True(t, polling(net, "quotas"), "the second drawer is not read")
}

// R3, R4. Shift-clicks on the cooler open its three lists in the order its
// switch reads, and a fourth closes the one opened last.
func TestShiftClicksOnACardOpenItsDetailsInTurn(t *testing.T) {
	cooler := coolerDetails()
	p, await := drawerPanel(t, cooler)

	for n := 1; n <= 3; n++ {
		gui.ShiftTapSection(p, "cooler")
		await("a drawer more", func() bool { return len(gui.OpenDrawers(p)) == n })
		if n == 2 {
			// Where nothing can say the room, the row stops at two: the third
			// takes the outermost's place.
			break
		}
	}
	assert.Equal(t, []string{"cooler/cpu", "cooler/gpu"}, gui.OpenDrawers(p))

	gui.ShiftTapSection(p, "cooler")
	await("the outermost to take memory", func() bool {
		o := gui.OpenDrawers(p)
		return len(o) == 2 && o[1] == "cooler/memory"
	})
	assert.Equal(t, glance.DrawersWithoutRoom, gui.OpenedInLibrary(p), "the cap was passed")
	assert.False(t, polling(cooler, "gpu"), "the replaced list went on being read")
	assert.True(t, polling(cooler, "memory"))

	// The lists out are processor and memory, so the next is the card.
	gui.ShiftTapSection(p, "cooler")
	await("the card's list in the outermost", func() bool { return gui.OpenDrawers(p)[1] == "cooler/gpu" })
}

// R3, R4 with room for three: the cooler's lists in three drawers, and a
// fourth Shift-click closes memory's.
func TestThreeListsWhereThereIsRoom(t *testing.T) {
	d := &compositor{left: 10, right: 5000, answers: true}
	cooler := coolerDetails()
	p, await := deskPanel(t, d, cooler)

	for _, want := range [][]string{
		{"cooler/cpu"},
		{"cooler/cpu", "cooler/gpu"},
		{"cooler/cpu", "cooler/gpu", "cooler/memory"},
		{"cooler/cpu", "cooler/gpu"},
	} {
		gui.ShiftTapSection(p, "cooler")
		await("the drawers", func() bool { return assert.ObjectsAreEqual(want, gui.OpenDrawers(p)) })
	}
	assert.Equal(t, 2, gui.OpenedInLibrary(p))
	assert.False(t, polling(cooler, "memory"), "a closed drawer went on being read")
}

// R9. Where the compositor says there is no room for another, the outermost
// shows the new detail and the window keeps its width.
func TestNoRoomReplacesTheOutermost(t *testing.T) {
	d := &compositor{left: 10, right: 10, answers: true}
	cooler, net := coolerDetails(), netDetails()
	p, await := deskPanel(t, d, cooler, net)

	gui.Tap(p, "cooler", "", -1)
	await("the first drawer", func() bool { return gui.DrawerOpen(p) })
	gui.ShiftTapSection(p, "net")
	await("net in the outermost", func() bool { return assert.ObjectsAreEqual([]string{"net/quotas"}, gui.OpenDrawers(p)) })
	assert.Equal(t, 1, gui.OpenedInLibrary(p))
	assert.False(t, polling(cooler, "cpu"))
}

// R4. A Shift-click on a one-detail card whose drawer is out closes only
// that drawer; the other goes on being read.
func TestAShiftClickOnAnOpenCardClosesOnlyItsDrawer(t *testing.T) {
	cooler, net := coolerDetails(), netDetails()
	p, await := drawerPanel(t, cooler, net)
	gui.Tap(p, "net", "", -1)
	await("net", func() bool { return gui.DrawerOpen(p) })
	gui.ShiftTapSection(p, "cooler")
	await("two", func() bool { return len(gui.OpenDrawers(p)) == 2 })

	gui.ShiftTapSection(p, "net")
	await("net to close", func() bool { return assert.ObjectsAreEqual([]string{"cooler/cpu"}, gui.OpenDrawers(p)) })
	assert.False(t, polling(net, "quotas"))
	assert.True(t, polling(cooler, "cpu"))
}

// R2. A plain click on any card with two out closes both and stops both
// polls; one with one out is spec 062's.
func TestAPlainClickWithSeveralOutClosesThemAll(t *testing.T) {
	cooler, net := coolerDetails(), netDetails()
	p, await := drawerPanel(t, cooler, net)
	gui.Tap(p, "cooler", "", -1)
	await("one", func() bool { return gui.DrawerOpen(p) })
	gui.ShiftTapSection(p, "net")
	await("two", func() bool { return len(gui.OpenDrawers(p)) == 2 })

	gui.Tap(p, "net", "", -1)
	await("none", func() bool { return len(gui.OpenDrawers(p)) == 0 })
	assert.Zero(t, gui.OpenedInLibrary(p))
	assert.False(t, polling(cooler, "cpu"))
	assert.False(t, polling(net, "quotas"))

	// One out: a click on another card changes it.
	gui.Tap(p, "cooler", "", -1)
	await("cooler", func() bool { return gui.DrawerOpen(p) })
	gui.Tap(p, "net", "", -1)
	await("net in its place", func() bool { return assert.ObjectsAreEqual([]string{"net/quotas"}, gui.OpenDrawers(p)) })
}

// R5. A Shift-click on a choice opens it in a drawer of its own, and again
// closes that drawer; on the drawer's own choice it closes that drawer.
func TestAShiftClickOnAChoiceOpensItBeside(t *testing.T) {
	cooler := coolerDetails()
	p, await := drawerPanel(t, cooler)
	gui.Tap(p, "cooler", "", -1)
	await("cpu", func() bool { return gui.DrawerOpen(p) })

	gui.ShiftChooseIn(p, 0, "gpu")
	await("gpu beside", func() bool {
		return assert.ObjectsAreEqual([]string{"cooler/cpu", "cooler/gpu"}, gui.OpenDrawers(p))
	})
	gui.ShiftChooseIn(p, 0, "gpu")
	await("gpu to close", func() bool { return assert.ObjectsAreEqual([]string{"cooler/cpu"}, gui.OpenDrawers(p)) })

	gui.ShiftChooseIn(p, 0, "cpu")
	await("cpu to close itself", func() bool { return len(gui.OpenDrawers(p)) == 0 })
}

// R6. A plain click on a choice another drawer shows moves it here and
// closes the other.
func TestAChoiceOpenElsewhereMovesHere(t *testing.T) {
	cooler := coolerDetails()
	p, await := drawerPanel(t, cooler)
	gui.ShiftTapSection(p, "cooler")
	gui.ShiftTapSection(p, "cooler")
	await("cpu and gpu", func() bool {
		return assert.ObjectsAreEqual([]string{"cooler/cpu", "cooler/gpu"}, gui.OpenDrawers(p))
	})

	gui.ChooseIn(p, 0, "gpu")
	await("gpu in the first, alone", func() bool { return assert.ObjectsAreEqual([]string{"cooler/gpu"}, gui.OpenDrawers(p)) })
}

// R10. Pausing one drawer leaves the other's poll running.
func TestPausingOneDrawerLeavesTheOther(t *testing.T) {
	cooler, net := coolerDetails(), netDetails()
	p, await := drawerPanel(t, cooler, net)
	gui.Tap(p, "cooler", "", -1)
	await("one", func() bool { return gui.DrawerOpen(p) })
	gui.ShiftTapSection(p, "net")
	await("two", func() bool { return len(gui.OpenDrawers(p)) == 2 })
	await("two readings each", func() bool {
		a, _ := cooler.count("cpu")
		b, _ := net.count("quotas")
		return a >= 2 && b >= 2
	})

	gui.PauseIn(p, 0)
	assert.False(t, polling(cooler, "cpu"), "a paused drawer went on reading")
	assert.True(t, polling(net, "quotas"), "pausing one drawer paused the other")
}

// R11. Hiding a section closes its drawers and leaves the others out.
func TestHidingASectionClosesOnlyItsDrawers(t *testing.T) {
	cooler, net := coolerDetails(), netDetails()
	p, await := drawerPanel(t, cooler, net)
	gui.Tap(p, "cooler", "", -1)
	await("one", func() bool { return gui.DrawerOpen(p) })
	gui.ShiftTapSection(p, "net")
	await("two", func() bool { return len(gui.OpenDrawers(p)) == 2 })

	c := config.Default()
	c.Sections = []string{"net", "tall"}
	p.Apply(c)
	assert.Equal(t, []string{"net/quotas"}, gui.OpenDrawers(p))
	assert.False(t, polling(cooler, "cpu"))
}

// R14. The place remembered is the cards' with two drawers out on the left,
// as with one.
func TestThePlaceIsTheCardsWithTwoDrawersOnTheLeft(t *testing.T) {
	s := store(t)
	d := &compositor{left: 5000, right: 10, answers: true}
	p, await := deskPanel(t, d, coolerDetails(), netDetails())
	moved := func(x, y, width int) {
		gui.Moved(p, s, x, y, width)
		await("the move to be taken", func() bool { return true })
	}
	cards, reach := int(gui.CardsWidth(p)+0.5), int(gui.DrawerReach(p)+0.5)
	moved(3700, 2037, cards)

	gui.Tap(p, "cooler", "", -1)
	await("one", func() bool { return gui.DrawerOpen(p) })
	require.Equal(t, glance.Left, gui.DrawerOnSide(p))
	gui.ShiftTapSection(p, "net")
	await("two", func() bool { return len(gui.OpenDrawers(p)) == 2 })

	moved(3700-2*reach, 2037, cards+2*reach)
	assert.Equal(t, [2]int{3700, 2037}, placed(s), "two drawers' corner was saved as the panel's place")
	moved(3700-reach, 2037, cards+reach)
	assert.Equal(t, [2]int{3700, 2037}, placed(s))

	// Half a drawer more is a width somebody dragged.
	moved(2500, 2100, cards+reach+reach/2)
	assert.Equal(t, [2]int{2500, 2100}, placed(s))
}
