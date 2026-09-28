package desktop

import (
	"fmt"
	"os"
)

// writeScript puts a KWin script somewhere KWin can read it.
//
// KWin's loadScript takes a **path**, not the script's text, so the script has
// to exist as a file for as long as the call takes. It is written with the
// user's own permissions in the user's own temporary directory, because it is
// loaded into the compositor and a file anybody could rewrite between the
// write and the load would be a file anybody could run there.
func writeScript(body string) (string, error) {
	f, err := os.CreateTemp("", "hayami-opacity-*.js")
	if err != nil {
		return "", fmt.Errorf("writing the opacity script: %w", err)
	}
	defer func() { _ = f.Close() }()

	if err := f.Chmod(0o600); err != nil {
		remove(f.Name())
		return "", fmt.Errorf("securing the opacity script: %w", err)
	}
	if _, err := f.WriteString(body); err != nil {
		remove(f.Name())
		return "", fmt.Errorf("writing the opacity script: %w", err)
	}
	return f.Name(), nil
}

// remove takes the script away again. A failure to clean up is not worth
// reporting to a caller who asked about opacity.
func remove(path string) { _ = os.Remove(path) }
