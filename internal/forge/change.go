package forge

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/google/go-github/v93/github"
	gitlab "gitlab.com/gitlab-org/api/client-go"
)

/*
A change's page (spec 111): one thing a day's page lists, read in full when
a person clicks it. A pull request, a merge request or an issue is its
state, its times, its branches, its size, its checks and reviews, its labels,
its description, and its commits and files; a push or a day's commits in a
repository is the commits.

It is read once per click, not on a timer, and only for a thing a reading
listed: the repository and the number come from the forge's own answer.
*/

// Change is one thing worked on, as its page draws it.
type Change struct {
	Kind  Kind   `json:"kind"`
	Repo  string `json:"repo"`
	Ref   string `json:"ref,omitempty"`
	Title string `json:"title,omitempty"`

	// State is where it is now: "open", "merged", "closed"; and for an
	// issue closed, StateReason says how ("completed", "not planned").
	State       string `json:"state,omitempty"`
	StateReason string `json:"state_reason,omitempty"`
	Draft       bool   `json:"draft,omitempty"`

	Opened time.Time `json:"opened"`
	Merged time.Time `json:"merged"`
	Closed time.Time `json:"closed"`

	// Source and Target are a pull or merge request's branches.
	Source string `json:"source,omitempty"`
	Target string `json:"target,omitempty"`

	// The size: lines added and removed, files changed, commits. SizeKnown
	// is false where the forge did not say.
	Additions   int  `json:"additions"`
	Deletions   int  `json:"deletions"`
	FilesCount  int  `json:"files_count"`
	CommitCount int  `json:"commit_count"`
	SizeKnown   bool `json:"size_known"`

	// Checks is the last commit's checks: "passed", "failed", "running", or
	// empty for none.
	Checks string `json:"checks,omitempty"`

	// Approvals and ChangesRequested are the reviews that said so, and
	// Decision is the forge's verdict ("approved", "changes requested",
	// "review required"), empty where it gives none.
	Approvals        int    `json:"approvals"`
	ChangesRequested int    `json:"changes_requested"`
	Decision         string `json:"decision,omitempty"`

	Labels   []string `json:"labels,omitempty"`
	Comments int      `json:"comments"`

	// Body is the description, as the forge's plain text where it has one.
	Body string `json:"body,omitempty"`

	Commits []Commit `json:"commits,omitempty"`
	Files   []File   `json:"files,omitempty"`

	ReadAt time.Time `json:"read_at"`
}

// Commit is one commit on a change's page.
type Commit struct {
	SHA       string    `json:"sha"`
	Title     string    `json:"title"`
	When      time.Time `json:"when"`
	Additions int       `json:"additions"`
	Deletions int       `json:"deletions"`
	// Stats is whether the forge gave the lines added and removed.
	Stats bool `json:"stats"`
	// Merge is a commit with more than one parent, whose lines are its
	// branch's: a page's totals leave it out.
	Merge bool `json:"merge,omitempty"`
}

// File is one file a change touched.
type File struct {
	Path      string `json:"path"`
	Additions int    `json:"additions"`
	Deletions int    `json:"deletions"`
	// Status is "added", "deleted", "renamed", or empty for changed.
	Status string `json:"status,omitempty"`
}

// Push is one push to a branch, as GitLab's event gives it.
type Push struct {
	From, To string
	Count    int
}

/*
ChangeRef is what a page reads: a thing by its repository and number, or a
repository's commits -- a day's on GitHub, a set of pushes on GitLab.
*/
type ChangeRef struct {
	Kind   Kind
	Repo   string
	Number int

	// Branch is a push's branch, for its page's heading.
	Branch string
	// Pushes are a GitLab page's pushes, newest first.
	Pushes []Push
	// Day is the day whose commits a page lists, from its start: GitHub's
	// on the default branch, GitLab's on the branches pushed. A push's own
	// commit count is not used, since a rebase pushed counts every commit
	// on the branch.
	Day time.Time
}

// The bounds of a change's page: a request's list is a page, and a page
// shows at most this many of each.
const (
	MaxChangeCommits = 100
	MaxChangeFiles   = 100
	maxDiffPages     = 3
)

// repoPath is a repository's path as a forge names one: segments of
// letters, digits, dots, dashes and underscores, none of them only dots.
var repoPath = regexp.MustCompile(`^[A-Za-z0-9_.-]+(/[A-Za-z0-9_.-]+)+$`)

// ErrBadRef is a reference a page will not read: a repository path that is
// not one, or a number that is not.
var ErrBadRef = errors.New("not a change this page can read")

/*
validRef refuses a reference before any request is made with it. The
repository came from the forge's answer and goes into a request's path, with
the key: a path that is not a repository's cannot be one (threat model B4).
*/
func validRef(r ChangeRef) error {
	if !repoPath.MatchString(r.Repo) {
		return ErrBadRef
	}
	for seg := range strings.SplitSeq(r.Repo, "/") {
		if strings.Trim(seg, ".") == "" {
			return ErrBadRef
		}
	}
	switch r.Kind {
	case KindPR, KindMR, KindIssue:
		if r.Number <= 0 {
			return ErrBadRef
		}
	case KindCommits, KindPushed:
	default:
		return ErrBadRef
	}
	return nil
}

// ---- GitHub ----

const githubThingQuery = `query($owner: String!, $name: String!, $number: Int!) {
  repository(owner: $owner, name: $name) {
    issueOrPullRequest(number: $number) {
      __typename
      ... on Issue {
        title state stateReason createdAt closedAt bodyText
        labels(first: 20) { nodes { name } }
        comments { totalCount }
      }
      ... on PullRequest {
        title state merged isDraft createdAt mergedAt closedAt bodyText
        headRefName baseRefName additions deletions changedFiles reviewDecision
        labels(first: 20) { nodes { name } }
        comments { totalCount }
        reviews(first: 100) { nodes { state } }
        commits(first: 100) { totalCount nodes { commit { oid messageHeadline additions deletions committedDate parents { totalCount } } } }
        last: commits(last: 1) { nodes { commit { statusCheckRollup { state } } } }
        files(first: 100) { totalCount nodes { path additions deletions changeType } }
      }
    }
  }
}`

const githubViewerQuery = `query { viewer { id } }`

const githubDayCommitsQuery = `query($owner: String!, $name: String!, $since: GitTimestamp!, $until: GitTimestamp!, $author: ID!) {
  repository(owner: $owner, name: $name) {
    defaultBranchRef { target { ... on Commit {
      history(first: 100, since: $since, until: $until, author: { id: $author }) {
        totalCount nodes { oid messageHeadline additions deletions committedDate parents { totalCount } }
      }
    } } }
  }
}`

type ghCommitNode struct {
	OID       string    `json:"oid"`
	Headline  string    `json:"messageHeadline"`
	Additions int       `json:"additions"`
	Deletions int       `json:"deletions"`
	Committed time.Time `json:"committedDate"`
	Parents   struct {
		Total int `json:"totalCount"`
	} `json:"parents"`
}

type ghNames struct {
	Nodes []struct {
		Name string `json:"name"`
	} `json:"nodes"`
}

type ghThingResponse struct {
	Data struct {
		Repository *struct {
			Thing *struct {
				Type        string     `json:"__typename"`
				Title       string     `json:"title"`
				State       string     `json:"state"`
				StateReason string     `json:"stateReason"`
				Merged      bool       `json:"merged"`
				Draft       bool       `json:"isDraft"`
				Created     *time.Time `json:"createdAt"`
				MergedAt    *time.Time `json:"mergedAt"`
				ClosedAt    *time.Time `json:"closedAt"`
				Body        string     `json:"bodyText"`
				Head        string     `json:"headRefName"`
				Base        string     `json:"baseRefName"`
				Additions   int        `json:"additions"`
				Deletions   int        `json:"deletions"`
				Changed     int        `json:"changedFiles"`
				Decision    string     `json:"reviewDecision"`
				Labels      ghNames    `json:"labels"`
				Comments    struct {
					Total int `json:"totalCount"`
				} `json:"comments"`
				Reviews struct {
					Nodes []struct {
						State string `json:"state"`
					} `json:"nodes"`
				} `json:"reviews"`
				Commits struct {
					Total int `json:"totalCount"`
					Nodes []struct {
						Commit ghCommitNode `json:"commit"`
					} `json:"nodes"`
				} `json:"commits"`
				Last struct {
					Nodes []struct {
						Commit struct {
							Rollup *struct {
								State string `json:"state"`
							} `json:"statusCheckRollup"`
						} `json:"commit"`
					} `json:"nodes"`
				} `json:"last"`
				Files struct {
					Total int `json:"totalCount"`
					Nodes []struct {
						Path      string `json:"path"`
						Additions int    `json:"additions"`
						Deletions int    `json:"deletions"`
						Change    string `json:"changeType"`
					} `json:"nodes"`
				} `json:"files"`
			} `json:"issueOrPullRequest"`
		} `json:"repository"`
	} `json:"data"`
	Errors []struct{} `json:"errors"`
}

type ghDayCommitsResponse struct {
	Data struct {
		Repository *struct {
			Default *struct {
				Target struct {
					History struct {
						Total int            `json:"totalCount"`
						Nodes []ghCommitNode `json:"nodes"`
					} `json:"history"`
				} `json:"target"`
			} `json:"defaultBranchRef"`
		} `json:"repository"`
	} `json:"data"`
	Errors []struct{} `json:"errors"`
}

// ReadChange is one change's page, with token.
func (r GitHubReader) ReadChange(ctx context.Context, token string, ref ChangeRef) (Change, error) {
	if err := validRef(ref); err != nil {
		return Change{}, err
	}
	now := time.Now
	if r.Now != nil {
		now = r.Now
	}
	c, err := r.newClient(token)
	if err != nil {
		return Change{}, err
	}
	owner, name, _ := strings.Cut(ref.Repo, "/")
	out := Change{Kind: ref.Kind, Repo: ref.Repo, ReadAt: now()}
	if ref.Kind == KindCommits {
		var viewer struct {
			Data struct {
				Viewer struct {
					ID string `json:"id"`
				} `json:"viewer"`
			} `json:"data"`
			Errors []struct{} `json:"errors"`
		}
		if err := ghPost(ctx, c, githubViewerQuery, nil, &viewer); err != nil {
			return Change{}, err
		}
		if len(viewer.Errors) > 0 || viewer.Data.Viewer.ID == "" {
			return Change{}, errRefused
		}
		var day ghDayCommitsResponse
		err := ghPost(ctx, c, githubDayCommitsQuery, map[string]any{
			"owner": owner, "name": name, "author": viewer.Data.Viewer.ID,
			"since": ref.Day.Format(time.RFC3339), "until": ref.Day.AddDate(0, 0, 1).Format(time.RFC3339),
		}, &day)
		if err != nil {
			return Change{}, err
		}
		if len(day.Errors) > 0 || day.Data.Repository == nil {
			return Change{}, errRefused
		}
		if d := day.Data.Repository.Default; d != nil {
			h := d.Target.History
			out.CommitCount = h.Total
			for _, n := range h.Nodes {
				out.Commits = append(out.Commits, ghCommit(n))
			}
			out.Additions, out.Deletions = commitLines(out.Commits)
			out.SizeKnown = len(h.Nodes) == h.Total
		}
		return out, nil
	}

	var resp ghThingResponse
	if err := ghPost(ctx, c, githubThingQuery, map[string]any{"owner": owner, "name": name, "number": ref.Number}, &resp); err != nil {
		return Change{}, err
	}
	if len(resp.Errors) > 0 || resp.Data.Repository == nil || resp.Data.Repository.Thing == nil {
		return Change{}, errRefused
	}
	t := resp.Data.Repository.Thing
	out.Ref = "#" + strconv.Itoa(ref.Number)
	out.Title, out.Body, out.Comments = t.Title, t.Body, t.Comments.Total
	out.State = strings.ToLower(t.State)
	out.StateReason = strings.ReplaceAll(strings.ToLower(t.StateReason), "_", " ")
	out.Opened, out.Merged, out.Closed = at(t.Created), at(t.MergedAt), at(t.ClosedAt)
	for _, l := range t.Labels.Nodes {
		out.Labels = append(out.Labels, l.Name)
	}
	if t.Type != "PullRequest" {
		out.Kind = KindIssue
		return out, nil
	}
	out.Kind = KindPR
	if t.Merged {
		out.State = "merged"
	}
	out.Draft = t.Draft
	out.Source, out.Target = t.Head, t.Base
	out.Additions, out.Deletions, out.FilesCount = t.Additions, t.Deletions, t.Changed
	out.CommitCount, out.SizeKnown = t.Commits.Total, true
	out.Decision = strings.ReplaceAll(strings.ToLower(t.Decision), "_", " ")
	for _, rv := range t.Reviews.Nodes {
		switch rv.State {
		case "APPROVED":
			out.Approvals++
		case "CHANGES_REQUESTED":
			out.ChangesRequested++
		}
	}
	for _, n := range t.Commits.Nodes {
		out.Commits = append(out.Commits, ghCommit(n.Commit))
	}
	if len(t.Last.Nodes) > 0 && t.Last.Nodes[0].Commit.Rollup != nil {
		out.Checks = checksWord(t.Last.Nodes[0].Commit.Rollup.State)
	}
	for _, f := range t.Files.Nodes {
		file := File{Path: f.Path, Additions: f.Additions, Deletions: f.Deletions}
		switch f.Change {
		case "ADDED":
			file.Status = "added"
		case "DELETED":
			file.Status = "deleted"
		case "RENAMED":
			file.Status = "renamed"
		}
		out.Files = append(out.Files, file)
	}
	return out, nil
}

// errRefused is an answer that is not one: the query refused, or a
// repository or number the forge does not have.
var errRefused = errors.New("the forge did not have it")

func (r GitHubReader) newClient(token string) (*github.Client, error) {
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
		return nil, fmt.Errorf("github: %w", err)
	}
	return c, nil
}

// ghPost sends one GraphQL query and decodes its answer into out.
func ghPost(ctx context.Context, c *github.Client, query string, vars map[string]any, out any) error {
	body := map[string]any{"query": query}
	if vars != nil {
		body["variables"] = vars
	}
	req, err := c.NewRequest(ctx, http.MethodPost, "graphql", body)
	if err != nil {
		return fmt.Errorf("github: %w", err)
	}
	if _, err := c.Do(req, out); err != nil {
		return githubError(err)
	}
	return nil
}

func ghCommit(n ghCommitNode) Commit {
	return Commit{SHA: n.OID, Title: n.Headline, When: n.Committed, Additions: n.Additions, Deletions: n.Deletions, Stats: true, Merge: n.Parents.Total > 1}
}

// commitLines are commits' lines added and removed, merges left out.
func commitLines(commits []Commit) (add, del int) {
	for _, c := range commits {
		if !c.Merge {
			add += c.Additions
			del += c.Deletions
		}
	}
	return add, del
}

// checksWord is a check rollup's state as a page says it.
func checksWord(state string) string {
	switch strings.ToUpper(state) {
	case "SUCCESS":
		return "passed"
	case "FAILURE", "ERROR":
		return "failed"
	case "PENDING", "EXPECTED":
		return "running"
	case "":
		return ""
	}
	return strings.ToLower(state)
}

func at(t *time.Time) time.Time {
	if t == nil {
		return time.Time{}
	}
	return *t
}

// ---- GitLab ----

// ErrBadAnswer is an answer the client library could not decode without
// failing outright.
var ErrBadAnswer = errors.New("the forge's answer could not be read")

/*
ReadChange is one change's page, with token.

client-go's decoders panic on some answers a real GitLab never sends -- an
issue with no id, a label list of mixed kinds -- and a panic in the reading's
goroutine would end the program. A host's answer is not allowed that
(threat model B4): the panic is the reading's error.
*/
func (r GitLabReader) ReadChange(ctx context.Context, token string, ref ChangeRef) (c Change, err error) {
	if err := validRef(ref); err != nil {
		return Change{}, err
	}
	defer func() {
		if recover() != nil {
			c, err = Change{}, ErrBadAnswer
		}
	}()
	return r.readChange(ctx, token, ref)
}

func (r GitLabReader) readChange(ctx context.Context, token string, ref ChangeRef) (Change, error) {
	now := time.Now
	if r.Now != nil {
		now = r.Now
	}
	c, err := r.newClient(token)
	if err != nil {
		return Change{}, err
	}
	out := Change{Kind: ref.Kind, Repo: ref.Repo, ReadAt: now()}
	with := gitlab.WithContext(ctx)
	switch ref.Kind {
	case KindMR:
		mr, _, err := c.MergeRequests.GetMergeRequest(ref.Repo, int64(ref.Number), nil, with)
		if err != nil {
			return Change{}, gitlabError(err)
		}
		out.Ref = "!" + strconv.Itoa(ref.Number)
		out.Title, out.Body, out.State, out.Draft = mr.Title, plainMarkdown(mr.Description), mr.State, mr.Draft
		out.Opened, out.Merged, out.Closed = at(mr.CreatedAt), at(mr.MergedAt), at(mr.ClosedAt)
		out.Source, out.Target = mr.SourceBranch, mr.TargetBranch
		out.Labels, out.Comments = []string(mr.Labels), int(mr.UserNotesCount)
		out.FilesCount, _ = strconv.Atoi(strings.TrimSuffix(mr.ChangesCount, "+"))
		if mr.HeadPipeline != nil {
			out.Checks = pipelineWord(mr.HeadPipeline.Status)
		}
		if ap, _, err := c.MergeRequestApprovals.GetConfiguration(ref.Repo, int64(ref.Number), with); err == nil && ap != nil {
			out.Approvals = len(ap.ApprovedBy)
			if ap.Approved && len(ap.ApprovedBy) > 0 {
				out.Decision = "approved"
			}
		}
		commits, _, err := c.MergeRequests.GetMergeRequestCommits(ref.Repo, int64(ref.Number),
			&gitlab.GetMergeRequestCommitsOptions{ListOptions: gitlab.ListOptions{PerPage: MaxChangeCommits}}, with)
		if err != nil {
			return Change{}, gitlabError(err)
		}
		for _, cm := range commits {
			if cm != nil {
				out.Commits = append(out.Commits, glCommit(cm))
			}
		}
		out.CommitCount = len(out.Commits)
		out.Files, out.Additions, out.Deletions, out.SizeKnown = glDiffs(ctx, c, ref)
		if out.FilesCount == 0 {
			out.FilesCount = len(out.Files)
		}
	case KindIssue:
		is, _, err := c.Issues.GetIssue(ref.Repo, int64(ref.Number), with)
		if err != nil {
			return Change{}, gitlabError(err)
		}
		out.Ref = "#" + strconv.Itoa(ref.Number)
		out.Title, out.Body, out.State = is.Title, plainMarkdown(is.Description), is.State
		out.Opened, out.Closed = at(is.CreatedAt), at(is.ClosedAt)
		out.Labels, out.Comments = []string(is.Labels), int(is.UserNotesCount)
	case KindPushed, KindCommits:
		out.Ref = ref.Branch
		seen := map[string]bool{}
		for _, p := range ref.Pushes {
			if p.To == "" || len(out.Commits) >= MaxChangeCommits {
				continue
			}
			stats := true
			since, until := ref.Day, ref.Day.AddDate(0, 0, 1)
			commits, _, err := c.Commits.ListCommits(ref.Repo, &gitlab.ListCommitsOptions{
				RefName: &p.To, WithStats: &stats, Since: &since, Until: &until,
				ListOptions: gitlab.ListOptions{PerPage: MaxChangeCommits},
			}, with)
			if err != nil {
				return Change{}, gitlabError(err)
			}
			for _, cm := range commits {
				if cm == nil || seen[cm.ID] || len(out.Commits) >= MaxChangeCommits {
					continue
				}
				seen[cm.ID] = true
				out.Commits = append(out.Commits, glCommit(cm))
			}
		}
		slices.SortStableFunc(out.Commits, func(a, b Commit) int { return b.When.Compare(a.When) })
		out.Additions, out.Deletions = commitLines(out.Commits)
		out.CommitCount, out.SizeKnown = len(out.Commits), true
	}
	return out, nil
}

func (r GitLabReader) newClient(token string) (*gitlab.Client, error) {
	base := r.BaseURL
	if base == "" {
		if r.Host == "" {
			return nil, fmt.Errorf("%w: no GitLab host; set one in Preferences or GITLAB_HOST in ~/.secrets.env", ErrNoCredential)
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
		return nil, fmt.Errorf("gitlab: %w", err)
	}
	return c, nil
}

func glCommit(cm *gitlab.Commit) Commit {
	out := Commit{SHA: cm.ID, Title: cm.Title, When: at(cm.CommittedDate), Merge: len(cm.ParentIDs) > 1}
	if cm.Stats != nil {
		out.Additions, out.Deletions, out.Stats = int(cm.Stats.Additions), int(cm.Stats.Deletions), true
	}
	return out
}

/*
glDiffs is a merge request's files and the lines added and removed, counted
from its diffs: GitLab gives no totals. A diff the forge collapsed or found
too large counts nothing, and known is false then, or when there were more
files than three pages.
*/
func glDiffs(ctx context.Context, c *gitlab.Client, ref ChangeRef) (files []File, add, del int, known bool) {
	known = true
	opt := &gitlab.ListMergeRequestDiffsOptions{ListOptions: gitlab.ListOptions{PerPage: MaxChangeFiles, Page: 1}}
	for page := range maxDiffPages {
		diffs, resp, err := c.MergeRequests.ListMergeRequestDiffs(ref.Repo, int64(ref.Number), opt, gitlab.WithContext(ctx))
		if err != nil {
			return files, add, del, false
		}
		for _, d := range diffs {
			if d == nil {
				continue
			}
			f := File{Path: d.NewPath}
			switch {
			case d.NewFile:
				f.Status = "added"
			case d.DeletedFile:
				f.Status, f.Path = "deleted", d.OldPath
			case d.RenamedFile:
				f.Status = "renamed"
			}
			if d.Collapsed || d.TooLarge {
				known = false
			}
			f.Additions, f.Deletions = diffLines(d.Diff)
			add += f.Additions
			del += f.Deletions
			if len(files) < MaxChangeFiles {
				files = append(files, f)
			}
		}
		if resp == nil || resp.NextPage == 0 {
			return files, add, del, known
		}
		if page == maxDiffPages-1 {
			return files, add, del, false
		}
		opt.Page = resp.NextPage
	}
	return files, add, del, known
}

/*
plainMarkdown is a GitLab description as text, as GitHub's bodyText is: a
heading's marks, code's backticks and bold's asterisks taken out. The rest
of Markdown reads as written.
*/
func plainMarkdown(s string) string {
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		if t := strings.TrimLeft(l, "#"); t != l && strings.HasPrefix(t, " ") {
			l = strings.TrimSpace(t)
		}
		l = strings.ReplaceAll(l, "**", "")
		lines[i] = strings.ReplaceAll(l, "`", "")
	}
	return strings.Join(lines, "\n")
}

// diffLines counts a unified diff's added and removed lines.
func diffLines(diff string) (add, del int) {
	for line := range strings.SplitSeq(diff, "\n") {
		switch {
		case strings.HasPrefix(line, "+++"), strings.HasPrefix(line, "---"):
		case strings.HasPrefix(line, "+"):
			add++
		case strings.HasPrefix(line, "-"):
			del++
		}
	}
	return add, del
}

// pipelineWord is a pipeline's status as a page says it.
func pipelineWord(s string) string {
	switch s {
	case "success":
		return "passed"
	case "failed":
		return "failed"
	case "running", "pending", "created", "preparing", "waiting_for_resource", "scheduled":
		return "running"
	case "":
		return ""
	}
	return strings.ReplaceAll(s, "_", " ")
}
