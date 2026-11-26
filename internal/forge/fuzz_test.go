package forge

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	gitlab "gitlab.com/gitlab-org/api/client-go"
)

// FuzzParseLine is the secrets file's lines (spec 102 R10): whatever a line
// says, a key has no '=' and no surrounding space, and a comment is nothing.
func FuzzParseLine(f *testing.F) {
	for _, s := range []string{"GLAB_TOKEN=x", `export GITLAB_TOKEN="a b"`, "# c", "K='v' # d", "=", "x", "A==b", "export =v"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, line string) {
		k, v, ok := parseLine(line)
		if !ok {
			return
		}
		if k == "" || strings.Contains(k, "=") || strings.TrimSpace(k) != k {
			t.Fatalf("key %q from %q", k, line)
		}
		if strings.HasPrefix(strings.TrimSpace(line), "#") {
			t.Fatalf("a comment gave %q=%q", k, v)
		}
	})
}

// FuzzGitHubResponse is a GraphQL answer, as GitHub or anyone answering in
// its place might send it (spec 102, B4): it decodes or is refused, and a
// reading from it has the window's fourteen days and items only within it.
func FuzzGitHubResponse(f *testing.F) {
	f.Add([]byte(`{"data":{"viewer":{"contributionsCollection":{"contributionCalendar":{"weeks":[{"contributionDays":[{"date":"2026-03-10","contributionCount":7}]}]}}}}}`))
	f.Add([]byte(`{"data":{"viewer":{"contributionsCollection":{"commitContributionsByRepository":[{"repository":{"nameWithOwner":"a/b"},"contributions":{"nodes":[{"occurredAt":"2026-03-10T12:00:00Z","commitCount":-5}]}}]}}}}`))
	f.Add([]byte(`{"errors":[{"message":"x"}]}`))
	f.Add([]byte(`{"data":{"viewer":{"login":"a","repositories":{"nodes":[{"nameWithOwner":"a/c","createdAt":"2026-03-12T00:00:00Z","defaultBranchRef":{"target":{"history":{"nodes":[{"authoredDate":"2026-03-12T10:00:00Z","author":{"user":{"login":"A"}}},{"authoredDate":"2026-03-12T10:00:00Z","author":{"user":null}}]}}}},{"nameWithOwner":"","defaultBranchRef":null}]}}}}`))
	f.Add([]byte(`{"data":{"viewer":{"contributionsCollection":{"repositoryContributions":{"nodes":[{"occurredAt":"2026-03-10T12:00:00Z","repository":{"nameWithOwner":"a/b"}}]},"pullRequestContributions":{"pageInfo":{"hasNextPage":true,"endCursor":"x"},"nodes":[{"occurredAt":"2026-03-11T12:00:00Z","pullRequest":{"number":1,"title":"t","state":"OPEN","repository":{"nameWithOwner":"a/b"}}}]}}}}}`))
	now := time.Date(2026, 3, 14, 12, 0, 0, 0, time.UTC)
	_, dates := Window(now)
	f.Fuzz(func(t *testing.T, body []byte) {
		var out ghResponse
		if json.Unmarshal(body, &out) != nil {
			return
		}
		a := githubActivity(out, dates, now)
		if len(a.Days) != Days {
			t.Fatalf("%d days", len(a.Days))
		}
		for _, d := range a.Days {
			if d.Count < 0 {
				t.Fatalf("%s counts %d", d.Date, d.Count)
			}
		}
		for _, it := range a.Items {
			if !dateIn(dates, it.Date) {
				t.Fatalf("an item on %s, outside the window", it.Date)
			}
		}
	})
}

// FuzzGitLabEvent is one event as a GitLab host might send it: it becomes
// one item counted once, whatever its push says.
func FuzzGitLabEvent(f *testing.F) {
	f.Add([]byte(`{"action_name":"pushed to","created_at":"2026-03-13T10:00:00Z","push_data":{"commit_count":1094,"ref":"main"}}`))
	f.Add([]byte(`{"action_name":"commented on","target_type":"Note","created_at":"2026-03-13T10:00:00Z","note":{"noteable_type":"Issue","noteable_iid":5}}`)) //nolint:misspell // "noteable" is GitLab's own field name
	f.Fuzz(func(t *testing.T, body []byte) {
		var e gitlab.ContributionEvent
		if json.Unmarshal(body, &e) != nil || e.CreatedAt == nil {
			return
		}
		if it := gitlabItem(&e); it.Count != 1 {
			t.Fatalf("an event counted %d times", it.Count)
		}
	})
}
