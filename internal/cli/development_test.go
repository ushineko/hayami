package cli_test

import (
	"bytes"
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/hayami/internal/cli"
	"github.com/ushineko/hayami/internal/panel"
	"github.com/ushineko/hayami/internal/view"
)

// settling is the Development section with a forge whose reading lands only
// once Settle is called, as a reading in the background does.
type settling struct{ settled atomic.Bool }

var day = time.Date(2026, 3, 14, 12, 0, 0, 0, time.Local)

func (s *settling) reading() view.DevReading {
	r := view.DevReading{Provider: "github", Title: "GitHub", Reading: !s.settled.Load()}
	if s.settled.Load() {
		r.Unit, r.ReadAt = "contributions", day
		for i := range 14 {
			r.Days = append(r.Days, view.DevDay{Date: time.Date(2026, 3, 1+i, 0, 0, 0, 0, time.Local).Format(time.DateOnly), Count: 1})
		}
		r.Items = []view.DevItem{{Date: "2026-03-14", Repo: "octo/widget", Kind: "pr", Ref: "#9", Title: "a widget", Action: "merged", Count: 1}}
	}
	return r
}

func (s *settling) Key() string                         { return "usage" }
func (s *settling) Poll(context.Context) (bool, error)  { return true, nil }
func (s *settling) Interval() time.Duration             { return time.Hour }
func (s *settling) Data() any                           { return nil }
func (s *settling) Section() view.Section               { return view.Section{Key: "usage", Title: "Development"} }
func (s *settling) DetailInterval(string) time.Duration { return time.Hour }
func (s *settling) CloseDetail(string)                  {}
func (s *settling) Settle(context.Context)              { s.settled.Store(true) }
func (s *settling) DrillUp(string) string               { return view.DetailAccounts }
func (s *settling) Details() []view.DetailInfo {
	return []view.DetailInfo{{ID: view.DetailAccounts, Title: view.DevTitle}}
}
func (s *settling) Drills(id string) bool {
	_, _, ok := view.ParseDevDayID(id)
	return ok
}
func (s *settling) PollDetail(_ context.Context, id string) (view.Detail, error) {
	if _, date, ok := view.ParseDevDayID(id); ok {
		return view.DevDayPage(s.reading(), day, date), nil
	}
	r := s.reading()
	parts := []view.Detail{view.DevPart(r, day, -1)}
	if recent := view.DevRecent([]view.DevReading{r}); !recent.Quiet() {
		parts = append(parts, recent, view.DevRepos([]view.DevReading{r}))
	}
	return view.Development(nil, parts), nil
}

var _ panel.Settler = (*settling)(nil)

// Spec 102 R14, AC12. `detail usage/accounts/github/<date>` prints the day,
// having waited for the reading in flight rather than printing "reading…".
func TestDetailPrintsADay(t *testing.T) {
	var out bytes.Buffer
	src := &settling{}
	err := cli.Detail(t.Context(), &out, []panel.Source{src}, "usage/"+view.DevDayID("github", "2026-03-14"), false, time.Millisecond)
	require.NoError(t, err)
	assert.Contains(t, out.String(), "#9 a widget")
	assert.Contains(t, out.String(), "merged")
	assert.NotContains(t, out.String(), "reading…")
}

// Spec 107 R1, threat model B6. The overview's Recent and Repositories name
// what a person worked on: `detail` prints them, and `readings`, whose output
// is what gets pasted into a bug report, does not.
func TestRecentIsInDetailAndNotInReadings(t *testing.T) {
	src := &settling{}
	src.Settle(t.Context())

	var shown bytes.Buffer
	require.NoError(t, cli.Detail(t.Context(), &shown, []panel.Source{src}, "usage", false, time.Millisecond))
	assert.Contains(t, shown.String(), "Recent")
	assert.Contains(t, shown.String(), "#9 a widget")

	var read bytes.Buffer
	require.NoError(t, cli.Readings(&read, []panel.Source{src}, func(panel.Source) error { return nil }))
	assert.NotContains(t, read.String(), "a widget", "readings printed a title")
	assert.NotContains(t, read.String(), "widget", "readings printed a repository")
	assert.Contains(t, read.String(), "contributions", "readings lost the forge's counts")
}
