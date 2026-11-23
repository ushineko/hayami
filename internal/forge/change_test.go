package forge_test

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/hayami/internal/forge"
)

// githubPR is a pull request as GitHub answers for one, invented: merged,
// two commits one of them a merge, reviews, checks passed, two files.
const githubPR = `{"data":{"repository":{"issueOrPullRequest":{
  "__typename":"PullRequest","title":"feat: a widget","state":"MERGED","merged":true,"isDraft":false,
  "createdAt":"2026-03-10T09:00:00Z","mergedAt":"2026-03-10T15:00:00Z","closedAt":"2026-03-10T15:00:00Z",
  "bodyText":"Adds a widget.\n\nIt squeaks less.","headRefName":"feat/widget","baseRefName":"main",
  "additions":40,"deletions":3,"changedFiles":2,"reviewDecision":"APPROVED",
  "labels":{"nodes":[{"name":"enhancement"}]},"comments":{"totalCount":2},
  "reviews":{"nodes":[{"state":"APPROVED"},{"state":"COMMENTED"},{"state":"CHANGES_REQUESTED"}]},
  "commits":{"totalCount":2,"nodes":[
    {"commit":{"oid":"aaaaaaa1111","messageHeadline":"feat: a widget","additions":40,"deletions":3,"committedDate":"2026-03-10T10:00:00Z","parents":{"totalCount":1}}},
    {"commit":{"oid":"bbbbbbb2222","messageHeadline":"Merge main","additions":900,"deletions":800,"committedDate":"2026-03-10T11:00:00Z","parents":{"totalCount":2}}}]},
  "last":{"nodes":[{"commit":{"statusCheckRollup":{"state":"SUCCESS"}}}]},
  "files":{"totalCount":2,"nodes":[
    {"path":"widget.go","additions":38,"deletions":3,"changeType":"MODIFIED"},
    {"path":"widget_test.go","additions":2,"deletions":0,"changeType":"ADDED"}]}
}}}}`

func githubChange(t *testing.T, h http.HandlerFunc) (forge.GitHubReader, *atomic.Int32) {
	t.Helper()
	var calls atomic.Int32
	srv, hc := tlsServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		assert.Contains(t, r.Header.Get("Authorization"), token)
		h(w, r)
	}))
	return forge.GitHubReader{Client: hc, BaseURL: srv.URL + "/", Now: func() time.Time { return now }}, &calls
}

// Spec 111 R2. A pull request's page is one request: its state, times,
// branches, size, checks, reviews, labels, comments, description, commits
// and files.
func TestGitHubReadsAPullRequest(t *testing.T) {
	var asked struct {
		Variables map[string]any `json:"variables"`
	}
	r, calls := githubChange(t, func(w http.ResponseWriter, req *http.Request) {
		require.NoError(t, json.NewDecoder(req.Body).Decode(&asked))
		_, _ = io.WriteString(w, githubPR)
	})
	c, err := r.ReadChange(t.Context(), token, forge.ChangeRef{Kind: forge.KindPR, Repo: "octo/widget", Number: 12})
	require.NoError(t, err)
	assert.EqualValues(t, 1, calls.Load())
	assert.Equal(t, "octo", asked.Variables["owner"])
	assert.Equal(t, "widget", asked.Variables["name"])
	assert.EqualValues(t, 12, asked.Variables["number"])

	assert.Equal(t, forge.KindPR, c.Kind)
	assert.Equal(t, "#12", c.Ref)
	assert.Equal(t, "merged", c.State)
	assert.Equal(t, time.Date(2026, 3, 10, 15, 0, 0, 0, time.UTC), c.Merged.UTC())
	assert.Equal(t, "feat/widget", c.Source)
	assert.Equal(t, "main", c.Target)
	assert.Equal(t, 40, c.Additions)
	assert.Equal(t, 2, c.FilesCount)
	assert.Equal(t, 2, c.CommitCount)
	assert.Equal(t, "passed", c.Checks)
	assert.Equal(t, 1, c.Approvals)
	assert.Equal(t, 1, c.ChangesRequested)
	assert.Equal(t, []string{"enhancement"}, c.Labels)
	assert.Equal(t, 2, c.Comments)
	assert.Equal(t, "Adds a widget.\n\nIt squeaks less.", c.Body)
	require.Len(t, c.Commits, 2)
	assert.False(t, c.Commits[0].Merge)
	assert.True(t, c.Commits[1].Merge, "a commit with two parents is a merge")
	require.Len(t, c.Files, 2)
	assert.Equal(t, "added", c.Files[1].Status)
}

// Spec 111 R2. A number that is an issue reads as one.
func TestGitHubReadsAnIssue(t *testing.T) {
	r, _ := githubChange(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"data":{"repository":{"issueOrPullRequest":{"__typename":"Issue","title":"it squeaks",
			"state":"CLOSED","stateReason":"NOT_PLANNED","createdAt":"2026-03-09T09:00:00Z","closedAt":"2026-03-10T09:00:00Z",
			"bodyText":"It squeaks.","labels":{"nodes":[]},"comments":{"totalCount":1}}}}}`)
	})
	c, err := r.ReadChange(t.Context(), token, forge.ChangeRef{Kind: forge.KindPR, Repo: "octo/widget", Number: 4})
	require.NoError(t, err)
	assert.Equal(t, forge.KindIssue, c.Kind, "the forge's answer says what it is")
	assert.Equal(t, "closed", c.State)
	assert.Equal(t, "not planned", c.StateReason)
	assert.Empty(t, c.Commits)
}

// Spec 111 R3. A repository's day on GitHub is the viewer's commits on its
// default branch that day; the totals leave a merge out.
func TestGitHubReadsADaysCommits(t *testing.T) {
	var asked []map[string]any
	r, calls := githubChange(t, func(w http.ResponseWriter, req *http.Request) {
		var body struct {
			Query     string         `json:"query"`
			Variables map[string]any `json:"variables"`
		}
		require.NoError(t, json.NewDecoder(req.Body).Decode(&body))
		asked = append(asked, body.Variables)
		if strings.Contains(body.Query, "viewer") {
			_, _ = io.WriteString(w, `{"data":{"viewer":{"id":"U_1"}}}`)
			return
		}
		_, _ = io.WriteString(w, `{"data":{"repository":{"defaultBranchRef":{"target":{"history":{"totalCount":2,"nodes":[
			{"oid":"ccccccc","messageHeadline":"fix: a squeak","additions":5,"deletions":1,"committedDate":"2026-03-10T10:00:00Z","parents":{"totalCount":1}},
			{"oid":"ddddddd","messageHeadline":"Merge pull request #12","additions":40,"deletions":3,"committedDate":"2026-03-10T15:00:00Z","parents":{"totalCount":2}}]}}}}}}`)
	})
	day := time.Date(2026, 3, 10, 0, 0, 0, 0, time.Local)
	c, err := r.ReadChange(t.Context(), token, forge.ChangeRef{Kind: forge.KindCommits, Repo: "octo/widget", Day: day})
	require.NoError(t, err)
	assert.EqualValues(t, 2, calls.Load())
	require.Len(t, asked, 2)
	assert.Equal(t, "U_1", asked[1]["author"])
	assert.Equal(t, day.Format(time.RFC3339), asked[1]["since"])
	assert.Equal(t, day.AddDate(0, 0, 1).Format(time.RFC3339), asked[1]["until"])
	require.Len(t, c.Commits, 2)
	assert.Equal(t, 5, c.Additions, "a merge's lines were counted")
	assert.Equal(t, 1, c.Deletions)
	assert.True(t, c.SizeKnown)
}

// gitlabChange is a GitLab answering for project team/agent: merge request
// 7, its approvals, commits and two pages of diffs; issue 3; and commits on
// a branch.
func gitlabChange(t *testing.T) (forge.GitLabReader, *[]string) {
	t.Helper()
	var asked []string
	srv, hc := tlsServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, token, r.Header.Get("PRIVATE-TOKEN"))
		path := r.URL.EscapedPath()
		asked = append(asked, path+"?"+r.URL.RawQuery)
		const p = "/api/v4/projects/team%2Fagent"
		switch path {
		case p + "/merge_requests/7":
			_, _ = io.WriteString(w, `{"iid":7,"title":"fix: ids","state":"merged","draft":false,
				"created_at":"2026-03-10T09:00:00Z","merged_at":"2026-03-10T12:00:00Z",
				"source_branch":"fix/ids","target_branch":"main","description":"## Why\n**Fixes** the `+"`ids`"+`.",
				"labels":["bug"],"user_notes_count":3,"changes_count":"2","head_pipeline":{"status":"failed"}}`)
		case p + "/merge_requests/7/approvals":
			_, _ = io.WriteString(w, `{"approved":true,"approved_by":[{"user":{"id":1}}]}`)
		case p + "/merge_requests/7/commits":
			_, _ = io.WriteString(w, `[{"id":"eeeeeeee","title":"fix: ids","committed_date":"2026-03-10T10:00:00Z","parent_ids":["x"]}]`)
		case p + "/merge_requests/7/diffs":
			if r.URL.Query().Get("page") == "2" {
				_, _ = io.WriteString(w, `[{"old_path":"gone.go","new_path":"gone.go","deleted_file":true,"diff":"@@ -1,2 +0,0 @@\n-a\n-b\n"}]`)
				return
			}
			w.Header().Set("X-Next-Page", "2")
			_, _ = io.WriteString(w, `[{"old_path":"ids.go","new_path":"ids.go","diff":"--- a/ids.go\n+++ b/ids.go\n@@ -1 +1,2 @@\n-x\n+y\n+z\n"}]`)
		case p + "/issues/4":
			// No id: client-go's decoder panics on it.
			_, _ = io.WriteString(w, `{"iid":4,"title":"x"}`)
		case p + "/issues/3":
			_, _ = io.WriteString(w, `{"id":30,"iid":3,"title":"ids clash","state":"opened","created_at":"2026-03-09T09:00:00Z",
				"description":"They clash.","labels":[],"user_notes_count":0}`)
		case p + "/repository/commits":
			if r.URL.Query().Get("ref_name") == "bbbb" {
				_, _ = io.WriteString(w, `[{"id":"c2","title":"two","committed_date":"2026-03-13T11:00:00Z","parent_ids":["c1"],"stats":{"additions":4,"deletions":0}},
					{"id":"c1","title":"one","committed_date":"2026-03-13T10:00:00Z","parent_ids":["c0"],"stats":{"additions":2,"deletions":1}}]`)
				return
			}
			_, _ = io.WriteString(w, `[{"id":"c1","title":"one","committed_date":"2026-03-13T10:00:00Z","parent_ids":["c0"],"stats":{"additions":2,"deletions":1}}]`)
		default:
			http.NotFound(w, r)
		}
	}))
	return forge.GitLabReader{Client: hc, BaseURL: srv.URL + "/api/v4/", Now: func() time.Time { return now }}, &asked
}

// Spec 111 R2. A merge request's page: itself, its approvals, its commits,
// and its diffs counted into lines and files, every page of them.
func TestGitLabReadsAMergeRequest(t *testing.T) {
	r, _ := gitlabChange(t)
	c, err := r.ReadChange(t.Context(), token, forge.ChangeRef{Kind: forge.KindMR, Repo: "team/agent", Number: 7})
	require.NoError(t, err)
	assert.Equal(t, "!7", c.Ref)
	assert.Equal(t, "merged", c.State)
	assert.Equal(t, "fix/ids", c.Source)
	assert.Equal(t, "Why\nFixes the ids.", c.Body, "a description's Markdown marks are taken out")
	assert.Equal(t, "failed", c.Checks)
	assert.Equal(t, 1, c.Approvals)
	assert.Equal(t, "approved", c.Decision)
	assert.Equal(t, []string{"bug"}, c.Labels)
	assert.Equal(t, 3, c.Comments)
	assert.Equal(t, 2, c.FilesCount)
	require.Len(t, c.Commits, 1)
	require.Len(t, c.Files, 2)
	assert.Equal(t, 2, c.Additions, "the diff's two added lines, not its header")
	assert.Equal(t, 3, c.Deletions)
	assert.Equal(t, "deleted", c.Files[1].Status)
	assert.True(t, c.SizeKnown)
}

// Spec 111 R2. An issue's page is one request.
func TestGitLabReadsAnIssue(t *testing.T) {
	r, asked := gitlabChange(t)
	c, err := r.ReadChange(t.Context(), token, forge.ChangeRef{Kind: forge.KindIssue, Repo: "team/agent", Number: 3})
	require.NoError(t, err)
	assert.Len(t, *asked, 1)
	assert.Equal(t, "#3", c.Ref)
	assert.Equal(t, "They clash.", c.Body)
}

// Spec 111 R3. A branch's pushes on a day are their commits that day, each
// once, newest first, bounded by the day rather than the push's count.
func TestGitLabReadsADaysPushes(t *testing.T) {
	r, asked := gitlabChange(t)
	day := time.Date(2026, 3, 13, 0, 0, 0, 0, time.Local)
	c, err := r.ReadChange(t.Context(), token, forge.ChangeRef{
		Kind: forge.KindPushed, Repo: "team/agent", Branch: "main", Day: day,
		Pushes: []forge.Push{{From: "aaaa", To: "bbbb", Count: 1094}, {From: "0000", To: "aaaa", Count: 1}},
	})
	require.NoError(t, err)
	require.Len(t, c.Commits, 2, "a commit in two pushes is listed once")
	assert.Equal(t, "c2", c.Commits[0].SHA, "newest first")
	assert.Equal(t, 6, c.Additions)
	assert.Equal(t, "main", c.Ref)
	require.NotEmpty(t, *asked)
	assert.Contains(t, (*asked)[0], "since=")
	assert.Contains(t, (*asked)[0], "until=")
	assert.Contains(t, (*asked)[0], "per_page=100", "a rebase's count asked for its commits")
}

// Spec 111 R6. A reference that is not a repository's, or a number that is
// not one, is refused before a request carries the key anywhere.
func TestABadChangeIsNotRead(t *testing.T) {
	gh, calls := githubChange(t, func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, githubPR) })
	gl, asked := gitlabChange(t)
	for _, ref := range []forge.ChangeRef{
		{Kind: forge.KindPR, Repo: "widget", Number: 1},
		{Kind: forge.KindPR, Repo: "octo/..", Number: 1},
		{Kind: forge.KindPR, Repo: "../../user", Number: 1},
		{Kind: forge.KindPR, Repo: "octo/widget?x=1", Number: 1},
		{Kind: forge.KindPR, Repo: "octo/widget", Number: 0},
		{Kind: forge.KindReview, Repo: "octo/widget", Number: 1},
	} {
		_, err := gh.ReadChange(t.Context(), token, ref)
		require.ErrorIs(t, err, forge.ErrBadRef, "%+v", ref)
		_, err = gl.ReadChange(t.Context(), token, ref)
		require.ErrorIs(t, err, forge.ErrBadRef, "%+v", ref)
	}
	assert.Zero(t, calls.Load())
	assert.Empty(t, *asked)
}

// Spec 111 R6. An answer the client library panics on is an error, not the
// end of the program.
func TestAnAnswerThatPanicsTheLibraryIsAnError(t *testing.T) {
	r, _ := gitlabChange(t)
	_, err := r.ReadChange(t.Context(), token, forge.ChangeRef{Kind: forge.KindIssue, Repo: "team/agent", Number: 4})
	require.ErrorIs(t, err, forge.ErrBadAnswer)
}

// Spec 111 R6. A failure is the package's own, and carries no key.
func TestAChangeFailureIsTyped(t *testing.T) {
	gh, _ := githubChange(t, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusUnauthorized) })
	_, err := gh.ReadChange(t.Context(), token, forge.ChangeRef{Kind: forge.KindPR, Repo: "octo/widget", Number: 1})
	require.ErrorIs(t, err, forge.ErrUnauthorized)
	assert.NotContains(t, err.Error(), token)

	gl, _ := gitlabChange(t)
	_, err = gl.ReadChange(t.Context(), token, forge.ChangeRef{Kind: forge.KindMR, Repo: "team/other", Number: 1})
	require.Error(t, err)
	assert.NotContains(t, err.Error(), token)
}
