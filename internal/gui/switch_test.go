package gui_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/ushineko/hayami/internal/gui"
	"github.com/ushineko/hayami/internal/panel"
	"github.com/ushineko/hayami/internal/view"
)

// twoLists offers the two lists a bandwidth section has, and nothing else:
// a switch is built from Details alone.
type twoLists struct{ panel.Detailer }

func (twoLists) Details() []view.DetailInfo {
	return []view.DetailInfo{
		view.ProcessInfo(view.DetailProcesses),
		view.ProcessInfo(view.DetailPeers),
	}
}

/*
A section with two lists is drawn with a switch over both of them.

A switch is built only from the choices that carry a label, and only when
the detail on show is among them. The bandwidth section shipped with its
process list unlabelled, so there was no switch and the peer breakdown
could not be reached from the panel: it was built, tested, released, and
nothing on screen led to it.
*/
func TestASectionWithTwoListsIsSwitchedBetweenThem(t *testing.T) {
	for _, open := range []string{view.DetailProcesses, view.DetailPeers} {
		got := gui.Choices(twoLists{}, open)
		assert.Len(t, got, 2, "no switch is drawn with %q on show", open)
		var ids []string
		for _, c := range got {
			ids = append(ids, c.ID)
			assert.NotZero(t, c.Key, "the choice %q has no key", c.ID)
		}
		assert.Equal(t, []string{view.DetailProcesses, view.DetailPeers}, ids)
	}
}
