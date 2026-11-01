package codex_test

import (
	"encoding/json"
	"testing"

	"github.com/ushineko/hayami/internal/codex"
)

// FuzzNormalise: the Codex app-server's reply, reshaped into what the cache
// holds (spec 074).
func FuzzNormalise(f *testing.F) {
	f.Add([]byte(reply))
	f.Add([]byte(`{}`))
	f.Add([]byte(`{"rateLimitsByLimitId":{"codex":null}}`))
	f.Fuzz(func(_ *testing.T, data []byte) {
		_, _ = codex.Normalise(json.RawMessage(data))
	})
}
