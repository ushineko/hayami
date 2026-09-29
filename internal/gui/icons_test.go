package gui_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/hayami/internal/view"
)

// AC. Every section names an icon, so none of the four draws a bare title.
func TestEverySectionNamesAnIcon(t *testing.T) {
	for _, s := range []struct {
		name string
		sec  view.Section
	}{
		{"peripherals", view.Peripherals(view.PeripheralsReading{})},
		{"cooler", view.Cooler(view.CoolerReading{})},
	} {
		t.Run(s.name, func(t *testing.T) {
			assert.NotEqual(t, view.IconNone, s.sec.Icon,
				"the section draws a bare title")
		})
	}
}

// AC. The names are distinct: four cards that shared a glyph would be worse
// than four with none, because the icon would stop telling them apart.
func TestTheSectionIconsAreDistinct(t *testing.T) {
	seen := map[view.IconName]bool{}
	for _, n := range []view.IconName{
		view.IconPeripherals, view.IconBandwidth, view.IconCooler, view.IconUsage,
	} {
		require.False(t, seen[n], "two sections asked for %q", n)
		seen[n] = true
	}
	assert.Len(t, seen, 4)
}
