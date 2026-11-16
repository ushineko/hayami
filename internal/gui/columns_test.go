package gui_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/fynedesygn/glance"

	"github.com/ushineko/hayami/internal/gui"
	"github.com/ushineko/hayami/internal/view"
)

// columnsCooler is a cooler whose processor list has every column: a count,
// each program's memory and its share of all memory. scale changes every
// figure without changing a width.
func columnsCooler(scale float64) *detailSource {
	return newDetailSource("cooler", coolerSection(), map[string]view.Detail{
		"cpu": columnsList(scale),
	})
}

func columnsList(scale float64) view.Detail {
	return columnsOf(view.DetailCPU, scale)
}

func columnsOf(id string, scale float64) view.Detail {
	var uses []view.ProcessUse
	for i := range 6 {
		uses = append(uses, view.ProcessUse{
			Name:   fmt.Sprintf("p%02d", i),
			Value:  scale * float64(6-i) * (1 << 20),
			CPU:    view.Some(scale * float64(6-i)),
			Memory: view.Some(scale * float64(i+1) * (1 << 20)),
			Share:  view.Some(scale * float64(i+1) / 10),
		})
	}
	return view.Processes("cooler", id, uses)
}

// cellsIn is how many cells a drawn line carries after its figure: a line
// with cells is its name, its value and its unit, then a text per cell; a
// line without is its name and its value.
func cellsIn(line string) int { return max(0, len(strings.Split(line, "|"))-3) }

// roomy is a compositor with room for any drawer on the right.
func roomy() *compositor { return &compositor{left: 10, right: 4000, answers: true} }

// Spec 093 R1, R3, R6. With the room for it, a process list's drawer opens
// wider than the usual width and shows every column under a heading line,
// without filling; each line is as wide as the heading over it.
func TestAListOpensWideEnoughForItsColumns(t *testing.T) {
	gui.SettleRoomAtOnce(t)
	p, await := deskPanel(t, roomy(), columnsCooler(1))
	gui.Tap(p, "cooler", "", -1)
	await("every column", func() bool {
		lines := gui.DrawerLines(p)
		return len(lines) > 0 && cellsIn(lines[0]) == 3
	})
	assert.False(t, gui.Filled(p))
	widths := gui.DrawerWidths(p)
	require.Len(t, widths, 1)
	assert.Greater(t, widths[0], float32(glance.DrawerWidthInEms), "the drawer is not wider than usual")

	heads, shown := gui.DrawerHeads(p)
	require.True(t, shown, "no heading line over the columns")
	assert.Equal(t, []string{"procs", "Mem", "of", "mem"}, strings.Fields(heads))
	for _, line := range gui.DrawerLines(p) {
		value := strings.Join(strings.Split(line, "|")[1:], "")
		assert.Equal(t, len([]rune(strings.Trim(heads, "|"))), len([]rune(value)), "a line not as wide as its heading: %q", line)
	}
}

// R6. A drawer whose list has no columns is the usual width, with no heading
// line: every section but the process lists is as it was.
func TestADrawerWithoutColumnsIsTheUsualWidth(t *testing.T) {
	gui.SettleRoomAtOnce(t)
	section := coolerSection()
	section.Opens = view.DetailProcesses
	src := newDetailSource("bandwidth", section, map[string]view.Detail{
		view.DetailProcesses: processes(view.DetailProcesses, 3, 1),
	})
	p, await := deskPanel(t, roomy(), src)
	gui.Tap(p, "bandwidth", "", -1)
	await("the list", func() bool { return len(gui.DrawerLines(p)) == 3 })
	assert.Equal(t, []float32{glance.DrawerWidthInEms}, gui.DrawerWidths(p))
	_, shown := gui.DrawerHeads(p)
	assert.False(t, shown)
}

// R3. Where nothing says the room, the drawer opens at the usual width, and
// shows the columns that width fits.
func TestNoRoomKnownIsTheUsualWidth(t *testing.T) {
	gui.SettleRoomAtOnce(t)
	d := &compositor{answers: false}
	p, await := deskPanel(t, d, columnsCooler(1))
	gui.Tap(p, "cooler", "", -1)
	await("the list", func() bool { return len(gui.DrawerLines(p)) == 6 })
	assert.Equal(t, []float32{glance.DrawerWidthInEms}, gui.DrawerWidths(p))
}

// R3, R6. With less room than the list wants the drawer is as wide as the
// room, never under the usual width, and shows the columns that fit whole:
// never a later column without an earlier one, and more room never fewer.
func TestARoomShowsOnlyWhatFits(t *testing.T) {
	gui.SettleRoomAtOnce(t)
	shown := map[int]bool{}
	last := -1
	for _, room := range []float32{1, 400, 460, 520, 580, 640, 4000} {
		d := &compositor{left: 0, right: room, answers: true}
		p, await := deskPanel(t, d, columnsCooler(1))
		gui.Tap(p, "cooler", "", -1)
		await("the list", func() bool { return len(gui.DrawerLines(p)) == 6 })
		widths := gui.DrawerWidths(p)
		require.Len(t, widths, 1)
		assert.GreaterOrEqual(t, widths[0], float32(glance.DrawerWidthInEms), "room %v: under the usual width", room)

		n := cellsIn(gui.DrawerLines(p)[0])
		heads, _ := gui.DrawerHeads(p)
		want := []string{"procs", "Mem", "of mem"}[:n]
		assert.Equal(t, strings.Fields(strings.Join(want, " ")), strings.Fields(heads), "room %v: the headings are not the first %d", room, n)
		assert.GreaterOrEqual(t, n, last, "room %v: more room, fewer columns", room)
		last = n
		shown[n] = true
	}
	assert.Greater(t, len(shown), 1, "every room showed the same columns: %v", shown)
	assert.True(t, shown[3], "the most room did not show every column")
}

// R5, the glance rule. A reading that changes every figure changes neither
// the window's width, nor which columns are shown, nor where any stands.
func TestAReadingMovesNoColumn(t *testing.T) {
	gui.SettleRoomAtOnce(t)
	src := columnsCooler(1)
	p, await := deskPanel(t, roomy(), src)
	gui.Tap(p, "cooler", "", -1)
	await("columns", func() bool {
		lines := gui.DrawerLines(p)
		return len(lines) > 0 && cellsIn(lines[0]) == 3
	})
	before := gui.DrawerLines(p)
	headsBefore, _ := gui.DrawerHeads(p)
	width := gui.WindowWidth(p)

	src.set("cpu", columnsList(37))
	await("the new reading", func() bool { return gui.DrawerLines(p)[0] != before[0] })
	after := gui.DrawerLines(p)
	headsAfter, _ := gui.DrawerHeads(p)

	assert.InDelta(t, width, gui.WindowWidth(p), 0, "a reading resized the window")
	assert.Equal(t, headsBefore, headsAfter)
	require.Len(t, after, len(before))
	for i := range before {
		b, a := strings.Split(before[i], "|"), strings.Split(after[i], "|")
		require.Len(t, a, len(b), "a reading changed the columns of line %d", i)
		for j := range b {
			assert.Equal(t, len([]rune(b[j])), len([]rune(a[j])), "line %d piece %d moved: %q to %q", i, j, b[j], a[j])
		}
	}
}

// R2, R5. A drawer wants the widest of the lists its switch offers, so it
// opens at one width on any of them, and choosing another resizes nothing.
func TestTheSwitchDoesNotResizeTheDrawer(t *testing.T) {
	gui.SettleRoomAtOnce(t)
	lists := func() *detailSource {
		return newDetailSource("cooler", coolerSection(), map[string]view.Detail{
			"cpu":    columnsOf(view.DetailCPU, 1),
			"gpu":    columnsOf(view.DetailGPU, 1),
			"memory": columnsOf(view.DetailMemory, 1),
		})
	}
	p, await := deskPanel(t, roomy(), lists())
	on := func(id string) func() bool {
		return func() bool {
			s, at := gui.DrawerSubject(p)
			return gui.DrawerOpen(p) && s == "cooler" && at == id && len(gui.DrawerLines(p)) == 6
		}
	}
	gui.Tap(p, "cooler", "", -1)
	await("the CPU list", on("cpu"))
	onCPU := gui.DrawerWidths(p)

	// Reopened, the card opens on the list it last showed: Memory, whose
	// figure is the widest. The drawer is the width it was on CPU.
	gui.ChooseIn(p, 0, "memory")
	await("the memory list", on("memory"))
	gui.Tap(p, "cooler", "", -1)
	await("the drawer to close", func() bool { return !gui.DrawerOpen(p) })
	gui.Tap(p, "cooler", "", -1)
	await("the memory list again", on("memory"))
	assert.Equal(t, onCPU, gui.DrawerWidths(p), "the drawer's width depends on the list it opened on")
	width := gui.WindowWidth(p)

	for _, id := range []string{"gpu", "cpu", "memory"} {
		gui.ChooseIn(p, 0, id)
		await(id, on(id))
		assert.Equal(t, onCPU, gui.DrawerWidths(p), "choosing %s changed the drawer's width", id)
		assert.InDelta(t, width, gui.WindowWidth(p), 0, "choosing %s resized the window", id)
	}
}

// R4. At the cap, a process list given to the outermost drawer, which had a
// list without columns, takes its want when the room beyond allows it.
func TestTheCapsReplacementTakesItsWant(t *testing.T) {
	gui.SettleRoomAtOnce(t)
	section := coolerSection()
	section.Opens = view.DetailProcesses
	bandwidth := newDetailSource("bandwidth", section, map[string]view.Detail{
		view.DetailProcesses: processes(view.DetailProcesses, 3, 1),
	})
	cooler := newDetailSource("cooler", coolerSection(), map[string]view.Detail{
		"cpu":    columnsOf(view.DetailCPU, 1),
		"memory": columnsOf(view.DetailMemory, 1),
	})
	d := roomy()
	p, await := deskPanel(t, d, cooler, bandwidth)
	usual := gui.DrawerReach(p)

	gui.Tap(p, "cooler", "", -1)
	await("the CPU list", func() bool { return len(gui.DrawerLines(p)) == 6 })
	want := gui.DrawerWidths(p)[0]
	// The text size and the gap, from the first drawer at two widths.
	em := (gui.DrawerReach(p) - usual) / (want - glance.DrawerWidthInEms)
	gap := usual - glance.DrawerWidthInEms*em

	gui.ShiftTapSection(p, "bandwidth")
	await("two drawers", func() bool { return len(gui.DrawerWidths(p)) == 2 })
	require.Equal(t, []float32{want, glance.DrawerWidthInEms}, gui.DrawerWidths(p))

	// The cap: no room for a drawer of the usual width, room enough beyond
	// the outermost for the difference between it and the want.
	d.mu.Lock()
	d.right = (want-glance.DrawerWidthInEms)*em + gap + 1
	d.mu.Unlock()
	gui.ShiftChooseIn(p, 0, "memory")
	await("Memory in the outermost", func() bool {
		ws := gui.DrawerWidths(p)
		return len(ws) == 2 && ws[1] != glance.DrawerWidthInEms
	})
	assert.Equal(t, []float32{want, want}, gui.DrawerWidths(p), "the replacement did not take its want")
}
