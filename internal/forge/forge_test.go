package forge_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/hayami/internal/forge"
)

// now is the readers' clock: the window is 1 to 14 March.
var now = time.Date(2026, 3, 14, 12, 0, 0, 0, time.Local)

const token = "tok-invented-0123456789"

// tlsServer is a forge on loopback over TLS, and a client for it built the
// way the readers' is.
func tlsServer(t *testing.T, h http.Handler) (*httptest.Server, *http.Client) {
	t.Helper()
	srv := httptest.NewTLSServer(h)
	t.Cleanup(srv.Close)
	return srv, forge.NewClient(srv.Client().Transport)
}

// githubResponse is a fortnight's contributions, invented: two repositories,
// one created, a pull request merged, a review, an issue, a day outside the
// window. Its calendar and restricted count are what GitHub would send if
// asked: a day counted that lists nothing (1 March), and one counted past
// what it lists (14 March). The reader does not ask for them (spec 109).
const githubResponse = `{"data":{"viewer":{"contributionsCollection":{
  "restrictedContributionsCount": 3,
  "contributionCalendar":{"weeks":[
    {"contributionDays":[{"date":"2026-02-28","contributionCount":9},{"date":"2026-03-01","contributionCount":2},{"date":"2026-03-02","contributionCount":0}]},
    {"contributionDays":[{"date":"2026-03-10","contributionCount":7},{"date":"2026-03-14","contributionCount":4}]}]},
  "commitContributionsByRepository":[
    {"repository":{"nameWithOwner":"octo/widget"},"contributions":{"nodes":[
      {"occurredAt":"2026-03-10T12:00:00Z","commitCount":5},
      {"occurredAt":"2026-02-27T12:00:00Z","commitCount":8}]}},
    {"repository":{"nameWithOwner":"octo/gadget"},"contributions":{"nodes":[
      {"occurredAt":"2026-03-14T12:00:00Z","commitCount":2}]}}],
  "repositoryContributions":{"nodes":[
    {"occurredAt":"2026-03-14T08:00:00Z","repository":{"nameWithOwner":"octo/gizmo"}}]},
  "pullRequestContributions":{"pageInfo":{"hasNextPage":false},"nodes":[
    {"occurredAt":"2026-03-10T15:00:00Z","pullRequest":{"number":12,"title":"feat: a widget","state":"MERGED","merged":true,"repository":{"nameWithOwner":"octo/widget"}}}]},
  "pullRequestReviewContributions":{"pageInfo":{"hasNextPage":false},"nodes":[
    {"occurredAt":"2026-03-10T16:00:00Z","pullRequestReview":{"state":"APPROVED"},"pullRequest":{"number":13,"title":"fix: a gadget","repository":{"nameWithOwner":"octo/gadget"}}}]},
  "issueContributions":{"pageInfo":{"hasNextPage":false},"nodes":[
    {"occurredAt":"2026-03-14T09:00:00Z","issue":{"number":4,"title":"the widget squeaks","state":"OPEN","repository":{"nameWithOwner":"octo/widget"}}}]}
}}}}`

// Spec 102 R4, R8, AC4; spec 109 R2, R3. One query becomes fourteen days,
// each counted from what it lists, and the items on their days; a day outside
// the window is not one of them, and what GitHub counts and will not name is
// not counted.
func TestGitHubReadsAFortnightInOneQuery(t *testing.T) {
	var calls atomic.Int32
	var asked struct {
		Query     string         `json:"query"`
		Variables map[string]any `json:"variables"`
	}
	srv, hc := tlsServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		assert.Equal(t, "/graphql", r.URL.Path)
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Contains(t, r.Header.Get("Authorization"), token)
		require.NoError(t, json.NewDecoder(r.Body).Decode(&asked))
		_, _ = io.WriteString(w, githubResponse)
	}))

	a, err := forge.GitHubReader{Client: hc, BaseURL: srv.URL + "/", Now: func() time.Time { return now }}.Read(t.Context(), token)
	require.NoError(t, err)
	assert.EqualValues(t, 1, calls.Load(), "a fortnight is one request")
	assert.Contains(t, asked.Query, "contributionsCollection")
	assert.NotContains(t, asked.Query, "restrictedContributionsCount")
	assert.NotContains(t, asked.Query, "contributionCalendar")
	assert.True(t, strings.HasPrefix(fmt.Sprint(asked.Variables["from"]), "2026-03-01T00:00:00"), asked.Variables["from"])
	assert.True(t, strings.HasPrefix(fmt.Sprint(asked.Variables["to"]), "2026-03-14T23:59:59"), asked.Variables["to"])

	require.Len(t, a.Days, forge.Days)
	assert.Equal(t, forge.Day{Date: "2026-03-01", Count: 0}, a.Days[0], "a day that lists nothing counts nothing")
	assert.Equal(t, forge.Day{Date: "2026-03-10", Count: 7}, a.Days[9])
	assert.Equal(t, forge.Day{Date: "2026-03-14", Count: 4}, a.Days[13], "two commits, an issue and a repository created")
	assert.Equal(t, "contributions", a.Unit)

	byKind := map[forge.Kind][]forge.Item{}
	for _, it := range a.Items {
		byKind[it.Kind] = append(byKind[it.Kind], it)
	}
	require.Len(t, byKind[forge.KindCommits], 2, "the commits before the window are not in it")
	assert.Equal(t, forge.Item{Date: "2026-03-10", Repo: "octo/widget", Kind: forge.KindCommits, Count: 5}, byKind[forge.KindCommits][0])
	require.Len(t, byKind[forge.KindPR], 1)
	assert.Equal(t, forge.Item{Date: "2026-03-10", Repo: "octo/widget", Kind: forge.KindPR, Ref: "#12", Title: "feat: a widget", Action: "merged", Count: 1}, byKind[forge.KindPR][0])
	require.Len(t, byKind[forge.KindReview], 1)
	assert.Equal(t, "approved", byKind[forge.KindReview][0].Action)
	require.Len(t, byKind[forge.KindIssue], 1)
	assert.Equal(t, "open", byKind[forge.KindIssue][0].Action)
	require.Len(t, byKind[forge.KindOther], 1)
	assert.Equal(t, forge.Item{Date: "2026-03-14", Repo: "octo/gizmo", Kind: forge.KindOther, Action: "created", Count: 1}, byKind[forge.KindOther][0])
}

// Spec 109 R1. A list with more than a page is read to its end: each later
// request asks only for the lists with a page left, after their cursors, and
// not again for the commits.
func TestGitHubReadsEveryPage(t *testing.T) {
	pr := func(n int) string {
		return fmt.Sprintf(`{"occurredAt":"2026-03-12T10:00:00Z","pullRequest":{"number":%d,"title":"t","state":"OPEN","merged":false,"repository":{"nameWithOwner":"octo/widget"}}}`, n)
	}
	issue := func(n int) string {
		return fmt.Sprintf(`{"occurredAt":"2026-03-12T11:00:00Z","issue":{"number":%d,"title":"t","state":"OPEN","repository":{"nameWithOwner":"octo/widget"}}}`, n)
	}
	var asked []map[string]any
	srv, hc := tlsServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Variables map[string]any `json:"variables"`
		}
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		v := body.Variables
		asked = append(asked, v)
		var parts []string
		if v["first"] == true {
			parts = append(parts, `"commitContributionsByRepository":[{"repository":{"nameWithOwner":"octo/widget"},"contributions":{"nodes":[{"occurredAt":"2026-03-12T09:00:00Z","commitCount":3}]}}]`,
				`"repositoryContributions":{"nodes":[]}`)
		}
		if v["prs"] == true {
			switch v["prAfter"] {
			case nil, "":
				parts = append(parts, `"pullRequestContributions":{"pageInfo":{"hasNextPage":true,"endCursor":"pr1"},"nodes":[`+pr(1)+`,`+pr(2)+`]}`)
			case "pr1":
				parts = append(parts, `"pullRequestContributions":{"pageInfo":{"hasNextPage":true,"endCursor":"pr2"},"nodes":[`+pr(3)+`]}`)
			case "pr2":
				parts = append(parts, `"pullRequestContributions":{"pageInfo":{"hasNextPage":false,"endCursor":"pr3"},"nodes":[`+pr(4)+`]}`)
			}
		}
		if v["reviews"] == true {
			parts = append(parts, `"pullRequestReviewContributions":{"pageInfo":{"hasNextPage":false},"nodes":[]}`)
		}
		if v["issues"] == true {
			switch v["issueAfter"] {
			case nil, "":
				parts = append(parts, `"issueContributions":{"pageInfo":{"hasNextPage":true,"endCursor":"is1"},"nodes":[`+issue(1)+`]}`)
			case "is1":
				parts = append(parts, `"issueContributions":{"pageInfo":{"hasNextPage":false,"endCursor":"is2"},"nodes":[`+issue(2)+`]}`)
			}
		}
		_, _ = io.WriteString(w, `{"data":{"viewer":{"contributionsCollection":{`+strings.Join(parts, ",")+`}}}}`)
	}))

	a, err := forge.GitHubReader{Client: hc, BaseURL: srv.URL + "/", Now: func() time.Time { return now }}.Read(t.Context(), token)
	require.NoError(t, err)
	require.Len(t, asked, 3, "a request a page, the longest list's")
	assert.Equal(t, false, asked[1]["first"], "the commits were asked for again")
	assert.Equal(t, "pr1", asked[1]["prAfter"])
	assert.Equal(t, "is1", asked[1]["issueAfter"])
	assert.Equal(t, false, asked[1]["reviews"], "a list with nothing left was asked for again")
	assert.Equal(t, false, asked[2]["issues"], "a list read to its end was asked for again")

	byKind := map[forge.Kind]int{}
	for _, it := range a.Items {
		byKind[it.Kind]++
	}
	assert.Equal(t, 4, byKind[forge.KindPR], "a page of pull requests was left out")
	assert.Equal(t, 2, byKind[forge.KindIssue], "a page of issues was left out")
	assert.Equal(t, 1, byKind[forge.KindCommits], "the commits were read twice")
	assert.Equal(t, forge.Day{Date: "2026-03-12", Count: 9}, a.Days[11])
}

// R5, R11, AC10. A refused token, and a rate limit, come back as the
// package's own errors, and neither carries the token.
func TestGitHubFailuresAreTyped(t *testing.T) {
	status := http.StatusUnauthorized
	srv, hc := tlsServer(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if status == http.StatusForbidden {
			w.Header().Set("X-RateLimit-Limit", "5000")
			w.Header().Set("X-RateLimit-Remaining", "0")
			w.Header().Set("X-RateLimit-Reset", fmt.Sprint(now.Add(time.Hour).Unix()))
		}
		w.WriteHeader(status)
		_, _ = io.WriteString(w, `{"message":"no"}`)
	}))
	r := forge.GitHubReader{Client: hc, BaseURL: srv.URL + "/", Now: func() time.Time { return now }}

	_, err := r.Read(t.Context(), token)
	require.ErrorIs(t, err, forge.ErrUnauthorized)
	assert.NotContains(t, err.Error(), token)

	status = http.StatusForbidden
	_, err = r.Read(t.Context(), token)
	var rl *forge.RateLimited
	require.ErrorAs(t, err, &rl)
	assert.WithinDuration(t, now.Add(time.Hour), rl.Until, time.Second)
	assert.NotContains(t, err.Error(), token)
}

// gitlabServer is a GitLab with two pages of events and one project, which
// counts how often each is asked for.
func gitlabServer(t *testing.T) (*httptest.Server, *http.Client, *atomic.Int32, *atomic.Int32) {
	t.Helper()
	var pages, projects atomic.Int32
	srv, hc := tlsServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, token, r.Header.Get("PRIVATE-TOKEN"))
		switch r.URL.Path {
		case "/api/v4/events":
			pages.Add(1)
			assert.Equal(t, "2026-02-28", r.URL.Query().Get("after"))
			if r.URL.Query().Get("page") == "2" {
				_, _ = io.WriteString(w, gitlabPage2)
				return
			}
			w.Header().Set("X-Next-Page", "2")
			_, _ = io.WriteString(w, gitlabPage1)
		case "/api/v4/projects/7":
			projects.Add(1)
			_, _ = io.WriteString(w, `{"id":7,"path_with_namespace":"team/agent"}`)
		default:
			http.NotFound(w, r)
		}
	}))
	return srv, hc, &pages, &projects
}

const gitlabPage1 = `[
 {"id":1,"project_id":7,"action_name":"pushed to","created_at":"2026-03-13T10:00:00Z","push_data":{"commit_count":1094,"ref":"main","commit_title":"rebase"}},
 {"id":2,"project_id":7,"action_name":"pushed to","created_at":"2026-03-13T11:00:00Z","push_data":{"commit_count":2,"ref":"main","commit_title":"fix"}},
 {"id":3,"project_id":7,"action_name":"opened","target_type":"MergeRequest","target_iid":693,"target_title":"fix: ids","created_at":"2026-03-13T12:00:00Z"}
]`

//nolint:misspell // "noteable" is GitLab's own field name
const gitlabPage2 = `[
 {"id":4,"project_id":7,"action_name":"accepted","target_type":"MergeRequest","target_iid":693,"target_title":"fix: ids","created_at":"2026-03-13T13:00:00Z"},
 {"id":5,"project_id":7,"action_name":"commented on","target_type":"Note","target_title":"fix: ids","note":{"noteable_type":"MergeRequest","noteable_iid":693},"created_at":"2026-03-12T12:00:00Z"},
 {"id":6,"project_id":7,"action_name":"opened","target_type":"Issue","target_iid":5,"target_title":"too old","created_at":"2026-02-20T12:00:00Z"}
]`

// R4, R7, AC5. GitLab's events, every page, each counted once whatever its
// commit count; a merge request accepted reads as merged; a comment names
// what it was on; the project's path is asked for once.
func TestGitLabCountsEachEventOnce(t *testing.T) {
	srv, hc, pages, projects := gitlabServer(t)
	a, err := forge.GitLabReader{Client: hc, BaseURL: srv.URL + "/api/v4/", Now: func() time.Time { return now }}.Read(t.Context(), token)
	require.NoError(t, err)

	assert.EqualValues(t, 2, pages.Load())
	assert.EqualValues(t, 1, projects.Load(), "the project was looked up more than once")
	require.Len(t, a.Days, forge.Days)
	assert.Equal(t, forge.Day{Date: "2026-03-13", Count: 4}, a.Days[12], "a push of 1,094 commits is one event")
	assert.Equal(t, forge.Day{Date: "2026-03-12", Count: 1}, a.Days[11])
	assert.Equal(t, "events", a.Unit)
	require.Len(t, a.Items, 5, "the event before the window is not in it")
	for _, it := range a.Items {
		assert.Equal(t, "team/agent", it.Repo)
	}
	assert.Equal(t, forge.Item{Date: "2026-03-13", Repo: "team/agent", Kind: forge.KindPushed, Ref: "main", Title: "rebase", Action: "pushed", Count: 1, Commits: 1094}, a.Items[0],
		"a push counts once, and keeps its commits for its page (spec 111)")
	assert.Equal(t, forge.KindMR, a.Items[3].Kind)
	assert.Equal(t, "merged", a.Items[3].Action)
	assert.Equal(t, "!693", a.Items[3].Ref)
	assert.Equal(t, forge.KindComment, a.Items[4].Kind)
	assert.Equal(t, "!693", a.Items[4].Ref)
}

// R5, R11. A refused key is ErrUnauthorized, and a host that cannot be
// reached is said without the host: rule 9 keeps it out of logs too.
func TestGitLabFailuresSayNoHost(t *testing.T) {
	srv, hc := tlsServer(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `{"message":"401 Unauthorized"}`)
	}))
	_, err := forge.GitLabReader{Client: hc, BaseURL: srv.URL + "/api/v4/", Now: func() time.Time { return now }}.Read(t.Context(), token)
	require.ErrorIs(t, err, forge.ErrUnauthorized)

	srv.Close()
	_, err = forge.GitLabReader{Client: hc, Host: "gitlab.example.invalid", Now: func() time.Time { return now }}.Read(t.Context(), token)
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "gitlab.example.invalid")
	assert.NotContains(t, err.Error(), token)
	_, err = forge.GitLabReader{Client: hc, BaseURL: srv.URL + "/api/v4/", Now: func() time.Time { return now }}.Read(t.Context(), token)
	require.Error(t, err)
	assert.NotContains(t, err.Error(), strings.TrimPrefix(srv.URL, "https://"))
}

// Spec 102 review, F4. A GitHub failure says what kind it was, not the host
// it failed on: a proxy's or an interceptor's name is a hostname too.
func TestGitHubFailuresSayNoHost(t *testing.T) {
	srv, hc := tlsServer(t, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	addr := strings.TrimPrefix(srv.URL, "https://")
	srv.Close()
	_, err := forge.GitHubReader{Client: hc, BaseURL: srv.URL + "/", Now: func() time.Time { return now }}.Read(t.Context(), token)
	require.Error(t, err)
	assert.NotContains(t, err.Error(), addr)
	assert.NotContains(t, err.Error(), "127.0.0.1")

	srv2, hc2 := tlsServer(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"errors":[{"message":"internal.example.corp refused"}]}`)
	}))
	_, err = forge.GitHubReader{Client: hc2, BaseURL: srv2.URL + "/", Now: func() time.Time { return now }}.Read(t.Context(), token)
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "example.corp")
}

// Spec 102 review, F5. A host that sends more than a fortnight could hold --
// pages without end, thousands of events a page, hundreds of projects -- is
// read to the reading's bounds and no further.
func TestAGreedyHostIsBounded(t *testing.T) {
	var pages, projects atomic.Int32
	srv, hc := tlsServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/v4/projects/") {
			projects.Add(1)
			_, _ = io.WriteString(w, `{"path_with_namespace":"a/b"}`)
			return
		}
		pages.Add(1)
		w.Header().Set("X-Next-Page", fmt.Sprint(pages.Load()+1))
		var b strings.Builder
		b.WriteString("[")
		for i := range 6000 {
			if i > 0 {
				b.WriteString(",")
			}
			fmt.Fprintf(&b, `{"project_id":%d,"action_name":"opened","target_type":"Issue","created_at":"2026-03-13T10:00:00Z"}`, i%300+1)
		}
		b.WriteString("]")
		_, _ = io.WriteString(w, b.String())
	}))
	a, err := forge.GitLabReader{Client: hc, BaseURL: srv.URL + "/api/v4/", Now: func() time.Time { return now }}.Read(t.Context(), token)
	require.NoError(t, err)
	assert.LessOrEqual(t, len(a.Items), 5000, "the reading kept every event the host sent")
	assert.LessOrEqual(t, pages.Load(), int32(10))
	assert.LessOrEqual(t, projects.Load(), int32(200))
}

// R11, AC8. A redirect is not followed: the token would go with it.
func TestARedirectIsNotFollowed(t *testing.T) {
	var elsewhere atomic.Int32
	other, _ := tlsServer(t, http.HandlerFunc(func(http.ResponseWriter, *http.Request) { elsewhere.Add(1) }))
	srv, hc := tlsServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, other.URL+"/graphql", http.StatusTemporaryRedirect)
	}))
	_, err := forge.GitHubReader{Client: hc, BaseURL: srv.URL + "/", Now: func() time.Time { return now }}.Read(t.Context(), token)
	require.Error(t, err)
	assert.Zero(t, elsewhere.Load(), "the redirect was followed")
}

// R11, AC8. A response longer than MaxBody is refused, not cut.
func TestALongResponseIsRefused(t *testing.T) {
	srv, hc := tlsServer(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"data":{"pad":"`)
		_, _ = io.WriteString(w, strings.Repeat("x", forge.MaxBody))
		_, _ = io.WriteString(w, `"}}`)
	}))
	_, err := forge.GitHubReader{Client: hc, BaseURL: srv.URL + "/", Now: func() time.Time { return now }}.Read(t.Context(), token)
	require.ErrorIs(t, err, forge.ErrTooLarge)
}

// R11, AC8. Nothing goes over plain HTTP, and a request is bounded in time.
func TestOnlyHTTPSAndBoundedInTime(t *testing.T) {
	plain := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("a plain-HTTP request was sent")
	}))
	t.Cleanup(plain.Close)
	_, err := forge.GitHubReader{Client: forge.NewClient(nil), BaseURL: plain.URL + "/", Now: func() time.Time { return now }}.Read(t.Context(), token)
	require.ErrorIs(t, err, forge.ErrNotHTTPS)

	assert.Positive(t, forge.NewClient(nil).Timeout)
	release := make(chan struct{})
	srv, hc := tlsServer(t, http.HandlerFunc(func(http.ResponseWriter, *http.Request) { <-release }))
	defer close(release)
	ctx, cancel := context.WithTimeout(t.Context(), 200*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err = forge.GitHubReader{Client: hc, BaseURL: srv.URL + "/", Now: func() time.Time { return now }}.Read(ctx, token)
	require.Error(t, err)
	assert.True(t, errors.Is(err, context.DeadlineExceeded) || strings.Contains(err.Error(), "deadline"), err.Error())
	assert.Less(t, time.Since(start), 5*time.Second)
}

// githubPrivate is the person's own private repositories, invented: one with
// their commits on two days, a commit by someone else, one by an address no
// account claims, and one before the window; one created in the window with
// nothing on it yet; and one the collection already lists.
const githubPrivate = `{"data":{"viewer":{"login":"Octo",
  "repositories":{"nodes":[
    {"nameWithOwner":"octo/secret","createdAt":"2025-01-01T00:00:00Z","defaultBranchRef":{"target":{"history":{"pageInfo":{"hasNextPage":false},"nodes":[
      {"authoredDate":"2026-03-12T10:00:00Z","author":{"user":{"login":"octo"}}},
      {"authoredDate":"2026-03-12T11:00:00Z","author":{"user":{"login":"octo"}}},
      {"authoredDate":"2026-03-13T10:00:00Z","author":{"user":{"login":"octo"}}},
      {"authoredDate":"2026-03-13T11:00:00Z","author":{"user":{"login":"someone"}}},
      {"authoredDate":"2026-03-13T12:00:00Z","author":{"user":null}},
      {"authoredDate":"2026-02-20T12:00:00Z","author":{"user":{"login":"octo"}}}]}}}},
    {"nameWithOwner":"octo/fresh","createdAt":"2026-03-11T09:00:00Z","defaultBranchRef":null},
    {"nameWithOwner":"octo/widget","createdAt":"2025-01-01T00:00:00Z","defaultBranchRef":{"target":{"history":{"pageInfo":{"hasNextPage":false},"nodes":[
      {"authoredDate":"2026-03-10T12:00:00Z","author":{"user":{"login":"octo"}}}]}}}}]},
  "contributionsCollection":{
    "commitContributionsByRepository":[
      {"repository":{"nameWithOwner":"octo/widget"},"contributions":{"nodes":[{"occurredAt":"2026-03-10T12:00:00Z","commitCount":5}]}}],
    "repositoryContributions":{"nodes":[]},
    "pullRequestContributions":{"pageInfo":{"hasNextPage":false},"nodes":[]},
    "pullRequestReviewContributions":{"pageInfo":{"hasNextPage":false},"nodes":[]},
    "issueContributions":{"pageInfo":{"hasNextPage":false},"nodes":[]}}}}}`

// Spec 114 R1, R2, R3. The collection lists none of a private repository's
// work, so the person's own private repositories are asked for beside it:
// the commits they wrote on each authored day, and a repository created,
// and nothing the collection already counts.
func TestGitHubListsThePersonsPrivateRepositories(t *testing.T) {
	var asked struct {
		Query     string         `json:"query"`
		Variables map[string]any `json:"variables"`
	}
	srv, hc := tlsServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, json.NewDecoder(r.Body).Decode(&asked))
		_, _ = io.WriteString(w, githubPrivate)
	}))
	a, err := forge.GitHubReader{Client: hc, BaseURL: srv.URL + "/", Now: func() time.Time { return now }}.Read(t.Context(), token)
	require.NoError(t, err)
	assert.Contains(t, asked.Query, "privacy: PRIVATE")
	assert.Contains(t, asked.Query, "ownerAffiliations: [OWNER]", "an organisation's repositories were asked for")
	assert.Equal(t, asked.Variables["from"], asked.Variables["since"])
	assert.Equal(t, asked.Variables["to"], asked.Variables["until"])

	var secret []forge.Item
	var other []forge.Item
	widget := 0
	for _, it := range a.Items {
		switch {
		case it.Repo == "octo/secret":
			secret = append(secret, it)
		case it.Kind == forge.KindOther:
			other = append(other, it)
		case it.Repo == "octo/widget":
			widget += it.Count
		}
	}
	assert.Equal(t, []forge.Item{
		{Date: "2026-03-12", Repo: "octo/secret", Kind: forge.KindCommits, Count: 2},
		{Date: "2026-03-13", Repo: "octo/secret", Kind: forge.KindCommits, Count: 1},
	}, secret, "a day's commits by the person, and only theirs, in the window")
	assert.Equal(t, []forge.Item{{Date: "2026-03-11", Repo: "octo/fresh", Kind: forge.KindOther, Action: "created", Count: 1}}, other)
	assert.Equal(t, 5, widget, "a repository the collection lists was counted twice")
	assert.Equal(t, forge.Day{Date: "2026-03-12", Count: 2}, a.Days[11])
	assert.Equal(t, forge.Day{Date: "2026-03-13", Count: 1}, a.Days[12])
}

// Spec 114 R4. A private repository with more than a page of commits is
// read on after its cursor, and a host that always has another page is
// stopped at the reading's ten requests.
func TestGitHubReadsAPrivateHistoryToItsEnd(t *testing.T) {
	commit := `{"authoredDate":"2026-03-12T10:00:00Z","author":{"user":{"login":"octo"}}}`
	endless := false
	var asked []map[string]any
	srv, hc := tlsServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Query     string         `json:"query"`
			Variables map[string]any `json:"variables"`
		}
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		asked = append(asked, body.Variables)
		if strings.Contains(body.Query, "contributionsCollection") {
			_, _ = io.WriteString(w, `{"data":{"viewer":{"login":"octo","repositories":{"nodes":[
			  {"nameWithOwner":"octo/secret","createdAt":"2025-01-01T00:00:00Z","defaultBranchRef":{"target":{"history":{
			    "pageInfo":{"hasNextPage":true,"endCursor":"c1"},"nodes":[`+commit+`,`+commit+`]}}}}]},
			  "contributionsCollection":{}}}}`)
			return
		}
		more := endless || body.Variables["after"] == "c1"
		_, _ = fmt.Fprintf(w, `{"data":{"repository":{"defaultBranchRef":{"target":{"history":{
		  "pageInfo":{"hasNextPage":%t,"endCursor":"c%d"},"nodes":[%s]}}}}}}`, more, len(asked), commit)
	}))
	r := forge.GitHubReader{Client: hc, BaseURL: srv.URL + "/", Now: func() time.Time { return now }}

	a, err := r.Read(t.Context(), token)
	require.NoError(t, err)
	require.Len(t, asked, 3, "a request a page of the history")
	assert.Equal(t, "octo", asked[1]["owner"])
	assert.Equal(t, "secret", asked[1]["name"])
	assert.Equal(t, "c1", asked[1]["after"])
	assert.Equal(t, "c2", asked[2]["after"])
	assert.Equal(t, forge.Day{Date: "2026-03-12", Count: 4}, a.Days[11], "a page of commits was left out")

	endless, asked = true, nil
	_, err = r.Read(t.Context(), token)
	require.NoError(t, err)
	assert.Len(t, asked, 10, "an endless history was read past the reading's bound")
}
