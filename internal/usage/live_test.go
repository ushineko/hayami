package usage_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/hayami/internal/usage"
)

// The canary.
//
// This repository shares a cache with code it does not own, and the day that
// code changes the format is the day the usage section goes quietly blank.
// This test reads the real files when a machine has them and skips when it
// does not, so the change is reported by a test run rather than discovered by
// a person wondering why a pane is empty.
//
// It reads and never writes: those files belong to programs that are running.
func TestTheRealCacheFilesOnThisMachineStillParse(t *testing.T) {
	dir, err := usage.Dir()
	require.NoError(t, err)

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Skip("no shared cache on this machine; nothing to check the format against")
	}

	checked := 0
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		body, err := os.ReadFile(filepath.Join(dir, e.Name()))
		require.NoError(t, err)

		var got usage.Entry
		require.NoError(t, json.Unmarshal(body, &got),
			"%s no longer parses as an entry; the format has changed", e.Name())

		// next_attempt_at is the gate and is the one field this package
		// cannot do without. A file with none is a file written by something
		// that is not keeping the protocol.
		assert.NotZero(t, got.NextAttemptAt,
			"%s carries no gate; the protocol has changed", e.Name())

		// data is opaque here, but it has to be an object: everything that
		// reads it treats it as one.
		if len(got.Data) > 0 {
			var obj map[string]any
			assert.NoError(t, json.Unmarshal(got.Data, &obj),
				"%s holds a payload that is not an object", e.Name())
		}
		checked++
	}

	if checked == 0 {
		t.Skip("the shared cache directory holds no entries yet")
	}
	t.Logf("checked %d cache files written by the programs this one is joining", checked)
}
