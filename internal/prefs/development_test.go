package prefs_test

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/hayami/internal/config"
	"github.com/ushineko/hayami/internal/prefs"
)

// Spec 102 R12. The Sections page switches the Development drawer's parts:
// AI usage on, the forges off, until ticked. A GitLab host is taken when it
// is finished (Enter), never at each key: a host half typed is another host,
// and the key would go to it (review F2).
func TestTheDevelopmentPreferences(t *testing.T) {
	a := test.NewApp()
	t.Cleanup(a.Quit)
	s := store(t, "")
	w := prefs.New(a, prefs.Options{Store: s})
	w.Shell().Select("Sections")

	checks := map[string]*widget.Check{}
	var host *widget.Entry
	for _, o := range test.LaidOutObjects(w.Shell().Window.Content()) {
		switch v := o.(type) {
		case *widget.Check:
			checks[v.Text] = v
		case *widget.Entry:
			if v.PlaceHolder == "From GITLAB_HOST or glab" {
				host = v
			}
		}
	}
	require.Contains(t, checks, prefs.DevGitHubLabel)
	require.Contains(t, checks, prefs.DevAILabel)
	require.NotNil(t, host)
	assert.True(t, checks[prefs.DevAILabel].Checked)
	assert.False(t, checks[prefs.DevGitHubLabel].Checked)
	assert.False(t, checks[prefs.DevGitLabLabel].Checked)

	test.Tap(checks[prefs.DevGitLabLabel])
	assert.True(t, config.Usage.Get(s.Config()).GitLab)

	test.Type(host, "gitlab.example.co")
	assert.Empty(t, config.Usage.Get(s.Config()).GitLabHost, "a host half typed was saved")
	test.Type(host, "m")
	host.TypedKey(&fyne.KeyEvent{Name: fyne.KeyReturn})
	assert.Equal(t, "gitlab.example.com", config.Usage.Get(s.Config()).GitLabHost)
}
