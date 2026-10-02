package view_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/hayami/internal/view"
)

/*
Spec 031, R2.4. The shortening table, on names as their sources print them.

The first five are from the two desks this panel runs on: /proc/cpuinfo,
nvidia-smi and sanshoku's identity of the cooler. The AMD processor is the
kernel's form of a common one; the cards after it are from the public PCI ID
database, which is where a card found through the kernel is named from, and
show that the chip's code name outside the bracket is dropped.
*/
func TestNamesAreShortenedToTheModel(t *testing.T) {
	for full, want := range map[string]string{
		"Intel(R) Core(TM) i9-14900K":                         "i9-14900K",
		"13th Gen Intel(R) Core(TM) i7-13700K":                "i7-13700K",
		"NVIDIA GeForce RTX 4090":                             "RTX 4090",
		"NVIDIA GeForce RTX 3080":                             "RTX 3080",
		"NZXT Kraken Elite V2":                                "Kraken Elite V2",
		"AMD Ryzen 9 7950X 16-Core Processor":                 "Ryzen 9 7950X",
		"AD102 [GeForce RTX 4090]":                            "RTX 4090",
		"GA102 [GeForce RTX 3080 Lite Hash Rate]":             "RTX 3080",
		"Navi 31 [Radeon RX 7900 XT/7900 XTX/7900 GRE/7900M]": "RX 7900 XT/7900 XTX/7900 GRE/7900M",
		"Raptor Lake-S GT1 [UHD Graphics 770]":                "UHD Graphics 770",
		"Intel(R) Core(TM) i7-4770K CPU @ 3.50GHz":            "i7-4770K",
		"AMD Ryzen 7 5800H with Radeon Graphics":              "Ryzen 7 5800H",
	} {
		assert.Equal(t, want, view.ShortName(full), "from %q", full)
	}
}

// A name the rules would empty is kept whole: a long name is better than none.
func TestANameTheRulesWouldEmptyIsKept(t *testing.T) {
	assert.Equal(t, "NVIDIA", view.ShortName(" NVIDIA "))
}

// R2.5. A name longer than the label column is cut with an ellipsis at
// exactly that width, and the full name is the row's tip.
func TestALongNameIsCutAndKeptWholeInTheTip(t *testing.T) {
	full := "Navi 31 [Radeon RX 7900 XT/7900 XTX/7900 GRE/7900M]"
	s := view.Cooler(view.CoolerReading{GPU: 52, HasGPU: true, GPUName: full})

	require.Len(t, s.Rows, 1)
	label := s.Rows[0].Label
	assert.Equal(t, view.LabelWidth, len([]rune(label)))
	assert.True(t, strings.HasSuffix(label, "…"), "%q is not cut with an ellipsis", label)
	assert.True(t, strings.HasPrefix(label, "RX 7900"), "%q lost the model", label)
	assert.Equal(t, "GPU: "+full, s.Rows[0].Tip)
	assert.Contains(t, s.Hover(), full, "the window's tip carries the full name")
}

// LabelWidth holds every short name from both desks whole.
func TestTheDesksNamesFitTheColumn(t *testing.T) {
	for _, full := range []string{
		"Intel(R) Core(TM) i9-14900K", "13th Gen Intel(R) Core(TM) i7-13700K",
		"NVIDIA GeForce RTX 4090", "NVIDIA GeForce RTX 3080", "NZXT Kraken Elite V2",
		"AMD Ryzen 9 7950X 16-Core Processor",
	} {
		assert.Equal(t, view.ShortName(full), view.NameLabel("CPU", full), "%q was cut", full)
	}
}

// R2.6. Where a name could not be read the row keeps its old label, and has
// no tip.
func TestAnUnreadNameKeepsTheOldLabel(t *testing.T) {
	s := view.Cooler(view.CoolerReading{CPU: 60, HasCPU: true, GPU: 41, HasGPU: true,
		Coolant: 38.9, HasLiquid: true})

	require.Len(t, s.Rows, 3)
	for i, want := range []string{"CPU", "GPU", "Coolant"} {
		assert.Equal(t, want, s.Rows[i].Label)
		assert.Empty(t, s.Rows[i].Tip)
	}
}

// R2.5. A name arriving moves nothing: in a pane's row arrangement the label
// column is LabelWidth with or without one, so the values stay in their
// column, the line is the same length, and nothing else in the pane -- a
// usage meter whose bar starts after the label column -- moves either.
func TestANameArrivingMovesNoValue(t *testing.T) {
	unnamed := view.CoolerReading{CPU: 60, HasCPU: true, CPULoad: 12, HasCPULoad: true,
		Coolant: 38.9, HasLiquid: true}
	withNames := unnamed
	withNames.CPUName = "Intel(R) Core(TM) i9-14900K"
	withNames.CoolerName = "NZXT Kraken Elite V2"
	meter := view.Section{Key: "usage", Title: "Usage", Meters: []view.Meter{
		{Label: "CC max", Caption: "5h: 4 %", Reset: "in 2h", Fraction: 0.04},
	}}

	for _, a := range []view.Arrangement{view.ArrangeStack, view.ArrangeRow, view.ArrangeGrid} {
		before := view.Render([]view.Section{view.Cooler(unnamed), meter}, a, 60)
		after := view.Render([]view.Section{view.Cooler(withNames), meter}, a, 60)

		require.Len(t, after, len(before), "%s: a name changed the number of lines", a)
		for i := range before {
			assert.Equal(t, len([]rune(before[i])), len([]rune(after[i])),
				"%s: line %d changed length:\n%q\n%q", a, i, before[i], after[i])
			if j := strings.Index(before[i], "CC max"); j >= 0 {
				// The meter, from its name to the end of the line: in a grid
				// the cooler's column shares the line with it.
				k := strings.Index(after[i], "CC max")
				require.GreaterOrEqual(t, k, 0)
				assert.Equal(t, before[i][j:], after[i][k:], "%s: another section's line moved with the name", a)
				assert.Equal(t, len([]rune(before[i][:j])), len([]rune(after[i][:k])), "%s: the meter moved", a)
			}
			if j := strings.Index(before[i], "60.0"); j >= 0 {
				assert.Equal(t, j, strings.Index(after[i], "60.0"), "%s: the temperature moved", a)
			}
			if j := strings.Index(before[i], "38.9"); j >= 0 {
				assert.Equal(t, j, strings.Index(after[i], "38.9"), "%s: the coolant moved", a)
			}
		}
		assert.Contains(t, strings.Join(after, "\n"), "i9-14900K")
	}
}
