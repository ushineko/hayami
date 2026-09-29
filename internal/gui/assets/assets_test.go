package assets_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/hayami/internal/gui/assets"
)

// The embedded icon and the one the installer copies are the same file.
//
// Embedding cannot reach outside its own package directory and the installer
// cannot read one from inside the binary, so there are two copies of one
// icon. This is what notices when somebody edits one of them.
//
// (The directive is spelled around, not written: a comment beginning with the
// directive's own name is read as a malformed one.)
func TestTheEmbeddedIconMatchesThePackagedOne(t *testing.T) {
	packaged, err := os.ReadFile(filepath.Join("..", "..", "..", "packaging", "hayami.svg"))
	require.NoError(t, err, "packaging/hayami.svg is what the desktop entry installs")

	assert.Equal(t, string(packaged), string(assets.IconSVG()),
		"the embedded icon and packaging/hayami.svg have drifted apart")
}
