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

// bannedVPNPrograms are what vpn-toggle ran. A panel in every terminal running one
// of these per check is what spec 085 R2 holds off.
var bannedVPNPrograms = map[string]bool{"nmcli": true, "openvpn3": true, "ping": true, "getent": true}

// programsRun are the banned programs a file's exec calls name. "ping" as a word is
// fine, and is the name of a check in core's table: what is banned is running it.
func programsRun(f *ast.File) []string {
	var out []string
	ast.Inspect(f, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || (sel.Sel.Name != "Command" && sel.Sel.Name != "CommandContext") {
			return true
		}
		for _, arg := range call.Args {
			lit, ok := arg.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				continue
			}
			if s, err := strconv.Unquote(lit.Value); err == nil && bannedVPNPrograms[s] {
				out = append(out, s)
			}
		}
		return true
	})
	return out
}

func TestNoProgramIsRunToReadAVPN(t *testing.T) {
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
		assert.Empty(t, programsRun(f), e.Name())
	}
	require.Positive(t, checked, "the walk found no sources")
}

// The check itself has to be able to fail, on both ways of running a program, and
// not on the word in a table.
func TestTheProgramCheckFindsAnExec(t *testing.T) {
	parse := func(src string) *ast.File {
		f, err := parser.ParseFile(token.NewFileSet(), "x.go", "package x\n"+src, 0)
		require.NoError(t, err)
		return f
	}

	assert.Equal(t, []string{"nmcli"}, programsRun(parse(`func a() { exec.Command("nmcli", "c") }`)))
	assert.Equal(t, []string{"ping"}, programsRun(parse(`func a() { exec.CommandContext(ctx, "ping", "h") }`)))
	assert.Empty(t, programsRun(parse(`var t = map[string]int{"ping": 1}`)))
	assert.Empty(t, programsRun(parse(`func a() { exec.Command("nvidia-smi") }`)))
}
