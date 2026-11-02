package structure_test

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v3"
)

var (
	pinnedByCommit = regexp.MustCompile(`^[^@\s]+@[0-9a-f]{40}$`)
	pinnedByDigest = regexp.MustCompile(`^docker://[^@\s]+@sha256:[0-9a-f]{64}$`)
)

// unpinnedActions says which `uses:` values in a workflow can change without
// the workflow changing. It reads the YAML rather than its lines, so a comment
// says nothing and an anchor, a flow map or a quoted key is what it means.
func unpinnedActions(src []byte) ([]string, error) {
	var doc any
	if err := yaml.Unmarshal(src, &doc); err != nil {
		return nil, err
	}
	var bad []string
	var walk func(any)
	walk = func(v any) {
		switch v := v.(type) {
		case map[string]any:
			for key, value := range v {
				if use, ok := value.(string); ok && key == "uses" {
					use = strings.TrimSpace(use)
					if !strings.HasPrefix(use, "./") && !pinnedByCommit.MatchString(use) && !pinnedByDigest.MatchString(use) {
						bad = append(bad, use)
					}
					continue
				}
				walk(value)
			}
		case []any:
			for _, item := range v {
				walk(item)
			}
		}
	}
	walk(doc)
	return bad, nil
}

func TestEveryActionIsPinned(t *testing.T) {
	root := filepath.Join("..", "..", ".github")
	checked := 0
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		ext := filepath.Ext(path)
		inWorkflows := filepath.Base(filepath.Dir(path)) == "workflows"
		isAction := strings.TrimSuffix(d.Name(), ext) == "action"
		if (ext != ".yml" && ext != ".yaml") || (!inWorkflows && !isAction) {
			return nil
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		bad, err := unpinnedActions(src)
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		assert.Empty(t, bad, "%s: pin these by full commit SHA, with the version in a comment", path)
		checked++
		return nil
	})
	require.NoError(t, err)
	assert.GreaterOrEqual(t, checked, 2, "the workflows were not found")
}

func TestAnUnpinnedActionIsFound(t *testing.T) {
	sha := strings.Repeat("0123456789", 4)
	digest := strings.Repeat("0123456789abcdef", 4)

	pinned := map[string]string{
		"a commit":           "- uses: a/b@" + sha + " # v1",
		"a quoted commit":    "- uses: 'a/b@" + sha + "'",
		"a local action":     "- uses: ./.github/actions/x",
		"a reusable commit":  "- uses: a/b/.github/workflows/c.yml@" + sha,
		"a container digest": "- uses: docker://a/b@sha256:" + digest,
	}
	for name, src := range pinned {
		t.Run(name, func(t *testing.T) {
			bad, err := unpinnedActions([]byte(src))
			require.NoError(t, err)
			assert.Empty(t, bad)
		})
	}

	unpinned := map[string]string{
		"a tag":                 "- uses: a/b@v1",
		"a commit in a comment": "- uses: a/b@main # @" + sha,
		"local in a comment":    "- uses: a/b@main # uses: ./x",
		"a flow map":            "- { uses: a/b@main }",
		"a quoted key":          "- \"uses\": a/b@main",
		"a space before":        "- uses : a/b@main",
		"a block scalar":        "- uses: >-\n    a/b@main",
		"the next line":         "- uses:\n    a/b@main",
		"an anchor":             "x: &a a/b@main\ny:\n  - uses: *a",
		"a container tag":       "- uses: docker://a/b:latest",
		"no version":            "- uses: a/b",
		"a short commit":        "- uses: a/b@012345",
		"a reusable tag":        "jobs:\n  x:\n    uses: a/b/.github/workflows/c.yml@v1",
	}
	for name, src := range unpinned {
		t.Run(name, func(t *testing.T) {
			bad, err := unpinnedActions([]byte(src))
			require.NoError(t, err)
			assert.NotEmpty(t, bad)
		})
	}
}
