package modules

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/assert"
)

// The modules are PawnIO.Modules 0.2.12's, byte for byte: a change to either
// is a change of what runs in the kernel, and is made here on purpose.
func TestTheModulesAreTheReleasesOwn(t *testing.T) {
	want := map[string]string{
		"AMDFamily17.bin": "dae74615761b78bdf064dfb3e136252ddcc6fc727d88f14738d0e5800d427a91",
		"IntelMSR.bin":    "30a5a302283de1e080dfee99e686eca91d7e53e911095cb91a320fb961f1ce12",
	}
	files := Files()
	assert.Len(t, files, len(want))
	for name, sum := range want {
		got := sha256.Sum256(files[name])
		assert.Equal(t, sum, hex.EncodeToString(got[:]), name)
	}
}
