package view

import (
	"fmt"
	"math"
	"slices"
	"strings"
	"time"
)

/*
The Development card's git parts (spec 102): a fortnight of a person's
activity on a forge, a day per cell, and a day's page behind each cell.

The view takes its own plain values, as UsageBreakdown is taken: the panel
turns a forge reading into a DevReading, and nothing here knows a forge's
API.
*/

// DevReading is one forge's state as the drawer draws it.
type DevReading struct {
	// Provider is the forge's key, "github" or "gitlab", and Title its
	// name, "GitHub". Never a host: rule 9.
	Provider string
	Title    string

	// Unit names what a day counts: "contributions", "events".
	Unit string

	// Days are the fortnight, oldest first; empty before the first reading.
	Days  []DevDay
	Items []DevItem

	// Private is how many contributions the forge counted and would not
	// list.
	Private int

	// ReadAt is when Days were read; zero before the first reading.
	ReadAt time.Time

	// Reading marks a read in flight.
	Reading bool

	// Failed is why the last read failed, nil when it did not.
	Failed *Reason
}

// DevDay is one day's count.
type DevDay struct {
	Date  string // YYYY-MM-DD
	Count int
}

// DevItem is one thing done: see forge.Item.
type DevItem struct {
	Date, Repo, Kind, Ref, Title, Action string
	Count                                int
}

// DayStrip is a row of cells a person chooses from (spec 102 R4): a fortnight,
// a day each, oldest left. A shell draws it as a heat strip with a word
// under each cell, and a click or Enter on a cell opens Opens[i].
type DayStrip struct {
	// Values are each cell's fraction, 0 to 1; a negative value is a cell
	// with nothing in it, drawn empty in its place.
	Values []float64 `json:"values"`
	Labels []string  `json:"labels"`
	Dates  []string  `json:"dates"`
	Counts []int     `json:"counts"`
	// Opens are the detail each cell opens.
	Opens []string `json:"-"`
	// Selected is the cell whose page is open, or -1.
	Selected int `json:"selected"`
}

// stripEmpty is the glyph for a day with nothing in it, and stripBlocks the
// rest from least to most.
const stripEmpty = '·'

var stripBlocks = []rune("▁▂▃▄▅▆▇█")

// Text is the strip as a terminal draws it: a glyph per day.
func (s DayStrip) Text() string {
	var b strings.Builder
	for _, v := range s.Values {
		if v < 0 || math.IsNaN(v) {
			b.WriteRune(stripEmpty)
			continue
		}
		i := int(math.Round(v * float64(len(stripBlocks)-1)))
		b.WriteRune(stripBlocks[min(max(i, 0), len(stripBlocks)-1)])
	}
	return b.String()
}

// ActRefresh is the action that reads a forge again, before the forge's key.
const ActRefresh = "!refresh"

// DevDayID is the ID of a forge's day's page, a drill under the overview:
// "accounts/github/2026-11-17".
func DevDayID(provider, date string) string {
	return DetailAccounts + "/" + provider + "/" + date
}

// ParseDevDayID is a day page's forge and date.
func ParseDevDayID(id string) (provider, date string, ok bool) {
	rest, ok := strings.CutPrefix(id, DetailAccounts+"/")
	if !ok {
		return "", "", false
	}
	provider, date, ok = strings.Cut(rest, "/")
	if !ok || len(date) != len(time.DateOnly) {
		return "", "", false
	}
	if _, err := time.Parse(time.DateOnly, date); err != nil {
		return "", "", false
	}
	return provider, date, provider != ""
}

// RefreshAct is what a click on a forge's heading asks for.
func RefreshAct(provider string) string { return ActRefresh + "/" + provider }

// ParseRefresh is the forge a refresh asks to read again.
func ParseRefresh(ask string) (string, bool) {
	p, ok := strings.CutPrefix(ask, ActRefresh+"/")
	return p, ok && p != ""
}

// DevSumName heads the line under a strip.
const DevSumName = "14 days"

// DevPart is a forge's part of the overview: its heading, which reads it
// again when clicked, its strip with the cell selected marked, and the
// fortnight's sum (spec 102 R4, R5).
func DevPart(r DevReading, now time.Time, selected int) Detail {
	d := Detail{
		Section:    UsageInfo.Key,
		ID:         r.Provider,
		Title:      r.Title,
		Open:       RefreshAct(r.Provider),
		TitleParts: []Part{{Text: "↻ " + devState(r, now), Status: devStateStatus(r)}},
	}
	if r.Failed != nil {
		d.Reasons = []Reason{*r.Failed}
	}
	if len(r.Days) == 0 {
		return d
	}
	d.Days = devStrip(r, selected)
	total, most := 0, 0
	for _, day := range r.Days {
		total, most = total+day.Count, max(most, day.Count)
	}
	d.Lines = []DetailLine{{Name: DevSumName, Value: fmt.Sprintf("%d %s, most %d", total, r.Unit, most)}}
	return d
}

func devState(r DevReading, now time.Time) string {
	switch {
	case r.Reading:
		return "reading…"
	case r.Failed != nil:
		return "failed"
	case r.ReadAt.IsZero():
		return ""
	}
	age := now.Sub(r.ReadAt)
	switch {
	case age < time.Minute:
		return "now"
	case age < time.Hour:
		return fmt.Sprintf("%dm ago", int(age.Minutes()))
	}
	return fmt.Sprintf("%dh ago", int(age.Hours()))
}

func devStateStatus(r DevReading) Status {
	if r.Failed != nil && !r.Reading {
		return Warn
	}
	return Info
}

// devStrip is the reading's days as cells, each against the fortnight's
// busiest. A day with something is never drawn as faint as one with
// nothing: the least of them is a fifth of the way up.
func devStrip(r DevReading, selected int) *DayStrip {
	most := 0
	for _, d := range r.Days {
		most = max(most, d.Count)
	}
	s := &DayStrip{Selected: -1}
	for i, d := range r.Days {
		v := -1.0
		if d.Count > 0 && most > 0 {
			v = 0.2 + 0.8*float64(d.Count)/float64(most)
		}
		s.Values = append(s.Values, v)
		s.Labels = append(s.Labels, weekdayInitial(d.Date))
		s.Dates = append(s.Dates, d.Date)
		s.Counts = append(s.Counts, d.Count)
		s.Opens = append(s.Opens, DevDayID(r.Provider, d.Date))
		if i == selected {
			s.Selected = i
		}
	}
	return s
}

func weekdayInitial(date string) string {
	t, err := time.Parse(time.DateOnly, date)
	if err != nil {
		return "?"
	}
	return t.Weekday().String()[:1]
}

// DevDayPage is a forge's day: its part with the day marked, then the day
// itself under a heading of its date and count (spec 102 R6, R7, R8).
func DevDayPage(r DevReading, now time.Time, date string) Detail {
	i := slices.IndexFunc(r.Days, func(d DevDay) bool { return d.Date == date })
	part := DevPart(r, now, i)
	if i < 0 {
		part.Reasons = append(part.Reasons, Reason{Text: "that day is no longer in the fortnight", Status: Info})
	} else {
		part.Lines = append(part.Lines, devDayLines(r, r.Days[i])...)
	}
	return Detail{
		Section: UsageInfo.Key,
		ID:      DevDayID(r.Provider, date),
		Title:   r.Title,
		Parts:   []Detail{part},
	}
}

// DevWideNameWidth is the width a day's line's name is cut to: a reference
// and a title, which take what the short value leaves.
const DevWideNameWidth = 36

// devDayLines are a day, by repository: a heading with the counts, then a
// line per thing worked on at the furthest it got.
func devDayLines(r DevReading, day DevDay) []DetailLine {
	when, _ := time.Parse(time.DateOnly, day.Date)
	out := []DetailLine{{Name: when.Format("Monday 2 January"), Value: fmt.Sprint(day.Count), Heading: true}}
	byRepo := map[string][]DevItem{}
	var repos []string
	for _, it := range r.Items {
		if it.Date != day.Date {
			continue
		}
		if _, ok := byRepo[it.Repo]; !ok {
			repos = append(repos, it.Repo)
		}
		byRepo[it.Repo] = append(byRepo[it.Repo], it)
	}
	// The busiest repository first; ties in the order the forge gave.
	slices.SortStableFunc(repos, func(a, b string) int { return len(byRepo[b]) - len(byRepo[a]) })
	for _, repo := range repos {
		items := byRepo[repo]
		// A repository a step under its day, and its things a step under
		// it (spec 107).
		out = append(out, DetailLine{
			Name: truncate(withoutOwner(repo), DetailNameWidth), Value: devCounts(items), Status: Good, Indent: 1,
		})
		out = append(out, devThings(items)...)
	}
	if r.Private > 0 {
		out = append(out, DetailLine{Name: fmt.Sprintf("%d private contributions not listed", r.Private), Wide: true, Indent: 1})
	}
	if len(repos) == 0 && day.Count > 0 {
		out = append(out, DetailLine{Name: "nothing the forge lists", Wide: true, Indent: 1})
	}
	return out
}

// withoutOwner is a repository's name without its owner or group, which is
// a user name (rule 9): "octo/widget" is "widget", "team/sub/agent" "agent".
//
// A repository named for its owner is itself a user name (spec 102 review):
// a profile's README ("octo/octo") is "profile", a Pages site
// ("octo/octo.github.io") is "pages".
func withoutOwner(repo string) string {
	i := strings.LastIndex(repo, "/")
	if i < 0 {
		return repo
	}
	owner, name := repo[:i], repo[i+1:]
	if j := strings.LastIndex(owner, "/"); j >= 0 {
		owner = owner[j+1:]
	}
	switch {
	case strings.EqualFold(name, owner):
		return "profile"
	case strings.EqualFold(name, owner+".github.io"), strings.EqualFold(name, owner+".gitlab.io"):
		return "pages"
	}
	return name
}

// devKinds are the kinds' short names in the order a repository's counts
// are written: commits, pushes, pull and merge requests, issues, reviews,
// comments; the rest are one "+n".
var devKinds = []struct{ kind, short string }{
	{"commits", "c"}, {"pushed", "↑"}, {"pr", "PR"}, {"mr", "MR"},
	{"issue", "i"}, {"review", "rev"}, {"comment", "cm"},
}

func devCounts(items []DevItem) string {
	n := map[string]int{}
	for _, it := range items {
		n[it.Kind] += it.Count
	}
	var parts []string
	for _, k := range devKinds {
		if n[k.kind] > 0 {
			parts = append(parts, fmt.Sprintf("%d%s", n[k.kind], k.short))
			delete(n, k.kind)
		}
	}
	other := 0
	for _, c := range n {
		other += c
	}
	if other > 0 {
		parts = append(parts, fmt.Sprintf("+%d", other))
	}
	return strings.Join(parts, " ")
}

// devThings are a repository's things, one line each: a pull request opened,
// reviewed and merged in a day is one line, merged; a branch pushed four
// times is one line, "pushed ×4". Commits are counted, not listed.
func devThings(items []DevItem) []DetailLine {
	type thing struct {
		name    string
		actions []string
		pushes  int
	}
	var order []*thing
	seen := map[string]*thing{}
	for _, it := range items {
		if it.Kind == "commits" {
			continue
		}
		key := it.Ref + "\x00" + it.Title
		name := strings.TrimSpace(it.Ref + " " + it.Title)
		if it.Kind == "pushed" {
			key, name = "push\x00"+it.Ref, it.Ref
		}
		t := seen[key]
		if t == nil {
			t = &thing{name: name}
			seen[key] = t
			order = append(order, t)
		}
		if it.Kind == "pushed" {
			t.pushes++
			continue
		}
		act := it.Action
		if it.Kind == "review" {
			act = "review " + it.Action
		}
		if !slices.Contains(t.actions, act) {
			t.actions = append(t.actions, act)
		}
	}
	out := make([]DetailLine, 0, len(order))
	for _, t := range order {
		value := furthest(t.actions)
		switch {
		case t.pushes == 1:
			value = "pushed"
		case t.pushes > 1:
			value = fmt.Sprintf("pushed ×%d", t.pushes)
		}
		out = append(out, DetailLine{Name: truncate(t.name, DevWideNameWidth), Value: value, Wide: true, Indent: 2})
	}
	return out
}

// devRank is how far a thing got, furthest first.
var devRank = []string{"merged", "closed", "review approved", "approved", "review changes_requested", "opened", "reopened", "open", "review commented", "commented"}

// furthest is the one action a thing's line shows.
func furthest(actions []string) string {
	for _, r := range devRank {
		if slices.Contains(actions, r) {
			return r
		}
	}
	if len(actions) > 0 {
		return actions[0]
	}
	return ""
}

// DevTitle is the heading of the usage section's overview (spec 102 R1).
const DevTitle = "Development"

// Development is the drawer's overview: the AI usage accounts, then the
// forges, each a part, in that order (spec 102 R2).
func Development(accounts, forges []Detail) Detail {
	parts := append(slices.Clone(accounts), forges...)
	// Each part's name is a heading, bold where the shell can (spec 107):
	// the accounts, the forges and the lists under them are told apart by it.
	for i := range parts {
		parts[i].TitleBold = true
	}
	return Detail{
		Section: UsageInfo.Key,
		ID:      DetailAccounts,
		Title:   DevTitle,
		Parts:   parts,
	}
}

// UsageOverview is an account's part of the overview: its meters, and a
// heading that opens the account's own page with everything else on it
// (spec 102 R3). An account with no meters keeps its first line, so it is
// not a bare heading.
func UsageOverview(d Detail) Detail {
	d.Open = d.ID
	if len(d.Meters) > 0 {
		d.Lines = nil
		return d
	}
	var keep []DetailLine
	for _, l := range d.Lines {
		if !l.Heading {
			keep = append(keep, l)
			break
		}
	}
	d.Lines = keep
	return d
}
