package core_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/ushineko/hayami/internal/core"
)

// Spec 036, 042. The probe asks without administrator rights and does not
// fail: whatever this machine has, the answer is three booleans. On a machine
// with LibreHardwareMonitor set up by install_windows.ps1 -WithSensors all
// three are true, which is logged rather than asserted -- the suite does not
// require it to be installed.
func TestTheHostProbeAnswersWithoutRights(t *testing.T) {
	host := core.ProbeLHMHost()
	t.Logf("PawnIO installed: %v, LibreHardwareMonitor running: %v, startup task: %v",
		host.PawnIO, host.Running, host.Task)
	assert.NotPanics(t, func() { core.ProbeLHMHost() })
}
