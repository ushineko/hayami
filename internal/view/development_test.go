package view_test

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/hayami/internal/view"
)

var devNow = time.Date(2026, 3, 14, 12, 0, 0, 0, time.UTC)

// fortnight is a reading over 1 to 14 March: nothing on the 2nd, the 10th
// the busiest.
func fortnight() view.DevReading {
	r := view.DevReading{Provider: "github", Title: "GitHub", Unit: "contributions", ReadAt: devNow.Add(-3 * time.Minute)}
	for i := range 14 {
		count := 2
		switch i {
		case 1:
			count = 0
		case 9:
			count = 20
		}
		r.Days = append(r.Days, view.DevDay{Date: time.Date(2026, 3, 1+i, 0, 0, 0, 0, time.UTC).Format(time.DateOnly), Count: count})
	}
	r.Items = []view.DevItem{
		{Date: "2026-03-10", Repo: "octo/widget", Kind: "commits", Count: 5},
		{Date: "2026-03-10", Repo: "octo/widget", Kind: "pr", Ref: "#12", Title: "feat: a widget", Action: "merged", Count: 1},
		{Date: "2026-03-10", Repo: "octo/widget", Kind: "review", Ref: "#12", Title: "feat: a widget", Action: "approved", Count: 1},
		{Date: "2026-03-10", Repo: "team/sub/agent", Kind: "pushed", Ref: "main", Count: 1},
		{Date: "2026-03-10", Repo: "team/sub/agent", Kind: "pushed", Ref: "main", Count: 1},
		{Date: "2026-03-10", Repo: "team/sub/agent", Kind: "mr", Ref: "!7", Title: "fix: ids", Action: "opened", Count: 1},
		{Date: "2026-03-10", Repo: "team/sub/agent", Kind: "mr", Ref: "!7", Title: "fix: ids", Action: "merged", Count: 1},
		{Date: "2026-03-10", Repo: "team/sub/agent", Kind: "comment", Ref: "!7", Title: "fix: ids", Action: "commented", Count: 1},
		{Date: "2026-03-11", Repo: "octo/widget", Kind: "issue", Ref: "#4", Title: "squeaks", Action: "open", Count: 1},
	}
	return r
}

// Spec 102 R4. A forge's part: a heading that reads it again and says when
// it was read, a strip of fourteen days with initials, each opening its day,
// an empty day empty and the busiest full, and the fortnight's sum.
func TestAForgePartIsAStripOfDays(t *testing.T) {
	part := view.DevPart(fortnight(), devNow, -1)
	assert.Equal(t, "GitHub", part.Title)
	assert.Equal(t, view.RefreshAct("github"), part.Open)
	require.Len(t, part.TitleParts, 1)
	assert.Contains(t, part.TitleParts[0].Text, "3m ago")

	require.NotNil(t, part.Days)
	s := part.Days
	require.Len(t, s.Values, 14)
	assert.Negative(t, s.Values[1], "a day with nothing is not drawn as a day with something")
	assert.InDelta(t, 1, s.Values[9], 1e-9, "the busiest day is full")
	assert.GreaterOrEqual(t, s.Values[0], 0.2, "the least day with something is visibly filled")
	assert.Equal(t, "S", s.Labels[0], "1 March 2026 is a Sunday")
	assert.Equal(t, view.DevDayID("github", "2026-03-10"), s.Opens[9])
	assert.Equal(t, -1, s.Selected)
	require.Len(t, part.Lines, 1)
	assert.Equal(t, "44 contributions, most 20", part.Lines[0].Value)

	assert.Equal(t, "▃·▃▃▃▃▃▃▃█▃▃▃▃", s.Text())
}

// R4, R5. Before a first reading the part is its heading and what it is
// doing; a failure is its reason, with the last reading kept.
func TestAForgePartSaysItsState(t *testing.T) {
	r := view.DevReading{Provider: "gitlab", Title: "GitLab", Reading: true}
	part := view.DevPart(r, devNow, -1)
	assert.Nil(t, part.Days)
	assert.Contains(t, part.TitleParts[0].Text, "reading…")

	r = fortnight()
	r.Failed = &view.Reason{Text: "the key was refused", Status: view.Warn}
	part = view.DevPart(r, devNow, -1)
	assert.Contains(t, part.TitleParts[0].Text, "failed")
	require.Len(t, part.Reasons, 1)
	assert.NotNil(t, part.Days, "a failure took the last reading away")
}

// R6, R7, R8, AC6. A day: the strip with the day marked, the date and its
// count, each repository without its owner and its counts in a fixed order,
// one line per thing at the furthest it got, pushes to a branch as one, and
// the private contributions said.
func TestADayIsItsRepositories(t *testing.T) {
	r := fortnight()
	r.Private = 3
	page := view.DevDayPage(r, devNow, "2026-03-10")
	assert.Equal(t, view.DevDayID("github", "2026-03-10"), page.ID)
	require.Len(t, page.Parts, 1)
	part := page.Parts[0]
	assert.Equal(t, 9, part.Days.Selected)

	got := map[string]string{}
	var order []string
	for _, l := range part.Lines[1:] {
		got[l.Name] = l.Value
		order = append(order, l.Name)
	}
	assert.Equal(t, "20", got["Tuesday 10 March"])
	assert.Equal(t, "2↑ 2MR 1cm", got["agent"], "counts in the fixed order, owner and group dropped")
	assert.Equal(t, "5c 1PR 1rev", got["widget"])
	assert.Equal(t, "pushed ×2", got["main"])
	assert.Equal(t, "merged", got["!7 fix: ids"], "an MR opened and merged in a day is merged")
	assert.Equal(t, "merged", got["#12 feat: a widget"], "a merged PR reviewed the same day is merged")
	assert.NotContains(t, order, "octo/widget", "a repository's owner is a user name")
	assert.Contains(t, order, "3 private contributions not listed")
	assert.NotContains(t, order, "#4 squeaks", "another day's issue is on this one")
	for _, l := range part.Lines {
		assert.LessOrEqual(t, len([]rune(l.Name)), view.DevWideNameWidth, l.Name)
	}
}

// R4, R6. A day no longer in the fortnight says so rather than drawing
// another day.
func TestADayOutsideTheFortnight(t *testing.T) {
	page := view.DevDayPage(fortnight(), devNow, "2026-02-01")
	require.Len(t, page.Parts, 1)
	assert.Equal(t, -1, page.Parts[0].Days.Selected)
	require.NotEmpty(t, page.Parts[0].Reasons)
}

// R13. The terminal's drawer marks the strip as one line among the part's,
// its day between brackets, and its heading as another that reads it again.
func TestATerminalDrawsTheStrip(t *testing.T) {
	d := view.Development(nil, []view.Detail{view.DevPart(fortnight(), devNow, -1)})
	sel := view.Selectable(d)
	require.GreaterOrEqual(t, len(sel), 2)
	assert.Equal(t, view.RefreshAct("github"), sel[0].Open)
	assert.Len(t, sel[1].DayOpens, 14)

	lines := view.RenderDetail(d, 60, 20, view.DrawerShape{Marked: 1, Day: 9})
	text := strings.Join(lines, "\n")
	assert.Contains(t, text, "[█]", "the marked day is not bracketed")
	assert.Contains(t, text, "↻ 3m ago", "the heading's state was cut to the width")
	assert.Contains(t, text, " S  M  T ", "the initials are not under the days")
	assert.Equal(t, 13, view.MarkedDay(-1, 14), "no day marked is today")
}

// Spec 102 review, F1. A title someone else wrote reaches a terminal with no
// control character in it, from inside a detail's parts as from its top.
func TestATitleCannotDriveTheTerminal(t *testing.T) {
	r := fortnight()
	r.Items = append(r.Items, view.DevItem{Date: "2026-03-10", Repo: "octo/x\x1b[2J", Kind: "pr", Ref: "#7",
		Title: "fix \x1b]52;c;SGVsbG8=\a done", Action: "open", Count: 1})
	page := view.DevDayPage(r, devNow, "2026-03-10")
	page.Parts[0].Reasons = []view.Reason{{Text: "bad\x9b31m"}}
	page.Parts[0].TitleParts = []view.Part{{Text: "↻ \x1b]0;title\a"}}
	pane := strings.Join(view.RenderDetail(view.PaneDetail(page, ""), 80, 60, view.DrawerShape{Marked: -1}), "\n")
	for _, bad := range []string{"\x1b", "\a", "\x9b"} {
		assert.NotContains(t, pane, bad)
		assert.NotContains(t, page.Text(), bad)
	}
	assert.Contains(t, pane, "fix ]52;c;SGVsbG8= done", "the title's own text went with its escape")
}

// Spec 102 review. A repository named for its owner is a user name too.
func TestARepositoryNamedForItsOwner(t *testing.T) {
	r := fortnight()
	r.Items = []view.DevItem{
		{Date: "2026-03-10", Repo: "octo/octo", Kind: "commits", Count: 1},
		{Date: "2026-03-10", Repo: "octo/Octo.github.io", Kind: "commits", Count: 1},
	}
	var names []string
	for _, l := range view.DevDayPage(r, devNow, "2026-03-10").Parts[0].Lines {
		names = append(names, l.Name)
	}
	assert.Contains(t, names, "profile")
	assert.Contains(t, names, "pages")
	assert.NotContains(t, strings.Join(names, " "), "octo")
}

// R14. The command line prints the strip and its initials.
func TestTheStripIsPrinted(t *testing.T) {
	text := view.DevPart(fortnight(), devNow, -1).Text()
	assert.Contains(t, text, " ▃  ·  ▃ ")
	assert.Contains(t, text, "S  M  T")
}

// R3. An account in the overview is its meters; with none, its first line.
func TestUsageOverviewKeepsTheMeters(t *testing.T) {
	d := view.UsageDetail(devNow, view.UsageBreakdown{Account: "CC desk", Plan: "max",
		Limits:      []view.UsageLimit{{Name: "Week", Fraction: 0.3}},
		SharesTitle: "Week by use", Shares: []view.UsageShare{{Name: "Claude Code", Fraction: 1}}})
	o := view.UsageOverview(d)
	assert.Len(t, o.Meters, 1)
	assert.Empty(t, o.Lines)
	assert.Equal(t, "CC desk", o.Open)

	d = view.UsageDetail(devNow, view.UsageBreakdown{Account: "CC work", Facts: []view.UsageFact{{Name: "Spend", Value: "$1"}, {Name: "Credits", Value: "2"}}})
	o = view.UsageOverview(d)
	require.Len(t, o.Lines, 1)
	assert.Equal(t, "Spend", o.Lines[0].Name)
}
