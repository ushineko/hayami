package panel

import (
	"context"
	"errors"
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
// The monitor keeps one indefinitely: its slots are fixed, so a device that
// never comes back costs the slot it already had. These cells are not fixed —
// they appear and disappear with the hardware — so a memory with no bound is a
// panel that accumulates every peripheral ever switched on near it. Ten
// minutes is long enough to cover a headset on its cradle over lunch and short
// enough that a device put away does not outlast the afternoon.
const PeripheralsForget = 10 * time.Minute

// Peripherals is the batteries of the devices on the desk.
//
// It remembers what each device last said, which is the whole reason this type
// exists rather than the view reading the devices directly: a peripheral goes
// quiet, and the panel is the only layer that knows what it said before it
// did. A wireless mouse that has been still for a minute answers nothing —
// measured, on the receiver this was written against — and a panel without a
// memory drops its cell and reflows, which is the fault this memory exists to
// prevent and the reason the monitor has one too.
type Peripherals struct {
	mu      sync.Mutex
	reading view.PeripheralsReading

	// seen is what each device last said, by name, and when.
	seen map[string]remembered

	// logitech, headsets and bluetooth are the sources, replaced by a test so
	// neither a real device nor a real subprocess is touched.
	logitech  func() ([]peripherals.Battery, error)
	headsets  func(context.Context) ([]peripherals.Battery, error)
	bluetooth func() ([]peripherals.Battery, error)

	// now is the clock, for the same reason.
	now func() time.Time
}

// remembered is one device's last reading that had a level in it.
type remembered struct {
	reading view.PeripheralReading
	at      time.Time

	// since is when this device was last detected after not being there. It
	// survives a poll the device answered and is set again only when the
	// device has been forgotten in between, which is what makes it "when you
	// switched this on" rather than "when this program started".
	since time.Time
}

// NewPeripherals builds the peripherals source.
func NewPeripherals() *Peripherals {
	logitech := peripherals.NewLogitech()
	bluetooth := peripherals.NewBluetooth()
	return &Peripherals{
		seen:      make(map[string]remembered),
		logitech:  logitech.Batteries,
		headsets:  peripherals.Headsets,
		bluetooth: bluetooth.Batteries,
		now:       time.Now,
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

	bluetooth, err := p.bluetooth()
	switch {
	case err == nil:
		found = append(found, bluetooth...)
	case errors.Is(err, peripherals.ErrNoBluez):
		// Also not a problem. A machine with no Bluetooth has no Bluetooth
		// rows, which is what it should look like.
	default:
		// A partial answer is still an answer: Batteries returns what it read
		// alongside the errors for what it could not.
		found = append(found, bluetooth...)
		errs = append(errs, err)
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	p.reading = p.readings(found)
	return len(p.reading.Devices) > 0, errors.Join(errs...)
}

/*
readings is this poll's devices folded into what is remembered, filtered and
put in the view's order.

Called with the lock held.

Three cases, which are the monitor's and the design system's alike:

  - A device that gave a level is a cell, and is remembered.
  - A device that is present and has never given one is **not a cell**. There
    is nothing to draw and, once the cells are ordered, the empty one would sit
    in front of the mouse. An Arctis whose receiver is in with the headset
    switched off is this case for a whole session.
  - A device that gave one before and has gone quiet keeps it, dim, until it
    has been quiet long enough to be gone rather than quiet.
*/
func (p *Peripherals) readings(found []peripherals.Battery) view.PeripheralsReading {
	now := p.now()
	fresh := make(map[string]bool, len(found))

	for _, b := range found {
		reading := view.PeripheralReading{
			Name:   b.Name,
			Level:  b.Level,
			Charge: charge(b.State),
			Kind:   kind(b.Kind),
			Cells:  cells(b.Cells),
		}

		// A device that is connected and not saying how full it is keeps the
		// level it last gave — but only while nothing else about it has
		// changed. A battery that has crossed between charging and
		// discharging is not a battery whose old level is merely stale; it is
		// one whose old level is wrong, and carrying it over would show a
		// headset filling from a number it has already left. The monitor
		// guards its own carry-over on the same two conditions.
		if !b.HasLevel {
			was, ok := p.seen[b.Name]
			if !ok || was.reading.Charge != reading.Charge {
				// Nothing to carry, or nothing worth carrying. A level from
				// before the cable is not stale, it is wrong, so it is
				// forgotten outright rather than left to be drawn dim — which
				// is what the monitor does too, clearing its cached reading on
				// the same transition. The device is present and has no level,
				// so it has no cell.
				delete(p.seen, b.Name)
				continue
			}
			reading.Level = was.reading.Level
		}

		since := now
		if was, ok := p.seen[b.Name]; ok {
			since = was.since
		}
		reading.Since, reading.Seen = since, now

		fresh[b.Name] = true
		p.seen[b.Name] = remembered{reading: reading, at: now, since: since}
	}

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
	return view.PeripheralsReading{Devices: view.OrderPeripherals(out)}
}

// cells translates a device's separate batteries into the view's.
//
// The names are made here rather than in the view because what a cell is
// called is the reader's business: the view is told "L" and draws it, and does
// not know that a left earbud is component 0x04 in somebody's protocol.
func cells(in []peripherals.CellReading) []view.PeripheralCell {
	if len(in) == 0 {
		return nil
	}
	out := make([]view.PeripheralCell, 0, len(in))
	for _, c := range in {
		out = append(out, view.PeripheralCell{Name: c.Cell.String(), Level: c.Level})
	}
	return out
}

// kind translates the reader's device type into the view's.
func kind(k peripherals.Kind) view.Kind {
	switch k {
	case peripherals.KindMouse:
		return view.KindMouse
	case peripherals.KindKeyboard:
		return view.KindKeyboard
	case peripherals.KindHeadset:
		return view.KindHeadset
	default:
		return view.KindOther
	}
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
