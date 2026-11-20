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
contributionsQuery is a fortnight of the viewer's contributions in one
request (spec 102): the calendar the profile's heatmap is drawn from, the
commits by repository by day, and the pull requests, reviews and issues
opened, with their titles. The REST events list is not used: it stops at
300 events, and its payloads no longer carry commit counts or titles.
*/
const contributionsQuery = `query($from: DateTime!, $to: DateTime!) {
  viewer {
    contributionsCollection(from: $from, to: $to) {
      restrictedContributionsCount
      contributionCalendar { weeks { contributionDays { date contributionCount } } }
      commitContributionsByRepository(maxRepositories: 100) {
        repository { nameWithOwner }
        contributions(first: 100) { nodes { occurredAt commitCount } }
      }
      pullRequestContributions(first: 100) { nodes { occurredAt pullRequest { number title state merged repository { nameWithOwner } } } }
      pullRequestReviewContributions(first: 100) { nodes { occurredAt pullRequestReview { state } pullRequest { number title repository { nameWithOwner } } } }
      issueContributions(first: 100) { nodes { occurredAt issue { number title state repository { nameWithOwner } } } }
    }
  }
}`

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
				Restricted int `json:"restrictedContributionsCount"`
				Calendar   struct {
					Weeks []struct {
						Days []struct {
							Date  string `json:"date"`
							Count int    `json:"contributionCount"`
						} `json:"contributionDays"`
					} `json:"weeks"`
				} `json:"contributionCalendar"`
				Commits []struct {
					Repository    ghRepo `json:"repository"`
					Contributions struct {
						Nodes []struct {
							OccurredAt  time.Time `json:"occurredAt"`
							CommitCount int       `json:"commitCount"`
						} `json:"nodes"`
					} `json:"contributions"`
				} `json:"commitContributionsByRepository"`
				PRs struct {
					Nodes []struct {
						OccurredAt  time.Time `json:"occurredAt"`
						PullRequest ghThing   `json:"pullRequest"`
					} `json:"nodes"`
				} `json:"pullRequestContributions"`
				Reviews struct {
					Nodes []struct {
						OccurredAt time.Time `json:"occurredAt"`
						Review     struct {
							State string `json:"state"`
						} `json:"pullRequestReview"`
						PullRequest ghThing `json:"pullRequest"`
					} `json:"nodes"`
				} `json:"pullRequestReviewContributions"`
				Issues struct {
					Nodes []struct {
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
	opts := []github.ClientOptionsFunc{
		github.WithHTTPClient(r.client()),
		github.WithAuthToken(token),
		github.WithUserAgent("hayami"),
	}
	if r.BaseURL != "" {
		opts = append(opts, github.WithURLs(&r.BaseURL, &r.BaseURL))
	}
	c, err := github.NewClient(opts...)
	if err != nil {
		return Activity{}, fmt.Errorf("github: %w", err)
	}
	start, dates := Window(now())
	end := start.AddDate(0, 0, Days).Add(-time.Second)
	body := map[string]any{
		"query": contributionsQuery,
		"variables": map[string]string{
			"from": start.Format(time.RFC3339),
			"to":   end.Format(time.RFC3339),
		},
	}
	req, err := c.NewRequest(ctx, http.MethodPost, "graphql", body)
	if err != nil {
		return Activity{}, fmt.Errorf("github: %w", err)
	}
	var out ghResponse
	if _, err := c.Do(req, &out); err != nil {
		return Activity{}, githubError(err)
	}
	if len(out.Errors) > 0 {
		// The message is GitHub's text, not this program's: it is not
		// carried, only that the query was refused.
		return Activity{}, errors.New("github: the query was refused")
	}
	return githubActivity(out, dates, now()), nil
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
	calendar := map[string]int{}
	for _, w := range cc.Calendar.Weeks {
		for _, d := range w.Days {
			calendar[d.Date] = d.Count
		}
	}
	a := Activity{Provider: GitHub, Unit: "contributions", Private: cc.Restricted, ReadAt: now}
	for _, d := range dates {
		a.Days = append(a.Days, Day{Date: d, Count: calendar[d]})
	}
	add := func(it Item) {
		if dateIn(dates, it.Date) {
			a.Items = append(a.Items, it)
		}
	}
	for _, r := range cc.Commits {
		for _, n := range r.Contributions.Nodes {
			add(Item{Date: DateOf(n.OccurredAt), Repo: r.Repository.NameWithOwner, Kind: KindCommits, Count: n.CommitCount})
		}
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
