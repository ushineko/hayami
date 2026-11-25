//go:build !windows

package installer

import (
	"context"
	"path/filepath"

	"github.com/ushineko/fynstall/extension"

	"github.com/ushineko/hayami/internal/api/unit"
	"github.com/ushineko/hayami/internal/nethelper"
)

// setUpAPIHere installs the service from the program just installed, or
// restarts it on that program, as the person: no administrator.
func setUpAPIHere(ctx context.Context, c *extension.Context) error {
	return unit.Install(ctx, filepath.Join(c.Root(), "bin", nethelper.ProgramName)) //nolint:wrapcheck // says which step
}

// removeAPIHere removes it, as the person.
func removeAPIHere(ctx context.Context, _ *extension.Context) error {
	return unit.Remove(ctx) //nolint:wrapcheck // says which step
}
