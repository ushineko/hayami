package forge

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
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

The collection lists no private repository's work to this sign-in, not even
the person's own, which it only counts as restricted (spec 114). The first
request also asks for the person's own private repositories pushed to
lately, with their default branch's commits in the fortnight, and the commits
the person wrote are counted as GitHub counts them: on their authored day,
merges included. A repository whose history has more than a page is read on
by historyQuery.
*/
const contributionsQuery = `query($from: DateTime!, $to: DateTime!, $since: GitTimestamp!, $until: GitTimestamp!, $first: Boolean!,
  $prs: Boolean!, $prAfter: String, $reviews: Boolean!, $reviewAfter: String, $issues: Boolean!, $issueAfter: String) {
  viewer {
    login @include(if: $first)
    repositories(first: 50, privacy: PRIVATE, ownerAffiliations: [OWNER], isFork: false,
      orderBy: {field: PUSHED_AT, direction: DESC}) @include(if: $first) {
      nodes {
        nameWithOwner createdAt
        defaultBranchRef { target { ... on Commit { history(first: 100, since: $since, until: $until) { ...commits } } } }
      }
    }
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
}
` + commitsFragment

// historyQuery is the next page of one private repository's commits.
const historyQuery = `query($owner: String!, $name: String!, $since: GitTimestamp!, $until: GitTimestamp!, $after: String!) {
  repository(owner: $owner, name: $name) {
    defaultBranchRef { target { ... on Commit { history(first: 100, since: $since, until: $until, after: $after) { ...commits } } } }
  }
}
` + commitsFragment

const commitsFragment = `fragment commits on CommitHistoryConnection {
  pageInfo { hasNextPage endCursor }
  nodes { authoredDate author { user { login } } }
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

// ghHistory is a page of a branch's commits.
type ghHistory struct {
	PageInfo ghPage `json:"pageInfo"`
	Nodes    []struct {
		AuthoredDate time.Time `json:"authoredDate"`
		Author       struct {
			User *struct {
				Login string `json:"login"`
			} `json:"user"`
		} `json:"author"`
	} `json:"nodes"`
}

// ghBranch is a default branch's commits, as a repository and historyQuery
// both answer it.
type ghBranch struct {
	DefaultBranchRef *struct {
		Target struct {
			History ghHistory `json:"history"`
		} `json:"target"`
	} `json:"defaultBranchRef"`
}

// ghPrivateRepo is one of the person's own private repositories.
type ghPrivateRepo struct {
	NameWithOwner string    `json:"nameWithOwner"`
	CreatedAt     time.Time `json:"createdAt"`
	ghBranch
}

// history is the repository's commits, nil for an empty repository.
func (b ghBranch) history() *ghHistory {
	if b.DefaultBranchRef == nil {
		return nil
	}
	return &b.DefaultBranchRef.Target.History
}

type ghResponse struct {
	Data struct {
		Repository ghBranch `json:"repository"`
		Viewer     struct {
			Login   string `json:"login"`
			Private struct {
				Nodes []ghPrivateRepo `json:"nodes"`
			} `json:"repositories"`
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
	from, to := start.Format(time.RFC3339), end.Format(time.RFC3339)
	vars := map[string]any{
		"from": from, "to": to, "since": from, "until": to,
		"first": true, "prs": true, "reviews": true, "issues": true,
	}
	var all ghResponse
	requests := 0
	for requests < maxGitHubPages {
		requests++
		out, err := githubQuery(ctx, c, contributionsQuery, vars)
		if err != nil {
			return Activity{}, err
		}
		got, have := &out.Data.Viewer.CC, &all.Data.Viewer.CC
		if vars["first"] == true {
			have.Commits, have.Created = got.Commits, got.Created
			all.Data.Viewer.Login, all.Data.Viewer.Private = out.Data.Viewer.Login, out.Data.Viewer.Private
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
	// A private repository with more than a page of commits is read on,
	// within what is left of the reading's requests.
	for i := range all.Data.Viewer.Private.Nodes {
		h := all.Data.Viewer.Private.Nodes[i].history()
		owner, name, ok := strings.Cut(all.Data.Viewer.Private.Nodes[i].NameWithOwner, "/")
		for ok && h != nil && h.PageInfo.HasNextPage && h.PageInfo.EndCursor != "" && requests < maxGitHubPages {
			requests++
			out, err := githubQuery(ctx, c, historyQuery, map[string]any{
				"owner": owner, "name": name, "since": from, "until": to, "after": h.PageInfo.EndCursor,
			})
			if err != nil {
				return Activity{}, err
			}
			next := out.Data.Repository.history()
			if next == nil {
				break
			}
			h.Nodes = append(h.Nodes, next.Nodes...)
			h.PageInfo = next.PageInfo
		}
	}
	return githubActivity(all, dates, now()), nil
}

// githubQuery is one request of query.
func githubQuery(ctx context.Context, c *github.Client, query string, vars map[string]any) (ghResponse, error) {
	body := map[string]any{"query": query, "variables": vars}
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
	privateItems(out, add)
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

// privateItems are the person's own private repositories' work (spec 114):
// each day's commits the person wrote on the default branch, and a
// repository created. A repository the collection already listed is left
// to it, so nothing is counted twice if GitHub starts listing them.
func privateItems(out ghResponse, add func(Item)) {
	v := out.Data.Viewer
	listed := map[string]bool{}
	for _, r := range v.CC.Commits {
		listed[strings.ToLower(r.Repository.NameWithOwner)] = true
	}
	for _, n := range v.CC.Created.Nodes {
		listed[strings.ToLower(n.Repository.NameWithOwner)] = true
	}
	for _, r := range v.Private.Nodes {
		if r.NameWithOwner == "" || listed[strings.ToLower(r.NameWithOwner)] {
			continue
		}
		listed[strings.ToLower(r.NameWithOwner)] = true
		add(Item{Date: DateOf(r.CreatedAt), Repo: r.NameWithOwner, Kind: KindOther, Action: "created", Count: 1})
		h := r.history()
		if h == nil || v.Login == "" {
			continue
		}
		perDay := map[string]int{}
		var days []string
		for _, c := range h.Nodes {
			if c.Author.User == nil || !strings.EqualFold(c.Author.User.Login, v.Login) {
				continue
			}
			d := DateOf(c.AuthoredDate)
			if perDay[d] == 0 {
				days = append(days, d)
			}
			perDay[d]++
		}
		slices.Sort(days)
		for _, d := range days {
			add(Item{Date: d, Repo: r.NameWithOwner, Kind: KindCommits, Count: perDay[d]})
		}
	}
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
