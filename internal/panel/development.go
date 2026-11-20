package panel

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/ushineko/hayami/internal/config"
	"github.com/ushineko/hayami/internal/forge"
	"github.com/ushineko/hayami/internal/safetext"
	"github.com/ushineko/hayami/internal/view"
)

// usage is the Development section built over the settings.
func (e Env) usage() Source {
	u := NewUsage()
	u.SetDevelopment(config.Usage.Get(e.Settings))
	return u
}

// forgeTimeout bounds one reading of a forge, credentials and every page.
const forgeTimeout = time.Minute

// forgeOrder is the forges in the order the drawer draws them, with their
// names.
var forgeOrder = []struct{ key, title string }{
	{forge.GitHub, "GitHub"},
	{forge.GitLab, "GitLab"},
}

// forgeRead reads one forge with the settings: the credential, then the
// fortnight. A test replaces it.
type forgeRead func(ctx context.Context, provider string, s config.UsageSettings) (forge.Activity, error)

/*
forges are the Development card's git parts (spec 102): each forge's last
reading, whether one is in flight, and why the last one failed.

**A forge is read only while a drawer shows it** (R9): the drawer's poll asks
for the parts, and a part whose reading is older than the interval, or that
has none, is read then, in the background, so the drawer is drawn at once
with "reading…" and again when the reading lands. A forge switched off, or a
drawer closed, reads nothing. A refused or rate-limited forge is not asked
again before the interval, or before the time it gave.
*/
type forges struct {
	mu       sync.Mutex
	settings config.UsageSettings
	state    map[string]*forgeState
	read     forgeRead
	now      func() time.Time
}

type forgeState struct {
	act     *forge.Activity
	reading bool
	failed  error
	failAt  time.Time
	// gen counts the settings a reading was started under, so one that
	// lands after the settings changed is not kept.
	gen int
}

func newForges(now func() time.Time) *forges {
	return &forges{state: map[string]*forgeState{}, read: readForge(forge.NewCreds()), now: now}
}

// readForge is the real reading: gh or the secrets file for the credential,
// then the forge's API.
func readForge(c forge.Creds) forgeRead {
	return func(ctx context.Context, provider string, s config.UsageSettings) (forge.Activity, error) {
		switch provider {
		case forge.GitHub:
			tok, err := c.GitHubToken(ctx, forge.Override{Token: s.GitHubToken})
			if err != nil {
				return forge.Activity{}, err
			}
			return forge.GitHubReader{}.Read(ctx, tok)
		case forge.GitLab:
			host := c.GitLabHost(ctx, forge.Override{Host: s.GitLabHost})
			if host == "" {
				return forge.Activity{}, errNoHost
			}
			tok, err := c.GitLabToken(ctx, host, forge.Override{Token: s.GitLabToken})
			if err != nil {
				return forge.Activity{}, err
			}
			return forge.GitLabReader{Host: host}.Read(ctx, tok)
		}
		return forge.Activity{}, errors.New("no such forge: " + provider)
	}
}

var errNoHost = fmt.Errorf("%w: no GitLab host; set one in Preferences or GITLAB_HOST in ~/.secrets.env", forge.ErrNoCredential)

// setSettings takes new settings. A forge whose credential or host changed
// is read afresh; one switched off keeps nothing.
func (f *forges) setSettings(s config.UsageSettings) {
	f.mu.Lock()
	defer f.mu.Unlock()
	old := f.settings
	f.settings = s
	if old.GitHubToken != s.GitHubToken || !s.GitHub {
		f.forget(forge.GitHub)
	}
	if old.GitLabToken != s.GitLabToken || old.GitLabHost != s.GitLabHost || !s.GitLab {
		f.forget(forge.GitLab)
	}
}

// forget drops a forge's reading. Under f.mu.
func (f *forges) forget(provider string) {
	if st, ok := f.state[provider]; ok {
		f.state[provider] = &forgeState{gen: st.gen + 1}
	}
}

// shown are the forges switched on, in the drawer's order.
func (f *forges) shown() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.shownLocked()
}

func (f *forges) shownLocked() []string {
	var out []string
	for _, p := range forgeOrder {
		if (p.key == forge.GitHub && f.settings.GitHub) || (p.key == forge.GitLab && f.settings.GitLab) {
			out = append(out, p.key)
		}
	}
	return out
}

// aiShown is whether the AI usage part is drawn.
func (f *forges) aiShown() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return !f.settings.HideAI
}

// due starts a reading of each shown forge whose last one is older than the
// interval, or that has none (R9).
func (f *forges) due() {
	f.mu.Lock()
	defer f.mu.Unlock()
	now := f.now()
	for _, p := range f.shownLocked() {
		st := f.stateLocked(p)
		if st.reading || !f.staleLocked(st, now) {
			continue
		}
		f.startLocked(p, st)
	}
}

// staleLocked is whether a forge's reading is old enough to read again.
func (f *forges) staleLocked(st *forgeState, now time.Time) bool {
	every := f.settings.Refresh()
	if st.failed != nil {
		var rl *forge.RateLimited
		if errors.As(st.failed, &rl) && rl.Until.After(now) {
			return false
		}
		return now.Sub(st.failAt) >= every
	}
	return st.act == nil || now.Sub(st.act.ReadAt) >= every
}

// refresh reads a forge now, unless a reading is in flight or it is
// switched off (a click on its heading).
func (f *forges) refresh(provider string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, p := range f.shownLocked() {
		if p == provider {
			if st := f.stateLocked(p); !st.reading {
				f.startLocked(p, st)
			}
		}
	}
}

func (f *forges) stateLocked(p string) *forgeState {
	st, ok := f.state[p]
	if !ok {
		st = &forgeState{}
		f.state[p] = st
	}
	return st
}

// startLocked reads a forge in the background.
func (f *forges) startLocked(p string, st *forgeState) {
	st.reading = true
	gen, settings, read := st.gen, f.settings, f.read
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), forgeTimeout)
		defer cancel()
		act, err := read(ctx, p, settings)
		f.mu.Lock()
		defer f.mu.Unlock()
		cur := f.stateLocked(p)
		if cur.gen != gen {
			return
		}
		cur.reading = false
		if err != nil {
			cur.failed, cur.failAt = err, f.now()
			return
		}
		cur.failed = nil
		cur.act = &act
	}()
}

// reading is whether any shown forge has a reading in flight.
func (f *forges) reading() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, p := range f.shownLocked() {
		if st, ok := f.state[p]; ok && st.reading {
			return true
		}
	}
	return false
}

// shows reports whether a forge is switched on.
func (f *forges) shows(provider string) bool {
	for _, p := range f.shown() {
		if p == provider {
			return true
		}
	}
	return false
}

// reading is a forge's state as the view draws it.
func (f *forges) viewOf(provider string) view.DevReading {
	f.mu.Lock()
	defer f.mu.Unlock()
	r := view.DevReading{Provider: provider}
	for _, p := range forgeOrder {
		if p.key == provider {
			r.Title = p.title
		}
	}
	st := f.stateLocked(provider)
	r.Reading = st.reading
	if st.failed != nil {
		reason := forgeReason(st.failed)
		r.Failed = &reason
	}
	if st.act == nil {
		return r
	}
	a := st.act
	r.Unit, r.Private, r.ReadAt = a.Unit, a.Private, a.ReadAt
	for _, d := range a.Days {
		r.Days = append(r.Days, view.DevDay{Date: d.Date, Count: d.Count})
	}
	for _, it := range a.Items {
		r.Items = append(r.Items, view.DevItem{
			Date: it.Date, Repo: clean(it.Repo), Kind: string(it.Kind), Ref: clean(it.Ref),
			Title: clean(it.Title), Action: clean(it.Action), Count: it.Count,
		})
	}
	return r
}

/*
forgeReason is a failure as the part says it (R5): what to do about a
missing credential, a refused one, a rate limit's end, or that the forge
could not be read. The words are this program's own; the error, which may
name a URL, goes only in the reason's detail.
*/
func forgeReason(err error) view.Reason {
	var rl *forge.RateLimited
	switch {
	case errors.Is(err, forge.ErrNoCredential):
		return view.Reason{Text: strings.TrimPrefix(err.Error(), forge.ErrNoCredential.Error()+": "), Status: view.Info}
	case errors.Is(err, forge.ErrUnauthorized):
		return view.Reason{Text: "the key was refused", Status: view.Warn}
	case errors.As(err, &rl):
		return view.Reason{Text: rl.Error(), Status: view.Warn}
	}
	return view.Reason{Text: "could not be read", Detail: err.Error(), Status: view.Warn}
}

// clean is text a forge returned, as any shell may draw it: what other people
// write (a title, a branch) carries no control character (spec 102 review, F1).
func clean(s string) string { return safetext.Printable(s, 0) }
