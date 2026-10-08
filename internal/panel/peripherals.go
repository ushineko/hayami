package panel

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/ushineko/sanshoku"
	"github.com/ushineko/sanshoku/aula"
	"github.com/ushineko/sanshoku/battery"
	"github.com/ushineko/sanshoku/hidraw"
	"github.com/ushineko/sanshoku/logitech"
	"github.com/ushineko/sanshoku/razer"
	"github.com/ushineko/sanshoku/steelseries"

	"github.com/ushineko/hayami/internal/readings"
	"github.com/ushineko/hayami/internal/view"
)

// PeripheralsInterval is how often the devices are asked.
//
// Fifteen seconds, which is the reference's. A battery moves over hours and
// one of these polls asks every device on the desk; the thing that has to be prompt is
// a device appearing, and fifteen seconds is below the threshold at which
// putting a headset on and glancing at the panel feels broken.
const PeripheralsInterval = 15 * time.Second

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

	// seen is what each device last said, by name, and when. A device stays
	// in it for the session once it has given a level (spec 022): the card
	// always draws two cells, and a headset switched off is the one it should
	// be drawing dim rather than a placeholder.
	seen map[string]remembered

	// known is where seen is kept between runs, and saved the bytes last
	// written there (spec 032). Empty is a source that remembers nothing
	// across a restart, which is what the tests build unless they ask.
	known string
	saved []byte

	// reasons are the sources that had nothing to say and why, rebuilt every
	// poll. A source that found something contributes none: five lines
	// explaining what is absent, over a card that is already showing a mouse,
	// would be a panel talking about itself.
	reasons []view.Reason

	// polling serialises Poll, which owns the held devices.
	polling sync.Mutex

	// held is the devices open across polls, and the scan that finds them:
	// sanshoku's, or a test's that finds fakes.
	held *held

	// now is the clock, replaced by a test.
	now func() time.Time
}

// vendor is one line of the card's reasons and the drivers that answer for
// it. Bluetooth is one vendor with two drivers, because to a reader "no
// Bluetooth device with a battery" is one fact whichever protocol would have
// read it.
type vendor struct {
	name    string
	absent  string
	drivers []sanshoku.Driver

	// quiet is whether a device that is listed and reads nothing gets a line
	// of its own. Not Logitech, whose receiver says what is quiet on it, and
	// not Bluetooth, where a device is listed only when it has a level.
	quiet bool
}

// vendors are the peripherals drivers, in the order they are asked and their
// reasons are given: logitech, razer, steelseries, aula, then Bluetooth where
// this system has one hayami can read (apple, bluez; not on Windows).
//
// AULA is quiet for the reason Razer is: its receiver is listed whether or
// not the keyboard is switched to it, and a receiver with a keyboard on its
// cable answers nothing (spec 035).
func vendors() []vendor {
	return append([]vendor{
		{name: "Logitech", absent: "no Logitech receiver", drivers: []sanshoku.Driver{logitech.Driver{}}},
		{name: "Razer", absent: "no Razer device", drivers: []sanshoku.Driver{razer.Driver{}}, quiet: true},
		{name: "SteelSeries", absent: "no SteelSeries device", drivers: []sanshoku.Driver{steelseries.Driver{}}, quiet: true},
		{name: "AULA", absent: "no AULA receiver", drivers: []sanshoku.Driver{aula.Driver{}}, quiet: true},
	}, bluetooth()...)
}

// remembered is one device's last reading that had a level in it.
type remembered struct {
	reading view.PeripheralReading

	// since is when this device was last detected after not being there. It
	// survives a poll the device answered and is set again when the device
	// was quiet or forgotten in between, which is what makes it "when you
	// switched this on" rather than "when this program started".
	since time.Time
}

// NewPeripherals builds the peripherals source over sanshoku's drivers,
// found by scan.
func NewPeripherals(scan Scan) *Peripherals {
	p := newPeripherals(scan, time.Now)
	if path, err := readings.File(knownFile); err == nil {
		p.remember(path)
	}
	return p
}

// remember keeps the source's memory of devices in a file, and loads what it
// held: the devices heard within ForgetAfter, as not answering yet.
func (p *Peripherals) remember(path string) {
	p.known = path
	p.seen = loadKnown(path, p.now())
	p.saved, _ = encodeKnown(p.seen)
}

// newPeripherals builds the source over a scan and a clock, which is the seam
// the tests use.
func newPeripherals(scan Scan, now func() time.Time) *Peripherals {
	return &Peripherals{seen: make(map[string]remembered), held: newHeld(scan), now: now}
}

// Key names the section.
func (p *Peripherals) Key() string { return view.PeripheralsInfo.Key }

// Interval is PeripheralsInterval.
func (p *Peripherals) Interval() time.Duration { return PeripheralsInterval }

// vendorPoll is what one vendor's drivers found this poll.
type vendorPoll struct {
	batteries []battery.Battery

	// listed is how many candidates the drivers found, opened or not, and
	// read how many of them are held and answered a read without an error.
	listed, read int

	// presence is the Logitech receivers' Presence, summed.
	presence logitech.Presence

	// said is a reason already given for the vendor as a whole -- it would
	// not answer, or there is no Bluetooth adapter -- so it says nothing
	// else about itself.
	said bool
}

// Poll asks every driver and merges what they say with what was said before.
//
// Any one vendor alone is a section worth drawing, and none is a section that
// is not drawn — a machine with no Logitech receiver and no headset is a
// machine this program looks at, and it should show no peripherals rather than
// an empty heading.
func (p *Peripherals) Poll(ctx context.Context) (bool, error) {
	p.polling.Lock()
	defer p.polling.Unlock()

	var errs []error
	var reasons, present []view.Reason

	p.held.begin()
	var found []battery.Battery
	for _, v := range vendors() {
		got, why, keep, err := p.pollVendor(ctx, v)
		found = append(found, got.batteries...)
		reasons = append(reasons, why...)
		present = append(present, keep...)
		errs = append(errs, err)
		if v.name == "Logitech" && !got.said && len(got.batteries) == 0 {
			if r := receiverReason(got.presence); r != nil {
				reasons = append(reasons, *r)
			}
		}
		// A device that answered and speaks a protocol generation older
		// than this build reads, and whose register would not read either.
		// Named, because it answered -- something is there.
		for _, name := range got.presence.TooOld {
			present = append(present, view.Reason{
				Label: name, Text: "speaks HID++ 1.0", Status: view.Info,
				Detail: "found, and not read: its HID++ 1.0 battery register would not answer",
			})
		}
	}
	p.held.prune()

	p.mu.Lock()
	defer p.mu.Unlock()
	p.reading = p.readings(found)
	_ = p.saveKnown() // a cache: see saveKnown
	if len(p.reading.Devices) > 0 {
		// The card is showing hardware. What else is *absent* is doctor's
		// business, not the panel's -- but a device that is present and
		// unreadable is still worth a line.
		reasons = nil
	}
	present = uniqueReasons(present)
	if len(p.reading.Devices) >= PresentAsideAt {
		for i := range present {
			// A device that may not be opened is not a footnote: it is the
			// one line here a reader can act on, and it stays drawn.
			if present[i].Detail != permissionDetail {
				present[i].Aside = true
			}
		}
	}
	p.reasons = append(uniqueReasons(reasons), present...)
	return len(p.reading.Devices) > 0, errors.Join(errs...)
}

/*
pollVendor lists one vendor's candidates, opens what is new and reads what is
held.

It returns what was read, the reasons that are about absence (dropped when the
card is drawing), the reasons that are about a device present and unreadable
(kept), and the failures worth logging.
*/
func (p *Peripherals) pollVendor(ctx context.Context, v vendor) (vendorPoll, []view.Reason, []view.Reason, error) {
	var out vendorPoll
	var reasons, present []view.Reason
	var errs []error

	warn := func(err error) {
		out.said = true
		errs = append(errs, err)
		reasons = append(reasons, view.Reason{
			Text: article(v.name) + " " + v.name + " device would not answer", Status: view.Warn, Detail: err.Error(),
		})
	}

	for _, d := range v.drivers {
		candidates, err := p.held.list(ctx, d)
		switch {
		case errors.Is(err, sanshoku.ErrUnavailable):
			// Not a problem. A driver that could not look is, for the only
			// drivers that say so (bluez and apple, through
			// bluez.ErrNoBlueZ), a machine with no Bluetooth: no Bluetooth
			// rows, which is what it should look like -- and the reason says
			// which of the two it is, because "no adapter" and "nothing
			// connected" are different things to go and do something about.
			reasons = append(reasons, view.Reason{Text: "no Bluetooth adapter", Status: view.Info, Detail: err.Error()})
			out.said = true
		case err != nil:
			warn(err)
		}
		out.listed += len(candidates)

		for _, c := range candidates {
			dev, err := p.held.device(ctx, c)
			if err != nil {
				r, failed := openFailure(c, err)
				switch {
				case failed:
					warn(err)
				case r != nil:
					present = append(present, *r)
				}
				continue
			}
			src, ok := dev.(battery.Source)
			if !ok {
				continue
			}
			batteries, err := src.Batteries(ctx)
			// A partial answer is still an answer: a receiver returns what it
			// read beside the error for what it could not.
			out.batteries = append(out.batteries, batteries...)
			switch {
			case err == nil:
				out.read++
			case errors.Is(err, sanshoku.ErrGone):
				p.held.drop(c)
				continue
			default:
				warn(err)
			}
			// Asked after the read, because it is what that read found.
			if pr, ok := dev.(logitech.Presencer); ok {
				out.presence = addPresence(out.presence, pr.Presence(), hidraw.PairedChild(c.Phys))
			}
		}
	}

	switch {
	case out.said:
	case out.listed == 0:
		reasons = append(reasons, view.Reason{Text: v.absent, Status: view.Info})
	case v.quiet && out.read > 0 && len(out.batteries) == 0:
		// Listed, opened and asked, and nothing came back: a mouse asleep in
		// its dock. Saying nothing made the vendor look absent; saying "no
		// Razer device" about a dock on the desk was the old reader's lie.
		reasons = append(reasons, view.Reason{Text: article(v.name) + " " + v.name + " device answered nothing", Status: view.Info})
	}
	return out, reasons, present, errors.Join(errs...)
}

// article is "an" before a vendor whose name starts with a vowel -- "an AULA
// device" -- and "a" before the rest.
func article(name string) string {
	if name != "" && strings.ContainsRune("AEIOUaeiou", rune(name[0])) {
		return "an"
	}
	return "a"
}

// PresentAsideAt is how many devices a card has to be drawing before a device
// that is present and unreadable stops taking a line of it (issue #77).
//
// Two, because that is a full card: one device drawn leaves room and a reader
// wondering where the headset went, and two means the card is doing its job
// and the rest is a footnote. The line is kept, as an aside, for the hover
// note and for doctor.
const PresentAsideAt = 2

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
  - A device that gave one before and has gone quiet keeps it, dim, for the
    rest of the session (spec 022). It used to be forgotten after ten
    minutes, and the card then collapsed to one cell and widened again when
    the device came back.
*/
func (p *Peripherals) readings(found []battery.Battery) view.PeripheralsReading {
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
			// The base station answered and the headset did not: the
			// device is off, and it is drawn dim with its last level, as a
			// device that stopped answering is (spec 022). Seen is when it
			// went off and stays there while it is off, so a device that
			// connects later out-ranks it for the slot.
			reading.Stale = true
			off := now
			if was.reading.Stale {
				off = was.reading.Seen
			}
			reading.Since, reading.Seen = was.since, off
			fresh[b.Name] = true
			p.seen[b.Name] = remembered{reading: reading, since: was.since}
			continue
		}

		// A device that was quiet on the last poll has just come back, which
		// is a change of state and a claim on the right-hand slot.
		since := now
		if was, ok := p.seen[b.Name]; ok && !was.reading.Stale {
			since = was.since
		}
		reading.Since, reading.Seen = since, now

		fresh[b.Name] = true
		p.seen[b.Name] = remembered{reading: reading, since: since}
	}

	var out []view.PeripheralReading
	for name, was := range p.seen {
		if !fresh[name] && was.reading.Stale && now.Sub(was.reading.Seen) > ForgetAfter {
			// Not heard for a week: put away, not asleep (spec 032).
			delete(p.seen, name)
			continue
		}
		if !fresh[name] && !was.reading.Stale {
			// Written back, so the poll it answers again in knows it was
			// away.
			was.reading.Stale = true
			p.seen[name] = was
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
func cells(in []battery.Cell) []view.PeripheralCell {
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
func kind(k battery.Kind) view.Kind {
	switch k {
	case battery.KindMouse:
		return view.KindMouse
	case battery.KindKeyboard:
		return view.KindKeyboard
	case battery.KindHeadset:
		return view.KindHeadset
	default:
		return view.KindOther
	}
}

// charge translates the reader's state into the view's.
func charge(s battery.State) view.Charge {
	switch s {
	case battery.Charging:
		return view.Filling
	case battery.Full:
		return view.Charged
	default:
		return view.Draining
	}
}

// addPresence sums two receivers' Presence, which is how a card with two
// receivers on it counts what is quiet across both.
//
// A paired child's node -- one device behind a receiver, with its own node --
// adds nothing to Quiet: the receiver's node asked every index, that device's
// among them, and counting it again put "8 indices" on a receiver that
// numbers six.
func addPresence(a, b logitech.Presence, child bool) logitech.Presence {
	a.Nodes += b.Nodes
	if !child {
		a.Quiet += b.Quiet
	}
	a.TooOld = append(a.TooOld, b.TooOld...)
	return a
}

/*
receiverReason is what a Logitech receiver that read nothing says about itself,
or nothing when no receiver was opened: an absent one is "no Logitech
receiver" already, and one that may not be opened says so by name.

Counted, and described as exactly what it is. Not named, because a pairing
table outlives the hardware in it: the receiver this was written against
carries a slot for a mouse its owner never had, and naming it would put a
device on the panel that was never on the desk.

And not called *paired* either. The receiver measured here answers the same
code for an empty slot as for a sleeping device, so a count of unanswered
indices is all this knows -- the first draft of this line said "8 paired
slots" on a receiver with two pairings (issue #66).
*/
func receiverReason(presence logitech.Presence) *view.Reason {
	switch {
	case presence.Nodes == 0:
		return nil
	case presence.Quiet > 0:
		return &view.Reason{
			Text: "a Logitech receiver, with nothing awake on it", Status: view.Info,
			Detail: fmt.Sprintf("%d %s asked and none answered; a sleeping device and an empty slot say the same thing",
				presence.Quiet, plural(presence.Quiet, "index", "indices")),
		}
	default:
		return &view.Reason{Text: "a Logitech receiver, with nothing paired to it", Status: view.Info}
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
