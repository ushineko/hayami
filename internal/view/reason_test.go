package view_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/hayami/internal/view"
)

// A reason is a line where the reading would have been, so it is read the same
// way: the label keeps its column and the text takes the value's place.
func TestAReasonBecomesALineWhereItsReadingWouldHaveBeen(t *testing.T) {
	s := view.Section{
		Rows:    []view.Row{{Label: "CPU", Value: "38", Unit: "°C", Status: view.Good}},
		Reasons: []view.Reason{{Label: "Coolant", Text: "no cooler", Status: view.Info}},
	}

	lines := s.Lines()

	require.Len(t, lines, 2)
	assert.Equal(t, "CPU", lines[0].Label)
	assert.Equal(t, "Coolant", lines[1].Label)
	assert.Equal(t, "no cooler", lines[1].Value)
}

// An Info reason is not a measurement, so it takes Dim -- the status this
// package keeps for a thing that is not a reading. A Warn one keeps its
// verdict, because a source that failed is the case worth a colour.
func TestAnInfoReasonIsDimAndAWarnReasonKeepsItsVerdict(t *testing.T) {
	s := view.Section{Reasons: []view.Reason{
		{Text: "no Bluetooth adapter", Status: view.Info},
		{Text: "the cooler would not answer", Status: view.Warn},
	}}

	lines := s.Lines()

	require.Len(t, lines, 2)
	assert.Equal(t, view.Dim, lines[0].Status)
	assert.Equal(t, view.Warn, lines[1].Status)
}

/*
A reason with no reading to stand in for reads from the left.

The label column is where a shell puts text; the value column is right-aligned
against the edge, which is correct for a number and wrong for a sentence. The
first photograph on the machine this was written for had a reason of this kind
hard against the right-hand edge under a heading, which read as a
reading whose label had gone missing.
*/
func TestALabellessReasonReadsFromTheLeft(t *testing.T) {
	s := view.Section{Reasons: []view.Reason{{Text: "no Bluetooth device with a battery"}}}

	lines := s.Lines()

	require.Len(t, lines, 1)
	assert.Equal(t, "no Bluetooth device with a battery", lines[0].Label)
	assert.Empty(t, lines[0].Value, "a sentence in the value column is right-aligned")
}

// Detail never reaches a line. A card is read at a glance and a device's error
// is not a glance; it is the hover.
func TestDetailStaysOffTheCardAndLandsOnTheHover(t *testing.T) {
	s := view.Section{
		Note: "and two more",
		Reasons: []view.Reason{
			{Label: "Coolant", Text: "no cooler", Detail: "no supported cooler detected"},
			{Text: "the cooler would not answer", Detail: "no 7501 reply: no reply"},
			{Text: "no Bluetooth adapter"},
		},
	}

	for _, line := range s.Lines() {
		assert.NotContains(t, line.Value, "7501")
		assert.NotContains(t, line.Value, "Kraken")
		assert.Empty(t, line.Detail, "a reason's detail must not become a second card line")
	}

	hover := s.Hover()
	assert.Contains(t, hover, "and two more", "the section's own note comes first")
	assert.Contains(t, hover, "Coolant: no supported cooler detected")
	assert.Contains(t, hover, "no 7501 reply")
}

// A section with no reasons hands back its rows untouched, so the common case
// allocates nothing.
func TestASectionWithNoReasonsIsItsRows(t *testing.T) {
	rows := []view.Row{{Label: "CPU", Value: "38"}}
	s := view.Section{Rows: rows}

	assert.Equal(t, rows, s.Lines())
	assert.Empty(t, s.Hover())
}

/*
Reasons are never written to the readings cache.

A reason is a statement about this moment. Restoring "the cooler would not
answer" from yesterday's file would be asserting a failure nobody has observed, and the
panel would open reporting a fault that may have been fixed since.
*/
func TestReasonsAreNeverSerialised(t *testing.T) {
	s := view.Section{
		Key:     "cooler",
		Rows:    []view.Row{{Label: "CPU", Value: "38"}},
		Reasons: []view.Reason{{Text: "the cooler would not answer", Detail: "no 7501 reply: no reply"}},
	}

	body, err := json.Marshal(s)
	require.NoError(t, err)
	assert.NotContains(t, string(body), "would not answer")
	assert.NotContains(t, string(body), "Reasons")

	var back view.Section
	require.NoError(t, json.Unmarshal(body, &back))
	assert.Empty(t, back.Reasons)
	assert.Len(t, back.Rows, 1, "the readings themselves still survive the round trip")
}

// Quiet is the only section a shell leaves out: no readings and no reason for
// having none.
func TestOnlyASectionWithNothingAtAllIsQuiet(t *testing.T) {
	assert.True(t, view.Section{}.Quiet())
	assert.False(t, view.Section{Rows: []view.Row{{}}}.Quiet())
	assert.False(t, view.Section{Reasons: []view.Reason{{Text: "no cooler"}}}.Quiet())
}
