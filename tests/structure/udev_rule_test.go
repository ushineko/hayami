package structure_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

/*
packaging/60-sanshoku.rules is sanshoku's rule, copied: hayami reads its
devices through that module, and a vendor the module drives but the rule
leaves out reads as "not permitted" on Linux. The copy fell behind once -- the
AULA receiver (sanshoku v0.1.8) was missing until hayami's 0.9.x README pass
found it -- so the copy is held, below its own header, to the rule in the
sanshoku version go.mod requires.
*/
func TestTheUdevRuleIsSanshokusRule(t *testing.T) {
	root := repoRoot(t)
	cmd := exec.CommandContext(t.Context(), "go", "list", "-m", "-f", "{{.Dir}}", "github.com/ushineko/sanshoku")
	cmd.Dir = root
	out, err := cmd.Output()
	require.NoError(t, err, "finding the sanshoku module")
	theirs, err := os.ReadFile(filepath.Join(strings.TrimSpace(string(out)), "packaging", "60-sanshoku.rules"))
	require.NoError(t, err)
	ours, err := os.ReadFile(filepath.Join(root, "packaging", "60-sanshoku.rules"))
	require.NoError(t, err)

	normal := func(b []byte) string { return strings.ReplaceAll(string(b), "\r\n", "\n") }
	require.True(t, strings.HasSuffix(normal(ours), normal(theirs)),
		"packaging/60-sanshoku.rules is not sanshoku's rule below its header: copy it from the version go.mod requires")
}
