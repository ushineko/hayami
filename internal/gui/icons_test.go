package gui_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/hayami/internal/gui"
	"github.com/ushineko/hayami/internal/view"
)

// AC. Every section names an icon, and the window has a glyph for it (spec
// 038): a section added to view.Sections with an icon the window does not
// know would draw a bare title, and nothing else would say so.
func TestEverySectionHasAnIcon(t *testing.T) {
	for _, s := range view.Sections() {
		t.Run(s.Key, func(t *testing.T) {
			require.NotEqual(t, view.IconNone, s.Icon, "the section draws a bare title")
			assert.NotNil(t, gui.SectionIcon(s.Icon), "the window has no glyph for %q", s.Icon)
		})
	}
}

// AC. A builder draws the icon its section is listed with.
func TestEachBuilderDrawsItsSectionsIcon(t *testing.T) {
	assert.Equal(t, view.PeripheralsInfo.Icon, view.Peripherals(view.PeripheralsReading{}).Icon)
	assert.Equal(t, view.CoolerInfo.Icon, view.Cooler(view.CoolerReading{}).Icon)
	assert.Equal(t, view.BandwidthInfo.Icon, view.Bandwidth(nil).Icon)
}

// AC. The names are distinct: cards that shared a glyph would be worse than
// cards with none, because the icon would stop telling them apart.
func TestTheSectionIconsAreDistinct(t *testing.T) {
	seen := map[view.IconName]bool{}
	for _, s := range view.Sections() {
		require.False(t, seen[s.Icon], "two sections asked for %q", s.Icon)
		seen[s.Icon] = true
	}
	assert.Len(t, seen, len(view.Sections()))
}
