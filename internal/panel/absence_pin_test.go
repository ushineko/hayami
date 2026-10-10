package panel_test

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/ushineko/sanshoku"
	"github.com/ushineko/sanshoku/battery"

	"github.com/ushineko/hayami/internal/core"
	"github.com/ushineko/hayami/internal/panel"
	"github.com/ushineko/hayami/internal/view"
)

// update rewrites the pinned reasons from what the code says now.
var update = flag.Bool("update", false, "rewrite testdata/reasons.golden")

/*
TestEveryReasonIsPinned holds every reason the sources can give -- its label,
text, detail, status, whether it is aside and whether it is a line a reader
can act on -- to a file written before spec 040 moved the wording to the
sources. A refactor of where reasons are made must leave this file unchanged.

The platform's own sentences (the udev rule or Windows' refusal, and the
sensors each platform looks for) are written as placeholders, so one file
holds for every platform.
*/
func TestEveryReasonIsPinned(t *testing.T) {
	var b strings.Builder
	for _, sc := range reasonScenarios(t) {
		fmt.Fprintf(&b, "## %s\n", sc.name)
		for _, r := range sc.reasons {
			if strings.Contains(r.Text, "Bluetooth") {
				// The Bluetooth vendor's lines differ by platform (its
				// drivers do), so vendors_test.go and host_test.go hold them.
				continue
			}
			fmt.Fprintf(&b, "- label=%q text=%q status=%v aside=%v actionable=%v\n  detail=%q\n",
				r.Label, r.Text, r.Status, r.Aside, actionable(r), mask(r.Detail))
		}
	}
	got := b.String()

	path := filepath.Join("testdata", "reasons.golden")
	if *update {
		require.NoError(t, os.MkdirAll("testdata", 0o750))
		require.NoError(t, os.WriteFile(path, []byte(got), 0o600))
	}
	want, err := os.ReadFile(path) //nolint:gosec // the test's own file
	require.NoError(t, err)
	assert.Equal(t, string(want), got, "a reason changed; rerun with -update only if that was the point")
}

// mask replaces the platform's own sentences with placeholders.
func mask(detail string) string {
	for _, p := range []struct{ text, as string }{
		{panel.PermissionDetail, "<permission>"},
		{panel.SensorDetail(errors.New("plain")), "<cpu-sensors>"},
		{panel.GPUSensorDetail(), "<gpu-sensors>"},
	} {
		detail = strings.ReplaceAll(detail, p.text, p.as)
	}
	return detail
}

type scenario struct {
	name    string
	reasons []view.Reason
}

// reasonScenarios drives each source through the states its reasons cover.
func reasonScenarios(t *testing.T) []scenario {
	t.Helper()
	var out []scenario
	add := func(name string, s panel.Source) {
		_, _ = s.Poll(t.Context())
		out = append(out, scenario{name: name, reasons: s.Section().Reasons})
	}

	// The cooler.
	add("cooler: nothing at all", (&rig{cpuErr: errors.New("no hwmon")}).section())
	add("cooler: a load and no temperature", (&rig{cpuErr: errors.New("no hwmon"), load: 3, hasLoad: true}).section())
	for _, a := range []string{
		"LibreHardwareMonitor is running but its web server is off: Options > Remote Web Server > Run",
		"Windows needs LibreHardwareMonitor and its PawnIO driver for a CPU temperature: see README, On Windows",
	} {
		add("cooler: an absence the source explains", (&rig{cpuErr: &core.SensorAbsence{Detail: a}, load: 3, hasLoad: true}).section())
	}
	add("cooler: a cooler not permitted", (&rig{cpu: 50, coolers: []*kraken{{path: "/dev/hidraw20", openErr: &fsError{errno: syscall.EACCES}}}}).section())
	add("cooler: a cooler unsupported", (&rig{cpu: 50, coolers: []*kraken{{path: "/dev/hidraw21", openErr: fmt.Errorf("x: %w", sanshoku.ErrUnsupported)}}}).section())
	add("cooler: a cooler that would not answer", (&rig{cpu: 50, coolers: []*kraken{{path: "/dev/hidraw22", openErr: errors.New("timeout")}}}).section())
	add("cooler: a scan that failed", (&rig{cpu: 50, failing: errors.New("scan broke")}).section())

	// Bandwidth.
	add("bandwidth: nothing chosen", panel.NewBandwidth(nil, fakeCounters))
	add("bandwidth: a name not listed", panel.NewBandwidth([]string{"eth0", "wlan9"}, fakeCounters))
	denied := panel.NewBandwidth([]string{"eth0"}, fakeCounters)
	denied.SetWirelessReader(func(context.Context) (map[string]core.Wireless, error) {
		return map[string]core.Wireless{"eth0": {Connected: true}}, core.ErrWirelessDenied
	})
	add("bandwidth: Wi-Fi details withheld", denied)

	// Peripherals.
	mouse := func(name string) *peripheral {
		return &peripheral{driver: "razer", name: name, path: "/dev/hidraw" + name,
			says: []battery.Battery{{Name: name, Level: 50, HasLevel: true, Kind: battery.KindMouse}}}
	}
	locked := &peripheral{driver: "steelseries", name: "SteelSeries Apex", path: "/dev/hidraw9", openErr: &fsError{errno: syscall.EACCES}}
	add("peripherals: an empty desk", (&desk{}).section(nil))
	add("peripherals: a device not permitted, alone", (&desk{devices: []*peripheral{locked}}).section(nil))
	add("peripherals: a device not permitted, on a full card",
		(&desk{devices: []*peripheral{mouse("A"), mouse("B"), locked, arctisUnsupported()}}).section(nil))
	add("peripherals: unsupported, alone", (&desk{devices: []*peripheral{arctisUnsupported()}}).section(nil))
	add("peripherals: a vendor that would not list", (&desk{failing: map[string]error{"razer": errors.New("enumeration failed")}}).section(nil))
	add("peripherals: a device that would not read",
		(&desk{devices: []*peripheral{{driver: "razer", name: "Razer Dock", path: "/dev/hidraw2", readErr: errors.New("io")}}}).section(nil))
	add("peripherals: a dock answering nothing", (&desk{devices: []*peripheral{sleepingDock()}}).section(nil))
	quiet := receiver()
	quiet.presence = sanshoku.Presence{Nodes: 1, Quiet: 2}
	add("peripherals: a receiver with nothing awake", (&desk{devices: []*peripheral{quiet}}).section(nil))
	add("peripherals: a receiver with nothing paired", (&desk{devices: []*peripheral{receiver()}}).section(nil))
	old := receiver()
	old.presence = sanshoku.Presence{Nodes: 1, TooOld: []string{"Logitech K800"}, OldProtocol: "HID++ 1.0"}
	add("peripherals: a device too old to read", (&desk{devices: []*peripheral{old}}).section(nil))

	// Usage: the reasons a failed gather gives.
	u := panel.NewUsage()
	panel.SetUsageRead(u, func(context.Context) ([]view.UsageWindow, time.Time, []view.Reason, error) {
		return nil, time.Time{}, nil, errors.New("cache unreadable")
	})
	add("usage: a gather that failed", u)
	return out
}

// actionable is whether a reason is the one line a reader can act on. The
// pin was written when that was told by the detail's words; since spec 040 it
// is the source's flag, and the file has not changed.
func actionable(r view.Reason) bool { return r.Actionable }
