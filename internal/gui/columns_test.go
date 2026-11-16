package gui_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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
	var uses []view.ProcessUse
	for i := range 6 {
		uses = append(uses, view.ProcessUse{
			Name:   fmt.Sprintf("p%02d", i),
			Value:  scale * float64(6-i),
			CPU:    view.Some(scale * float64(6-i)),
			Memory: view.Some(scale * float64(i+1) * (1 << 20)),
			Share:  view.Some(scale * float64(i+1) / 10),
		})
	}
	return view.Processes("cooler", view.DetailCPU, uses)
}

// cellsIn is how many cells a drawn line carries after its figure: a line
// with cells is its name, its value and its unit, then a text per cell; a
// line without is its name and its value.
func cellsIn(line string) int { return max(0, len(strings.Split(line, "|"))-3) }

// Spec 092 R7. A drawer at its usual width is the drawer it was: no column
// and no heading line, though the detail has columns to give.
func TestAUsualDrawerIsUnchanged(t *testing.T) {
	gui.SettleRoomAtOnce(t)
	d := &compositor{left: 10, right: 400, answers: true}
	p, await := deskPanel(t, d, columnsCooler(1))
	gui.Tap(p, "cooler", "", -1)
	await("the list", func() bool { return len(gui.DrawerLines(p)) == 6 })

	for _, line := range gui.DrawerLines(p) {
		assert.Equal(t, 0, cellsIn(line), "a column on a drawer at its usual width: %q", line)
	}
	_, shown := gui.DrawerHeads(p)
	assert.False(t, shown, "a heading line over a list with no columns shown")
}

// R5, R6, R8. Filled with the room to spare, every column is shown under a
// heading line naming each; unfilled, they go.
func TestFillingShowsTheColumns(t *testing.T) {
	gui.SettleRoomAtOnce(t)
	d := &compositor{left: 10, right: 400, answers: true}
	p, await := deskPanel(t, d, columnsCooler(1))
	gui.Tap(p, "cooler", "", -1)
	await("Fill", func() bool { return gui.FillOffered(p) })

	gui.PressFill(p)
	await("every column", func() bool {
		lines := gui.DrawerLines(p)
		return len(lines) > 0 && cellsIn(lines[0]) == 3
	})
	heads, shown := gui.DrawerHeads(p)
	require.True(t, shown, "no heading line over the columns")
	assert.Equal(t, []string{"procs", "Mem", "of", "mem"}, strings.Fields(heads))
	// Each line is as wide as the heading, so in a right-aligned run the
	// headings stand over their cells.
	for _, line := range gui.DrawerLines(p) {
		value := strings.Join(strings.Split(line, "|")[1:], "")
		assert.Equal(t, len([]rune(strings.Trim(heads, "|"))), len([]rune(value)), "a line not as wide as its heading: %q", line)
	}

	gui.PressFill(p)
	await("no column", func() bool {
		lines := gui.DrawerLines(p)
		return len(lines) > 0 && cellsIn(lines[0]) == 0
	})
	_, shown = gui.DrawerHeads(p)
	assert.False(t, shown)
}

// R5. Filled with little room, the columns that fit whole and none after the
// first that does not: never a later column without an earlier one.
func TestANarrowFillShowsOnlyWhatFits(t *testing.T) {
	gui.SettleRoomAtOnce(t)
	shown := map[int]bool{}
	for _, room := range []float32{1, 30, 60, 120, 400} {
		d := &compositor{left: 0, right: room, answers: true}
		p, await := deskPanel(t, d, columnsCooler(1))
		gui.Tap(p, "cooler", "", -1)
		await("Fill", func() bool { return gui.FillOffered(p) })
		gui.PressFill(p)
		await("filled", func() bool { return gui.Filled(p) })
		lines := gui.DrawerLines(p)
		require.NotEmpty(t, lines)
		n := cellsIn(lines[0])
		heads, _ := gui.DrawerHeads(p)
		want := []string{"procs", "Mem", "of mem"}[:n]
		assert.Equal(t, strings.Fields(strings.Join(want, " ")), strings.Fields(heads), "room %v: the headings are not the first %d", room, n)
		shown[n] = true
	}
	assert.Greater(t, len(shown), 1, "every room showed the same columns: %v", shown)
}

// R5, the glance rule. At one width, a reading that changes every figure
// changes neither which columns are shown nor where any of them stands.
func TestAReadingMovesNoColumn(t *testing.T) {
	gui.SettleRoomAtOnce(t)
	d := &compositor{left: 10, right: 400, answers: true}
	src := columnsCooler(1)
	p, await := deskPanel(t, d, src)
	gui.Tap(p, "cooler", "", -1)
	await("Fill", func() bool { return gui.FillOffered(p) })
	gui.PressFill(p)
	await("columns", func() bool {
		lines := gui.DrawerLines(p)
		return len(lines) > 0 && cellsIn(lines[0]) == 3
	})
	before := gui.DrawerLines(p)
	headsBefore, _ := gui.DrawerHeads(p)

	src.set("cpu", columnsList(37))
	await("the new reading", func() bool { return gui.DrawerLines(p)[0] != before[0] })
	after := gui.DrawerLines(p)
	headsAfter, _ := gui.DrawerHeads(p)

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
