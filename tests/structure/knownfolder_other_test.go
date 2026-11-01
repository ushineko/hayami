//go:build !windows

package structure_test

import "errors"

// windowsKnownProgramFiles has no answer off Windows; its one caller skips
// there first.
func windowsKnownProgramFiles() (string, error) {
	return "", errors.New("not Windows")
}
