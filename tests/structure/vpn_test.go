package structure

import (
	"go/ast"
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

// The VPNs are read over the system bus, and their checks are made natively
// (spec 085 R2). vpn-toggle ran a program for each of these, and a panel in
// every terminal doing the same is what this holds off.
func TestNoProgramIsRunToReadAVPN(t *testing.T) {
	banned := map[string]bool{"nmcli": true, "openvpn3": true, "ping": true, "getent": true}

	dir := filepath.Join("..", "..", "internal", "core")
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)

	checked := 0
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		f, err := parser.ParseFile(token.NewFileSet(), filepath.Join(dir, e.Name()), nil, 0)
		require.NoError(t, err)
		checked++
		ast.Inspect(f, func(n ast.Node) bool {
			if lit, ok := n.(*ast.BasicLit); ok && lit.Kind == token.STRING {
				if s, err := strconv.Unquote(lit.Value); err == nil {
					assert.False(t, banned[s], "%s names the program %q", e.Name(), s)
				}
			}
			return true
		})
	}
	require.Positive(t, checked, "the walk found no sources")
}
