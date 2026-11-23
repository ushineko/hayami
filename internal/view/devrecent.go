package view

import (
	"cmp"
	"slices"
)

/*
The overview's two parts under the forges (spec 107): what was done lately,
newest first, and where the fortnight's work went. Both are drawn from the
items the forges' reads already hold, so they cost no request.
*/

// The parts' IDs, and how much of each the overview shows: Recent is cut to
// the drawer's room by the shell (Detail.Fit), and carries no more than
// DevRecentLines for it to cut from.
const (
	DevRecentID    = "recent"
	DevReposID     = "repositories"
	DevRecentLines = 40
	DevReposShown  = 3
)

// combined is every shown forge's items as one reading, for the parts that
// do not care which forge an item came from. A forge's private count is
// left out: it belongs to that forge's own strip.
func combined(forges []DevReading) DevReading {
	var all DevReading
	counts := map[string]int{}
	for _, f := range forges {
		for _, it := range f.Items {
			if it.Provider == "" {
				it.Provider = f.Provider
			}
			all.Items = append(all.Items, it)
		}
		for _, d := range f.Days {
			counts[d.Date] += d.Count
		}
	}
	for date, n := range counts {
		all.Days = append(all.Days, DevDay{Date: date, Count: n})
	}
	slices.SortFunc(all.Days, func(a, b DevDay) int { return cmp.Compare(b.Date, a.Date) })
	return all
}

/*
DevRecent is what was done lately across the forges, newest day first: each
day's heading and, under it, its repositories and the things worked on, as a
day's page draws them. It fills the room the drawer has left and no more
(Fit); it is empty, and not drawn, before a forge has listed anything.
*/
func DevRecent(forges []DevReading) Detail {
	d := Detail{Section: UsageInfo.Key, ID: DevRecentID, Title: "Recent", Fit: true, Personal: true}
	all := combined(forges)
	for _, day := range all.Days {
		if !slices.ContainsFunc(all.Items, func(it DevItem) bool { return it.Date == day.Date }) {
			continue
		}
		lines := devDayLines(all, day)
		room := DevRecentLines - len(d.Lines)
		if room <= 1 {
			// A heading with nothing under it says nothing.
			break
		}
		d.Lines = append(d.Lines, lines[:min(len(lines), room)]...)
	}
	return d
}

/*
DevRepos is where the fortnight's work went: the busiest repositories across
the forges, a bar each against the busiest, with its counts by kind. It is
empty, and not drawn, before a forge has listed anything.
*/
func DevRepos(forges []DevReading) Detail {
	d := Detail{Section: UsageInfo.Key, ID: DevReposID, Title: "Repositories", Personal: true}
	byRepo := map[string][]DevItem{}
	total := map[string]int{}
	var repos []string
	for _, it := range combined(forges).Items {
		name := withoutOwner(it.Repo)
		if _, ok := byRepo[name]; !ok {
			repos = append(repos, name)
		}
		byRepo[name] = append(byRepo[name], it)
		total[name] += max(1, it.Count)
	}
	slices.SortStableFunc(repos, func(a, b string) int { return total[b] - total[a] })
	if len(repos) == 0 {
		return d
	}
	most := float64(total[repos[0]])
	for _, repo := range repos[:min(len(repos), DevReposShown)] {
		d.Meters = append(d.Meters, Meter{
			Label:    truncate(repo, DetailNameWidth),
			Caption:  devCounts(byRepo[repo]),
			Fraction: float64(total[repo]) / most,
			Status:   Info,
		})
	}
	return d
}
