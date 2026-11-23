package view_test

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/hayami/internal/view"
)

// Spec 111 R1. What each kind of line opens: a review or a comment the thing
// it was on, a push its branch, an event with nothing to read nothing.
func TestWhatALineOpens(t *testing.T) {
	for _, c := range []struct{ provider, kind, ref, open string }{
		{"github", "pr", "#12", view.DevOpenPR},
		{"github", "review", "#12", view.DevOpenPR},
		{"github", "issue", "#4", view.DevOpenIssue},
		{"gitlab", "mr", "!7", view.DevOpenMR},
		{"gitlab", "comment", "!7", view.DevOpenMR},
		{"gitlab", "comment", "#3", view.DevOpenIssue},
		{"gitlab", "pushed", "main", view.DevOpenPush},
		{"github", "other", "", ""},
		{"gitlab", "comment", "", ""},
	} {
		open, _ := view.DevOpens(c.provider, c.kind, c.ref)
		assert.Equal(t, c.open, open, "%+v", c)
	}
}

// Spec 111 R1. A change's ID is its day's and a key, and the two parse apart.
func TestAChangeIsADrillUnderItsDay(t *testing.T) {
	key := view.DevChangeKey(view.DevOpenPR, "octo/widget", "#12")
	assert.NotEqual(t, key, view.DevChangeKey(view.DevOpenPR, "octo/widget", "#13"))
	id := view.DevChangeID("github", "2026-03-10", key)
	p, date, k, ok := view.ParseDevChangeID(id)
	require.True(t, ok)
	assert.Equal(t, []string{"github", "2026-03-10", key}, []string{p, date, k})
	_, _, ok = view.ParseDevDayID(id)
	assert.False(t, ok, "a change parses as a day")
	_, _, _, ok = view.ParseDevChangeID(view.DevDayID("github", "2026-03-10"))
	assert.False(t, ok, "a day parses as a change")
	assert.NotContains(t, id, "octo")
}

// Spec 111 R1. A day's repositories and things open their pages; a
// repository created opens nothing.
func TestADaysLinesOpenTheirPages(t *testing.T) {
	r := fortnight()
	r.Items = append(r.Items, view.DevItem{Date: "2026-03-10", Repo: "octo/gizmo", Kind: "other", Action: "created", Count: 1})
	page := view.DevDayPage(r, devNow, "2026-03-10")
	opens := map[string]string{}
	for _, l := range page.Parts[0].Lines {
		opens[l.Name] = l.Open
	}
	assert.Equal(t, view.DevChangeID("github", "2026-03-10", view.DevChangeKey(view.DevOpenRepo, "octo/widget", "")), opens["widget"])
	assert.Equal(t, view.DevChangeID("github", "2026-03-10", view.DevChangeKey(view.DevOpenPR, "octo/widget", "#12")), opens["#12 feat: a widget"])
	assert.Equal(t, view.DevChangeID("github", "2026-03-10", view.DevChangeKey(view.DevOpenPush, "team/sub/agent", "main")), opens["main"])
	assert.Empty(t, opens["created"])
}

// Spec 111 R2. Before the change is read the page is what its line said,
// marked reading.
func TestAChangeBeingReadSaysWhatItsLineSaid(t *testing.T) {
	page := view.DevChangePage("GitHub", "x", view.DevOpenPR, "octo/widget", "#12", "#12 feat: a widget", nil, true, nil)
	require.Len(t, page.Parts, 1)
	head := page.Parts[0]
	assert.Equal(t, "#12 · widget", head.Title)
	require.NotEmpty(t, head.TitleParts)
	assert.Equal(t, "reading…", head.TitleParts[0].Text)
	require.NotEmpty(t, head.Lines)
	assert.Equal(t, "#12 feat: a widget", head.Lines[0].Name)
}

// Spec 111 R2, R5. A change read is its heading, facts, description,
// commits and files, every line within the drawer's width; a long
// description is cut and says so.
func TestAChangeReadIsItsWholePage(t *testing.T) {
	merged := time.Date(2026, 3, 10, 15, 0, 0, 0, time.UTC)
	body := strings.Repeat("A word that goes on. ", 1000)
	c := &view.DevChange{
		Kind: "pr", Ref: "#12", Title: "feat: a widget that is called something long enough to wrap",
		State: "merged", Opened: merged.Add(-time.Hour), Merged: merged, Source: "feat/widget", Target: "main",
		Additions: 40, Deletions: 3, FilesCount: 2, CommitCount: 1, SizeKnown: true, Checks: "failed",
		Approvals: 1, Decision: "review required", Labels: []string{"enhancement"}, Comments: 2, Body: body,
		Commits: []view.DevCommit{{SHA: "aaaaaaa1111", Title: "feat: a widget", Stats: true, Additions: 40, Deletions: 3}},
		Files:   []view.DevFile{{Path: "internal/view/a/very/deep/path/to/widget_test.go", Additions: 2, Status: "added"}},
	}
	page := view.DevChangePage("GitHub", "x", view.DevOpenPR, "octo/widget", "#12", "", c, false, nil)
	var ids []string
	for _, p := range page.Parts {
		ids = append(ids, p.ID)
	}
	assert.Equal(t, []string{"change", "description", "commits", "files"}, ids)

	head := page.Parts[0]
	assert.Equal(t, "merged", head.TitleParts[0].Text)
	facts := map[string]string{}
	for _, l := range head.Lines {
		facts[l.Name] = l.Value
		assert.LessOrEqual(t, len([]rune(l.Name)), view.DevWideNameWidth, l.Name)
	}
	assert.Equal(t, "+40 −3", facts["Lines"])
	for _, l := range head.Lines {
		if l.Name == "Lines" {
			require.Len(t, l.Parts, 2)
			assert.Equal(t, view.Bad, l.Parts[1].Status, "lines removed are not coloured apart")
		}
	}
	assert.Equal(t, "failed", facts["Checks"])
	assert.Equal(t, "1 approved", facts["Reviews"])
	assert.Contains(t, facts, "feat/widget")
	assert.Contains(t, facts, "→ main")

	desc := page.Parts[1].Lines
	assert.Len(t, desc, view.DevChangeLines+1)
	assert.Equal(t, "… the rest on the forge", desc[len(desc)-1].Name)
	assert.Equal(t, "Commits (1)", page.Parts[2].Title)
	assert.Equal(t, "aaaaaaa feat: a widget", page.Parts[2].Lines[0].Name)
	file := page.Parts[3].Lines[0].Name
	assert.True(t, strings.HasSuffix(file, "widget_test.go (added)"), file)
	assert.LessOrEqual(t, len([]rune(file)), view.DevWideNameWidth)

	c.Title = strings.Repeat("long ", 1000)
	page = view.DevChangePage("GitHub", "x", view.DevOpenPR, "octo/widget", "#12", "", c, false, nil)
	words := 0
	for _, l := range page.Parts[0].Lines {
		if strings.HasPrefix(l.Name, "long") {
			words++
		}
	}
	assert.Equal(t, 6, words, "a long title is not held to six lines (review F1)")

	c.Approvals = 0
	page = view.DevChangePage("GitHub", "x", view.DevOpenPR, "octo/widget", "#12", "", c, false, nil)
	for _, l := range page.Parts[0].Lines {
		assert.NotEqual(t, "Reviews", l.Name, "a merged change says its repository's review rule")
	}
}
