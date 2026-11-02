package usage_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ushineko/hayami/internal/usage"
)

// FuzzClaudePayload: a usage reply from Anthropic's endpoint, or from the
// cache another program wrote, decoded as windows and as the breakdown
// (spec 074).
func FuzzClaudePayload(f *testing.F) {
	for _, s := range []string{claudeFull, claudeWindows, claudeSpend, `{}`, `null`} {
		f.Add([]byte(s))
	}
	now := time.Unix(1790000000, 0)
	f.Fuzz(func(_ *testing.T, data []byte) {
		_, _ = usage.Claude(now, json.RawMessage(data))
		_, _ = usage.ClaudeBreakdown(now, json.RawMessage(data))
	})
}

// FuzzCodexPayload: the same for Codex.
func FuzzCodexPayload(f *testing.F) {
	for _, s := range []string{codexFull, codexPayload, `{}`, `null`} {
		f.Add([]byte(s))
	}
	now := time.Unix(1790000000, 0)
	f.Fuzz(func(_ *testing.T, data []byte) {
		_, _ = usage.Codex(json.RawMessage(data))
		_, _ = usage.CodexBreakdown(now, json.RawMessage(data))
	})
}

// FuzzCacheFile: the cache file, which other programs write, read from a
// real file.
func FuzzCacheFile(f *testing.F) {
	f.Add([]byte(`{"next_attempt_at":1,"fetched_at":1,"data":{"five_hour":{"utilization":1}}}`))
	f.Add([]byte(`{"next_attempt_at":1e300,"fetched_at":null,"data":null}`))
	f.Add([]byte(`not json`))
	dir := tempCache(f)
	f.Fuzz(func(t *testing.T, data []byte) {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "usage-fuzz.json"), data, 0o600); err != nil {
			t.Fatal(err)
		}
		e, _ := usage.Read("fuzz", usage.ProviderClaude)
		if e != nil {
			_, _ = e.Fetched()
			_ = e.NextAttempt()
		}
	})
}
