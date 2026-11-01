package panel

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/hayami/internal/claude"
	"github.com/ushineko/hayami/internal/view"
)

// Spec 076. A credential store that cannot be read gives an error naming
// its path, which is under the user's home; the reason it becomes, which
// doctor prints and the readings JSON carries, writes that home as ~.
// The error is the filesystem's own, from a real file this account may not
// read.
func TestAReasonDoesNotNameTheUsersHome(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not deny a file by its mode")
	}
	if os.Geteuid() == 0 {
		t.Skip("root reads any file")
	}
	home := t.TempDir()
	was := userHome
	userHome = func() (string, error) { return home, nil }
	t.Cleanup(func() { userHome = was })

	store := filepath.Join(home, ".claude", ".credentials.json")
	require.NoError(t, os.MkdirAll(filepath.Dir(store), 0o700))
	require.NoError(t, os.WriteFile(store, []byte(`{}`), 0))
	_, err := claude.Read(claude.Store{Path: store})
	require.Error(t, err)
	require.Contains(t, err.Error(), home, "the filesystem's error names the file")

	r := reason(view.Reason{Label: "CC max"}, err)
	assert.NotContains(t, r.Detail, home)
	assert.Contains(t, r.Detail, "~/.claude/.credentials.json")
	assert.Contains(t, r.Detail, "permission denied")

	// A home of "/" is not one to replace.
	userHome = func() (string, error) { return "/", nil }
	assert.Equal(t, "open /etc/x: denied", reason(view.Reason{}, errors.New("open /etc/x: denied")).Detail)
}
