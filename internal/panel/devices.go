package panel

import (
	"context"
	"errors"
	"fmt"

	"github.com/ushineko/sanshoku"

	"github.com/ushineko/hayami/internal/core"
	"github.com/ushineko/hayami/internal/view"
)

// Scan lists the candidates a set of drivers finds: sanshoku.Scan, or a
// test's stand-in that finds fakes. It is the seam the device sections are
// tested through, so the suite opens no device.
type Scan func(ctx context.Context, drivers ...sanshoku.Driver) ([]sanshoku.Candidate, error)

/*
held is the devices a section has open, kept across polls.

Hotplug by pull: every poll lists what the drivers find, closes what is no
longer listed, and opens what is new. A device is opened once and read many
times, which is what keeps the drivers' memory -- a Logitech receiver's
located indices, a Razer device's transaction ID, the Kraken's freshness --
from one poll to the next. Opening every device on every poll would throw that
away for nothing: the scan is the cheap part.
*/
type held struct {
	scan    Scan
	devices map[string]sanshoku.Device

	// listed is this poll's candidates, by key, so prune knows what has gone.
	listed map[string]bool

	// permission is the host's account of an Open refused for want of
	// permission, with its platform's advice (spec 043).
	permission func(error) (*core.Absence, bool)
}

func newHeld(scan Scan, permission func(error) (*core.Absence, bool)) *held {
	return &held{scan: scan, devices: make(map[string]sanshoku.Device), permission: permission}
}

// key is a candidate's place in the map. The driver is part of it because
// two drivers can find the same path family: a hidraw node is a Logitech
// receiver to one and nothing to the next, and a Bluetooth object path is
// both a BlueZ battery and an Apple accessory's.
func key(c sanshoku.Candidate) string { return c.Driver + "\x00" + c.Path }

// begin starts a poll's listing.
func (h *held) begin() { h.listed = make(map[string]bool) }

/*
list is one driver's candidates.

One driver at a time rather than the section's drivers in one call, so a
driver's own failure can be named in its own words: sanshoku.Scan joins what
went wrong across the drivers it is given, and a joined error cannot say
which vendor it was about without being parsed.
*/
func (h *held) list(ctx context.Context, d sanshoku.Driver) ([]sanshoku.Candidate, error) {
	found, err := h.scan(ctx, d)
	for _, c := range found {
		h.listed[key(c)] = true
	}
	return found, err
}

// device is the open device for a candidate, opening it if this section does
// not hold it yet. An Open that fails leaves nothing held, so the next poll
// tries again: a udev rule installed and a device replugged is picked up
// without a restart.
func (h *held) device(ctx context.Context, c sanshoku.Candidate) (sanshoku.Device, error) {
	if dev, ok := h.devices[key(c)]; ok {
		return dev, nil
	}
	dev, err := c.Open(ctx)
	if err != nil {
		// The driver's name and not the device's: a Bluetooth alias is a
		// name somebody chose, and an error ends up in logs and issues.
		return nil, fmt.Errorf("opening a %s device: %w", c.Driver, err)
	}
	h.devices[key(c)] = dev
	return dev, nil
}

// has reports whether a candidate is already open.
func (h *held) has(c sanshoku.Candidate) bool {
	_, ok := h.devices[key(c)]
	return ok
}

// drop closes a device and forgets it. It is found again by the next scan if
// it is back.
func (h *held) drop(c sanshoku.Candidate) {
	if dev, ok := h.devices[key(c)]; ok {
		_ = dev.Close()
		delete(h.devices, key(c))
	}
}

// prune closes every held device this poll's listing did not name.
func (h *held) prune() {
	for k, dev := range h.devices {
		if !h.listed[k] {
			_ = dev.Close()
			delete(h.devices, k)
		}
	}
}

// openFailure is what a candidate that would not open says: a reason for a
// device that is there and cannot or will not be read, nothing for an Open that
// found the device absent after all (a Kraken node that does not answer the
// status probe, beside the one that does), and failed for anything else, which
// the section reports in its own words and returns for logging.
func (h *held) openFailure(c sanshoku.Candidate, err error) (said *view.Reason, failed bool) {
	if refused, ok := h.permission(err); ok {
		// Not a failure to log every poll: nothing will change until
		// somebody does what the detail says.
		r := reason(view.Reason{Text: c.Name + " is not permitted", Status: view.Warn}, refused)
		return &r, false
	}
	switch {
	case errors.Is(err, sanshoku.ErrUnsupported):
		// Detected and deliberately not spoken to (spec 017). The name is the
		// label and the verdict one word, so the line is a row like any other
		// rather than a sentence as wide as the panel (issue #77); the
		// explanation is the detail, on hover and in doctor.
		return &view.Reason{
			Label: c.Name, Text: "unsupported", Status: view.Info,
			Detail: "found, and left alone: its battery protocol is not one this build knows",
		}, false
	case errors.Is(err, sanshoku.ErrAbsent):
		return nil, false
	default:
		return nil, true
	}
}

// uniqueReasons drops a reason already said. A device can present several
// nodes that a driver lists, and a missing udev rule would otherwise name the
// same receiver once per node.
func uniqueReasons(in []view.Reason) []view.Reason {
	seen := make(map[view.Reason]bool, len(in))
	out := in[:0]
	for _, r := range in {
		if seen[r] {
			continue
		}
		seen[r] = true
		out = append(out, r)
	}
	return out
}
