package view

import (
	"cmp"
	"slices"
	"strconv"
	"strings"
	"time"
)

// The battery bands, in percent.
//
// The reference's, and round numbers because a battery has no alarm to measure
// one from — unlike the coolant, whose bands in spec 006 are the pump's own.
// They are here rather than inline so it is plain they are a convention and
// not a measurement.
const (
	BatteryLow      = 50
	BatteryCritical = 20
)

// Discharging is what a battery that is merely running says.
//
// Named because it is the one state that is also the default, and the one
// place that needs to recognise it -- a device already listing its separate
// batteries has no width left for a word that says nothing had happened.
const Discharging = "Discharging"

// Charge is what a peripheral's battery is doing, in the view's own words.
//
// The view does not import the reader, so the state crosses as this rather
// than as the reader's type. It is three cases and not a string, so a spelling
// cannot arrive here that the panel does not draw.
type Charge int

const (
	// Draining is the ordinary case and is not said.
	Draining Charge = iota
	// Filling is on the cable.
	Filling
	// Charged is done filling and still on the cable.
	Charged
)

/*
Kind is what sort of device a reading belongs to, in the view's own words.

It is here because the order of the cells is a property of the view, and it
crosses as this rather than as the reader's type for the reason Charge does:
the view does not import the reader.

It carries no other meaning. A cell does not say what sort of device it is —
the name already does, and better — so this is read by the sort and by nothing
else that draws.
*/
type Kind int

const (
	// KindOther is the zero value: a device whose sort is not known, or is
	// known and is none of the below.
	KindOther Kind = iota
	// KindMouse is the pointing device on the desk.
	KindMouse
	// KindKeyboard is a keyboard or a numpad.
	KindKeyboard
	// KindHeadset is a headset, headphones or earbuds.
	KindHeadset
)

/*
Rank is where a kind's cells go, lowest first.

**The mouse is first.** A desk has one, it is there whenever the machine is,
and its battery is the one a glance at this panel is usually after; the things
that come and go should come and go around it rather than in front of it.
Ordering by name alone put "Arctis Nova Pro Wireless" in the first cell on the
machine this was written on, which is the device its owner thinks about least.

Then the keyboard, for the same reason and one step weaker, then the headset,
then everything else. Within a kind it is still by name, so a cell moves only
when the hardware does.
*/
func (k Kind) Rank() int {
	switch k {
	case KindMouse:
		return 0
	case KindKeyboard:
		return 1
	case KindHeadset:
		return 2
	default:
		return 3
	}
}

// PeripheralReading is one device, measured and not yet formatted.
//
// Every reading has a level, because a cell without one has nothing to draw.
// The panel is what guarantees it: see PeripheralsReading.
type PeripheralReading struct {
	// Name is the device's own name, which is the cell's label.
	Name string

	// Level is a percentage. Zero and meaningless for a device that reports a
	// band instead.
	Level int

	// Band is how full a device without a fuel gauge says it is, in the four
	// steps such a device knows, and Segments is how many of four that fills.
	// Zero means this cell has a percentage.
	//
	// **Never converted into a percentage.** A device saying "good" does not
	// mean 75 %; the cell draws the four steps where a number would go, which
	// is how the device's own indicator shows it (spec 018).
	Band     string
	Segments int

	Charge Charge

	// Kind orders the cells and is drawn nowhere.
	Kind Kind

	// Stale marks a device that answered before and did not this time. Its
	// last level is kept and the cell is drawn dim, which is what the monitor
	// does with a second, darker palette and the word "(Offline)": the number
	// stays legible and stops asserting itself.
	Stale bool

	// Since is when this device was last detected after not being there, and
	// Seen is when it last answered. Both are set by the panel, which is the
	// only layer that watches devices come and go.
	//
	// They exist for SelectPeripherals and are read by nothing that draws. A
	// panel with two slots has to choose which of three headsets gets the
	// second one, and "the one you just switched on" is the only answer that
	// is ever right -- name and kind cannot tell a headset put on from a
	// headset in a drawer.
	Since time.Time
	Seen  time.Time

	// Cells are the separate batteries inside a device that has more than
	// one, already named and in the order to draw them. Empty for the
	// ordinary device with a single battery.
	//
	// Level is the row's number and these are the line beneath it: a pair of
	// earbuds is one device on the desk and should be one row on the panel,
	// but which ear is low is exactly what the wearer wants to know.
	Cells []PeripheralCell
}

// PeripheralCell is one battery inside a device: an earbud, a case.
type PeripheralCell struct {
	// Name is short because these sit several to a line: "L", "R", "case".
	Name  string
	Level int
}

/*
PeripheralsReading is every device worth a cell, in the order to draw them.

**A device this panel has never had a level from is not a cell.** A headset
whose receiver is plugged in with the headset switched off is reported as
present and silent, and drawn it is a name, a dash and a word explaining that
there is nothing to say — a third of a panel 260 pixels wide spent on the
absence of a fact, and, once the cells are ordered, spent in front of the
mouse. The design system's own degradation model has a name for this case:
"source never present: not a Reading at all."

**A device that has answered and has gone quiet is still a cell**, dim, with
the last level it gave. That is the next case in the same model — "was read,
source has gone" — and it is what the monitor does. It matters most for the
device most likely to go quiet: a wireless mouse that has been still for a
minute answers nothing, which is a mouse idle and not a mouse gone.
*/
type PeripheralsReading struct {
	Devices []PeripheralReading
}

/*
Peripherals turns a reading into a section of cells.

A cell per device: the name over the level, the level large and in the middle,
and what the battery is doing under it. The monitor's shape, and the right one
for a battery -- the number *is* the reading and the name is only which one.

This replaces a row per device, which was argued for here and was wrong. What
that argument got right is kept and is worth restating: a cell per device,
appearing and disappearing with the hardware, rather than the monitor's two
fixed slots each pointed at a device type through a submenu. The slots were an
artefact of a panel 260 pixels wide and are not something a reader ever asked
for. The *row* was the part that did not survive being looked at beside the
program it replaces.

Charging is **said and not coloured**. A device on its cable is not a warning
however empty it is -- it is being dealt with -- and colouring it would put a
red cell on the panel for the one battery nobody needs to think about.
*/
func Peripherals(r PeripheralsReading) Section {
	s := Section{Key: "peripherals", Title: "Peripherals", Icon: IconPeripherals}
	shown, overflow := SelectPeripherals(r.Devices)
	for _, d := range shown {
		s.Cells = append(s.Cells, peripheral(d))
	}
	// The card keeps its shape with nothing behind a slot (spec 022). A card
	// that collapsed to one centred cell when the headset was forgotten, and
	// widened again when it came back, reflowed the panel over a battery.
	for i := len(s.Cells); i < PeripheralSlots; i++ {
		s.Cells = append(s.Cells, placeholder(i))
	}
	s.Note = overflowNote(overflow)
	return s
}

// The placeholders' labels, by slot: the left slot is the mouse's.
const (
	NoMouse  = "no mouse"
	NoDevice = "no device"
)

/*
placeholder is the cell for a slot with no device behind it.

Drawn dim, like a device that has gone quiet, and with no reading, so the card
is the same shape with one device as with two and says which slot is empty.
It is not a reason: nothing is wrong, and doctor does not report it.

Only the right slot is empty on a desk with a mouse. The left one says "no
mouse" only when there is nothing at all, because without a mouse the left
slot goes to the next device rather than staying empty (see SelectPeripherals).
*/
func placeholder(slot int) Cell {
	label := NoDevice
	if slot == 0 {
		label = NoMouse
	}
	return Cell{Label: label, Value: NoQuantity(), Stale: true, Placeholder: true}
}

// overflowNote names the devices there was no slot for, most recent first.
//
// A line each rather than a count: "2 more" says a number is missing and not
// which, and the reader asking is asking about a particular pair of
// headphones. The state is said too, because a device in this list is one the
// panel is not drawing and the whole of what it would have drawn is a level
// and a state.
func overflowNote(overflow []PeripheralReading) string {
	if len(overflow) == 0 {
		return ""
	}
	lines := make([]string, 0, len(overflow)+1)
	lines = append(lines, "Also connected:")
	for _, d := range overflow {
		lines = append(lines, "  "+d.Name+"  "+strings.TrimSpace(Count(d.Level))+" %  "+chargeNote(d))
	}
	return strings.Join(lines, "\n")
}

/*
OrderPeripherals puts devices in the order their cells are drawn: by kind, and
by name within a kind. See Kind.Rank for why the mouse is first.

The rule is here and not in the panel, because which cell comes first is a
decision about what the section says and this package is where those are made.
It is applied by the panel rather than by Peripherals, because the panel's
reading is also the JSON the command line prints, and one ordered section
beside an unordered dump of the same devices is two answers to one question.
*/
func OrderPeripherals(devices []PeripheralReading) []PeripheralReading {
	out := slices.Clone(devices)
	slices.SortFunc(out, func(a, b PeripheralReading) int {
		if n := cmp.Compare(a.Kind.Rank(), b.Kind.Rank()); n != 0 {
			return n
		}
		return cmp.Compare(a.Name, b.Name)
	})
	return out
}

// PeripheralSlots is how many devices the section draws.
//
// **Two, always the same two places.** A cell per device was honest and it
// made the card breathe: plug a second pair of headphones in and the card grew
// a third of its width, everything beside it moved, and the panel the eye had
// learned was a different panel. A battery reading is glanced at, and a glance
// wants the number to be where it was last time more than it wants every
// number at once.
const PeripheralSlots = 2

/*
SelectPeripherals cuts the devices down to the two the section draws, and
returns the rest.

**The mouse holds the left slot**, for the reason Kind.Rank gives: a desk has
one, it is there whenever the machine is, and it is what the panel is usually
being asked about. It holds the slot even when it has gone quiet, because a
mouse idle is not a mouse gone and moving the pointer brings it straight back
-- a slot that emptied every time the hand left the desk would be the same
flicker in a smaller place. Without a mouse the slots are filled from the rest,
as they always were.

**The right slot is the device whose state changed last**, whether that change
was arriving or going quiet (spec 022). A headset switched off keeps the slot,
dim, with its last level; a pair of earbuds connected after that takes it.
Live and quiet are not ranked against each other: the reader's most recent
change is what the slot shows.

Everything beyond the two is returned as overflow, in the same order, for the
caller to say somewhere that does not take space on the card.
*/
func SelectPeripherals(devices []PeripheralReading) (shown, overflow []PeripheralReading) {
	rest := slices.Clone(devices)

	// The mouse is taken out first so the ordering below never has to make an
	// exception for it.
	var left []PeripheralReading
	if i := slices.IndexFunc(rest, func(d PeripheralReading) bool { return d.Kind == KindMouse }); i >= 0 {
		left = append(left, rest[i])
		rest = slices.Delete(rest, i, i+1)
	}

	slices.SortStableFunc(rest, byRecentChange)

	take := min(PeripheralSlots-len(left), len(rest))
	return append(left, rest[:take]...), rest[take:]
}

/*
byRecentChange puts the device whose state changed most recently first.

Recency is Since for a live device -- when it arrived -- and Seen for a quiet
one -- when it was last heard, which is when it went quiet. They are different
questions and the same intent: the device whose state changed last is the one
the reader changed.

The name breaks a tie so a panel with two devices detected in the same poll
does not swap them between polls.
*/
func byRecentChange(a, b PeripheralReading) int {
	if n := changed(b).Compare(changed(a)); n != 0 {
		return n
	}

	// Two devices detected in the same poll -- everything already on at
	// startup -- have nothing to separate them in time, and Kind.Rank is the
	// same judgement the whole section is ordered by. Then the name, so the
	// slot does not swap between polls.
	if n := cmp.Compare(a.Kind.Rank(), b.Kind.Rank()); n != 0 {
		return n
	}
	return cmp.Compare(a.Name, b.Name)
}

// changed is when a device's state last changed: when it arrived if it is
// answering, when it was last heard if it is not.
func changed(d PeripheralReading) time.Time {
	if d.Stale {
		return d.Seen
	}
	return d.Since
}

// peripheral is one device's cell.
func peripheral(d PeripheralReading) Cell {
	if d.Segments > 0 {
		return bandCell(d)
	}

	cell := Cell{
		Label: d.Name, Unit: "%", Note: note(d),
		Value: strings.TrimSpace(Count(d.Level)), Stale: d.Stale,
		Bar: float64(d.Level) / 100, HasBar: true,
	}

	// The verdict is kept on a stale cell rather than dropped. The shells dim
	// it, which is the whole of what "this is the last number heard" needs to
	// say; blanking the verdict as well would take a low battery's colour away
	// at the moment it is least likely to be charged.
	switch {
	case d.Charge != Draining:
		cell.Status = Info
	case d.Level <= BatteryCritical:
		cell.Status = Bad
	case d.Level <= BatteryLow:
		cell.Status = Warn
	default:
		cell.Status = Good
	}
	return cell
}

// BandSegments is how many steps a device without a gauge reports in, and so
// how many the cell draws.
const BandSegments = 4

/*
The glyphs a band is drawn with: filled and empty.

An outline rather than a gap for the empty one, so the four steps stay visible
as four and a reader can see how much is missing as well as how much is left --
which is what the device's own indicator does.

They are the same width. The first photograph of this made the empty segment
look like a sliver and it was read as a font problem; measured in the panel's
own face both glyphs advance identically, and what the eye had picked up was an
outline sitting beside filled bars, which is the point of it.
*/
const (
	SegmentFull  = '▮'
	SegmentEmpty = '▯'
)

/*
bandCell draws a device that reports a band rather than a percentage.

The segments go where the number goes, so a row of cells still lines up and the
eye lands in the same place; the band's word takes the quiet line, where a
percentage cell says "Discharging". A band device is then obviously not a
measured one without a caption having to say so, and nothing invents a figure
the device never gave (spec 018).

Charging wins the quiet line when it applies: it is the more urgent fact, and
the segments already carry the band.
*/
func bandCell(d PeripheralReading) Cell {
	cell := Cell{
		Label: d.Name,
		Value: segments(d.Segments),
		Note:  d.Band,
		Stale: d.Stale,
	}
	if d.Stale || d.Charge != Draining {
		cell.Note = chargeNote(d)
	}

	// The same thresholds a percentage cell uses, applied to what exists: the
	// lowest step is critical and the one above it is low.
	switch {
	case d.Charge != Draining:
		cell.Status = Info
	case d.Segments <= 1:
		cell.Status = Bad
	case d.Segments == 2:
		cell.Status = Warn
	default:
		cell.Status = Good
	}
	return cell
}

// segments draws n of BandSegments filled.
func segments(n int) string {
	out := make([]rune, 0, BandSegments)
	for i := range BandSegments {
		if i < n {
			out = append(out, SegmentFull)
			continue
		}
		out = append(out, SegmentEmpty)
	}
	return string(out)
}

/*
note is the line under a cell's reading, and there is always one.

The row form said this only for a battery that was charging, which left the
ordinary case -- a battery discharging normally -- looking exactly like a
device nobody had heard from. The monitor says "Discharging", "Wired",
"Charging" or "Disconnected" under every cell and it is right to: the state is
a third of what a cell is for, and a blank third reads as a cell that has not
finished loading.

The separate batteries of a device that has several come first when there are
any: for a pair of earbuds they *are* the detail, and the state is said beside
them rather than instead of them -- except when the state is the ordinary one.
"L 80  R 90  case 50  Discharging" does not fit the width of a cell and the
last word of it is the one worth least: a battery that is going down is what a
battery does, and for this device the three numbers are the reading.
*/
func note(d PeripheralReading) string {
	state := chargeNote(d)

	if cells := cellNote(d.Cells); cells != "" {
		if state != "" && state != Discharging {
			return cells + "  " + state
		}
		return cells
	}
	return state
}

// chargeNote is what the cell says about what the battery is doing.
//
// A device that has gone quiet says so instead of saying what its battery was
// doing when it last spoke, because that is no longer the question. The
// monitor writes "(Offline)" in the same place and for the same reason; the
// brackets are its own and are not carried, since nothing else on this panel
// is bracketed.
func chargeNote(d PeripheralReading) string {
	if d.Stale {
		return "Offline"
	}
	switch d.Charge {
	case Filling:
		return "Charging"
	case Charged:
		return "Charged"
	default:
		return Discharging
	}
}

// cellNote lists the separate batteries: "L 100  R 100  case 80".
func cellNote(cells []PeripheralCell) string {
	var parts []string
	for _, c := range cells {
		parts = append(parts, c.Name+" "+strconv.Itoa(c.Level))
	}
	return strings.Join(parts, "  ")
}
