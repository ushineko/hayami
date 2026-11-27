//go:build !nogui

package installer

import (
	"testing"

	"fyne.io/fyne/v2/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/fynstall/extension/gui"
)

// Spec 115 R12. On a first install the Port page's box starts ticked, as the
// Service page's does; a port removed before is not proposed again; with no
// service to give a port to, the wizard passes the page over.
func TestThePortPageProposesThePortOnAFirstInstall(t *testing.T) {
	test.NewApp()
	inline(t)
	a := APITCP{Service: true, Port: 47930}
	answerAPITCP(t, &a, true, nil)
	build, ok := gui.Lookup(PageAPITCP)
	require.True(t, ok, "the page has no window registered")
	page, ok := build(tcpContext(Auto, false, nil)).(*offerPage)
	require.True(t, ok)
	content := page.Build(nil)
	page.Enter(nil)

	assert.False(t, page.Skip())
	assert.Equal(t, "Port", page.Title())
	assert.True(t, page.offer.Visible())
	assert.Equal(t, "Also serve on a local TCP port", page.offer.Text)
	assert.True(t, page.offer.Checked, "on by default: a first install proposes the port")
	assert.Contains(t, labels(content)[0], "unless you clear the box")

	page.offer.SetChecked(false)
	assert.Equal(t, No, page.c.Param(ParamAPITCP))

	a = APITCP{Service: true, Asked: true, Port: 47930}
	page2, ok := build(tcpContext(Auto, true, nil)).(*offerPage)
	require.True(t, ok)
	page2.Build(nil)
	page2.Enter(nil)
	assert.False(t, page2.offer.Checked, "a port removed before was proposed again")

	a = APITCP{}
	assert.True(t, page2.Skip(), "a page with no service was shown")
}
