package panel_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/hayami/internal/config"
	"github.com/ushineko/hayami/internal/forge"
	"github.com/ushineko/hayami/internal/panel"
	"github.com/ushineko/hayami/internal/view"
)

// forgeFake is a forge reader that counts its readings and answers what it
// is told to, per forge.
type forgeFake struct {
	mu    sync.Mutex
	reads map[string]int
	err   map[string]error
	hold  chan struct{} // a reading waits on it when not nil
	now   func() time.Time
}

func (f *forgeFake) read(_ context.Context, p string, _ config.UsageSettings) (forge.Activity, error) {
	f.mu.Lock()
	f.reads[p]++
	err, hold := f.err[p], f.hold
	f.mu.Unlock()
	if hold != nil {
		<-hold
	}
	if err != nil {
		return forge.Activity{}, err
	}
	_, dates := forge.Window(f.now())
	a := forge.Activity{Provider: p, Unit: "events", ReadAt: f.now()}
	for i, d := range dates {
		a.Days = append(a.Days, forge.Day{Date: d, Count: i})
	}
	a.Items = []forge.Item{{Date: dates[13], Repo: "octo/widget", Kind: forge.KindPR, Ref: "#1", Title: "a widget", Action: "merged", Count: 1}}
	return a, nil
}

func (f *forgeFake) count(p string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.reads[p]
}

func (f *forgeFake) fail(p string, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.err[p] = err
}

// devUsage is the usage section with one account and the forges given,
// over a clock the test moves.
func devUsage(t *testing.T, s config.UsageSettings) (*panel.Usage, *forgeFake, *time.Time) {
	t.Helper()
	now := time.Date(2026, 3, 14, 12, 0, 0, 0, time.Local)
	u := panel.NewUsage()
	panel.SetUsageNow(u, func() time.Time { return now })
	panel.SetUsageRead(u, func(context.Context) ([]view.UsageWindow, time.Time, []view.Reason, error) {
		return []view.UsageWindow{{Account: "CC desk", Name: "Week", Fraction: 0.4}}, now, nil, nil
	})
	panel.SetUsageBreakdown(u, func(label string) (view.UsageBreakdown, error) {
		return view.UsageBreakdown{Account: label, Plan: "max", Limits: []view.UsageLimit{{Name: "Week", Fraction: 0.4}},
			SharesTitle: "Week by use", Shares: []view.UsageShare{{Name: "Claude Code", Fraction: 0.9}}}, nil
	})
	f := &forgeFake{reads: map[string]int{}, err: map[string]error{}, now: func() time.Time { return now }}
	panel.SetForgeRead(u, f.read)
	u.SetDevelopment(s)
	_, err := u.Poll(t.Context())
	require.NoError(t, err)
	return u, f, &now
}

// overview reads the drawer's overview, after waiting for any reading.
func overview(t *testing.T, u *panel.Usage) view.Detail {
	t.Helper()
	_, _ = u.PollDetail(t.Context(), view.DetailAccounts)
	u.Settle(t.Context())
	d, _ := u.PollDetail(t.Context(), view.DetailAccounts)
	return d
}

func titles(d view.Detail) []string {
	var out []string
	for _, p := range d.Parts {
		out = append(out, p.Title)
	}
	return out
}

// Spec 102 R1, R2, R12, AC1, AC2. The drawer is "Development"; its parts
// follow the switches, in order; a forge switched off is not read.
func TestTheOverviewFollowsTheSwitches(t *testing.T) {
	u, f, _ := devUsage(t, config.UsageSettings{})
	d := overview(t, u)
	assert.Equal(t, view.DevTitle, d.Title)
	assert.Equal(t, "Development", view.UsageInfo.Title)
	assert.Equal(t, "usage", view.UsageInfo.Key, "the key a settings file names is unchanged")
	assert.Equal(t, []string{"CC desk"}, titles(d), "the forges are off until switched on")
	assert.Zero(t, f.count(forge.GitHub)+f.count(forge.GitLab))

	u.SetDevelopment(config.UsageSettings{GitHub: true, GitLab: true})
	// What was done lately and where, under the forges once they list
	// something (spec 107).
	assert.Equal(t, []string{"CC desk", "GitHub", "GitLab", "Recent", "Repositories"}, titles(overview(t, u)))

	u.SetDevelopment(config.UsageSettings{HideAI: true, GitLab: true})
	assert.Equal(t, []string{"GitLab", "Recent", "Repositories"}, titles(overview(t, u)))
	before := f.count(forge.GitHub)
	overview(t, u)
	assert.Equal(t, before, f.count(forge.GitHub), "a forge switched off was read")
}

// R3, AC3. The overview's account is its meters, its heading opening its own
// page; the page keeps the plan and the week by use.
func TestAnAccountKeepsItsMetersInTheOverview(t *testing.T) {
	u, _, _ := devUsage(t, config.UsageSettings{})
	part := overview(t, u).Parts[0]
	require.Len(t, part.Meters, 1)
	assert.Empty(t, part.Lines)
	assert.Equal(t, "CC desk", part.Open)

	page, err := u.PollDetail(t.Context(), "CC desk")
	require.NoError(t, err)
	names := map[string]bool{}
	for _, l := range page.Lines {
		names[l.Name] = true
	}
	assert.True(t, names["Plan"])
	assert.True(t, names["Week by use"])
	assert.True(t, names["Claude Code"])
	assert.Equal(t, view.DetailAccounts, u.DrillUp("CC desk"), "an account's page goes back to the overview")
}

// R9, AC9. A reading younger than the interval is used again; one older is
// read again; a click on the heading reads at once; the card's own poll,
// with no drawer, reads nothing.
func TestAForgeIsReadWhenItsReadingIsOld(t *testing.T) {
	u, f, now := devUsage(t, config.UsageSettings{GitHub: true, RefreshMinutes: 10})
	_, err := u.Poll(t.Context())
	require.NoError(t, err)
	assert.Zero(t, f.count(forge.GitHub), "the card's poll read a forge")

	overview(t, u)
	assert.Equal(t, 1, f.count(forge.GitHub))
	*now = now.Add(9 * time.Minute)
	overview(t, u)
	assert.Equal(t, 1, f.count(forge.GitHub), "a reading nine minutes old was read again")
	*now = now.Add(2 * time.Minute)
	overview(t, u)
	assert.Equal(t, 2, f.count(forge.GitHub))

	assert.True(t, u.Refresh(view.RefreshAct(forge.GitHub)))
	u.Settle(t.Context())
	assert.Equal(t, 3, f.count(forge.GitHub))
	assert.False(t, u.Refresh(view.RefreshAct(forge.GitLab)), "a forge switched off was refreshed")
	assert.False(t, u.Refresh(view.ActBack))
}

// R4, R9. While a forge is read the part says so and the drawer is asked
// for again within a second; once it lands, at the section's interval.
func TestAReadingInFlightIsDrawnAndWatched(t *testing.T) {
	u, f, _ := devUsage(t, config.UsageSettings{GitHub: true})
	f.hold = make(chan struct{})
	d, _ := u.PollDetail(t.Context(), view.DetailAccounts)
	require.Len(t, d.Parts, 2)
	assert.Contains(t, d.Parts[1].TitleParts[0].Text, "reading")
	assert.LessOrEqual(t, u.DetailInterval(view.DetailAccounts), time.Second)

	close(f.hold)
	u.Settle(t.Context())
	assert.Equal(t, panel.UsageInterval, u.DetailInterval(view.DetailAccounts))
	d, _ = u.PollDetail(t.Context(), view.DetailAccounts)
	require.NotNil(t, d.Parts[1].Days)
	assert.Len(t, d.Parts[1].Days.Values, forge.Days)
}

// R5, AC10. Each failure has its reason, the last reading stays under it,
// and a rate-limited forge is not asked again before its time.
func TestAFailedReadingSaysWhy(t *testing.T) {
	u, f, now := devUsage(t, config.UsageSettings{GitHub: true, RefreshMinutes: 5})
	require.NotNil(t, overview(t, u).Parts[1].Days)

	cases := []struct {
		err  error
		text string
	}{
		{fmt.Errorf("%w: sign in with gh auth login", forge.ErrNoCredential), "sign in with gh auth login"},
		{forge.ErrUnauthorized, "the key was refused"},
		{errors.New("gitlab: the host could not be reached"), "could not be read"},
		{&forge.RateLimited{Until: now.Add(time.Hour)}, "rate limited until"},
	}
	for _, c := range cases {
		f.fail(forge.GitHub, c.err)
		*now = now.Add(6 * time.Minute)
		part := overview(t, u).Parts[1]
		require.Len(t, part.Reasons, 1, "%v", c.err)
		assert.Contains(t, part.Reasons[0].Text, c.text)
		assert.NotNil(t, part.Days, "the last reading went with the failure: %v", c.err)
	}
	reads := f.count(forge.GitHub)
	*now = now.Add(30 * time.Minute)
	overview(t, u)
	assert.Equal(t, reads, f.count(forge.GitHub), "a rate-limited forge was asked again before its time")
	*now = now.Add(31 * time.Minute)
	overview(t, u)
	assert.Equal(t, reads+1, f.count(forge.GitHub))
}

// R6, R14. A day is a drill under the overview, offered while its forge is
// on, and its page has the day's things; the day is not in the list of
// details that readings and doctor print.
func TestADayIsADrillUnderTheOverview(t *testing.T) {
	u, _, now := devUsage(t, config.UsageSettings{GitLab: true})
	overview(t, u)
	day := view.DevDayID(forge.GitLab, now.Format(time.DateOnly))
	assert.True(t, panel.Offers(u, day))
	assert.Equal(t, view.DetailAccounts, u.DrillUp(day))
	for _, info := range u.Details() {
		assert.NotEqual(t, day, info.ID, "a day is listed, so readings would print it")
	}
	page, err := u.PollDetail(t.Context(), day)
	require.NoError(t, err)
	require.Len(t, page.Parts, 1)
	assert.Equal(t, forge.Days-1, page.Parts[0].Days.Selected)
	var found bool
	for _, l := range page.Parts[0].Lines {
		if l.Name == "#1 a widget" {
			found = true
			assert.Equal(t, "merged", l.Value)
		}
	}
	assert.True(t, found, "the day's pull request is not on its page")

	u.SetDevelopment(config.UsageSettings{})
	assert.False(t, panel.Offers(u, day), "a day of a forge switched off is offered")
}
