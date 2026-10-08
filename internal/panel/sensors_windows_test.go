package panel_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/ushineko/sanshoku/hwmon"

	"github.com/ushineko/hayami/internal/view"
)

// Spec 034. On Windows the reason says why there is no temperature, not which
// Linux sensors were missing, and names no Linux path.
func TestOnWindowsTheProcessorsReasonSaysWhyNotWhere(t *testing.T) {
	r, _ := withKraken(38.9, 2650)
	r.cpuErr = hwmon.ErrNoSensor
	c := r.section()

	poll(t, c)

	reason := find(t, c.Section(), "no sensor")
	assert.Equal(t, "CPU", reason.Label)
	assert.Equal(t, view.Info, reason.Status)
	assert.Contains(t, reason.Detail, "Windows offers no CPU temperature")
	assert.NotContains(t, reason.Detail, hwmon.Root)
}
