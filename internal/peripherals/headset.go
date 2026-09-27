package peripherals

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// HeadsetTimeout is how long headsetcontrol may take.
//
// It answers in about twenty-five milliseconds, so this is a bound and not a
// budget: the subprocess is on a poll loop and one that hung would take the
// loop with it, the same reason liquidctl is given a deadline.
const HeadsetTimeout = 4 * time.Second

// ErrNoHeadsetcontrol is the absence of the program. A machine without it is
// not a machine with a problem; there is simply no headset row.
var ErrNoHeadsetcontrol = errors.New("headsetcontrol is not installed")

// headsetReply is headsetcontrol's JSON.
//
// The reference parses `headsetcontrol -b -c`, which prints a bare number.
// That output now prints "Warning: short output deprecated" alongside it, and
// the JSON it points at carries the device's real name and a battery status
// distinct from its level — which is the difference between a headset that is
// flat and one that is on its cradle and not saying.
type headsetReply struct {
	Devices []headsetDevice `json:"devices"`
}

type headsetDevice struct {
	Device  string `json:"device"`
	Product string `json:"product"`
	Battery struct {
		Status string `json:"status"`
		Level  int    `json:"level"`
	} `json:"battery"`
}

// Headsets asks headsetcontrol what it can see.
func Headsets(ctx context.Context) ([]Battery, error) {
	path, err := exec.LookPath("headsetcontrol")
	if err != nil {
		return nil, ErrNoHeadsetcontrol
	}

	ctx, cancel := context.WithTimeout(ctx, HeadsetTimeout)
	defer cancel()

	out, err := exec.CommandContext(ctx, path, "-o", "json").Output() //nolint:gosec // the headsetcontrol on the user's own PATH
	if err != nil {
		return nil, fmt.Errorf("asking headsetcontrol for the headsets: %w", err)
	}
	return ParseHeadsets(out)
}

/*
ParseHeadsets reads headsetcontrol's report.

A device it can see but cannot get a level out of is still a device: it is
reported with no level rather than dropped, so the panel can say the headset is
there and quiet rather than implying it is gone. `BATTERY_UNAVAILABLE` is what
an Arctis on its charging cradle answers, and it is the ordinary case at the
end of a working day, not an error.
*/
func ParseHeadsets(reply []byte) ([]Battery, error) {
	var r headsetReply
	if err := json.Unmarshal(reply, &r); err != nil {
		return nil, fmt.Errorf("reading headsetcontrol's report: %w", err)
	}

	var found []Battery
	for _, d := range r.Devices {
		b := Battery{Name: headsetName(d)}

		switch strings.ToUpper(d.Battery.Status) {
		case "BATTERY_AVAILABLE":
			b.State = Discharging
		case "BATTERY_CHARGING":
			b.State = Charging
		default:
			// BATTERY_UNAVAILABLE, and the error and timeout statuses. The
			// device is there; its battery is not answering.
			found = append(found, b)
			continue
		}

		if l := d.Battery.Level; l >= 0 && l <= 100 {
			b.Level, b.HasLevel = l, true
		}
		found = append(found, b)
	}
	return found, nil
}

// headsetName picks what to call a headset.
//
// `device` is the full name — "SteelSeries Arctis Nova Pro Wireless" — and
// `product` is the short one. The short one is used where there is one,
// because the vendor is already obvious to whoever owns the thing and a row's
// label has a column to fit in.
func headsetName(d headsetDevice) string {
	if p := strings.TrimSpace(d.Product); p != "" {
		return p
	}
	if n := strings.TrimSpace(d.Device); n != "" {
		return n
	}
	return "Headset"
}
