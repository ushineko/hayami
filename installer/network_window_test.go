//go:build !nogui

package installer

import (
	"testing"

	"fyne.io/fyne/v2/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/fynstall/extension/gui"
)

// Spec 061. The network page in a window: with the helper missing it says
// so and offers one check box, clear, with what ticking it does under it;
// with this version installed it says so and offers nothing.
func TestTheNetworkPageOffersACheckBoxThatStartsClear(t *testing.T) {
	test.NewApp()
	inline(t)
	answerNet(t, netMissing)
	build, ok := gui.Lookup(PageNetwork)
	require.True(t, ok, "the page has no window registered")
	page, ok := build(installNet(No, false, nil)).(*offerPage)
	require.True(t, ok)
	content := page.Build(nil)
	page.Enter(nil)

	assert.Equal(t, "Network", page.Title())
	text := labels(content)
	require.NotEmpty(t, text)
	assert.Contains(t, text[0], "is not installed")
	assert.True(t, page.offer.Visible())
	assert.Equal(t, "Install the network helper", page.offer.Text)
	assert.False(t, page.offer.Checked, "the box was ticked for the person")

	page.offer.SetChecked(true)
	assert.Equal(t, Yes, page.c.Param(ParamNetwork))
	assert.Equal(t, No, page.c.Param(ParamSensors), "one page set the other's parameter")
	page.offer.SetChecked(false)
	assert.Equal(t, No, page.c.Param(ParamNetwork))

	answerNet(t, netCurrent)
	page.Enter(nil)
	assert.False(t, page.offer.Visible())
	assert.False(t, page.detail.Visible())
	assert.Contains(t, labels(content)[0], "nothing to do")
}
