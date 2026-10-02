/*
Package testenv points the per-user directories a test could reach at
directories the test owns, under every name the platforms read them by.

A test that set only the Unix names was not sandboxed on Windows, where Go's
os package reads others: os.UserHomeDir reads USERPROFILE, os.UserConfigDir
reads APPDATA, and the usage cache prefers LOCALAPPDATA as the widget it
shares with does. A test that took the credential store away with HOME
alone still found the real one there, and one that pointed the cache at a
temporary directory read the real cache instead.
*/
package testenv

import "testing"

// Home makes dir the user's home directory.
func Home(t testing.TB, dir string) {
	t.Helper()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)
}

// Cache makes dir the user's cache directory. LOCALAPPDATA is emptied rather
// than pointed at dir, so every platform lays the cache out the XDG way and a
// test can write a fixture where it expects to find it.
func Cache(t testing.TB, dir string) {
	t.Helper()
	t.Setenv("XDG_CACHE_HOME", dir)
	t.Setenv("LOCALAPPDATA", "")
}

// Config makes dir the user's configuration directory.
func Config(t testing.TB, dir string) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("APPDATA", dir)
}
