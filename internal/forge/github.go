package forge

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/go-github/v93/github"
)

/*
contributionsQuery is a fortnight of the viewer's contributions (spec 102):
the commits by repository by day, the repositories created, and the pull
requests, reviews and issues opened, with their titles. The REST events list
is not used: it stops at 300 events, and its payloads no longer carry commit
counts or titles.

The pull requests, reviews and issues come a hundred at a time, and a busy
fortnight has more (spec 109): the first request asks for everything, and
each later one only for the lists with a page left, after their cursors.

The calendar and the restricted count are not asked for: the calendar counts
contributions in an organisation's private repositories that this sign-in
cannot see and GitHub will not name, and a day is what it lists (spec 109).
*/
const contributionsQuery = `query($from: DateTime!, $to: DateTime!, $first: Boolean!,
  $prs: Boolean!, $prAfter: String, $reviews: Boolean!, $reviewAfter: String, $issues: Boolean!, $issueAfter: String) {
  viewer {
    contributionsCollection(from: $from, to: $to) {
      commitContributionsByRepository(maxRepositories: 100) @include(if: $first) {
        repository { nameWithOwner }
        contributions(first: 100) { nodes { occurredAt commitCount } }
      }
      repositoryContributions(first: 100) @include(if: $first) { nodes { occurredAt repository { nameWithOwner } } }
      pullRequestContributions(first: 100, after: $prAfter) @include(if: $prs) {
        pageInfo { hasNextPage endCursor }
        nodes { occurredAt pullRequest { number title state merged repository { nameWithOwner } } }
      }
      pullRequestReviewContributions(first: 100, after: $reviewAfter) @include(if: $reviews) {
        pageInfo { hasNextPage endCursor }
        nodes { occurredAt pullRequestReview { state } pullRequest { number title repository { nameWithOwner } } }
      }
      issueContributions(first: 100, after: $issueAfter) @include(if: $issues) {
        pageInfo { hasNextPage endCursor }
        nodes { occurredAt issue { number title state repository { nameWithOwner } } }
      }
    }
  }
}`

// maxGitHubPages bounds the requests one reading makes, as maxEventPages
// does GitLab's: a thousand of each kind is far past a person's fortnight.
const maxGitHubPages = 10

// ghPage is where a list stopped.
type ghPage struct {
	HasNextPage bool   `json:"hasNextPage"`
	EndCursor   string `json:"endCursor"`
}

type ghRepo struct {
	NameWithOwner string `json:"nameWithOwner"`
}

type ghThing struct {
	Number     int    `json:"number"`
	Title      string `json:"title"`
	State      string `json:"state"`
	Merged     bool   `json:"merged"`
	Repository ghRepo `json:"repository"`
}

type ghResponse struct {
	Data struct {
		Viewer struct {
			CC struct {
				Commits []struct {
					Repository    ghRepo `json:"repository"`
					Contributions struct {
						Nodes []struct {
							OccurredAt  time.Time `json:"occurredAt"`
							CommitCount int       `json:"commitCount"`
						} `json:"nodes"`
					} `json:"contributions"`
				} `json:"commitContributionsByRepository"`
				Created struct {
					Nodes []struct {
						OccurredAt time.Time `json:"occurredAt"`
						Repository ghRepo    `json:"repository"`
					} `json:"nodes"`
				} `json:"repositoryContributions"`
				PRs struct {
					PageInfo ghPage `json:"pageInfo"`
					Nodes    []struct {
						OccurredAt  time.Time `json:"occurredAt"`
						PullRequest ghThing   `json:"pullRequest"`
					} `json:"nodes"`
				} `json:"pullRequestContributions"`
				Reviews struct {
					PageInfo ghPage `json:"pageInfo"`
					Nodes    []struct {
						OccurredAt time.Time `json:"occurredAt"`
						Review     struct {
							State string `json:"state"`
						} `json:"pullRequestReview"`
						PullRequest ghThing `json:"pullRequest"`
					} `json:"nodes"`
				} `json:"pullRequestReviewContributions"`
				Issues struct {
					PageInfo ghPage `json:"pageInfo"`
					Nodes    []struct {
						OccurredAt time.Time `json:"occurredAt"`
						Issue      ghThing   `json:"issue"`
					} `json:"nodes"`
				} `json:"issueContributions"`
			} `json:"contributionsCollection"`
		} `json:"viewer"`
	} `json:"data"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

// GitHubReader reads the viewer's fortnight from GitHub.
type GitHubReader struct {
	// Client is the HTTP client, NewClient's; a test gives one for its
	// server.
	Client *http.Client
	// BaseURL is the API's root, ending in "/": GitHub's own when empty.
	BaseURL string
	Now     func() time.Time
}

// Read is one reading with token.
func (r GitHubReader) Read(ctx context.Context, token string) (Activity, error) {
	now := time.Now
	if r.Now != nil {
		now = r.Now
	}
	c, err := r.newClient(token)
	if err != nil {
		return Activity{}, err
	}
	start, dates := Window(now())
	end := start.AddDate(0, 0, Days).Add(-time.Second)
	vars := map[string]any{
		"from": start.Format(time.RFC3339), "to": end.Format(time.RFC3339),
		"first": true, "prs": true, "reviews": true, "issues": true,
	}
	var all ghResponse
	for range maxGitHubPages {
		out, err := githubQuery(ctx, c, vars)
		if err != nil {
			return Activity{}, err
		}
		got, have := &out.Data.Viewer.CC, &all.Data.Viewer.CC
		if vars["first"] == true {
			have.Commits, have.Created = got.Commits, got.Created
		}
		have.PRs.Nodes = append(have.PRs.Nodes, got.PRs.Nodes...)
		have.Reviews.Nodes = append(have.Reviews.Nodes, got.Reviews.Nodes...)
		have.Issues.Nodes = append(have.Issues.Nodes, got.Issues.Nodes...)
		// The next request asks only for the lists with a page left.
		more := false
		for _, l := range []struct {
			on, after string
			was       bool
			page      ghPage
		}{
			{"prs", "prAfter", vars["prs"] == true, got.PRs.PageInfo},
			{"reviews", "reviewAfter", vars["reviews"] == true, got.Reviews.PageInfo},
			{"issues", "issueAfter", vars["issues"] == true, got.Issues.PageInfo},
		} {
			next := l.was && l.page.HasNextPage && l.page.EndCursor != ""
			vars[l.on], vars[l.after] = next, l.page.EndCursor
			more = more || next
		}
		vars["first"] = false
		if !more {
			break
		}
	}
	return githubActivity(all, dates, now()), nil
}

// githubQuery is one request of contributionsQuery.
func githubQuery(ctx context.Context, c *github.Client, vars map[string]any) (ghResponse, error) {
	body := map[string]any{"query": contributionsQuery, "variables": vars}
	req, err := c.NewRequest(ctx, http.MethodPost, "graphql", body)
	if err != nil {
		return ghResponse{}, fmt.Errorf("github: %w", err)
	}
	var out ghResponse
	if _, err := c.Do(req, &out); err != nil {
		return ghResponse{}, githubError(err)
	}
	if len(out.Errors) > 0 {
		// The message is GitHub's text, not this program's: it is not
		// carried, only that the query was refused.
		return ghResponse{}, errors.New("github: the query was refused")
	}
	return out, nil
}

func (r GitHubReader) client() *http.Client {
	if r.Client != nil {
		return r.Client
	}
	return NewClient(nil)
}

// githubActivity turns a response into a reading over dates.
func githubActivity(out ghResponse, dates []string, now time.Time) Activity {
	cc := out.Data.Viewer.CC
	a := Activity{Provider: GitHub, Unit: "contributions", ReadAt: now}
	add := func(it Item) {
		if dateIn(dates, it.Date) && len(a.Items) < MaxItems {
			a.Items = append(a.Items, it)
		}
	}
	for _, r := range cc.Commits {
		for _, n := range r.Contributions.Nodes {
			if n.CommitCount < 1 {
				// A day is counted from its items: no host's number takes
				// a day below nothing.
				continue
			}
			add(Item{Date: DateOf(n.OccurredAt), Repo: r.Repository.NameWithOwner, Kind: KindCommits, Count: n.CommitCount})
		}
	}
	for _, n := range cc.Created.Nodes {
		add(Item{Date: DateOf(n.OccurredAt), Repo: n.Repository.NameWithOwner, Kind: KindOther, Action: "created", Count: 1})
	}
	for _, n := range cc.PRs.Nodes {
		pr := n.PullRequest
		add(Item{Date: DateOf(n.OccurredAt), Repo: pr.Repository.NameWithOwner, Kind: KindPR,
			Ref: fmt.Sprintf("#%d", pr.Number), Title: pr.Title, Action: prState(pr), Count: 1})
	}
	for _, n := range cc.Reviews.Nodes {
		pr := n.PullRequest
		add(Item{Date: DateOf(n.OccurredAt), Repo: pr.Repository.NameWithOwner, Kind: KindReview,
			Ref: fmt.Sprintf("#%d", pr.Number), Title: pr.Title, Action: strings.ToLower(n.Review.State), Count: 1})
	}
	for _, n := range cc.Issues.Nodes {
		is := n.Issue
		add(Item{Date: DateOf(n.OccurredAt), Repo: is.Repository.NameWithOwner, Kind: KindIssue,
			Ref: fmt.Sprintf("#%d", is.Number), Title: is.Title, Action: strings.ToLower(is.State), Count: 1})
	}
	// A day is what it lists (spec 109).
	a.Days = countItems(dates, a.Items)
	return a
}

func dateIn(dates []string, d string) bool {
	return len(dates) > 0 && d >= dates[0] && d <= dates[len(dates)-1]
}

// prState is where a pull request is now: merged, open or closed.
func prState(pr ghThing) string {
	if pr.Merged {
		return "merged"
	}
	return strings.ToLower(pr.State)
}

// githubError is go-github's error as one of this package's.
func githubError(err error) error {
	var rl *github.RateLimitError
	if errors.As(err, &rl) {
		return &RateLimited{Until: rl.Rate.Reset.Time}
	}
	var ab *github.AbuseRateLimitError
	if errors.As(err, &ab) {
		until := time.Time{}
		if ab.RetryAfter != nil {
			until = time.Now().Add(*ab.RetryAfter)
		}
		return &RateLimited{Until: until}
	}
	var er *github.ErrorResponse
	if errors.As(err, &er) && er.Response != nil {
		if er.Response.StatusCode == http.StatusUnauthorized {
			return ErrUnauthorized
		}
		return fmt.Errorf("github: the API answered %d", er.Response.StatusCode)
	}
	return hostless(GitHub, err)
}
