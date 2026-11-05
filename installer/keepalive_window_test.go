//go:build !nogui

package installer

import (
	"testing"

	"fyne.io/fyne/v2/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/fynstall/extension/gui"
)

// The box starts clear, and what ticking it does is under it. Installed, it offers
// nothing; where there is no service to be, the wizard passes over it.
func TestTheKeepalivePageOffersACheckBoxThatStartsClear(t *testing.T) {
	test.NewApp()
	inline(t)
	answerKeep(t, keepMissing, nil)
	build, ok := gui.Lookup(PageKeepalive)
	require.True(t, ok, "the page has no window registered")
	page, ok := build(keepContext(No, false, nil)).(*offerPage)
	require.True(t, ok)
	content := page.Build(nil)
	page.Enter(nil)

	assert.False(t, page.Skip())
	assert.Equal(t, "VPN", page.Title())
	assert.True(t, page.offer.Visible())
	assert.Equal(t, "Install the VPN keepalive helper", page.offer.Text)
	assert.False(t, page.offer.Checked, "the box was ticked for the person")
	assert.Contains(t, labels(content)[0], "is not installed")

	page.offer.SetChecked(true)
	assert.Equal(t, Yes, page.c.Param(ParamKeepalive))

	answerKeep(t, keepThere, nil)
	page.Enter(nil)
	assert.False(t, page.offer.Visible())
	assert.Equal(t, No, page.c.Param(ParamKeepalive), "nothing is set up twice")

	answerKeep(t, keepNotHere, nil)
	assert.True(t, page.Skip())
}
