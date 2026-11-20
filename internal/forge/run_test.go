//go:build unix

package forge

import (
	"os"
	"path/filepath"
	"testing"
)

// Spec 102 review, F2. glab answers `config get token` from GITLAB_TOKEN and
// its kin for any host, so they are not in its environment: what it gives
// for a host is the key it keeps for that host.
func TestGlabIsNotGivenTheEnvironmentsToken(t *testing.T) {
	dir := t.TempDir()
	script := "#!/bin/sh\nprintf '%s%s%s' \"$GITLAB_TOKEN\" \"$GLAB_TOKEN\" \"$OAUTH_TOKEN\"\n"
	if err := os.WriteFile(filepath.Join(dir, "glab"), []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	t.Setenv("GITLAB_TOKEN", "from-the-environment")
	t.Setenv("GLAB_TOKEN", "from-the-environment")
	t.Setenv("OAUTH_TOKEN", "from-the-environment")
	out, err := run(t.Context(), "glab", "config", "get", "token", "--host", "gitlab.example.com")
	if err != nil {
		t.Fatal(err)
	}
	if out != "" {
		t.Fatalf("glab was given %q from the environment", out)
	}
}
