package forge

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	gitlab "gitlab.com/gitlab-org/api/client-go"
)

// A reading's bounds (spec 102 review, F5): a fortnight of a busy person's
// events is a few pages of a hundred in a dozen projects. A host that sends
// more is kept to these, so the panel's memory and the requests it makes are
// not the host's to choose.
const (
	maxEventPages = 10
	MaxItems      = 5000
	maxProjects   = 200
)

// GitLabReader reads the current user's fortnight from a GitLab host.
type GitLabReader struct {
	Host   string
	Client *http.Client
	Now    func() time.Time
	// BaseURL overrides https://<Host>/api/v4/ for a test.
	BaseURL string
}

// Read is one reading with token.
func (r GitLabReader) Read(ctx context.Context, token string) (Activity, error) {
	now := time.Now
	if r.Now != nil {
		now = r.Now
	}
	base := r.BaseURL
	if base == "" {
		if r.Host == "" {
			return Activity{}, fmt.Errorf("%w: no GitLab host; set one in Preferences or GITLAB_HOST in ~/.secrets.env", ErrNoCredential)
		}
		base = "https://" + r.Host + "/api/v4/"
	}
	hc := r.Client
	if hc == nil {
		hc = NewClient(nil)
	}
	c, err := gitlab.NewClient(token,
		gitlab.WithBaseURL(base),
		gitlab.WithHTTPClient(hc),
		gitlab.WithoutRetries(),
		gitlab.WithUserAgent("hayami"),
	)
	if err != nil {
		return Activity{}, fmt.Errorf("gitlab: %w", err)
	}
	start, dates := Window(now())
	// after is exclusive, and a day: the day before the window's first.
	after := gitlab.ISOTime(start.AddDate(0, 0, -1))
	opt := &gitlab.ListContributionEventsOptions{
		After:       &after,
		ListOptions: gitlab.ListOptions{PerPage: 100, Page: 1},
	}
	paths := map[int64]string{}
	var items []Item
	for range maxEventPages {
		evs, resp, err := c.Events.ListCurrentUserContributionEvents(opt, gitlab.WithContext(ctx))
		if err != nil {
			return Activity{}, gitlabError(err)
		}
		for _, e := range evs {
			if e == nil || e.CreatedAt == nil {
				continue
			}
			it := gitlabItem(e)
			if !dateIn(dates, it.Date) {
				continue
			}
			if _, ok := paths[e.ProjectID]; !ok && e.ProjectID != 0 {
				if len(paths) < maxProjects {
					paths[e.ProjectID] = projectPath(ctx, c, e.ProjectID)
				} else {
					paths[e.ProjectID] = "project " + strconv.FormatInt(e.ProjectID, 10)
				}
			}
			it.Repo = paths[e.ProjectID]
			items = append(items, it)
			if len(items) >= MaxItems {
				break
			}
		}
		if len(items) >= MaxItems {
			break
		}
		if resp == nil || resp.NextPage == 0 {
			break
		}
		opt.Page = resp.NextPage
	}
	return Activity{Provider: GitLab, Unit: "events", Days: countItems(dates, items), Items: items, ReadAt: now()}, nil
}

// projectPath is a project's path with its namespace, or its number when it
// cannot be read (a project since deleted, or one the key cannot see).
func projectPath(ctx context.Context, c *gitlab.Client, id int64) string {
	p, _, err := c.Projects.GetProject(id, nil, gitlab.WithContext(ctx))
	if err != nil || p == nil {
		return "project " + strconv.FormatInt(id, 10)
	}
	return p.PathWithNamespace
}

/*
gitlabItem is one event, counted once: GitLab's own calendar counts events,
and a push's commit count is the push's, not the work's (a rebase pushed
reads as a thousand commits).
*/
func gitlabItem(e *gitlab.ContributionEvent) Item {
	it := Item{Date: DateOf(*e.CreatedAt), Count: 1, Kind: KindOther, Action: e.ActionName}
	if strings.HasPrefix(e.ActionName, "pushed") {
		it.Kind, it.Action = KindPushed, "pushed"
		it.Ref = e.PushData.Ref
		it.Title = e.PushData.CommitTitle
		return it
	}
	it.Title = e.TargetTitle
	switch strings.ToLower(e.TargetType) {
	case "mergerequest":
		it.Kind, it.Ref = KindMR, fmt.Sprintf("!%d", e.TargetIID)
	case "issue":
		it.Kind, it.Ref = KindIssue, fmt.Sprintf("#%d", e.TargetIID)
	case "note", "diffnote", "discussionnote":
		it.Kind, it.Action = KindComment, "commented"
		if e.Note != nil && e.Note.NoteableIID != 0 {
			sigil := "!"
			if e.Note.NoteableType == "Issue" {
				sigil = "#"
			}
			it.Ref = fmt.Sprintf("%s%d", sigil, e.Note.NoteableIID)
		}
	}
	if e.ActionName == "accepted" {
		it.Action = "merged"
	}
	return it
}

// gitlabError is client-go's error as one of this package's.
func gitlabError(err error) error {
	var er *gitlab.ErrorResponse
	if errors.As(err, &er) && er.Response != nil {
		switch er.Response.StatusCode {
		case http.StatusUnauthorized:
			return ErrUnauthorized
		case http.StatusTooManyRequests:
			until := time.Time{}
			if s, err := strconv.Atoi(er.Response.Header.Get("Retry-After")); err == nil {
				until = time.Now().Add(time.Duration(min(s, 3600)) * time.Second)
			}
			return &RateLimited{Until: until}
		}
		return fmt.Errorf("gitlab: the host answered %d", er.Response.StatusCode)
	}
	return hostless(GitLab, err)
}

/*
hostless is a failure without the request it failed on. client-go's,
go-github's and net/http's errors name the URL, and the URL names the host,
as a proxy's or a TLS interceptor's failure names theirs: rule 9 keeps
hostnames out of logs as much as off the screen (spec 102 review, F4). What
kind of failure it was is what a reader of a log needs.
*/
func hostless(forge string, err error) error {
	var ne net.Error
	switch {
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return err
	case errors.Is(err, ErrNotHTTPS), errors.Is(err, ErrTooLarge):
		return err
	case errors.As(err, &ne) && ne.Timeout():
		return errors.New(forge + ": the host did not answer in time")
	}
	return errors.New(forge + ": the host could not be reached")
}
