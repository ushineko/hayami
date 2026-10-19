package view_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/ushineko/hayami/internal/view"
)

/*
A section with more than one list needs a label and a key on every one of
them.

A switch is built only from the choices that carry a label, and is drawn
only when the open detail is among those (gui.choices). The bandwidth
section shipped with one of its two lists unlabelled, so the switch was
never drawn and the peer breakdown could not be reached from the panel at
all: it existed, and nothing led to it.
*/
func TestEveryListOfASectionWithTwoCarriesALabelAndKey(t *testing.T) {
	for _, group := range [][]string{
		{view.DetailProcesses, view.DetailPeers},
		{view.DetailCPU, view.DetailGPU, view.DetailMemory},
	} {
		keys := map[string]string{}
		for _, id := range group {
			info := view.ProcessInfo(id)
			assert.NotEmpty(t, info.Label, "detail %q has no label, so no switch can offer it", id)
			assert.NotEmpty(t, info.Key, "detail %q has no key", id)
			assert.NotContains(t, keys, info.Key,
				"detail %q takes the key %q that %q already has", id, info.Key, keys[info.Key])
			keys[info.Key] = id
		}
	}
}
