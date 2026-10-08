package panel_test

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/ushineko/hayami/internal/panel"
	"github.com/ushineko/hayami/internal/testenv"
	"github.com/ushineko/hayami/internal/usage"
	"github.com/ushineko/hayami/internal/view"
)

// Invented payloads, one per provider, in the shapes internal/usage decodes.
const (
	pinClaude = `{"five_hour": {"utilization": 12.5, "resets_at": "2030-01-01T11:00:00+00:00"},
  "seven_day": {"utilization": 40.0, "resets_at": "2030-01-05T16:00:00+00:00"}}`
	pinCodex = `{"provider": "codex",
  "primary":   {"utilization": 25.0, "window_minutes": 300,   "resets_at": 1893495600},
  "secondary": {"utilization": 60.0, "window_minutes": 10080, "resets_at": 1893841200},
  "individual_limit": {"utilization": 33.0, "resets_at": 1893668400, "used": "400.5", "limit": "1200"}}`
	pinUnreadable = `{"five_hour": {"utilization": "a lot", "resets_at": 7}}`
)

// pinDesk is a desk the usage section reads: cache files, credential stores
// and whether a codex is on the PATH. Every gate is closed, so nothing is
// fetched and nothing reaches the network.
type pinDesk struct {
	name   string
	cache  map[usage.Account]string // account -> data ("" is null data)
	stores map[string]string        // profile -> plan
	codex  bool
}

func (d pinDesk) build(t *testing.T) {
	t.Helper()
	root := t.TempDir()
	testenv.Cache(t, filepath.Join(root, "cache"))
	testenv.Home(t, filepath.Join(root, "home"))
	profiles := filepath.Join(root, "profiles")
	t.Setenv("CLAUDE_USAGE_PROFILE_ROOT", profiles)

	closed := float64(time.Now().Add(time.Hour).Unix())
	fetched := float64(time.Now().Add(-time.Minute).Unix())
	for a, data := range d.cache {
		path, err := usage.Path(a.Name, a.Provider)
		require.NoError(t, err)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
		body := fmt.Sprintf(`{"next_attempt_at": %f, "fetched_at": %f, "data": %s}`, closed, fetched, data)
		if data == "" {
			body = fmt.Sprintf(`{"next_attempt_at": %f, "fetched_at": null, "data": null}`, closed)
		}
		require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
	}
	for name, plan := range d.stores {
		dir := filepath.Join(profiles, name)
		require.NoError(t, os.MkdirAll(dir, 0o700))
		creds := fmt.Sprintf(`{"claudeAiOauth": {"accessToken": "invented", "refreshToken": "invented",
  "expiresAt": %d, "subscriptionType": %q}}`, time.Now().Add(24*time.Hour).UnixMilli(), plan)
		require.NoError(t, os.WriteFile(filepath.Join(dir, ".credentials.json"), []byte(creds), 0o600))
	}
	bin := filepath.Join(root, "bin")
	require.NoError(t, os.MkdirAll(bin, 0o700))
	if d.codex {
		name := "codex"
		if runtime.GOOS == "windows" {
			name = "codex.exe"
		}
		require.NoError(t, os.WriteFile(filepath.Join(bin, name), []byte("#!/bin/sh\nexit 1\n"), 0o700)) //nolint:gosec // a stand-in the PATH finds, never run
	}
	t.Setenv("PATH", bin)
}

// hhmm is a clock time, which depends on the machine's zone.
var hhmm = regexp.MustCompile(`\b\d{2}:\d{2}\b`)

// goType is the Go type encoding/json names in a decode error, which differs
// between Go releases for the same input.
var goType = regexp.MustCompile(`Go struct field \w+\.`)

// maskUsage removes what depends on the machine or the toolchain from a detail.
func maskUsage(s string) string {
	return goType.ReplaceAllString(hhmm.ReplaceAllString(s, "HH:MM"), "Go struct field ")
}

// render is what the usage section says, as text: its windows and its reasons.
func render(s view.Section, windows []view.UsageWindow) string {
	var b strings.Builder
	for _, w := range windows {
		fmt.Fprintf(&b, "- window account=%q badge=%q name=%q fraction=%.4f resets=%s detail=%q used=%q limit=%q severity=%q\n",
			w.Account, w.Badge, w.Name, w.Fraction, w.ResetsAt.UTC().Format(time.RFC3339), w.Detail, w.Used, w.Limit, w.Severity)
	}
	for _, r := range s.Reasons {
		fmt.Fprintf(&b, "- reason label=%q text=%q status=%d detail=%q\n", r.Label, r.Text, r.Status, maskUsage(r.Detail))
	}
	return b.String()
}

/*
TestEveryUsageDeskIsPinned holds what the usage section says on each kind of
desk to a file written before the provider table (spec 045): accounts from the
cache and from credential stores, both providers, a codex on the PATH and not,
no account at all, a payload that does not decode, and a gate closed with
nothing behind it. The table must change none of it.
*/
func TestEveryUsageDeskIsPinned(t *testing.T) {
	claude := func(name string) usage.Account { return usage.Account{Provider: usage.ProviderClaude, Name: name} }
	codex := usage.Account{Provider: usage.ProviderCodex}
	desks := []pinDesk{
		{name: "nothing at all"},
		{name: "cached accounts, no store, no codex", cache: map[usage.Account]string{claude("work"): pinClaude, codex: pinCodex}},
		{name: "a codex on the path", cache: map[usage.Account]string{codex: pinCodex}, codex: true},
		{name: "stores with plans", cache: map[usage.Account]string{claude("max"): pinClaude, claude("work"): ""},
			stores: map[string]string{"max": "max", "work": "pro"}},
		{name: "the pre-profile file beside a profile", cache: map[usage.Account]string{claude(""): pinClaude, claude("max"): pinClaude}},
		{name: "a payload that does not decode", cache: map[usage.Account]string{claude("work"): pinUnreadable, codex: pinCodex}},
	}

	var got strings.Builder
	for _, d := range desks {
		t.Run(d.name, func(t *testing.T) {
			d.build(t)
			u := panel.NewUsage()
			_, err := u.Poll(t.Context())
			require.NoError(t, err)
			windows, _ := u.Data().([]view.UsageWindow)
			fmt.Fprintf(&got, "## %s\n%s", d.name, render(u.Section(), windows))
		})
	}

	golden := filepath.Join("testdata", "usage.golden")
	if os.Getenv("HAYAMI_UPDATE_GOLDEN") == "1" {
		require.NoError(t, os.WriteFile(golden, []byte(got.String()), 0o600))
	}
	want, err := os.ReadFile(golden)
	require.NoError(t, err)
	require.Equal(t, string(want), got.String())
}
