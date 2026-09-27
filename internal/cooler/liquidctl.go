package cooler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// LiquidctlTimeout is how long the subprocess may take.
//
// Bounded because a panel polls on a timer: a liquidctl that hung would take
// the whole poll loop with it, the same reason the Codex app-server is given a
// deadline.
const LiquidctlTimeout = 8 * time.Second

// ErrNoLiquidctl is the absence of the program. A machine without it is not a
// machine with a problem; the section draws what the kernel gave it.
var ErrNoLiquidctl = errors.New("liquidctl is not installed")

// ErrNoCooler is liquidctl running and finding no cooler. Also not a problem:
// this program looks at a machine that may not have one.
var ErrNoCooler = errors.New("liquidctl reports no liquid cooler")

// Liquid is what the cooler says about itself.
type Liquid struct {
	// Coolant is the liquid temperature in degrees.
	Coolant float64

	// PumpRPM and FanRPM are speeds. Zero means the device did not report
	// one, which is different from a stopped pump and is why HasPump and
	// HasFan exist.
	PumpRPM int
	FanRPM  int

	HasPump bool
	HasFan  bool
}

// device is one entry in liquidctl's report.
type device struct {
	Description string   `json:"description"`
	Status      []status `json:"status"`
}

type status struct {
	Key   string          `json:"key"`
	Value json.RawMessage `json:"value"`
	Unit  string          `json:"unit"`
}

// Cooling asks liquidctl what the cooler is doing.
func Cooling(ctx context.Context) (Liquid, error) {
	path, err := exec.LookPath("liquidctl")
	if err != nil {
		return Liquid{}, ErrNoLiquidctl
	}

	ctx, cancel := context.WithTimeout(ctx, LiquidctlTimeout)
	defer cancel()

	out, err := exec.CommandContext(ctx, path, "--json", "status").Output() //nolint:gosec // the liquidctl on the user's own PATH
	if err != nil {
		return Liquid{}, fmt.Errorf("asking liquidctl for the cooler: %w", err)
	}
	return Parse(out)
}

/*
Parse reads liquidctl's report and picks the cooler out of it.

**liquidctl reports every device it can see**, and the trap is that several of
them have temperatures. On the machine this was written on the list holds a
Kraken and a Corsair HX1000i power supply, and the power supply reports a "VRM
temperature" and a "Case temperature". Matching on temperature keys alone would
put the power supply's numbers under a heading that says coolant — the wrong
number under the right label, which nobody would catch by looking.

The cooler is the device that reports a *liquid* temperature. Nothing else
qualifies, and a machine with no such device has no cooler as far as this
program is concerned.
*/
func Parse(reply []byte) (Liquid, error) {
	var devices []device
	if err := json.Unmarshal(reply, &devices); err != nil {
		return Liquid{}, fmt.Errorf("reading liquidctl's report: %w", err)
	}

	for _, d := range devices {
		liquid, ok := cooler(d)
		if ok {
			return liquid, nil
		}
	}
	return Liquid{}, ErrNoCooler
}

// cooler reads one device, reporting whether it is a liquid cooler at all.
func cooler(d device) (Liquid, bool) {
	var out Liquid
	found := false

	for _, s := range d.Status {
		key := strings.ToLower(s.Key)
		switch {
		case strings.Contains(key, "liquid") && strings.Contains(key, "temperature"):
			if v, ok := number(s.Value); ok {
				out.Coolant, found = v, true
			}
		case strings.Contains(key, "pump") && strings.Contains(key, "speed"):
			if v, ok := number(s.Value); ok {
				out.PumpRPM, out.HasPump = int(v), true
			}
		case strings.Contains(key, "fan") && strings.Contains(key, "speed"):
			if v, ok := number(s.Value); ok {
				out.FanRPM, out.HasFan = int(v), true
			}
		}
	}
	return out, found
}

// number reads a status value, which liquidctl writes as a number or, for a
// device that has nothing to say about it, as null.
//
// null is checked for rather than unmarshalled: decoding JSON null into a
// float64 succeeds and leaves the zero behind, so a device that reported
// nothing about its fan would have shown as a fan that had stopped.
func number(raw json.RawMessage) (float64, bool) {
	if len(raw) == 0 || string(raw) == "null" {
		return 0, false
	}
	var v float64
	if err := json.Unmarshal(raw, &v); err != nil {
		return 0, false
	}
	return v, true
}
