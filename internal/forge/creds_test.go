package forge_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/hayami/internal/forge"
)

// tools is a fake gh and glab: what each answers, by its arguments, and
// every call made.
type tools struct {
	answers map[string]string
	calls   []string
}

func (f *tools) run(_ context.Context, name string, args ...string) (string, error) {
	call := name + " " + strings.Join(args, " ")
	f.calls = append(f.calls, call)
	if a, ok := f.answers[call]; ok {
		return a, nil
	}
	return "", errors.New("exit status 1")
}

// creds are a Creds over a temp HOME holding secrets and glab's config.
func creds(t *testing.T, secrets, glab string, f *tools) forge.Creds {
	t.Helper()
	dir := t.TempDir()
	c := forge.Creds{
		Secrets:    filepath.Join(dir, ".secrets.env"),
		GlabConfig: filepath.Join(dir, "glab-cli", "config.yml"),
		Run:        f.run,
	}
	if secrets != "" {
		require.NoError(t, os.WriteFile(c.Secrets, []byte(secrets), 0o600))
	}
	if glab != "" {
		require.NoError(t, os.MkdirAll(filepath.Dir(c.GlabConfig), 0o700))
		require.NoError(t, os.WriteFile(c.GlabConfig, []byte(glab), 0o600))
	}
	return c
}

const glabConfig = `hosts:
  gitlab.com:
    api_protocol: https
  gitlab.example.com:
    api_protocol: https
`

// Spec 102 R10, AC7. GitHub's token: the override, else gh's; with neither,
// a reason that says how to give one.
func TestGitHubTokenOrder(t *testing.T) {
	f := &tools{answers: map[string]string{"gh auth token --hostname github.com": "from-gh"}}
	c := creds(t, "", "", f)

	tok, err := c.GitHubToken(t.Context(), forge.Override{Token: " typed "})
	require.NoError(t, err)
	assert.Equal(t, "typed", tok)
	assert.Empty(t, f.calls, "gh was asked although a token was typed")

	tok, err = c.GitHubToken(t.Context(), forge.Override{})
	require.NoError(t, err)
	assert.Equal(t, "from-gh", tok)

	f.answers = nil
	_, err = c.GitHubToken(t.Context(), forge.Override{})
	require.ErrorIs(t, err, forge.ErrNoCredential)
	assert.Contains(t, err.Error(), "gh auth login")
}

// R10, AC7. GitLab's host and key, each in its order: the override, the
// secrets file, glab. glab's first host with a token is the host, not
// merely its first host.
func TestGitLabHostAndKeyOrder(t *testing.T) {
	f := &tools{answers: map[string]string{
		"glab config get token --host gitlab.example.com": "from-glab",
	}}
	c := creds(t, "", glabConfig, f)
	ctx := t.Context()

	assert.Equal(t, "gitlab.example.com", c.GitLabHost(ctx, forge.Override{}),
		"gitlab.com, which glab has no token for, was taken")
	assert.Equal(t, "typed.example.com", c.GitLabHost(ctx, forge.Override{Host: "https://typed.example.com/"}))
	tok, err := c.GitLabToken(ctx, "gitlab.example.com", forge.Override{})
	require.NoError(t, err)
	assert.Equal(t, "from-glab", tok)

	c = creds(t, "export GITLAB_HOST=https://secret.example.com\nGLAB_TOKEN='from-secrets'\n", glabConfig, f)
	assert.Equal(t, "secret.example.com", c.GitLabHost(ctx, forge.Override{}))
	tok, err = c.GitLabToken(ctx, "secret.example.com", forge.Override{})
	require.NoError(t, err)
	assert.Equal(t, "from-secrets", tok)
	tok, err = c.GitLabToken(ctx, "secret.example.com", forge.Override{Token: "typed"})
	require.NoError(t, err)
	assert.Equal(t, "typed", tok)

	c = creds(t, "", "", &tools{})
	assert.Empty(t, c.GitLabHost(ctx, forge.Override{}))
	_, err = c.GitLabToken(ctx, "", forge.Override{})
	require.ErrorIs(t, err, forge.ErrNoCredential)
}

// R10, AC7. The secrets file is parsed, not run: a line that would do
// something in a shell is only text, quotes and export are understood, and
// GITLAB_TOKEN wins over GLAB_TOKEN.
func TestTheSecretsFileIsParsedNotRun(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "ran")
	secrets := strings.Join([]string{
		"# a comment",
		"AWS_SECRET=never-kept",
		"touch " + marker,
		"GLAB_TOKEN=second",
		`export GITLAB_TOKEN="first" `,
		"GITLAB_HOST=gitlab.example.com # the work one",
		"$(touch " + marker + ")",
	}, "\n")
	c := creds(t, secrets, "", &tools{})
	tok, err := c.GitLabToken(t.Context(), "gitlab.example.com", forge.Override{})
	require.NoError(t, err)
	assert.Equal(t, "first", tok)
	assert.Equal(t, "gitlab.example.com", c.GitLabHost(t.Context(), forge.Override{}))
	_, statErr := os.Stat(marker)
	assert.True(t, os.IsNotExist(statErr), "a line of the secrets file was run")
}

// Spec 102 review, F2. A key found goes only to the host it was found with:
// the secrets file's key, with no GITLAB_HOST or another one, is not lent to
// the host glab knows; glab's own key for that host is used instead.
func TestAKeyIsNotLentToAnotherHost(t *testing.T) {
	f := &tools{answers: map[string]string{"glab config get token --host gitlab.example.com": "glab-key"}}
	ctx := t.Context()

	c := creds(t, "GITLAB_TOKEN=work-key\n", glabConfig, f)
	host := c.GitLabHost(ctx, forge.Override{})
	require.Equal(t, "gitlab.example.com", host)
	tok, err := c.GitLabToken(ctx, host, forge.Override{})
	require.NoError(t, err)
	assert.Equal(t, "glab-key", tok, "the secrets file's key went to a host it does not name")

	c = creds(t, "GITLAB_TOKEN=work-key\nGITLAB_HOST=work.example.com\n", "", &tools{})
	_, err = c.GitLabToken(ctx, "typed.example.com", forge.Override{})
	require.ErrorIs(t, err, forge.ErrNoCredential, "the key went to a host typed into Preferences")
	tok, err = c.GitLabToken(ctx, "work.example.com", forge.Override{})
	require.NoError(t, err)
	assert.Equal(t, "work-key", tok)
}

// R11, AC7. No failure carries a token: not gh's, not one typed, not the
// secrets file's.
func TestNoTokenInAFailure(t *testing.T) {
	f := &tools{}
	c := creds(t, "UNRELATED=s3cr3t-unrelated\n", "", f)
	_, err := c.GitHubToken(t.Context(), forge.Override{})
	require.Error(t, err)
	_, err2 := c.GitLabToken(t.Context(), "", forge.Override{})
	require.Error(t, err2)
	for _, e := range []error{err, err2} {
		assert.NotContains(t, e.Error(), "s3cr3t")
	}
}
