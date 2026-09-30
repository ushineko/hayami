package panel

import (
	"context"
	"errors"
	"fmt"
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

	// reasons are the sources that had nothing to say and why, rebuilt every
	// poll. A source that found something contributes none: five lines
	// explaining what is absent, over a card that is already showing a mouse,
	// would be a panel talking about itself.
	reasons []view.Reason

	// The sources, replaced by a test so neither a real device nor a real
	// subprocess is touched.
	logitech    func() ([]peripherals.Battery, error)
	headsets    func(context.Context) ([]peripherals.Battery, error)
	bluetooth   func() ([]peripherals.Battery, error)
	razer       func() ([]peripherals.Battery, error)
	steelseries func() ([]peripherals.Battery, error)

	// unsupported names devices a source found and would not speak to. A
	// device this build does not know is detected and left alone (spec 017),
	// and saying which one is the difference between that and a bug.
	unsupported func() []string

	// logitechPresence is what the Logitech reader found besides batteries: a
	// receiver with nothing awake on it reads very differently from no
	// receiver, and used to read the same (issue #66).
	logitechPresence func() peripherals.Presence

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
	razer := peripherals.NewRazer()
	steelseries := peripherals.NewSteelSeries()
	return &Peripherals{
		seen:             make(map[string]remembered),
		logitech:         logitech.Batteries,
		headsets:         peripherals.Headsets,
		bluetooth:        bluetooth.Batteries,
		razer:            razer.Batteries,
		steelseries:      steelseries.Batteries,
		unsupported:      steelseries.Unsupported,
		logitechPresence: logitech.Presence,
		now:              time.Now,
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
	var reasons, present []view.Reason

	found, err := p.logitech()
	presence := p.logitechPresence()
	switch {
	case err != nil:
		errs = append(errs, err)
		reasons = append(reasons, view.Reason{
			Text: "the Logitech receiver would not answer", Status: view.Warn,
			Detail: err.Error(),
		})
	case len(found) > 0:
		// Drawing. Nothing to explain.
	case presence.Nodes == 0:
		reasons = append(reasons, view.Reason{
			Text: "no Logitech receiver", Status: view.Info,
		})
	case presence.Quiet > 0:
		// Counted, and described as exactly what it is.
		//
		// Not named, because a pairing table outlives the hardware in it: the
		// receiver this was written against carries a slot for a mouse its
		// owner never had, and naming it would put a device on the panel that
		// was never on the desk.
		//
		// And not called *paired* either. The receiver measured here answers
		// the same code for an empty slot as for a sleeping device, so a count
		// of unanswered indices is all this knows -- the first draft of this
		// line said "8 paired slots" on a receiver with two pairings.
		reasons = append(reasons, view.Reason{
			Text: "a Logitech receiver, with nothing awake on it", Status: view.Info,
			Detail: fmt.Sprintf("%d %s asked and none answered; a sleeping device and an empty slot say the same thing",
				presence.Quiet, plural(presence.Quiet, "index", "indices")),
		})
	default:
		reasons = append(reasons, view.Reason{
			Text: "a Logitech receiver, with nothing paired to it", Status: view.Info,
		})
	}

	headsets, err := p.headsets(ctx)
	switch {
	case err == nil:
		found = append(found, headsets...)
		if len(headsets) == 0 {
			reasons = append(reasons, view.Reason{
				Text: "headsetcontrol found no headset", Status: view.Info,
			})
		}
	case errors.Is(err, peripherals.ErrNoHeadsetcontrol):
		// Not a problem. A machine without it has no headset row -- but a
		// reader looking at a card with no headset on it deserves to know
		// that nothing looked, rather than that nothing was found.
		reasons = append(reasons, view.Reason{
			Text: "headsetcontrol is not installed", Status: view.Info,
		})
	default:
		errs = append(errs, err)
		reasons = append(reasons, view.Reason{
			Text: "headsetcontrol failed", Status: view.Warn, Detail: err.Error(),
		})
	}

	razer, err := p.razer()
	switch {
	case err != nil:
		errs = append(errs, err)
		reasons = append(reasons, view.Reason{
			Text: "a Razer device would not answer", Status: view.Warn, Detail: err.Error(),
		})
	case len(razer) == 0:
		reasons = append(reasons, view.Reason{
			Text: "no Razer device", Status: view.Info,
		})
	default:
		found = append(found, razer...)
	}

	steelseries, err := p.steelseries()
	switch {
	case err != nil:
		errs = append(errs, err)
		reasons = append(reasons, view.Reason{
			Text: "a SteelSeries device would not answer", Status: view.Warn, Detail: err.Error(),
		})
	case len(steelseries) == 0:
		reasons = append(reasons, view.Reason{
			Text: "no SteelSeries device", Status: view.Info,
		})
	default:
		found = append(found, steelseries...)
	}

	// A device that answered and speaks a protocol generation older than this
	// build reads. Named, because it answered -- something is there.
	for _, name := range presence.TooOld {
		present = append(present, view.Reason{
			Text: name + ": speaks HID++ 1.0", Status: view.Info,
			Detail: "found, and not read: its battery is a HID++ 1.0 register this build does not ask for",
		})
	}

	// A device that was found and deliberately not spoken to.
	//
	// Kept apart from the reasons above because it survives a card that is
	// already showing hardware. "No Logitech receiver" is noise beside a mouse
	// that is drawing; "this device is on your desk and I cannot read it" is
	// not, and it is the difference between a gap this build knows about and
	// one it does not.
	for _, name := range p.unsupported() {
		present = append(present, view.Reason{
			Text: name + ": not a device this build can read", Status: view.Info,
			Detail: "found, and left alone: its battery protocol is not one this build knows",
		})
	}

	bluetooth, err := p.bluetooth()
	switch {
	case err == nil:
		found = append(found, bluetooth...)
		if len(bluetooth) == 0 {
			reasons = append(reasons, view.Reason{
				Text: "no Bluetooth device with a battery", Status: view.Info,
			})
		}
	case errors.Is(err, peripherals.ErrNoBluez):
		// Also not a problem. A machine with no Bluetooth has no Bluetooth
		// rows, which is what it should look like -- and the reason says
		// which of the two it is, because "no adapter" and "the daemon is
		// down" are different things to go and do something about.
		reasons = append(reasons, view.Reason{
			Text: "no Bluetooth adapter", Status: view.Info, Detail: err.Error(),
		})
	default:
		// A partial answer is still an answer: Batteries returns what it read
		// alongside the errors for what it could not.
		found = append(found, bluetooth...)
		errs = append(errs, err)
		reasons = append(reasons, view.Reason{
			Text: "a Bluetooth device would not answer", Status: view.Warn,
			Detail: err.Error(),
		})
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	p.reading = p.readings(found)
	if len(p.reading.Devices) > 0 {
		// The card is showing hardware. What else is *absent* is doctor's
		// business, not the panel's -- but a device that is present and
		// unreadable is still worth a line.
		reasons = nil
	}
	reasons = append(reasons, present...)
	p.reasons = reasons
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
		if b.HasBand {
			// A device from before the feature protocol: four steps and no
			// percentage. The two never both apply.
			reading.Band, reading.Segments = b.Band.String(), b.Band.Segments()
		}

		// A device that is connected and not saying how full it is keeps the
		// level it last gave — but only while nothing else about it has
		// changed. A battery that has crossed between charging and
		// discharging is not a battery whose old level is merely stale; it is
		// one whose old level is wrong, and carrying it over would show a
		// headset filling from a number it has already left. The monitor
		// guards its own carry-over on the same two conditions.
		if !b.HasLevel && !b.HasBand {
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
	sec := view.Peripherals(p.reading)
	sec.Reasons = p.reasons
	return sec
}

// Data is the reading as plain values, for the JSON the command line prints.
func (p *Peripherals) Data() any {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.reading
}

// plural picks a word for a count, so a reason reads like a sentence.
func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}
