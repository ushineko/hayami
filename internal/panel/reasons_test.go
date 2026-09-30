package panel_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/hayami/internal/core"
	"github.com/ushineko/hayami/internal/panel"
	"github.com/ushineko/hayami/internal/view"
)

// reasonTexts is what a section says it could not read, as plain strings.
func reasonTexts(s view.Section) []string {
	out := make([]string, 0, len(s.Reasons))
	for _, r := range s.Reasons {
		out = append(out, r.Text)
	}
	return out
}

// find returns the reason with a given text.
func find(t *testing.T, s view.Section, text string) view.Reason {
	t.Helper()
	for _, r := range s.Reasons {
		if r.Text == text {
			return r
		}
	}
	require.FailNowf(t, "no such reason", "%q is not among %v", text, reasonTexts(s))
	return view.Reason{}
}

// A usage section with no accounts says so, rather than being absent.
func TestUsageWithNoAccountsSaysSo(t *testing.T) {
	u := panel.NewUsage()
	panel.SetUsageRead(u, func(context.Context) ([]view.UsageWindow, time.Time, []view.Reason, error) {
		return nil, time.Time{}, []view.Reason{{
			Text: "no Claude or Codex account", Status: view.Info,
		}}, nil
	})

	drawn, err := u.Poll(t.Context())

	require.NoError(t, err)
	assert.False(t, drawn)
	assert.Equal(t, []string{"no Claude or Codex account"}, reasonTexts(u.Section()))
}

// A gather that fails outright still produces a section that says something.
func TestUsageThatCannotBeGatheredSaysWhy(t *testing.T) {
	u := panel.NewUsage()
	panel.SetUsageRead(u, func(context.Context) ([]view.UsageWindow, time.Time, []view.Reason, error) {
		return nil, time.Time{}, nil, errors.New("listing the usage cache: permission denied")
	})

	_, err := u.Poll(t.Context())

	require.Error(t, err)
	sec := u.Section()
	require.Len(t, sec.Reasons, 1)
	assert.Equal(t, view.Warn, sec.Reasons[0].Status)
	assert.Contains(t, sec.Reasons[0].Detail, "permission denied")
}

/*
A bandwidth section with no interfaces chosen says so, and one whose interface
the kernel does not list names it.

A renamed interface after a hardware change is the ordinary way the second
happens, and the row it leaves behind is blanks of the right width -- correct
for the column, and silent about the name being wrong.
*/
func TestBandwidthSaysWhenThereIsNothingToWatch(t *testing.T) {
	b := panel.NewBandwidth(nil, fakeCounters)

	_, err := b.Poll(t.Context())

	require.NoError(t, err)
	assert.Equal(t, []string{"no interfaces chosen"}, reasonTexts(b.Section()))
}

func TestBandwidthNamesAnInterfaceTheKernelDoesNotHave(t *testing.T) {
	b := panel.NewBandwidth([]string{"eth0", "wlan9"}, fakeCounters)

	_, err := b.Poll(t.Context())

	require.NoError(t, err)
	sec := b.Section()
	require.Len(t, sec.Reasons, 1)
	assert.Equal(t, "wlan9", sec.Reasons[0].Label)
	assert.Equal(t, "not present", sec.Reasons[0].Text)
	assert.Equal(t, view.Info, sec.Reasons[0].Status)

	// And it takes that interface's row rather than sitting under it. A blank
	// row and a reason both about wlan9 is one line too many.
	for _, r := range sec.Rows {
		assert.NotEqual(t, "wlan9", r.Label, "the absent interface kept a row of blanks as well")
	}
}

func fakeCounters() (map[string]core.Counters, error) {
	return map[string]core.Counters{"eth0": {Rx: 1000, Tx: 500}}, nil
}

/*
An account waiting out a backoff with nothing cached says so, and says until
when.

The exact state the machine this spec came from was in: `usage-max.json` held
`"data": null` behind a gate thirty-five minutes out, and the section drew
nothing with no explanation anywhere. The gate is not shortened -- it is
written into a file three programs read -- so saying what is happening is the
whole of the remedy.

Driven through the real gather, with the cache and the home directory taken
away, because the decision is made from a cache file's contents and a stub
would be asserting the stub.
*/
func TestAUsageAccountWaitingOutABackoffSaysSo(t *testing.T) {
	cache := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cache)
	t.Setenv("HOME", t.TempDir()) // no credential store, so nothing fetches
	t.Setenv("PATH", t.TempDir()) // and no codex either

	// A gate an hour out, and nothing behind it.
	dir := filepath.Join(cache, "claude-usage-widget")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	gate := float64(time.Now().Add(time.Hour).UnixNano()) / float64(time.Second)
	body := fmt.Sprintf(`{"next_attempt_at":%f,"fetched_at":null,"data":null}`, gate)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "usage-max.json"), []byte(body), 0o600))

	u := panel.NewUsage()
	drawn, err := u.Poll(t.Context())

	require.NoError(t, err)
	assert.False(t, drawn, "there is no reading behind the gate")

	sec := u.Section()
	r := find(t, sec, "waiting to retry")
	assert.Equal(t, "max", r.Label)
	assert.Equal(t, view.Warn, r.Status, "a backoff a person may want to understand is marked")
	assert.Contains(t, r.Detail, "gate opens at",
		"a panel that is waiting should say until when")
}

// An account that has simply never fetched, with its gate open, is stated
// rather than marked: nothing has gone wrong yet.
func TestAnAccountThatHasNeverFetchedIsNotMarkedAsAFailure(t *testing.T) {
	cache := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cache)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("PATH", t.TempDir())

	dir := filepath.Join(cache, "claude-usage-widget")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "usage-max.json"),
		[]byte(`{"next_attempt_at":1,"fetched_at":null,"data":null}`), 0o600))

	u := panel.NewUsage()
	_, err := u.Poll(t.Context())

	require.NoError(t, err)
	r := find(t, u.Section(), "nothing fetched yet")
	assert.Equal(t, view.Info, r.Status)
}
