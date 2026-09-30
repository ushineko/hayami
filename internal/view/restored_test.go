package view_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/hayami/internal/view"
)

// painted records what each piece of text was painted as.
func painted(t *testing.T, s view.Section) map[view.Status][]string {
	t.Helper()
	seen := map[view.Status][]string{}
	view.RenderWith([]view.Section{s}, view.ArrangeStack, 40,
		func(text string, st view.Status) string {
			seen[st] = append(seen[st], text)
			return text
		})
	return seen
}

// AC1, AC3. A restored section is drawn dim, verdict and all: its readings
// are the last ones heard and a warning nobody is refreshing should not keep
// shouting.
func TestARestoredSectionIsDrawnDim(t *testing.T) {
	s := view.Peripherals(view.PeripheralsReading{
		Devices: []view.PeripheralReading{{Name: "G502 X PLUS", Level: 15, Kind: view.KindMouse}},
	})
	require.Len(t, s.Cells, view.PeripheralSlots)
	require.Equal(t, view.Bad, s.Cells[0].Status, "the reading must be one that would be coloured")

	s.Restored = true
	seen := painted(t, s)

	assert.Contains(t, strings.Join(seen[view.Dim], ""), "15")
	assert.NotContains(t, strings.Join(seen[view.Bad], ""), "15",
		"a restored reading kept its verdict colour")
}

// AC6. And it does not call itself unavailable. That is what a section whose
// source has stopped answering says; a panel that has only just started has
// not asked yet.
func TestARestoredSectionDoesNotSayUnavailable(t *testing.T) {
	s := view.Section{Key: "cooler", Title: "Cooler", Restored: true,
		Rows: []view.Row{{Label: "Coolant", Value: "40", Unit: "°C"}}}

	lines := strings.Join(view.Render([]view.Section{s}, view.ArrangeStack, 40), "\n")
	assert.Contains(t, lines, "Cooler")
	assert.NotContains(t, lines, "unavailable")
}

// AC6. A section whose source has stopped still does.
func TestAGoneSectionStillSaysUnavailable(t *testing.T) {
	s := view.Section{Key: "cooler", Title: "Cooler", Gone: true,
		Rows: []view.Row{{Label: "Coolant", Value: "40", Unit: "°C"}}}

	lines := strings.Join(view.Render([]view.Section{s}, view.ArrangeStack, 40), "\n")
	assert.Contains(t, lines, "unavailable")
}

// AC3. A live reading is not dim: the dimming stops when the panel hears
// something.
func TestALiveSectionIsNotDim(t *testing.T) {
	s := view.Peripherals(view.PeripheralsReading{
		Devices: []view.PeripheralReading{{Name: "G502 X PLUS", Level: 15, Kind: view.KindMouse}},
	})
	seen := painted(t, s)

	assert.Contains(t, strings.Join(seen[view.Bad], ""), "15")
	assert.False(t, s.Dimmed())
}
