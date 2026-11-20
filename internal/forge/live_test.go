package forge_test

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/hayami/internal/forge"
)

/*
Spec 102 AC13. Against the real GitHub API and the desk's GitLab, with the
desk's own credentials found as the panel finds them: a fortnight comes back
from each, with something in it. Off unless HAYAMI_DEV_E2E=1, because it
needs a person's sign-in and the network, and it prints no figure, repository
or title: the assertions are on counts' shape only.
*/
func TestLiveForges(t *testing.T) {
	if os.Getenv("HAYAMI_DEV_E2E") != "1" {
		t.Skip("set HAYAMI_DEV_E2E=1 to read the real forges with this desk's credentials")
	}
	c := forge.NewCreds()
	ctx := t.Context()

	tok, err := c.GitHubToken(ctx, forge.Override{})
	require.NoError(t, err)
	gh, err := forge.GitHubReader{}.Read(ctx, tok)
	require.NoError(t, err)
	assert.Len(t, gh.Days, forge.Days)
	assert.Positive(t, total(gh), "a fortnight of nothing on GitHub")

	host := c.GitLabHost(ctx, forge.Override{})
	require.NotEmpty(t, host, "no GitLab host found")
	key, err := c.GitLabToken(ctx, host, forge.Override{})
	require.NoError(t, err)
	gl, err := forge.GitLabReader{Host: host}.Read(ctx, key)
	require.NoError(t, err)
	assert.Len(t, gl.Days, forge.Days)
	assert.Equal(t, total(gl), len(gl.Items), "a GitLab day counts its events, each once")
}

func total(a forge.Activity) int {
	n := 0
	for _, d := range a.Days {
		n += d.Count
	}
	return n
}
