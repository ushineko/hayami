package structure_test

import "golang.org/x/sys/windows"

// windowsKnownProgramFiles is Program Files as the shell knows it, which the
// user's environment does not change.
func windowsKnownProgramFiles() (string, error) {
	return windows.KnownFolderPath(windows.FOLDERID_ProgramFiles, 0) //nolint:wrapcheck // a test's helper
}
