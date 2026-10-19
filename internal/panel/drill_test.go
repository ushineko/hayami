package panel_test

import (
	"testing"

	"github.com/ushineko/hayami/internal/panel"
)

// The kill reads its process from here, from either section's page. A
// cooler page's ID is not a bandwidth one, and reading it as one ended
// nothing when the button was pressed.
func TestDrillPID(t *testing.T) {
	cases := []struct {
		id   string
		pid  int
		isPI bool
	}{
		{"cpu/firefox/4411", 4411, true},
		{"memory/firefox/4411", 4411, true},
		{"gpu/firefox/4411", 4411, true},
		{"peers/eno2/fpsd/1534", 1534, true},
		{"cpu", 0, false},
		{"cpu/firefox", 0, false},
		{"peers/eno2/fpsd", 0, false},
		{"peers/eno2/fpsd/@nas", 0, false},
		{"cpu/firefox/0", 0, false},
	}
	for _, c := range cases {
		pid, ok := panel.DrillPID(c.id)
		if pid != c.pid && c.isPI || ok != c.isPI {
			t.Errorf("%s: %d, %v; want %d, %v", c.id, pid, ok, c.pid, c.isPI)
		}
	}
}
