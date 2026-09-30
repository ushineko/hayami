package peripherals

import (
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"
)

// RequestTimeout is how long one HID++ request may take.
//
// Generous against what a request costs — a reply arrives in about five
// milliseconds, and an empty index is refused in about one — because it is not
// what discovery runs on. It is the backstop for a device that is asleep and
// answers nothing at all, and a panel polling on a timer cannot wait longer
// than this for one that never will.
const RequestTimeout = 300 * time.Millisecond

// deviceIndices are the indices a receiver can hold. A receiver pairs at most
// six devices and answers for every index, so all six are asked once and the
// ones that answer are kept.
var deviceIndices = []byte{1, 2, 3, 4, 5, 6}

// wiredIndex is the index a device plugged in by its own cable answers on,
// where there is no receiver to number it.
const wiredIndex = 0xFF

// Logitech reads Logitech batteries over HID++.
//
// It remembers what it found. Discovery is cheap but not free, and the node
// and index of a mouse that is on the desk do not change between polls; they
// change when it is unplugged, and the reading failing is how that is noticed.
type Logitech struct {
	// open is how an endpoint is obtained, replaced by a test so the suite
	// touches no device.
	open func(path string) (endpoint, error)

	// known is what the last poll found: a node, and the indices on it that
	// answered. Empty until the first poll, and emptied when a poll finds
	// nothing there any more.
	known []located

	// timeout is how long one request may take, a field rather than the
	// constant so a test can exercise the bound without waiting for it.
	timeout time.Duration

	// presence is what the last discovery learned besides the batteries.
	presence Presence
}

/*
Presence is what is on the Logitech hardware beyond what could be read.

An empty result used to mean "no Logitech receiver", and on a machine with a
receiver, a ten-year-old keyboard and a leftover pairing slot it meant three
other things instead (issue #66). Each of them is a different sentence and a
different thing for a reader to do.
*/
type Presence struct {
	// Nodes is how many hidraw endpoints speak HID++ at all. Zero is the only
	// state that honestly reads as "no receiver".
	Nodes int

	// Quiet is how many paired indices did not answer. **Not named**, on
	// purpose: a pairing table outlives the hardware in it, and a receiver
	// that has been round a few machines carries slots for devices that were
	// never on this desk. A count says something true; a name would not.
	Quiet int

	// TooOld are devices that answered and do not speak HID++ 2.0, by the name
	// the kernel gives their node. These are named because they answered --
	// something is there.
	TooOld []string
}

// Presence is what the last poll found besides batteries.
func (l *Logitech) Presence() Presence { return l.presence }

// located is one device, where it was found.
type located struct {
	node  string
	index byte

	// old marks a device that answered that it does not know HID++ 2.0 at all.
	// Its battery is a register rather than a feature (spec 018).
	old bool

	// name is what to call it if even the register will not read, and empty
	// where the node cannot lend a device a name of its own -- see the
	// exclusions where this is set.
	name string
}

// NewLogitech builds the reader.
func NewLogitech() *Logitech {
	return &Logitech{open: openHidraw, timeout: RequestTimeout}
}

// openHidraw opens a node for reading and writing.
//
// Read *and* write: HID++ is a conversation, and a node that cannot be written
// to is no use. Where the seat's ACL does not grant it, this fails and the
// caller reports no Logitech device — which is a permission problem wearing
// the clothes of absent hardware, so the error says which node it was.
func openHidraw(path string) (endpoint, error) {
	f, err := os.OpenFile(path, os.O_RDWR, 0) //nolint:gosec // a hidraw node the kernel named
	if err != nil {
		return nil, fmt.Errorf("opening %s: %w", path, err)
	}
	return f, nil
}

// Batteries reads every Logitech device that answers.
//
// A device that is paired but asleep answers nothing, and is absent from the
// result rather than being reported at zero. The section above decides what to
// do about a device that was here last time and is not now; that is not a
// question this layer can answer, because it does not remember what a panel
// has drawn.
func (l *Logitech) Batteries() ([]Battery, error) {
	where, err := l.locate()
	if err != nil {
		return nil, err
	}

	var (
		found []Battery
		still []located
		errs  []error
	)
	unread := map[string]bool{}

	for _, loc := range where {
		e, err := l.open(loc.node)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		b, err := read(e, l.timeout, loc)
		closeEndpoint(e)
		if err != nil && loc.old && loc.name != "" {
			// A device from before the feature protocol whose register would
			// not read either. It is there and it is not a reading, and the
			// section says so by name rather than by silence.
			unread[loc.name] = true
		}
		if err != nil {
			// The device has gone quiet, or gone. Either way its index is not
			// worth remembering; the next poll rediscovers.
			// errOldProtocol among them: a register read that comes back
			// "invalid sub-id" is a device from before this protocol refusing
			// the question, which is an answer and not a fault. It is named
			// above instead.
			if !errors.Is(err, errNoDevice) && !errors.Is(err, errUnknownFeature) &&
				!errors.Is(err, errSilent) && !errors.Is(err, errOldProtocol) {
				errs = append(errs, err)
			}
			continue
		}
		found = append(found, b)
		still = append(still, loc)
	}

	l.known = still
	l.presence.TooOld = names(unread)
	return found, errors.Join(errs...)
}

// locate is where the devices are, discovering only when it has to.
func (l *Logitech) locate() ([]located, error) {
	if len(l.known) > 0 {
		return l.known, nil
	}
	return l.discover()
}

// discover asks every index on every HID++ node which of them hold a device.
//
// Every index answers — a paired one with a feature index, an empty one with a
// HID++ 1.0 error — so this costs milliseconds rather than the timeout, and
// only because [reply] knows both error forms. A reader that knew only the 2.0
// form would sit out RequestTimeout on each empty index instead.
func (l *Logitech) discover() ([]located, error) {
	nodes, err := hidppNodes()
	if err != nil {
		return nil, err
	}

	var found []located
	presence := Presence{Nodes: len(nodes)}
	for _, node := range nodes {
		e, err := l.open(node.Path)
		if err != nil {
			// One unreadable node does not stop the others being asked.
			continue
		}
		for _, index := range append([]byte{wiredIndex}, deviceIndices...) {
			_, err := featureIndex(e, l.timeout, index, featureUnifiedBattery)
			switch {
			case err == nil, errors.Is(err, errUnknownFeature):
				// There, and either with a fuel gauge or with the older
				// feature read() falls back to.
				found = append(found, located{node: node.Path, index: index})
			case errors.Is(err, errOldProtocol) && index != wiredIndex && pairedDevice(node.Phys):
				// A *paired device* answered, and answered that it has no
				// features at all.
				// Its battery is a HID++ 1.0 register, which read() asks for
				// instead (spec 018).
				//
				// Two exclusions, and the real hardware taught both.
				//
				// Index 0xFF addresses the thing being spoken to, and a
				// Unifying receiver is a 1.0 device by construction with no
				// battery of its own -- reading it asked a receiver for a
				// register it does not have, and reporting it made every
				// machine with one claim an unreadable device two lines under
				// a mouse that was drawing fine.
				//
				// And a receiver node answers for every device paired to it,
				// so what arrives there carries the receiver's name and not
				// the device's: the keyboard was reported twice, once
				// correctly and once as "Logitech USB Receiver".
				//
				// A device wired in by its own cable would answer on 0xFF and
				// is missed by this. None is to hand to check against, and a
				// missed reading is the better of the two mistakes.
				found = append(found, located{
					node: node.Path, index: index, old: true, name: node.Name,
				})

			case errors.Is(err, errNotReachable):
				presence.Quiet++
			}
		}
		closeEndpoint(e)
	}

	l.presence = presence
	return found, nil
}

// names is a set of device names, in a stable order so a panel's lines do not
// change places between two polls.
func names(set map[string]bool) []string {
	if len(set) == 0 {
		return nil
	}
	out := make([]string, 0, len(set))
	for name := range set {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// read takes one device's battery, in whichever protocol it speaks.
//
// A device from before the feature protocol has no 0x0005 to ask its name of
// either, so it keeps the one the kernel gave its node.
func read(e endpoint, timeout time.Duration, loc located) (Battery, error) {
	if loc.old {
		b, err := readOldBattery(e, timeout, loc.index)
		if err != nil {
			return Battery{}, err
		}
		b.Name = loc.name
		if b.Name == "" {
			b.Name = defaultName
		}
		return b, nil
	}

	b, err := readUnified(e, timeout, loc.index)
	if errors.Is(err, errUnknownFeature) {
		b, err = readStatus(e, timeout, loc.index)
	}
	if err != nil {
		return Battery{}, err
	}

	b.Name, b.Kind = identify(e, timeout, loc.index)
	return b, nil
}

// identify asks the device what it is called and what it is.
//
// Both come from feature 0x0005, which is looked up once and used twice: the
// name is a label and the kind is where the cell goes, and neither is worth a
// second round trip to the receiver for.
func identify(e endpoint, timeout time.Duration, index byte) (string, Kind) {
	feature, err := featureIndex(e, timeout, index, featureDeviceName)
	if err != nil {
		return defaultName, KindOther
	}
	return name(e, timeout, index, feature), kind(e, timeout, index, feature)
}

// readUnified reads feature 0x1004.
func readUnified(e endpoint, timeout time.Duration, index byte) (Battery, error) {
	feature, err := featureIndex(e, timeout, index, featureUnifiedBattery)
	if err != nil {
		return Battery{}, err
	}
	params, err := request(e, timeout, index, feature, 0x01)
	if err != nil {
		return Battery{}, err
	}
	return decodeUnifiedBattery(params)
}

// readStatus reads feature 0x1000, for a device without a fuel gauge.
func readStatus(e endpoint, timeout time.Duration, index byte) (Battery, error) {
	feature, err := featureIndex(e, timeout, index, featureBatteryStatus)
	if err != nil {
		return Battery{}, err
	}
	params, err := request(e, timeout, index, feature, 0x00)
	if err != nil {
		return Battery{}, err
	}
	return decodeBatteryStatus(params)
}

// name asks the device what it is called.
//
// A device that will not say is not a failure. The name is a label, and a row
// labelled "Logitech" that carries the right number is better than no row —
// but it is *not* used to tell devices apart, which is why a blank one here
// does not stop the reading.
func name(e endpoint, timeout time.Duration, index, feature byte) string {
	params, err := request(e, timeout, index, feature, functionDeviceName)
	if err != nil || len(params) < 1 {
		return defaultName
	}

	length := int(params[0])
	if length <= 0 || length > maxNameLength {
		return defaultName
	}

	out := make([]byte, 0, length)
	for len(out) < length {
		// The offset fits a byte because length is bounded by maxNameLength
		// above, which is well under one.
		offset := byte(len(out) & 0xFF)
		chunk, err := request(e, timeout, index, feature, functionDeviceNameChunk, offset)
		if err != nil || len(chunk) == 0 {
			break
		}
		out = append(out, chunk...)
	}

	/*
		**A name has to arrive whole, and be a name all the way through.**

		The device declares its own length before sending any of it, so a reply
		that does not fill that length did not belong to this request -- and
		the printable-byte filter is happy to turn one stray byte into a
		plausible label. That is how a battery level of 81 became a peripheral
		called "Q" on a panel, beside the mouse it had been read from:
		`chr(81)`, drawn as confidently as the real name next to it (issue #58).

		Counting the bytes is not enough on its own, because a short reply
		arrives in a report padded with zeroes and a few of those make it look
		long enough. What a name cannot survive is a hole: the declared run has
		to be printable from end to end, which the real thing is and a stray
		byte followed by padding is not.

		defaultName is the honest answer. A device whose name will not read is
		still a battery worth drawing, and an unnamed one says so rather than
		inventing something that looks right.
	*/
	if len(out) < length {
		return defaultName
	}
	name, whole := printableName(out[:length])
	if !whole {
		return defaultName
	}
	// Trimmed only for display: the check above is against the declared run,
	// and a device whose name really does end in a space should keep its name.
	if trimmed := strings.TrimSpace(name); trimmed != "" {
		return trimmed
	}
	return defaultName
}

// featureDeviceName is feature 0x0005, which carries the name the device calls
// itself — "G502 X PLUS" rather than "Logitech USB Receiver", which is all the
// kernel knows — and, on its third function, what sort of device it is.
const featureDeviceName = 0x0005

// The functions of feature 0x0005: the name's length, a chunk of the name at an
// offset, and the device type.
const (
	functionDeviceName      = 0x00
	functionDeviceNameChunk = 0x01
	functionDeviceType      = 0x02
)

// The device types feature 0x0005 reports, as Logitech numbers them.
//
// Named rather than inline because the mapping below is the only thing in this
// program that has an opinion about what a trackball is, and a reader checking
// it against the protocol should not have to count the constants.
const (
	typeKeyboard  = 0x00
	typeNumpad    = 0x02
	typeMouse     = 0x03
	typeTouchpad  = 0x04
	typeTrackball = 0x05
)

// kind asks the device what sort of device it is.
//
// A device that will not say is KindOther, which orders it after the ones that
// did. This is a label like the name and not part of the reading: it is not
// worth failing a battery over, and a mouse whose type request was lost is
// still a mouse with a percentage.
func kind(e endpoint, timeout time.Duration, index, feature byte) Kind {
	params, err := request(e, timeout, index, feature, functionDeviceType)
	if err != nil || len(params) < 1 {
		return KindOther
	}

	switch params[0] {
	case typeMouse, typeTouchpad, typeTrackball:
		return KindMouse
	case typeKeyboard, typeNumpad:
		return KindKeyboard
	default:
		// A remote, a presenter, the receiver itself, and anything Logitech
		// has numbered since. None of them is a device this panel orders
		// specially.
		return KindOther
	}
}

// defaultName is what a device that will not say is called.
const defaultName = "Logitech"

// maxNameLength bounds the name a device may claim to have. The protocol hands
// over a length and then that many bytes in chunks, and a device answering
// with nonsense should cost one refused read rather than a loop.
const maxNameLength = 64

// printableName keeps a name's printable bytes and reports whether *every*
// byte was one.
//
// The two answers are separate because they are asked for different reasons.
// A control character is dropped rather than drawn, since one in a label would
// move the column it sits in. But a hole in the declared run is the mark of a
// reply that did not belong to this request, and only the caller comparing
// against the declared length can see that -- so the fact is handed back
// rather than quietly repaired.
func printableName(b []byte) (string, bool) {
	out := make([]rune, 0, len(b))
	for _, c := range b {
		if c >= 0x20 && c < 0x7F {
			out = append(out, rune(c))
		}
	}
	return string(out), len(out) == len(b)
}

// closeEndpoint closes an endpoint that can be closed.
//
// The interface does not require Close, because a test's stand-in has nothing
// to close; a real hidraw node does, and leaving it open would hold the
// descriptor until the panel exits.
func closeEndpoint(e endpoint) {
	if c, ok := e.(io.Closer); ok {
		_ = c.Close()
	}
}
