package panel_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/hayami/internal/panel"
	"github.com/ushineko/hayami/internal/view"
)

// The kill reads its process from here, from either section's page. A
// cooler page's ID is not a bandwidth one, and reading it as one ended
// nothing when the button was pressed.
func TestDrillPID(t *testing.T) {
	cases := []struct {
		id   string
		pid  int
		isPI bool
	}{
		{"cpu/firefox/4411", 4411, true},
		{"memory/firefox/4411", 4411, true},
		{"gpu/firefox/4411", 4411, true},
		{"peers/eno2/fpsd/1534", 1534, true},
		{"cpu", 0, false},
		{"cpu/firefox", 0, false},
		{"peers/eno2/fpsd", 0, false},
		{"peers/eno2/fpsd/@nas", 0, false},
		{"cpu/firefox/0", 0, false},
	}
	for _, c := range cases {
		pid, ok := panel.DrillPID(c.id)
		if pid != c.pid && c.isPI || ok != c.isPI {
			t.Errorf("%s: %d, %v; want %d, %v", c.id, pid, ok, c.pid, c.isPI)
		}
	}
}

/*
Spec 066 R15. A drill is one part and not a list of lines, so the drawer
draws the whole of it and scrolls.

The lines path cuts to the drawer's height and the cut is a prefix, so the
rows at the end go first -- and on a process page those are parent, state,
threads, sockets and children, which is what a person opened the page to
read. Beside the cooler the page kept "now" and "pid" and dropped the rest,
saying nothing about it (spec 066 gap 4, fynedesygn spec 072 gap 6).
*/
func TestADrillIsOnePartSoNothingIsCutFromIt(t *testing.T) {
	d := view.Detail{
		Section: "cooler",
		ID:      "cpu/firefox/4411",
		Title:   "CPU by process",
		Lines: []view.DetailLine{
			{Name: "CPU by process > firefox > 4411", Heading: true},
			{Name: "now", Value: "1.0"},
			{Name: "pid", Value: "4411"},
			{Name: "parent", Value: "systemd (1)"},
			{Name: "kill", Open: view.ActKill},
		},
	}
	page := panel.AsPage(d)

	assert.Empty(t, page.Lines, "a page keeps its lines in its part, not beside it")
	require.Len(t, page.Parts, 1)
	assert.Equal(t, "CPU by process > firefox > 4411", page.Parts[0].Title,
		"the trail is the part's heading")
	assert.Len(t, page.Parts[0].Lines, 4, "every line under the trail is kept")
	assert.Equal(t, "kill", page.Parts[0].Lines[3].Name,
		"the last line is the first one a prefix cut would have dropped")
}

// The kill is found wherever a drill keeps its lines, so moving them into a
// part does not take the button away.
func TestTheKillIsFoundInsideAPage(t *testing.T) {
	d := panel.AsPage(view.Detail{
		Lines: []view.DetailLine{
			{Name: "trail", Heading: true},
			{Name: "kill", Open: view.ActKill},
		},
	})
	var found bool
	for _, l := range panel.DrillLines(d) {
		found = found || l.Open == view.ActKill
	}
	assert.True(t, found, "the kill line was lost when the lines moved into a part")
}

// A detail that is already parts, or has no lines, is left alone.
func TestAPageLeavesWhatIsNotADrillAlone(t *testing.T) {
	parts := view.Detail{Parts: []view.Detail{{Title: "eno2"}}}
	assert.Equal(t, parts, panel.AsPage(parts))
	assert.Equal(t, view.Detail{}, panel.AsPage(view.Detail{}))
}
