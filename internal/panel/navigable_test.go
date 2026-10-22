package panel_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/ushineko/hayami/internal/panel"
	"github.com/ushineko/hayami/internal/view"
)

/*
Spec 066 R18. A section whose rows lead somewhere says so by being a
Driller, and the drawer asks the section rather than the detail on show.

The drawer draws the button that stops the reading, and lights the row
under the pointer, for a section that carries the navigation. Asking the
open detail instead left the cooler's three lists inert: a list is not
itself a drill -- only the pages below it are -- so they answered no for
themselves and were drawn with no pause and no hover, exactly as they had
been before any of this existed.
*/
func TestTheSectionsWhoseRowsLeadSomewhereSaySo(t *testing.T) {
	for name, src := range map[string]any{
		"bandwidth": &panel.Bandwidth{},
		"cooler":    &panel.Cooler{},
	} {
		_, ok := src.(panel.Driller)
		assert.True(t, ok, "%s's rows lead somewhere but it is not a Driller, "+
			"so its drawer carries no pause and no hover", name)
	}
}

// The paths each section routes, which is what Drills is for once the
// drawer has decided to carry the navigation at all.
func TestEachSectionRoutesItsOwnDrillPaths(t *testing.T) {
	b := &panel.Bandwidth{}
	assert.True(t, b.Drills(view.DetailPeers))
	assert.True(t, b.Drills("peers/eno2/fpsd/1534"))
	assert.False(t, b.Drills("cpu/firefox"))

	c := &panel.Cooler{}
	assert.True(t, c.Drills("cpu/firefox"), "a name under a list is a page")
	assert.True(t, c.Drills("memory/vivaldi-bin/1"), "a process under a name is a page")
	assert.False(t, c.Drills("nope/x"), "an unknown list is not one")
	assert.False(t, c.Drills("cpu/a/b/c"), "a path too deep is not one")
}

// Where each page's way out leads. A top list leads nowhere, so the drawer
// draws no Back on it: the process list once led to the peers, a Back on a
// page that is the peers' neighbour and not their child (#226).
func TestOnlyADrillHasAWayOut(t *testing.T) {
	for id, up := range map[string]string{
		view.DetailProcesses:          view.DetailProcesses,
		view.DetailPeers:              view.DetailPeers,
		"peers/eth0/curl":             view.DetailPeers,
		"peers/eth0/curl/1534":        "peers/eth0/curl",
		"peers/eth0/curl/@192.0.2.10": "peers/eth0/curl",
	} {
		assert.Equal(t, up, (&panel.Bandwidth{}).DrillUp(id), "the way out of %q", id)
	}
	for id, up := range map[string]string{
		view.DetailCPU:  view.DetailCPU,
		"cpu/firefox":   view.DetailCPU,
		"cpu/firefox/1": "cpu/firefox",
	} {
		assert.Equal(t, up, (&panel.Cooler{}).DrillUp(id), "the way out of %q", id)
	}
}
