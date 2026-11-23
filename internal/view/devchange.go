package view

import (
	"fmt"
	"hash/fnv"
	"strconv"
	"strings"
	"time"
)

/*
A change's page (spec 111): what a click on a line of a day's page opens.
A pull request, merge request or issue is its state, times, branches, size,
checks, reviews, labels, comments, description, commits and files; a push,
or a repository's day, is its commits.

The page is a drill under its day: its ID is the day's and a key, a hash of
what the line is about, so no repository or title is in an ID, which a log
or a bug report may carry.
*/

// DevChange is a change as the page draws it: see forge.Change.
type DevChange struct {
	Kind, Ref, Title         string
	State, StateReason       string
	Draft                    bool
	Opened, Merged, Closed   time.Time
	Source, Target           string
	Additions, Deletions     int
	FilesCount, CommitCount  int
	SizeKnown                bool
	Checks                   string
	Approvals, ChangesWanted int
	Decision                 string
	Labels                   []string
	Comments                 int
	Body                     string
	Commits                  []DevCommit
	Files                    []DevFile
}

// DevCommit is a commit on a change's page.
type DevCommit struct {
	SHA, Title           string
	When                 time.Time
	Additions, Deletions int
	Stats                bool
}

// DevFile is a file a change touched.
type DevFile struct {
	Path                 string
	Additions, Deletions int
	Status               string
}

// The kinds of page a line opens. A review or a comment opens the thing it
// was on; a repository's line opens its day's commits.
const (
	DevOpenPR    = "pr"
	DevOpenMR    = "mr"
	DevOpenIssue = "issue"
	DevOpenPush  = "push"
	DevOpenRepo  = "repo"
)

/*
DevOpens is the page an item's line opens, and the reference it is opened by:
a pull request's or a review's "#12", a merge request's or a comment on one
"!7", a push's branch. Empty for an item with no page: a repository created,
an event with nothing to read.
*/
func DevOpens(provider, kind, ref string) (open, by string) {
	switch kind {
	case "pr", "review":
		return DevOpenPR, ref
	case "mr":
		return DevOpenMR, ref
	case "issue":
		return DevOpenIssue, ref
	case "pushed":
		return DevOpenPush, ref
	case "comment":
		switch {
		case strings.HasPrefix(ref, "!"):
			return DevOpenMR, ref
		case strings.HasPrefix(ref, "#") && provider == "github":
			return DevOpenPR, ref
		case strings.HasPrefix(ref, "#"):
			return DevOpenIssue, ref
		}
	}
	return "", ""
}

// DevChangeKey is a page's key: what it opens, in which repository, by
// which reference, hashed.
func DevChangeKey(open, repo, ref string) string {
	h := fnv.New64a()
	_, _ = h.Write([]byte(open + "\x00" + repo + "\x00" + ref))
	return "c" + strconv.FormatUint(h.Sum64(), 36)
}

// DevChangeID is a change's page, under its day.
func DevChangeID(provider, date, key string) string {
	return DevDayID(provider, date) + "/" + key
}

// ParseDevChangeID is a change page's forge, day and key.
func ParseDevChangeID(id string) (provider, date, key string, ok bool) {
	i := strings.LastIndex(id, "/")
	if i < 0 {
		return "", "", "", false
	}
	provider, date, ok = ParseDevDayID(id[:i])
	key = id[i+1:]
	if !ok || !strings.HasPrefix(key, "c") || len(key) < 2 {
		return "", "", "", false
	}
	return provider, date, key, true
}

// DevNumber is a reference's number: 12 for "#12" or "!12", 0 for none.
func DevNumber(ref string) int {
	n, err := strconv.Atoi(strings.TrimLeft(ref, "#!"))
	if err != nil || n < 0 {
		return 0
	}
	return n
}

// DevChangeLines is how many lines of a description a page shows: the page
// has the drawer to itself and scrolls, so most of one.
const DevChangeLines = 150

/*
DevChangePage is a change's page. What the day's line said -- the heading
and title -- is drawn at once; the rest is c, nil while it is read (reading)
or when it could not be (failed).
*/
func DevChangePage(title, id, open, repo, ref, line string, c *DevChange, reading bool, failed *Reason) Detail {
	name := withoutOwner(repo)
	head := Detail{Section: UsageInfo.Key, ID: "change", TitleBold: true}
	switch open {
	case DevOpenRepo:
		head.Title = name + " · commits"
	case DevOpenPush:
		head.Title = ref + " · " + name
	default:
		head.Title = ref + " · " + name
	}
	if reading {
		head.TitleParts = []Part{{Text: "reading…", Status: Info}}
	}
	if failed != nil {
		head.Reasons = []Reason{*failed}
	}
	page := Detail{Section: UsageInfo.Key, ID: id, Title: title}
	if c == nil {
		if line != "" {
			head.Lines = wrapLines(line, Good)
		}
		page.Parts = []Detail{head}
		return page
	}
	if c.State != "" {
		head.TitleParts = append([]Part{{Text: stateWord(c), Status: stateStatus(c)}}, head.TitleParts...)
	}
	head.Lines = append(head.Lines, wrapLines(c.Title, Good)...)
	head.Lines = append(head.Lines, changeFacts(c)...)
	page.Parts = append(page.Parts, head)

	if body := strings.TrimSpace(c.Body); body != "" {
		desc := Detail{Section: UsageInfo.Key, ID: "description", Title: "Description", TitleBold: true}
		var lines []DetailLine
		for para := range strings.SplitSeq(body, "\n") {
			para = strings.TrimRight(para, " \t\r")
			if para == "" {
				if len(lines) > 0 && lines[len(lines)-1].Name != "" {
					lines = append(lines, DetailLine{Wide: true})
				}
				continue
			}
			lines = append(lines, wrapLines(para, Info)...)
		}
		if len(lines) > DevChangeLines {
			lines = append(lines[:DevChangeLines], DetailLine{Name: "… the rest on the forge", Wide: true, Status: Info})
		}
		desc.Lines = lines
		page.Parts = append(page.Parts, desc)
	}
	if len(c.Commits) > 0 {
		part := Detail{Section: UsageInfo.Key, ID: "commits", Title: countTitle("Commits", len(c.Commits), c.CommitCount), TitleBold: true}
		for _, cm := range c.Commits {
			sha := cm.SHA
			if len(sha) > 7 {
				sha = sha[:7]
			}
			l := DetailLine{Name: truncate(sha+" "+cm.Title, DevWideNameWidth), Wide: true, Status: Good}
			if cm.Stats {
				l.Value, l.Parts = lineCounts(cm.Additions, cm.Deletions), lineParts(cm.Additions, cm.Deletions)
			}
			part.Lines = append(part.Lines, l)
		}
		page.Parts = append(page.Parts, part)
	}
	if len(c.Files) > 0 {
		part := Detail{Section: UsageInfo.Key, ID: "files", Title: countTitle("Files", len(c.Files), c.FilesCount), TitleBold: true}
		for _, f := range c.Files {
			path := f.Path
			if f.Status != "" {
				path += " (" + f.Status + ")"
			}
			part.Lines = append(part.Lines, DetailLine{
				Name: truncateLeft(path, DevWideNameWidth), Value: lineCounts(f.Additions, f.Deletions),
				Parts: lineParts(f.Additions, f.Deletions), Wide: true, Status: Good,
			})
		}
		page.Parts = append(page.Parts, part)
	}
	return page
}

// changeFacts are a change's state, times, branches, size, checks, reviews,
// labels and comments, each a line where the forge said it.
func changeFacts(c *DevChange) []DetailLine {
	var out []DetailLine
	fact := func(name, value string, st Status) {
		if value != "" {
			out = append(out, DetailLine{Name: name, Value: value, Status: st})
		}
	}
	fact("Opened", when(c.Opened), Info)
	fact("Merged", when(c.Merged), Good)
	if c.Merged.IsZero() {
		fact("Closed", when(c.Closed), Info)
	}
	if c.StateReason != "" && c.State == "closed" {
		fact("Closed as", c.StateReason, Info)
	}
	if c.Source != "" {
		// Two lines: a long branch name would cut off where it goes.
		out = append(out,
			DetailLine{Name: truncate(c.Source, DevWideNameWidth), Wide: true, Status: Info},
			DetailLine{Name: truncate("→ "+c.Target, DevWideNameWidth), Wide: true, Status: Info})
	}
	if c.SizeKnown && (c.Additions > 0 || c.Deletions > 0) {
		out = append(out, DetailLine{Name: "Lines", Value: lineCounts(c.Additions, c.Deletions), Parts: lineParts(c.Additions, c.Deletions)})
	}
	if c.FilesCount > 0 {
		fact("Files", strconv.Itoa(c.FilesCount), Info)
	}
	if c.CommitCount > 0 {
		fact("Commits", strconv.Itoa(c.CommitCount), Info)
	}
	switch c.Checks {
	case "passed":
		fact("Checks", "passed", Good)
	case "failed":
		fact("Checks", "failed", Bad)
	default:
		fact("Checks", c.Checks, Info)
	}
	var reviews []string
	if c.Approvals > 0 {
		reviews = append(reviews, fmt.Sprintf("%d approved", c.Approvals))
	}
	if c.ChangesWanted > 0 {
		reviews = append(reviews, fmt.Sprintf("%d changes", c.ChangesWanted))
	}
	// The forge's verdict is about what is still to be merged; on a change
	// merged or closed it is only the repository's rule.
	if len(reviews) == 0 && c.Decision != "" && stateWord(c) != "merged" && stateWord(c) != "closed" {
		reviews = append(reviews, c.Decision)
	}
	st := Info
	if c.ChangesWanted > 0 {
		st = Warn
	} else if c.Approvals > 0 {
		st = Good
	}
	fact("Reviews", strings.Join(reviews, ", "), st)
	if c.Comments > 0 {
		fact("Comments", strconv.Itoa(c.Comments), Info)
	}
	if len(c.Labels) > 0 {
		out = append(out, wrapLines("Labels: "+strings.Join(c.Labels, ", "), Info)...)
	}
	return out
}

// stateWord is a change's state as its heading says it.
func stateWord(c *DevChange) string {
	if c.Draft && c.State == "opened" || c.Draft && c.State == "open" {
		return "draft"
	}
	if c.State == "opened" {
		return "open"
	}
	return c.State
}

func stateStatus(c *DevChange) Status {
	switch stateWord(c) {
	case "merged":
		return Good
	case "closed":
		return Warn
	}
	return Info
}

// countTitle is a list's heading with its count, and the forge's when the
// page shows fewer: "Commits (100 of 140)".
func countTitle(name string, shown, total int) string {
	if total > shown {
		return fmt.Sprintf("%s (%d of %d)", name, shown, total)
	}
	return fmt.Sprintf("%s (%d)", name, shown)
}

// lineCounts is lines added and removed: "+40 −3".
func lineCounts(add, del int) string {
	return fmt.Sprintf("+%d −%d", add, del)
}

// lineParts colour lines added and removed apart, as a diff does.
func lineParts(add, del int) []Part {
	return []Part{{Text: fmt.Sprintf("+%d ", add), Status: Good}, {Text: fmt.Sprintf("−%d", del), Status: Bad}}
}

// when is a moment on the desk's clock, or empty for none.
func when(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Local().Format("Mon 2 Jan 15:04")
}

// wrapLines is text wrapped at word boundaries into wide lines.
func wrapLines(text string, st Status) []DetailLine {
	var out []DetailLine
	for _, l := range wrapWords(text, DevWideNameWidth) {
		out = append(out, DetailLine{Name: l, Wide: true, Status: st})
	}
	return out
}

// wrapWords breaks text into lines of at most width runes, at spaces where
// it can and inside a word longer than a line where it cannot.
func wrapWords(text string, width int) []string {
	var out []string
	var cur []rune
	flush := func() {
		if len(cur) > 0 {
			out = append(out, string(cur))
			cur = cur[:0]
		}
	}
	for w := range strings.FieldsSeq(text) {
		word := []rune(w)
		for len(word) > width {
			flush()
			out = append(out, string(word[:width]))
			word = word[width:]
		}
		switch {
		case len(cur) == 0:
			cur = append(cur, word...)
		case len(cur)+1+len(word) <= width:
			cur = append(cur, ' ')
			cur = append(cur, word...)
		default:
			flush()
			cur = append(cur, word...)
		}
	}
	flush()
	return out
}

// truncateLeft keeps a path's end, which names the file: "…/view/devchange.go".
func truncateLeft(s string, width int) string {
	r := []rune(s)
	if len(r) <= width {
		return s
	}
	return "…" + string(r[len(r)-width+1:])
}
