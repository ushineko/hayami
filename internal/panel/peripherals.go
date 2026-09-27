package panel

import (
	"context"
	"errors"
	"sort"
	"sync"
	"time"

	"github.com/ushineko/hayami/internal/peripherals"
	"github.com/ushineko/hayami/internal/view"
)

// PeripheralsInterval is how often the devices are asked.
//
// Fifteen seconds, which is the reference's. A battery moves over hours and
// one of these polls starts a subprocess; the thing that has to be prompt is
// a device appearing, and fifteen seconds is below the threshold at which
// putting a headset on and glancing at the panel feels broken.
const PeripheralsInterval = 15 * time.Second

// PeripheralsForget is how long a device that has stopped answering is kept.
//
// A device is drawn stale rather than dropped, because the reader's question
// is whether its last level is still true. That stops being a useful question
// eventually: a mouse unplugged this morning is not a mouse that is quiet, it
// is a mouse that is gone, and a panel still showing it is furniture. Ten
// minutes is long enough to cover a headset on its cradle over lunch and short
// enough that a device put away does not outlast the afternoon.
const PeripheralsForget = 10 * time.Minute

// Peripherals is the batteries of the devices on the desk.
type Peripherals struct {
	mu      sync.Mutex
	reading view.PeripheralsReading

	// seen is what each device last said, by name, and when. It is the whole
	// reason this type exists rather than the view reading the devices
	// directly: a peripheral goes away, and the panel is the only layer that
	// knows what it said before it did.
	seen map[string]remembered

	// logitech and headsets are the two sources, replaced by a test so
	// neither a real device nor a real subprocess is touched.
	logitech func() ([]peripherals.Battery, error)
	headsets func(context.Context) ([]peripherals.Battery, error)

	// now is the clock, for the same reason.
	now func() time.Time
}

// remembered is one device's last good reading.
type remembered struct {
	reading view.PeripheralReading
	at      time.Time
}

// NewPeripherals builds the peripherals source.
func NewPeripherals() *Peripherals {
	logitech := peripherals.NewLogitech()
	return &Peripherals{
		seen:     make(map[string]remembered),
		logitech: logitech.Batteries,
		headsets: peripherals.Headsets,
		now:      time.Now,
	}
}

// Key names the section.
func (p *Peripherals) Key() string { return "peripherals" }

// Title is what the section is called.
func (p *Peripherals) Title() string { return "Peripherals" }

// Interval is PeripheralsInterval.
func (p *Peripherals) Interval() time.Duration { return PeripheralsInterval }

// Poll asks both sources and merges what they say with what was said before.
//
// Either source alone is a section worth drawing, and neither is a section
// that is not drawn — a machine with no Logitech receiver and no headset is a
// machine this program looks at, and it should show no peripherals rather than
// an empty heading.
func (p *Peripherals) Poll(ctx context.Context) (bool, error) {
	var errs []error

	found, err := p.logitech()
	if err != nil {
		errs = append(errs, err)
	}

	headsets, err := p.headsets(ctx)
	switch {
	case err == nil:
		found = append(found, headsets...)
	case errors.Is(err, peripherals.ErrNoHeadsetcontrol):
		// Not a problem. A machine without it has no headset row.
	default:
		errs = append(errs, err)
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	p.reading = p.merge(found)
	return len(p.reading.Devices) > 0, errors.Join(errs...)
}

// merge folds this poll's devices into what is remembered.
//
// Called with the lock held.
func (p *Peripherals) merge(found []peripherals.Battery) view.PeripheralsReading {
	now := p.now()
	fresh := make(map[string]bool, len(found))

	for _, b := range found {
		reading := view.PeripheralReading{
			Name:     b.Name,
			Level:    b.Level,
			HasLevel: b.HasLevel,
			Charge:   charge(b.State),
		}

		// A device that is connected and not saying how full it is keeps the
		// level it last gave — but only while nothing else about it has
		// changed. A battery that has crossed between charging and
		// discharging is not a battery whose old level is merely stale; it is
		// one whose old level is wrong, and carrying it over would show a
		// headset filling from a number it has already left.
		if was, ok := p.seen[b.Name]; ok && !reading.HasLevel && was.reading.HasLevel && was.reading.Charge == reading.Charge {
			reading.Level, reading.HasLevel = was.reading.Level, true
		}

		fresh[b.Name] = true
		p.seen[b.Name] = remembered{reading: reading, at: now}
	}

	// Anything remembered and not found this time is drawn stale, until it has
	// been quiet long enough to be gone rather than quiet.
	var out []view.PeripheralReading
	for name, was := range p.seen {
		if !fresh[name] {
			if now.Sub(was.at) > PeripheralsForget {
				delete(p.seen, name)
				continue
			}
			was.reading.Stale = true
		}
		out = append(out, was.reading)
	}

	// By name, so a device appearing does not reorder the ones already drawn.
	// The panel is read at a glance and a row that moves is a row that has to
	// be found again.
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return view.PeripheralsReading{Devices: out}
}

// charge translates the reader's state into the view's.
func charge(s peripherals.State) view.Charge {
	switch s {
	case peripherals.Charging:
		return view.Filling
	case peripherals.Full:
		return view.Charged
	default:
		return view.Draining
	}
}

// Section turns the reading into rows.
func (p *Peripherals) Section() view.Section {
	p.mu.Lock()
	defer p.mu.Unlock()
	return view.Peripherals(p.reading)
}

// Data is the reading as plain values, for the JSON the command line prints.
func (p *Peripherals) Data() any {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.reading
}
