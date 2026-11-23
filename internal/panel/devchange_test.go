package panel_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/hayami/internal/config"
	"github.com/ushineko/hayami/internal/forge"
	"github.com/ushineko/hayami/internal/panel"
	"github.com/ushineko/hayami/internal/view"
)

// changeFake answers a change's page and keeps what it was asked.
type changeFake struct {
	mu    sync.Mutex
	asked []forge.ChangeRef
}

func (c *changeFake) read(_ context.Context, _ string, _ config.UsageSettings, ref forge.ChangeRef) (forge.Change, error) {
	c.mu.Lock()
	c.asked = append(c.asked, ref)
	c.mu.Unlock()
	return forge.Change{
		Kind: forge.KindPR, Repo: ref.Repo, Ref: "#1", Title: "a widget\x1b[31m", State: "merged",
		Body: "line one\x07\nline two", Commits: []forge.Commit{{SHA: "abcdef0123", Title: "feat: a widget", Stats: true, Additions: 3}},
	}, nil
}

func (c *changeFake) refs() []forge.ChangeRef {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]forge.ChangeRef(nil), c.asked...)
}

// openOf is the Open of the day page's line named name.
func openOf(t *testing.T, page view.Detail, name string) string {
	t.Helper()
	for _, part := range page.Parts {
		for _, l := range part.Lines {
			if l.Name == name {
				return l.Open
			}
		}
	}
	require.Failf(t, "no line", "%q is not on the page", name)
	return ""
}

// changePage reads a change's page after waiting for what it reads.
func changePage(t *testing.T, u *panel.Usage, id string) view.Detail {
	t.Helper()
	_, _ = u.PollDetail(t.Context(), id)
	u.Settle(t.Context())
	d, err := u.PollDetail(t.Context(), id)
	require.NoError(t, err)
	return d
}

// Spec 111 R1, R4, R5. A line of a day's page opens its change's page: a
// drill under the day, whose ID names no repository, read once in the
// background and kept, its text made printable.
func TestALineOpensItsChange(t *testing.T) {
	u, _, now := devUsage(t, config.UsageSettings{GitHub: true})
	c := &changeFake{}
	panel.SetChangeRead(u, c.read)
	overview(t, u)
	date := now.Format(time.DateOnly)
	day, err := u.PollDetail(t.Context(), view.DevDayID(forge.GitHub, date))
	require.NoError(t, err)

	id := openOf(t, day, "#1 a widget")
	require.NotEmpty(t, id, "the pull request's line opens nothing")
	assert.NotContains(t, id, "widget", "an ID names the repository")
	assert.True(t, u.Drills(id))
	assert.Equal(t, view.DevDayID(forge.GitHub, date), u.DrillUp(id), "a change's way out is not its day")
	repo := openOf(t, day, "widget")
	assert.NotEqual(t, id, repo)

	page := changePage(t, u, id)
	refs := c.refs()
	require.Len(t, refs, 1)
	assert.Equal(t, forge.ChangeRef{Kind: forge.KindPR, Repo: "octo/widget", Number: 1}, refs[0])
	require.NotEmpty(t, page.Parts)
	assert.Equal(t, "#1 · widget", page.Parts[0].Title)
	for _, part := range page.Parts {
		for _, l := range part.Lines {
			assert.NotContains(t, l.Name, "\x1b", "a forge's text reached the page unprintable")
			assert.NotContains(t, l.Name, "\x07")
		}
	}

	changePage(t, u, id)
	assert.Len(t, c.refs(), 1, "a page read again within the interval")

	*now = now.Add(time.Hour)
	changePage(t, u, id)
	assert.Len(t, c.refs(), 2, "a page older than the interval was not read again")
}

// Spec 111 R3. A repository's line opens its day's commits.
func TestARepositorysLineOpensItsDay(t *testing.T) {
	u, _, now := devUsage(t, config.UsageSettings{GitHub: true})
	c := &changeFake{}
	panel.SetChangeRead(u, c.read)
	overview(t, u)
	date := now.Format(time.DateOnly)
	day, _ := u.PollDetail(t.Context(), view.DevDayID(forge.GitHub, date))
	changePage(t, u, openOf(t, day, "widget"))
	refs := c.refs()
	require.Len(t, refs, 1)
	assert.Equal(t, forge.KindCommits, refs[0].Kind)
	assert.Equal(t, "octo/widget", refs[0].Repo)
	assert.Equal(t, date, refs[0].Day.Format(time.DateOnly))
}

// Spec 111 R4. A key the reading no longer lists reads nothing, and a forge
// switched off offers no page.
func TestAChangeNoLongerListedReadsNothing(t *testing.T) {
	u, _, now := devUsage(t, config.UsageSettings{GitHub: true})
	c := &changeFake{}
	panel.SetChangeRead(u, c.read)
	overview(t, u)
	id := view.DevChangeID(forge.GitHub, now.Format(time.DateOnly), view.DevChangeKey(view.DevOpenPR, "octo/other", "#9"))
	page := changePage(t, u, id)
	assert.Empty(t, c.refs())
	require.NotEmpty(t, page.Reasons)

	u.SetDevelopment(config.UsageSettings{})
	assert.False(t, u.Drills(id))
}
