package client_test

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

/*
R29. The reference client shows what any client needs: the contract types,
the standard library, and golang.org/x/sys for the system's own calls.
Nothing of the service's -- no panel, view or core -- so nothing a client in
another language could not have. The contract types are held to the same.
*/
func TestTheClientImportsOnlyTheContract(t *testing.T) {
	for _, dir := range []string{".", "../contract"} {
		files, err := filepath.Glob(filepath.Join(dir, "*.go"))
		require.NoError(t, err)
		require.NotEmpty(t, files)
		for _, f := range files {
			if strings.HasSuffix(f, "_test.go") {
				continue
			}
			src, err := os.ReadFile(f)
			require.NoError(t, err)
			parsed, err := parser.ParseFile(token.NewFileSet(), f, src, parser.ImportsOnly)
			require.NoError(t, err)
			for _, imp := range parsed.Imports {
				path, _ := strconv.Unquote(imp.Path.Value)
				first, _, _ := strings.Cut(path, "/")
				allowed := !strings.Contains(first, ".") ||
					path == "github.com/ushineko/hayami/internal/api/contract" ||
					strings.HasPrefix(path, "golang.org/x/sys/")
				assert.True(t, allowed, "%s imports %s", f, path)
			}
		}
	}
}
