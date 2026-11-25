package api

import (
	"errors"
	"fmt"
)

// ErrNotHere is a system the service does not run on yet: Windows has a
// per-user service and macOS a launchd agent, each in its own phase of spec
// 113.
var ErrNotHere = errors.New("the service does not run on this system yet")

// checkPath refuses a socket's path longer than the system takes, rather
// than letting it be cut short (R3).
func checkPath(path string, limit int) error {
	if len(path) >= limit {
		return fmt.Errorf("the socket's path is %d bytes and this system takes fewer than %d: %s", len(path), limit, path)
	}
	return nil
}
