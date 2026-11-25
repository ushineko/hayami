package panel

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/ushineko/hayami/internal/config"
	"github.com/ushineko/hayami/internal/forge"
	"github.com/ushineko/hayami/internal/view"
)

/*
A change's page (spec 111): a click on a line of a day's page opens it, and
it is read then, in the background, once, and kept for the refresh interval.
The page is drawn at once from what the day's line already says, with
"reading…", and again when the reading lands.

The ID names the day and a key, a hash; the thing is found again among the
forge's last reading's items on that day, so a page can only ever read what a
reading listed.
*/

// changeRead reads one change with the settings. A test replaces it.
type changeRead func(ctx context.Context, provider string, s config.UsageSettings, ref forge.ChangeRef) (forge.Change, error)

type changeState struct {
	// c is the change as the view takes it, made once when its reading
	// lands rather than on every poll of the page.
	c       *view.DevChange
	reading bool
	failed  error
	at      time.Time
	gen     int
}

// readChange is the real reading: the forge's credential, as a fortnight's
// reading finds it, then the change.
func readChange(c forge.Creds) changeRead {
	return func(ctx context.Context, provider string, s config.UsageSettings, ref forge.ChangeRef) (forge.Change, error) {
		switch provider {
		case forge.GitHub:
			tok, err := c.GitHubToken(ctx, forge.Override{Token: s.GitHubToken})
			if err != nil {
				return forge.Change{}, err
			}
			return forge.GitHubReader{}.ReadChange(ctx, tok, ref)
		case forge.GitLab:
			host := c.GitLabHost(ctx, forge.Override{Host: s.GitLabHost})
			if host == "" {
				return forge.Change{}, errNoHost
			}
			tok, err := c.GitLabToken(ctx, host, forge.Override{Token: s.GitLabToken})
			if err != nil {
				return forge.Change{}, err
			}
			return forge.GitLabReader{Host: host}.ReadChange(ctx, tok, ref)
		}
		return forge.Change{}, errors.New("no such forge: " + provider)
	}
}

// found is a page's thing among a reading's items: what it opens, its
// repository and reference, what to read, and the day's line for it.
type found struct {
	open, repo, ref, line string
	read                  forge.ChangeRef
}

/*
findLocked is the item a change page's key names, on its day, in a forge's
last reading. Under f.mu. ok is false when the reading no longer lists it.
*/
func (f *forges) findLocked(provider, date, key string) (found, bool) {
	st, ok := f.state[provider]
	if !ok || st.act == nil {
		return found{}, false
	}
	var out found
	hit := false
	for _, it := range st.act.Items {
		if it.Date != date {
			continue
		}
		repo, ref := clean(it.Repo), clean(it.Ref)
		if !hit && view.DevChangeKey(view.DevOpenRepo, repo, "") == key {
			out, hit = found{open: view.DevOpenRepo, repo: repo, read: forge.ChangeRef{Kind: forge.KindCommits, Repo: it.Repo}}, true
		}
		open, by := view.DevOpens(provider, string(it.Kind), ref)
		if !hit && open != "" && view.DevChangeKey(open, repo, by) == key {
			out = found{open: open, repo: repo, ref: by, line: strings.TrimSpace(ref + " " + clean(it.Title)), read: forge.ChangeRef{Repo: it.Repo}}
			switch open {
			case view.DevOpenPR:
				out.read.Kind, out.read.Number = forge.KindPR, view.DevNumber(by)
			case view.DevOpenMR:
				out.read.Kind, out.read.Number = forge.KindMR, view.DevNumber(by)
			case view.DevOpenIssue:
				out.read.Kind, out.read.Number = forge.KindIssue, view.DevNumber(by)
			case view.DevOpenPush:
				out.read.Kind, out.read.Branch = forge.KindPushed, it.Ref
			}
			hit = true
		}
	}
	if !hit {
		return found{}, false
	}
	// A repository's day, or a branch's pushes: every push of the day that
	// it covers, for GitLab; the day itself, for GitHub.
	if out.read.Kind == forge.KindCommits || out.read.Kind == forge.KindPushed {
		for _, it := range st.act.Items {
			if it.Date != date || it.Kind != forge.KindPushed || it.Repo != out.read.Repo {
				continue
			}
			if out.read.Kind == forge.KindPushed && it.Ref != out.read.Branch {
				continue
			}
			out.read.Pushes = append(out.read.Pushes, forge.Push{From: it.From, To: it.To, Count: it.Commits})
		}
		day, err := time.ParseInLocation(time.DateOnly, date, time.Local)
		if err == nil {
			out.read.Day = day
		}
	}
	return out, true
}

/*
readChangePage reads a change's page: what the day's line said at once, and
the change itself once read. A page not read, or read longer ago than the
interval, is read now in the background.
*/
func (f *forges) readChangePage(id string) RawChange {
	provider, date, key, _ := view.ParseDevChangeID(id)
	f.mu.Lock()
	defer f.mu.Unlock()
	title := view.DevTitle
	for _, p := range forgeOrder {
		if p.key == provider {
			title = p.title
		}
	}
	thing, ok := f.findLocked(provider, date, key)
	if fst := f.state[provider]; !ok && (fst == nil || fst.act == nil) {
		// The fortnight is not read yet -- a page opened from the command
		// line, or after a credential changed -- and the thing is found once
		// it is.
		return RawChange{State: ChangeWaiting, Forge: title}
	}
	if !ok {
		return RawChange{State: ChangeGone, Forge: title}
	}
	st := f.changes[id]
	if st == nil {
		st = &changeState{}
		f.changes[id] = st
	}
	if !st.reading && (st.c == nil && st.failed == nil || f.now().Sub(st.at) >= f.settings.Refresh()) {
		f.startChangeLocked(id, provider, thing.read, st)
	}
	var failed *view.Reason
	if st.failed != nil {
		r := forgeReason(st.failed)
		failed = &r
	}
	return RawChange{
		State: ChangeFound, Forge: title,
		Open: thing.open, Repo: thing.repo, Ref: thing.ref, Line: thing.line,
		Change: st.c, Reading: st.reading, Failed: failed,
	}
}

// drawChangePage is a change's page as the shells show it.
func drawChangePage(raw RawDetail) view.Detail {
	c := raw.Change
	switch c.State {
	case ChangeWaiting:
		return view.Detail{Section: raw.Section, ID: raw.ID, Title: c.Forge, Parts: []view.Detail{
			{Section: raw.Section, ID: "change", Title: "reading…", TitleBold: true},
		}}
	case ChangeGone:
		return view.Detail{Section: raw.Section, ID: raw.ID, Title: c.Forge, Reasons: []view.Reason{
			{Text: "no longer in the forge's last reading", Status: view.Info},
		}}
	}
	return view.DevChangePage(c.Forge, raw.ID, c.Open, c.Repo, c.Ref, c.Line, c.Change, c.Reading, c.Failed)
}

// startChangeLocked reads a change in the background. Under f.mu.
func (f *forges) startChangeLocked(id, provider string, ref forge.ChangeRef, st *changeState) {
	st.reading = true
	gen, settings, read := st.gen, f.settings, f.readChange
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), forgeTimeout)
		defer cancel()
		c, err := guarded(func() (forge.Change, error) { return read(ctx, provider, settings, ref) })
		f.mu.Lock()
		defer f.mu.Unlock()
		cur, ok := f.changes[id]
		if !ok || cur.gen != gen {
			return
		}
		cur.reading, cur.at = false, f.now()
		if err != nil {
			cur.failed = err
			return
		}
		cur.failed, cur.c = nil, devChange(c)
	}()
}

/*
guarded is a forge's reading with a panic in it made its error. A reading
decodes what a host sent, through libraries that are not this program's, in
a goroutine of its own; a panic there would end the panel (threat model B4).
*/
func guarded[T any](read func() (T, error)) (out T, err error) {
	defer func() {
		if recover() != nil {
			var zero T
			out, err = zero, forge.ErrBadAnswer
		}
	}()
	return read()
}

// forgetChangesLocked drops a forge's pages, for a credential changed or a
// forge switched off. Under f.mu.
func (f *forges) forgetChangesLocked(provider string) {
	for id, st := range f.changes {
		if p, _, _, ok := view.ParseDevChangeID(id); ok && p == provider {
			f.changes[id] = &changeState{gen: st.gen + 1}
		}
	}
}

// changeReading is whether a change's page is being read.
func (f *forges) changeReadingLocked() bool {
	for _, st := range f.changes {
		if st.reading {
			return true
		}
	}
	return false
}

// devChange is a change as the view takes it, its forge's text made
// printable: what other people write reaches a screen only through clean.
func devChange(c forge.Change) *view.DevChange {
	out := &view.DevChange{
		Kind: string(c.Kind), Ref: clean(c.Ref), Title: clean(c.Title),
		State: clean(c.State), StateReason: clean(c.StateReason), Draft: c.Draft,
		Opened: c.Opened, Merged: c.Merged, Closed: c.Closed,
		Source: clean(c.Source), Target: clean(c.Target),
		Additions: c.Additions, Deletions: c.Deletions, FilesCount: c.FilesCount, CommitCount: c.CommitCount,
		SizeKnown: c.SizeKnown, Checks: clean(c.Checks),
		Approvals: c.Approvals, ChangesWanted: c.ChangesRequested, Decision: clean(c.Decision),
		Comments: c.Comments,
	}
	for _, l := range c.Labels {
		out.Labels = append(out.Labels, clean(l))
	}
	// The description keeps its lines: each is made printable on its own.
	lines := strings.Split(c.Body, "\n")
	for i, l := range lines {
		lines[i] = clean(l)
	}
	out.Body = strings.Join(lines, "\n")
	for _, cm := range c.Commits {
		out.Commits = append(out.Commits, view.DevCommit{
			SHA: clean(cm.SHA), Title: clean(cm.Title), When: cm.When, Additions: cm.Additions, Deletions: cm.Deletions, Stats: cm.Stats,
		})
	}
	for _, fl := range c.Files {
		out.Files = append(out.Files, view.DevFile{Path: clean(fl.Path), Additions: fl.Additions, Deletions: fl.Deletions, Status: clean(fl.Status)})
	}
	return out
}
