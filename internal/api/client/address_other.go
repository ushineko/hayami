//go:build !linux

package client

import "errors"

// Address has no answer here yet: the service on Windows and macOS comes in
// its own phases of spec 113, and the client's address with it.
func Address() (string, error) {
	return "", errors.New("the service does not run on this system yet; name the socket")
}
