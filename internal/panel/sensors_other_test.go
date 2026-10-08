//go:build !windows

package panel_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/ushineko/sanshoku/hwmon"

	"github.com/ushineko/hayami/internal/view"
)

// R3.8. A processor this build cannot find lists every sensor it looked for,
// not the last one tried: "no coretemp/Package id 0" on an AMD machine sent
// somebody looking for an Intel driver that was never going to be there.
func TestAProcessorWithNoSensorNamesEverySensorLookedFor(t *testing.T) {
	r, _ := withKraken(38.9, 2650)
	r.cpuErr = hwmon.ErrNoSensor
	c := r.section()

	poll(t, c)

	reason := find(t, c.Section(), "no sensor")
	assert.Equal(t, "CPU", reason.Label)
	assert.Equal(t, view.Info, reason.Status)
	for _, s := range hwmon.CPU {
		assert.Contains(t, reason.Detail, s.String())
	}
	assert.Contains(t, reason.Detail, hwmon.Root)
}
