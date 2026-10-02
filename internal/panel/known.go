package panel

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"time"

	"github.com/ushineko/hayami/internal/readings"
	"github.com/ushineko/hayami/internal/view"
)

/*
ForgetAfter is how long a device that has stopped answering is remembered.

Seven days (spec 032). Long enough that a mouse asleep over a weekend, or a
headset left off for a few days, is still drawn dim at its last level after a
restart rather than read as absent; short enough that hardware sold or put in
a drawer drops out of the card without anyone removing it.
*/
const ForgetAfter = 7 * 24 * time.Hour

// knownFile is the memory of devices, beside the section cache.
const knownFile = "peripherals.json"

// knownDevice is one remembered device as it is written down.
type knownDevice struct {
	Reading view.PeripheralReading `json:"reading"`
	Since   time.Time              `json:"since"`
}

/*
loadKnown is the devices heard within ForgetAfter of now, each marked as not
answering, keyed by name.

Marked quiet because nothing has been heard from them in this run yet: the
first poll then does with them what a running panel does with a device that
went quiet -- draws it dim at its last level, ranked by when it was last
heard, and live again the moment it answers. A missing, unreadable or
malformed file is a first run.
*/
func loadKnown(path string, now time.Time) map[string]remembered {
	seen := make(map[string]remembered)
	if path == "" {
		return seen
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return seen
	}
	var devices []knownDevice
	if json.Unmarshal(raw, &devices) != nil {
		return seen
	}
	for _, d := range devices {
		if d.Reading.Name == "" || now.Sub(d.Reading.Seen) > ForgetAfter {
			continue
		}
		d.Reading.Stale = true
		seen[d.Reading.Name] = remembered{reading: d.Reading, since: d.Since}
	}
	return seen
}

// encodeKnown is the memory as it is written, in name order so an unchanged
// memory encodes to the same bytes and is not written again.
func encodeKnown(seen map[string]remembered) ([]byte, error) {
	devices := make([]knownDevice, 0, len(seen))
	for _, was := range seen {
		devices = append(devices, knownDevice{Reading: was.reading, Since: was.since})
	}
	sort.Slice(devices, func(i, j int) bool { return devices[i].Reading.Name < devices[j].Reading.Name })
	raw, err := json.Marshal(devices)
	if err != nil {
		return nil, fmt.Errorf("encoding the devices: %w", err)
	}
	return raw, nil
}

/*
saveKnown writes the memory when it has changed since the last write.

The error is the caller's to drop: it is a cache, and a panel that cannot
write its own cache directory still draws what it read.
*/
func (p *Peripherals) saveKnown() error {
	if p.known == "" {
		return nil
	}
	raw, err := encodeKnown(p.seen)
	if err != nil {
		return err
	}
	if string(raw) == string(p.saved) {
		return nil
	}
	if err := readings.WriteAtomic(p.known, raw); err != nil {
		return err
	}
	p.saved = raw
	return nil
}
