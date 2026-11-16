package view_test

import (
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/hayami/internal/view"
)

func columnUses() []view.ProcessUse {
	return []view.ProcessUse{
		{Name: "browser", Value: 3, CPU: view.Some(3.0), Memory: view.Some(2.0 * (1 << 30)), Share: view.Some(6.25)},
		{Name: "browser", Value: 1, CPU: view.Some(1.0), Memory: view.Some(1.0 * (1 << 30)), Share: view.Some(3.125)},
		{Name: "idle", Value: 0},
	}
}

// Spec 093 R1. A list's want is its figure and every column its kind can
// carry, which is a line's widest: the same whatever the list reads, and
// none for a list without columns.
func TestAListWantsItsWidestLine(t *testing.T) {
	for _, id := range []string{view.DetailCPU, view.DetailGPU, view.DetailMemory} {
		big := view.ProcessUse{Name: "x", Value: 999 * (1 << 30), Count: 999,
			CPU: view.Some(999.9), Memory: view.Some(999.0 * (1 << 30)), Share: view.Some(99.9)}
		small := view.ProcessUse{Name: "y", Value: 0.1, CPU: view.Some(0.0), Memory: view.Some(1.0), Share: view.Some(0.0)}
		for _, uses := range [][]view.ProcessUse{{big}, {small}} {
			d := view.Processes("cooler", id, uses)
			require.NotEmpty(t, d.Columns, id)
			got := view.ColumnsWidth(view.FigureWidth(d.Lines), d.Columns, len(d.Columns))
			assert.Equal(t, view.ColumnsWant(id), got, "%s: the want is not a line with every column", id)
		}
	}
	assert.Zero(t, view.ColumnsWant(view.DetailProcesses))
	assert.Zero(t, view.ColumnsWant(view.DetailPeers))
}

// Spec 092 R1, R2. Uses of one name are one line of their count and their
// figures summed; a use with no other figure is the empty form at the same
// width, never a zero.
func TestAListSumsItsColumns(t *testing.T) {
	d := view.Processes("cooler", view.DetailCPU, columnUses())

	require.Len(t, d.Columns, 3)
	require.Equal(t, "browser", d.Lines[0].Name)
	cells := d.Lines[0].Cells
	assert.Equal(t, "×2", strings.TrimSpace(cells[0]))
	assert.Equal(t, "3.0 GiB", strings.TrimSpace(cells[1]))
	assert.Equal(t, "9.4 %", strings.TrimSpace(cells[2]))

	idle := d.Lines[1].Cells
	assert.Equal(t, "×1", strings.TrimSpace(idle[0]))
	assert.Equal(t, view.Blank, strings.TrimSpace(idle[1]))
	assert.Equal(t, view.Blank, strings.TrimSpace(idle[2]))
	for i, c := range d.Columns {
		assert.Equal(t, c.Width, utf8.RuneCountInString(cells[i]), "a cell not at its column's width")
		assert.Equal(t, c.Width, utf8.RuneCountInString(idle[i]), "an empty cell not at its column's width")
	}
}

// R1. A list with no other figure offers the count alone; a bandwidth list,
// whose kind has no columns, offers none.
func TestOnlyTheColumnsTheUsesCarryAreOffered(t *testing.T) {
	d := view.Processes("cooler", view.DetailCPU, []view.ProcessUse{{Name: "a", Value: 1}})
	require.Len(t, d.Columns, 1)
	assert.Equal(t, view.ColumnCount, d.Columns[0].Key)

	net := view.Processes("net", view.DetailProcesses, []view.ProcessUse{{Name: "a", Value: 1, Second: 2}})
	assert.Empty(t, net.Columns)
	assert.Empty(t, net.Lines[0].Cells)
}

// R5, R6. A line with n cells is as wide as the heading over n columns, so
// in a right-aligned run each heading stands over its cells; with none it is
// the line it always was.
func TestTheHeadingIsAsWideAsItsLine(t *testing.T) {
	d := view.Processes("cooler", view.DetailCPU, columnUses())
	figure := view.FigureWidth(d.Lines)
	for n := 0; n <= len(d.Columns); n++ {
		row := d.Lines[0].Row(n)
		value := row.Value
		if n == 0 {
			assert.Equal(t, d.Lines[0].Value, row.Value)
			assert.Equal(t, d.Lines[0].Unit, row.Unit)
			value += " " + row.Unit
		}
		heads := view.ColumnHeads(figure, d.Columns, n)
		assert.Equal(t, utf8.RuneCountInString(heads), utf8.RuneCountInString(value), "n=%d", n)
		assert.Equal(t, view.ColumnsWidth(figure, d.Columns, n), utf8.RuneCountInString(value), "n=%d", n)
	}
}

// R9. The JSON carries the columns and each line's cells.
func TestTheColumnsAreInTheJSON(t *testing.T) {
	d := view.Processes("cooler", view.DetailMemory, columnUses())
	raw, err := json.Marshal(d)
	require.NoError(t, err)

	var back struct {
		Columns []struct{ Key, Heading string } `json:"columns"`
		Lines   []struct {
			Name  string   `json:"name"`
			Cells []string `json:"cells"`
		} `json:"lines"`
	}
	require.NoError(t, json.Unmarshal(raw, &back))
	require.Len(t, back.Columns, 3)
	assert.Equal(t, view.ColumnCount, back.Columns[0].Key)
	assert.Equal(t, "procs", back.Columns[0].Heading)
	require.NotEmpty(t, back.Lines)
	assert.Len(t, back.Lines[0].Cells, 3)
}
