package installer

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"io"
	"os"

	"github.com/ushineko/fynstall/extension"

	"github.com/ushineko/hayami/internal/api/unit"
	"github.com/ushineko/hayami/internal/nethelper"
)

/*
setUpAPIHere registers the service, or puts the program just installed in
place of the copy it runs, with the installed hayami-tui's `api-service
install` as an administrator: one UAC prompt, a --yes upgrade's included
(decided with the operator). The copy is in Program Files because every
account that signs in runs it (R6). A copy that is already this program asks
for nothing.
*/
func setUpAPIHere(ctx context.Context, c *extension.Context) error {
	program := helperProgram(c)
	if unit.Installed() && sameFile(program, unit.Program()) {
		return nil
	}
	if err := nethelper.RunElevated(ctx, program, "api-service", "install"); err != nil {
		return err //nolint:wrapcheck // says what Windows answered
	}
	if !unit.Installed() || !sameFile(program, unit.Program()) {
		return errors.New("the elevated step ended and the service is not registered on this version") //nolint:err113 // said plainly
	}
	return nil
}

// removeAPIHere removes it with `api-service remove`, as an administrator.
func removeAPIHere(ctx context.Context, c *extension.Context) error {
	if err := nethelper.RunElevated(ctx, helperProgram(c), "api-service", "remove"); err != nil {
		return err //nolint:wrapcheck // says what Windows answered
	}
	if unit.Installed() {
		return errors.New("the elevated step ended and the service is still registered") //nolint:err113 // said plainly
	}
	return nil
}

// sameFile reports whether the files at a and b hold the same bytes.
func sameFile(a, b string) bool {
	ha, ok := digest(a)
	if !ok {
		return false
	}
	hb, ok := digest(b)
	return ok && bytes.Equal(ha, hb)
}

func digest(path string) ([]byte, bool) {
	f, err := os.Open(path) //nolint:gosec // the install's own program, or the service's copy
	if err != nil {
		return nil, false
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return nil, false
	}
	return h.Sum(nil), true
}
