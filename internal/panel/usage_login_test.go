package panel_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/hayami/internal/panel"
	"github.com/ushineko/hayami/internal/testenv"
	"github.com/ushineko/hayami/internal/view"
)

/*
Spec 089: the machine this came from. A Claude store whose access token has
expired and whose refresh token the endpoint refuses, over a cache holding a
reading two days old: a five-hour window that reset since, and a week that
has not.

Driven through the real gather, with the home, the cache and the network taken
away: the decision is made from a credential file, a cache file and a reply,
and a stub of any of them would be asserting the stub.
*/
type deadLogin struct {
	cache string
	now   time.Time
	asked atomic.Int32
}

func newDeadLogin(t *testing.T, reply http.HandlerFunc) *deadLogin {
	t.Helper()
	d := &deadLogin{cache: t.TempDir(), now: time.Now()}
	testenv.Cache(t, d.cache)
	home := t.TempDir()
	testenv.Home(t, home)
	t.Setenv("PATH", t.TempDir()) // no codex

	// A refresh token of this test's own: the process remembers refused ones.
	store := filepath.Join(home, ".claude")
	require.NoError(t, os.MkdirAll(store, 0o700))
	creds := fmt.Sprintf(`{"claudeAiOauth":{"accessToken":"not-a-real-access-token",`+
		`"refreshToken":"refresh-%s","expiresAt":%d,"subscriptionType":"max"}}`,
		t.Name(), d.now.Add(-48*time.Hour).UnixMilli())
	require.NoError(t, os.WriteFile(filepath.Join(store, ".credentials.json"), []byte(creds), 0o600))

	d.write(t, d.now.Add(-48*time.Hour), 1)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		d.asked.Add(1)
		reply(w, r)
	}))
	t.Cleanup(srv.Close)
	to, err := url.Parse(srv.URL)
	require.NoError(t, err)
	panel.SetHTTPClient(t, &http.Client{Transport: redirectTo{to: to}})
	return d
}

// write puts a reading fetched at fetched into the cache, behind a gate at
// gate seconds since the epoch.
func (d *deadLogin) write(t *testing.T, fetched time.Time, gate float64) {
	t.Helper()
	dir := filepath.Join(d.cache, "claude-usage-widget")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	at := float64(fetched.UnixNano()) / float64(time.Second)
	session := fetched.Add(3 * time.Hour).UTC().Format(time.RFC3339)
	week := d.now.Add(72 * time.Hour).UTC().Format(time.RFC3339)
	body := fmt.Sprintf(`{"next_attempt_at":%f,"fetched_at":%f,"data":{`+
		`"five_hour":{"utilization":1.0,"resets_at":%q},`+
		`"seven_day":{"utilization":31.0,"resets_at":%q},`+
		`"limits":[{"kind":"session","percent":1,"resets_at":%q},`+
		`{"kind":"weekly_all","percent":31,"resets_at":%q}]}}`,
		gate, at, session, week, session, week)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "usage-max.json"), []byte(body), 0o600))
}

// redirectTo sends every request to a test's server.
type redirectTo struct{ to *url.URL }

func (r redirectTo) RoundTrip(req *http.Request) (*http.Response, error) {
	out := req.Clone(req.Context())
	out.URL.Scheme, out.URL.Host = r.to.Scheme, r.to.Host
	return http.DefaultTransport.RoundTrip(out) //nolint:wrapcheck // a transport returns what the base returned
}

func invalidGrant(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusBadRequest)
	_, _ = w.Write([]byte(`{"error":"invalid_grant"}`))
}

// lines are what a card or a pane draws below the meters, as plain strings.
func lines(s view.Section) []string {
	out := []string{}
	for _, r := range s.Lines() {
		out = append(out, strings.TrimSpace(r.Label+" "+r.Value))
	}
	return out
}

func TestADeadLoginIsALineBesideTheFiguresItLeft(t *testing.T) {
	d := newDeadLogin(t, invalidGrant)
	u := panel.NewUsage()
	panel.SetUsageNow(u, func() time.Time { return d.now })

	drawn, err := u.Poll(t.Context())
	require.NoError(t, err)
	assert.True(t, drawn, "the week still stands and is drawn")

	sec := u.Section()
	r := find(t, sec, "needs a login")
	assert.Equal(t, "CC max", r.Label)
	assert.Equal(t, view.Warn, r.Status)
	assert.False(t, r.Aside, "a login the server refused is a line, not a hover note")
	assert.Contains(t, lines(sec), "CC max needs a login")

	require.Len(t, sec.Meters, 1)
	assert.Equal(t, "7d", sec.Meters[0].Window,
		"the five-hour window reset two days ago and its 1 % is not drawn")

	// The polls between fetches, with the gate closed, say it too: a line that
	// came and went every thirty seconds would move the panel each time.
	for range 3 {
		_, err := u.Poll(t.Context())
		require.NoError(t, err)
		find(t, u.Section(), "needs a login")
	}
	assert.Equal(t, int32(1), d.asked.Load(), "the gate was not honoured")
}

// Another program fetched after the failure, as Claude Code's own widget would
// once the person signed in: the line goes.
func TestTheLineGoesWhenANewerReadingArrives(t *testing.T) {
	d := newDeadLogin(t, invalidGrant)
	u := panel.NewUsage()
	panel.SetUsageNow(u, func() time.Time { return d.now })
	_, err := u.Poll(t.Context())
	require.NoError(t, err)
	find(t, u.Section(), "needs a login")

	gate := float64(d.now.Add(2*time.Minute).UnixNano()) / float64(time.Second)
	d.write(t, d.now.Add(time.Second), gate)
	panel.SetUsageNow(u, func() time.Time { return d.now.Add(2 * time.Second) })
	_, err = u.Poll(t.Context())
	require.NoError(t, err)

	sec := u.Section()
	assert.NotContains(t, reasonTexts(sec), "needs a login")
	require.Len(t, sec.Meters, 1)
	assert.Equal(t, "5h", sec.Meters[0].Window, "the new reading's five hours are drawn again")
}

// A second viewer, a pane beside the window or a panel started again, never
// fetched while the gate was closed. It says "needs a login" too, from the
// cache: the desk's real window lacked the line when doctor had fetched first.
func TestAViewerThatDidNotFetchSaysItToo(t *testing.T) {
	d := newDeadLogin(t, invalidGrant)
	first := panel.NewUsage()
	panel.SetUsageNow(first, func() time.Time { return d.now })
	_, err := first.Poll(t.Context())
	require.NoError(t, err)

	second := panel.NewUsage()
	panel.SetUsageNow(second, func() time.Time { return d.now.Add(10 * time.Second) })
	_, err = second.Poll(t.Context())
	require.NoError(t, err)
	find(t, second.Section(), "needs a login")
	assert.Equal(t, int32(1), d.asked.Load(), "the second viewer fetched through a closed gate")
}

func serverError(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusInternalServerError)
}

// Review of spec 089, path 1: a cache file that says it was read in the future
// is not a newer reading. Taken as one, it cleared the failure on every poll
// but the fetch's.
func TestAReadingFromTheFutureDoesNotClearTheFailure(t *testing.T) {
	d := newDeadLogin(t, serverError)
	d.write(t, d.now.Add(100*365*24*time.Hour), 1)
	u := panel.NewUsage()
	panel.SetUsageNow(u, func() time.Time { return d.now })
	for range 3 {
		_, err := u.Poll(t.Context())
		require.NoError(t, err)
		find(t, u.Section(), "not refreshed")
	}
}

// A failure that is not a login, behind figures, is a hover note and no line:
// a minute without a network must not move the panel. It stays across the
// polls between fetches, as the fetch reported it.
func TestAnotherFailureBehindFiguresIsAnAside(t *testing.T) {
	d := newDeadLogin(t, serverError)
	u := panel.NewUsage()
	panel.SetUsageNow(u, func() time.Time { return d.now })
	for range 3 {
		_, err := u.Poll(t.Context())
		require.NoError(t, err)

		sec := u.Section()
		r := find(t, sec, "not refreshed")
		assert.True(t, r.Aside)
		assert.Equal(t, view.Warn, r.Status)
		assert.Contains(t, r.Detail, "500")
		for _, l := range lines(sec) {
			assert.NotContains(t, l, "not refreshed")
		}
	}
	assert.Equal(t, int32(1), d.asked.Load())
}

// An account whose every window started over since its reading says so,
// rather than drawing nothing or the old figures.
func TestAnAccountWithEveryWindowTurnedOverSaysWhy(t *testing.T) {
	d := newDeadLogin(t, invalidGrant)
	d.now = d.now.Add(5 * 24 * time.Hour) // the week has reset too
	u := panel.NewUsage()
	panel.SetUsageNow(u, func() time.Time { return d.now })
	_, err := u.Poll(t.Context())
	require.NoError(t, err)

	sec := u.Section()
	assert.Empty(t, sec.Meters)
	assert.Equal(t, []string{"needs a login"}, reasonTexts(sec)[:1],
		"a refused login is the reason, not a generic failure")
	assert.NotContains(t, reasonTexts(sec), "could not be read")
}

// The same, with no store to fetch with: nothing failed, and the reading is
// simply out of date.
func TestACacheOnlyAccountTurnedOverIsOutOfDate(t *testing.T) {
	d := newDeadLogin(t, invalidGrant)
	require.NoError(t, os.Remove(filepath.Join(os.Getenv("HOME"), ".claude", ".credentials.json")))
	d.now = d.now.Add(5 * 24 * time.Hour)
	u := panel.NewUsage()
	panel.SetUsageNow(u, func() time.Time { return d.now })
	_, err := u.Poll(t.Context())
	require.NoError(t, err)

	r := find(t, u.Section(), "out of date")
	assert.Equal(t, view.Info, r.Status)
	assert.Equal(t, int32(0), d.asked.Load())
}

// A window that reset a minute ago is kept until the next fetch: the margin is
// the stale threshold, so a window does not vanish between its reset and the
// reading that follows it.
func TestAWindowThatResetAMomentAgoIsKept(t *testing.T) {
	d := newDeadLogin(t, invalidGrant)
	fetched := d.now.Add(-3*time.Hour - time.Minute) // its five hours reset a minute ago
	d.write(t, fetched, 1)
	u := panel.NewUsage()
	panel.SetUsageNow(u, func() time.Time { return d.now })
	_, err := u.Poll(t.Context())
	require.NoError(t, err)

	sec := u.Section()
	require.Len(t, sec.Meters, 1)
	assert.Equal(t, "5h", sec.Meters[0].Window)
}

// The drawer drops what the card drops: its session limit reset two days ago.
func TestTheDrawerDropsAWindowThatTurnedOver(t *testing.T) {
	d := newDeadLogin(t, invalidGrant)
	u := panel.NewUsage()
	panel.SetUsageNow(u, func() time.Time { return d.now })

	got, err := u.PollDetail(t.Context(), "CC max")
	require.NoError(t, err)
	text := got.Text()
	assert.Contains(t, text, "Week")
	assert.NotContains(t, text, "Session", "the drawer drew a limit that has started over")
}

// A 429 from the token endpoint moves the shared gate by its Retry-After, as
// one from the usage endpoint does: every program reading the cache waits.
func TestATooManyRequestsFromTheTokenEndpointMovesTheGate(t *testing.T) {
	d := newDeadLogin(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Retry-After", "600")
		w.WriteHeader(http.StatusTooManyRequests)
	})
	u := panel.NewUsage()
	panel.SetUsageNow(u, func() time.Time { return d.now })
	_, err := u.Poll(t.Context())
	require.NoError(t, err)

	body, err := os.ReadFile(filepath.Join(d.cache, "claude-usage-widget", "usage-max.json"))
	require.NoError(t, err)
	var entry struct {
		NextAttemptAt float64 `json:"next_attempt_at"`
	}
	require.NoError(t, json.Unmarshal(body, &entry))
	gate := time.Unix(0, int64(entry.NextAttemptAt*float64(time.Second)))
	assert.WithinDuration(t, d.now.Add(10*time.Minute), gate, time.Second)
	assert.NotContains(t, reasonTexts(u.Section()), "needs a login", "being asked to wait is not a dead login")
}
