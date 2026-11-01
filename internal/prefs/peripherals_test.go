package prefs_test

import (
	"testing"

	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/hayami/internal/config"
	"github.com/ushineko/hayami/internal/prefs"
)

// Spec 071 R2. The Sections page offers every device on one line; ticking it
// saves the setting, and unticking takes the key back out.
func TestTheSectionsPageSavesEveryDeviceOnOneLine(t *testing.T) {
	a := test.NewApp()
	t.Cleanup(a.Quit)
	s := store(t, "")
	w := prefs.New(a, prefs.Options{Store: s})
	w.Shell().Select("Sections")

	var check *widget.Check
	for _, o := range test.LaidOutObjects(w.Shell().Window.Content()) {
		if c, ok := o.(*widget.Check); ok && c.Text == prefs.OneLineLabel {
			check = c
		}
	}
	require.NotNil(t, check, "the Sections page does not offer the setting")
	require.False(t, check.Checked, "the setting is off by default")

	test.Tap(check)
	assert.True(t, config.Peripherals.Get(s.Config()).OneLine)
	assert.Contains(t, string(s.Config().SectionSettings["peripherals"]), `"oneLine":true`)

	test.Tap(check)
	assert.False(t, config.Peripherals.Get(s.Config()).OneLine)
	assert.NotContains(t, string(s.Config().SectionSettings["peripherals"]), "oneLine")
}
