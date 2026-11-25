package client

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// sunPath is the size of a Unix socket address's path on macOS, its NUL
// included.
const sunPath = 104

/*
Address is the service's address on this system, by the contract's rule
(spec 113 R3): $TMPDIR/hayami/api.sock, $TMPDIR being the per-user directory
under /var/folders; where the variable is unset, the system's answer for this
account (`getconf DARWIN_USER_TEMP_DIR`). It is the service's own rule written
again, as every client writes it; a test holds the two together.
*/
func Address() (string, error) {
	dir := os.Getenv("TMPDIR")
	if dir == "" {
		out, err := exec.Command("/usr/bin/getconf", "DARWIN_USER_TEMP_DIR").Output() //nolint:gosec,noctx // a fixed system program
		if err != nil {
			return "", fmt.Errorf("TMPDIR is not set and the system did not say this account's temporary directory: %w", err)
		}
		dir = strings.TrimSpace(string(out))
	}
	if dir == "" {
		return "", errors.New("TMPDIR is not set, so there is no address to find; name the socket")
	}
	path := filepath.Join(dir, "hayami", "api.sock")
	if len(path) >= sunPath {
		return "", fmt.Errorf("the socket's path is %d bytes and this system takes fewer than %d: %s", len(path), sunPath, path)
	}
	return path, nil
}
