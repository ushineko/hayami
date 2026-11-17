package core

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTheUptimeIsTheFirstFieldInSeconds(t *testing.T) {
	path := filepath.Join(t.TempDir(), "uptime")
	require.NoError(t, os.WriteFile(path, []byte("350735.47 234388.90\n"), 0o600))

	d, err := ProcUptime(path)(context.Background())

	require.NoError(t, err)
	assert.Equal(t, 350735470*time.Millisecond, d)
}

func TestAnUptimeThatIsNotANumberIsAnError(t *testing.T) {
	for _, s := range []string{"", "\n", "abc 1", "-5 1", "NaN 1", "+Inf 1", "1e300 1"} {
		_, err := ParseUptime(s)
		assert.Error(t, err, "%q", s)
	}
}

func TestNoUptimeFileIsAnError(t *testing.T) {
	_, err := ProcUptime(filepath.Join(t.TempDir(), "uptime"))(context.Background())
	assert.Error(t, err)
}

func FuzzParseUptime(f *testing.F) {
	f.Add("350735.47 234388.90\n")
	f.Fuzz(func(t *testing.T, s string) {
		if d, err := ParseUptime(s); err == nil && d < 0 {
			t.Fatalf("%q: %v", s, d)
		}
	})
}
