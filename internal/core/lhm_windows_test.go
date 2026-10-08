package core_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/ushineko/hayami/internal/core"
)

// Spec 036. The probe asks without administrator rights and does not fail:
// whatever this machine has, the answer is two booleans. On a machine with
// LibreHardwareMonitor running both are true, which is logged rather than
// asserted -- the suite does not require it to be installed.
func TestTheHostProbeAnswersWithoutRights(t *testing.T) {
	host := core.ProbeLHMHost()
	t.Logf("PawnIO installed: %v, LibreHardwareMonitor running: %v", host.PawnIO, host.Running)
	assert.NotPanics(t, func() { core.ProbeLHMHost() })
}
