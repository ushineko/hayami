package view_test

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/hayami/internal/view"
)

// twoForges are a fortnight's tail on GitHub and GitLab: today on both,
// yesterday on GitHub only.
func twoForges() []view.DevReading {
	return []view.DevReading{
		{
			Provider: "github", Title: "GitHub", Unit: "contributions",
			Days: []view.DevDay{{Date: "2026-11-20", Count: 3}, {Date: "2026-11-21", Count: 5}},
			Items: []view.DevItem{
				{Date: "2026-11-20", Repo: "octo/widget", Kind: "commits", Count: 3},
				{Date: "2026-11-21", Repo: "octo/widget", Kind: "pr", Ref: "#7", Title: "a change", Action: "merged", Count: 1},
				{Date: "2026-11-21", Repo: "octo/widget", Kind: "commits", Count: 4},
			},
		},
		{
			Provider: "gitlab", Title: "GitLab", Unit: "events",
			Days:  []view.DevDay{{Date: "2026-11-21", Count: 2}},
			Items: []view.DevItem{{Date: "2026-11-21", Repo: "team/agent", Kind: "pushed", Ref: "main", Count: 2}},
		},
	}
}

// Spec 107 R1, R3. Recent is the forges' days together, newest first: a
// day's heading, its repositories a step in, their things two; it is cut to
// the room by the shell.
func TestRecentIsBothForgesNewestFirstAndIndented(t *testing.T) {
	d := view.DevRecent(twoForges())
	assert.True(t, d.Fit)
	require.NotEmpty(t, d.Lines)

	first := d.Lines[0]
	assert.True(t, first.Heading)
	assert.Contains(t, first.Name, "Saturday 21 November")
	assert.Equal(t, "7", first.Value, "today's count is both forges'")

	var names []string
	for _, l := range d.Lines {
		names = append(names, strings.Repeat(">", l.Indent)+l.Name)
	}
	assert.Contains(t, names, ">widget")
	assert.Contains(t, names, ">agent")
	assert.Contains(t, names, ">>#7 a change")
	assert.Contains(t, names, ">>main")
	assert.NotContains(t, strings.Join(names, "\n"), "octo", "an owner is a user name (rule 9)")

	heads := 0
	for _, l := range d.Lines {
		if l.Heading {
			heads++
		}
	}
	assert.Equal(t, 2, heads, "today and yesterday")
	assert.Empty(t, view.DevRecent(nil).Lines, "nothing listed is nothing drawn")
}

// Spec 107 R2. Repositories are the fortnight's busiest across the forges, a
// bar each against the busiest, owners dropped.
func TestRepositoriesAreTheBusiestWithABarEach(t *testing.T) {
	d := view.DevRepos(twoForges())
	require.Len(t, d.Meters, 2)
	assert.Equal(t, "widget", d.Meters[0].Label)
	assert.InDelta(t, 1.0, d.Meters[0].Fraction, 0.001)
	assert.Contains(t, d.Meters[0].Caption, "7c")
	assert.Equal(t, "agent", d.Meters[1].Label)
	assert.InDelta(t, 2.0/8.0, d.Meters[1].Fraction, 0.001)
	assert.Empty(t, view.DevRepos(nil).Meters)
}

// Spec 107 R1, R4. In a drawer of a given height, Recent takes what is left
// and the drawer is that height; Repositories, under it, is still drawn; no
// day's heading is left at the bottom with nothing under it; with no room,
// Recent goes whole.
func TestRecentFillsTheRoomAndNoMore(t *testing.T) {
	forges := twoForges()
	overview := view.Development(nil, []view.Detail{
		view.DevPart(forges[0], time.Date(2026, 11, 21, 12, 0, 0, 0, time.Local), -1), view.DevRecent(forges), view.DevRepos(forges),
	})
	full := view.RenderDetail(overview, 70, 200, view.DrawerShape{Marked: -1})
	whole := len(full)

	for height := whole - 1; height > whole-6; height-- {
		lines := view.RenderDetail(overview, 70, height, view.DrawerShape{Marked: -1})
		// The room, or a line under it where a day's heading went with
		// nothing under it.
		require.LessOrEqual(t, len(lines), height, "height %d", height)
		require.GreaterOrEqual(t, len(lines), height-1, "height %d", height)
		text := strings.Join(lines, "\n")
		assert.Contains(t, text, "Repositories", "height %d cut the part under Recent", height)
		assert.Contains(t, text, "Recent", "height %d", height)
		for i, l := range lines {
			if strings.Contains(l, "Repositories") && i > 0 {
				prev := strings.TrimSpace(lines[i-2])
				assert.NotContains(t, prev, "November", "height %d left a day's heading with nothing under it", height)
			}
		}
	}

	small := view.RenderDetail(overview, 70, whole-len(view.DevRecent(forges).Lines)-1, view.DrawerShape{Marked: -1})
	text := strings.Join(small, "\n")
	assert.NotContains(t, text, "Recent", "a Recent with no room kept its title")
	assert.Contains(t, text, "Repositories")
}
