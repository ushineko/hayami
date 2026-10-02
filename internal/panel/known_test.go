package panel_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/hayami/internal/panel"
	"github.com/ushineko/sanshoku/battery"
)

// remembering is a section over the desk that keeps its memory of devices in
// path.
func remembering(k *desk, c *clock, path string) *panel.Peripherals {
	p := k.section(c)
	panel.RememberIn(p, path)
	return p
}

// AC (spec 032). A device read, then a restart with nothing answering: the
// device is drawn dim at its last level, in the slot it had, rather than the
// card reading as if nothing were on the desk.
func TestARestartWithTheMouseAsleepStillDrawsTheMouse(t *testing.T) {
	path := filepath.Join(t.TempDir(), "peripherals.json")
	mouse := receiver(battery.Battery{Name: "G502 X PLUS", Level: 86, HasLevel: true})
	k := &desk{devices: []*peripheral{mouse}}
	c := &clock{at: time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)}
	poll(t, remembering(k, c, path))

	mouse.says = nil
	c.tick(10 * time.Hour)
	restarted := remembering(k, c, path)
	assert.True(t, poll(t, restarted), "a restarted panel drew no section for a sleeping mouse")

	found := devices(restarted)
	require.Len(t, found, 1)
	assert.Equal(t, "G502 X PLUS", found[0].Name)
	assert.Equal(t, 86, found[0].Level)
	assert.True(t, found[0].Stale, "a device nothing has been heard from this run is not live")
	assert.Equal(t, "G502 X PLUS", restarted.Section().Cells[0].Label, "the mouse lost its slot")
}

// AC (spec 032). Seven days: a device last heard eight days ago is not
// loaded, one heard six days ago is, and a running panel forgets a device
// once it has been quiet for seven.
func TestADeviceUnheardForAWeekIsForgotten(t *testing.T) {
	path := filepath.Join(t.TempDir(), "peripherals.json")
	old := receiver(battery.Battery{Name: "Old Headset", Level: 40, HasLevel: true})
	k := &desk{devices: []*peripheral{old}}
	c := &clock{at: time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)}
	poll(t, remembering(k, c, path))
	old.says = nil

	c.tick(6 * 24 * time.Hour)
	recent := remembering(k, c, path)
	poll(t, recent)
	require.Len(t, devices(recent), 1, "a device heard six days ago was not loaded")

	c.tick(2 * 24 * time.Hour)
	poll(t, recent)
	assert.Empty(t, devices(recent), "a running panel kept a device quiet for eight days")
	assert.Empty(t, devices(remembering(k, c, path)), "a device unheard for eight days was loaded")
}

// AC (spec 032). A device remembered as quiet that answers on the first poll
// is live, and it arrived at that poll: a restart is when it was detected.
func TestARememberedDeviceThatAnswersIsLiveFromThatPoll(t *testing.T) {
	path := filepath.Join(t.TempDir(), "peripherals.json")
	mouse := receiver(battery.Battery{Name: "G502 X PLUS", Level: 86, HasLevel: true})
	k := &desk{devices: []*peripheral{mouse}}
	c := &clock{at: time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)}
	poll(t, remembering(k, c, path))

	c.tick(time.Hour)
	mouse.says = []battery.Battery{{Name: "G502 X PLUS", Level: 80, HasLevel: true}}
	restarted := remembering(k, c, path)
	poll(t, restarted)

	found := devices(restarted)
	require.Len(t, found, 1)
	assert.False(t, found[0].Stale)
	assert.Equal(t, 80, found[0].Level)
	assert.Equal(t, c.at, found[0].Since, "a device answering after a restart was not detected then")
}

// AC (spec 032). A malformed file is a first run, not an error.
func TestAMalformedMemoryIsAFirstRun(t *testing.T) {
	path := filepath.Join(t.TempDir(), "peripherals.json")
	require.NoError(t, os.WriteFile(path, []byte("{not json"), 0o600))
	k := &desk{devices: []*peripheral{receiver()}}
	p := remembering(k, &clock{at: time.Now()}, path)
	poll(t, p)
	assert.Empty(t, devices(p))
}
