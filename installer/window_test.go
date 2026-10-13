//go:build !nogui

package installer

import (
	"runtime"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/fynstall/extension/gui"
)

// inline runs the page's check and its answer in line, so a test sees the
// page as it is once the check has answered.
func inline(t *testing.T) {
	t.Helper()
	bg, ui := background, onWindow
	background = func(f func()) { f() }
	onWindow = func(f func()) { f() }
	t.Cleanup(func() { background, onWindow = bg, ui })
}

// labels are the texts of every label under o.
func labels(o fyne.CanvasObject) []string {
	var out []string
	var walk func(fyne.CanvasObject)
	walk = func(o fyne.CanvasObject) {
		if !o.Visible() {
			return
		}
		switch v := o.(type) {
		case *widget.Label:
			out = append(out, v.Text)
		case *fyne.Container:
			for _, c := range v.Objects {
				walk(c)
			}
		}
	}
	walk(o)
	return out
}

// built is the sensors page, built and entered, on a machine in state s.
func built(t *testing.T, s Sensors, sensors string) (*offerPage, fyne.CanvasObject) {
	t.Helper()
	test.NewApp()
	inline(t)
	answer(t, s)
	build, ok := gui.Lookup(PageSensors)
	require.True(t, ok, "the page has no window registered")
	page, ok := build(install(sensors, nil)).(*offerPage)
	require.True(t, ok)
	content := page.Build(nil)
	page.Enter(nil)
	return page, content
}

// The page skips itself off Windows, and only there.
func TestThePageIsShownOnWindowsOnly(t *testing.T) {
	page, _ := built(t, notInstalled, No)
	assert.Equal(t, runtime.GOOS != "windows", page.Skip())
}

/*
Spec 059. With LibreHardwareMonitor missing the page says so, offers one
check box, clear, and lists under it what ticking it does. Ticking it sets
the parameter, and clearing it sets it back.
*/
func TestThePageOffersACheckBoxThatStartsClear(t *testing.T) {
	page, content := built(t, notInstalled, No)
	text := labels(content)
	require.NotEmpty(t, text)
	assert.Contains(t, text[0], "is not installed")
	assert.True(t, page.offer.Visible())
	assert.Equal(t, "Install LibreHardwareMonitor and configure it for hayami", page.offer.Text)
	assert.False(t, page.offer.Checked, "the box was ticked for the person")
	assert.Equal(t, No, page.c.Param(ParamSensors))

	joined := ""
	for _, l := range text {
		joined += l + "\n"
	}
	assert.Contains(t, joined, "winget")
	assert.Contains(t, joined, "asks for an administrator once")
	assert.Contains(t, joined, "kernel driver")

	page.offer.SetChecked(true)
	assert.Equal(t, Yes, page.c.Param(ParamSensors))
	page.offer.SetChecked(false)
	assert.Equal(t, No, page.c.Param(ParamSensors))
}

// A flag that said yes starts the box ticked: the person has already said.
func TestAFlagThatSaidYesStartsTheBoxTicked(t *testing.T) {
	page, _ := built(t, installed, Yes)
	assert.True(t, page.offer.Checked)
	assert.Equal(t, "Configure LibreHardwareMonitor for hayami", page.offer.Text)
}

// With it set up already the page says what hayami reads and offers
// nothing; the parameter is no whatever a flag said.
func TestThePageOffersNothingWhenItIsSetUp(t *testing.T) {
	page, content := built(t, ready, Yes)
	assert.False(t, page.offer.Visible())
	assert.False(t, page.detail.Visible())
	text := labels(content)
	require.Len(t, text, 1)
	assert.Contains(t, text[0], "Core (Tctl/Tdie) 48.1 °C")
	assert.Equal(t, No, page.c.Param(ParamSensors))
}
