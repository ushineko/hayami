package nethelper

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// #244. Program Files is the known folder's: a %ProgramFiles% the user's
// environment sets is not where the service's program goes.
func TestTheHelpersDirectoryIsNotTheEnvironments(t *testing.T) {
	elsewhere := t.TempDir()
	t.Setenv("ProgramFiles", elsewhere)
	dir := Dir()
	assert.False(t, strings.HasPrefix(strings.ToLower(dir), strings.ToLower(elsewhere)), "%s follows the environment", dir)
	assert.Equal(t, "hayami", filepath.Base(dir))
}

// #244. While the install holds its own program open, the file cannot be
// renamed aside or replaced: what it copies is what it opened.
func TestAHeldProgramCannotBeSwapped(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hayami-tui.exe")
	require.NoError(t, os.WriteFile(path, []byte("the program"), 0o600))

	held, err := openLocked(path)
	require.NoError(t, err)
	assert.Error(t, os.Rename(path, filepath.Join(dir, "aside.exe")), "the held file was renamed")
	assert.Error(t, os.WriteFile(path, []byte("another program"), 0o600), "the held file was rewritten")
	require.NoError(t, held.Close())

	require.NoError(t, os.Rename(path, filepath.Join(dir, "aside.exe")), "let go, it is the user's again")
}
