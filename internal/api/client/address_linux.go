package client

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// sunPath is the size of a Unix socket address's path on Linux, its NUL
// included.
const sunPath = 108

/*
Address is the service's address on this system, by the contract's rule
(spec 113 R3): $XDG_RUNTIME_DIR/hayami/api.sock. It is the service's own rule
written again, as every client writes it; a test holds the two together.
*/
func Address() (string, error) {
	dir := os.Getenv("XDG_RUNTIME_DIR")
	if dir == "" {
		return "", errors.New("XDG_RUNTIME_DIR is not set, so there is no address to find; name the socket")
	}
	path := filepath.Join(dir, "hayami", "api.sock")
	if len(path) >= sunPath {
		return "", fmt.Errorf("the socket's path is %d bytes and this system takes fewer than %d: %s", len(path), sunPath, path)
	}
	return path, nil
}
