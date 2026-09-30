/*
Package peripherals reads the batteries of the devices on the desk.

Logitech is read by speaking HID++ to the receiver over hidraw, not by running
solaar. The CLI takes three and a half seconds to answer and the library it
wraps is Python; the protocol underneath both is a seven-byte request and a
seven- or twenty-byte reply, and it answers in about five milliseconds.
*/
package peripherals

import (
	"errors"
	"fmt"
	"io"
	"os"
	"time"
)

// The two HID++ report kinds. A request is sent as a short report; the reply
// may come back as either, which is the trap below.
const (
	reportShort = 0x10
	reportLong  = 0x11
)

/*
softwareID marks a reply as an answer to *this process's* request.

Devices echo it back in the low nibble of the function byte, and HID++ reserves
four bits of every request for it so that concurrent clients can tell their
answers apart. It used to be the constant 0x08, which separated hayami from
Solaar and did nothing at all between two hayamis -- and there are routinely
several: the panel polls the receiver every fifteen seconds while
`hayami-tui readings`, a pane, or `doctor` asks the same node. Two requests
agreeing on device index, feature index and function would then accept each
other's replies.

That is not theoretical. A phantom peripheral called "Q" appeared beside a real
mouse on the panel, at the same percentage: 81 is `chr('Q')`, so a battery
level had been decoded as a device name (issue #58).

Taken from the process ID because it has to differ between processes and
nothing else about it matters.

**Solaar's own ID is excluded.** It uses 0x0B -- `SOLAAR_SOFTWARE_ID` in
logitech_receiver/base.py -- and a value picked freely from 1..15 lands on it
one run in fifteen, at which point hayami and solaar accept each other's
replies. The constant this replaced was 0x08, which never collided with solaar;
narrowing the gap between two hayamis had quietly opened one against the tool
most likely to be running beside it. The live test caught it: hayami read 71 %
where solaar read 79 % in the same second, which is a reply belonging to
somebody else rather than a battery moving.

Two hayamis can still collide -- one chance in fourteen -- so this narrows the
window rather than closing it, and the name check in logitech.go is the other
half.
*/
var softwareID = softwareIDs[os.Getpid()%len(softwareIDs)]

// softwareIDs are the values this program will use: every ID a request may
// carry except zero, which marks a request as nobody's, and solaar's.
var softwareIDs = []byte{
	0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0A,
	// 0x0B is solaar's.
	0x0C, 0x0D, 0x0E, 0x0F,
}

// rootFeature is feature 0x0000, the one every HID++ 2.0 device has at index
// zero. Its function 0 maps a feature ID to that device's index for it.
const rootFeature = 0x00

// The battery features, newest first.
//
// 0x1004 is what a device with a fuel gauge reports. 0x1000 is the older one,
// still all an early wireless mouse has, and reading it costs one extra
// lookup on a device that has neither.
const (
	featureUnifiedBattery = 0x1004
	featureBatteryStatus  = 0x1000
)

// errUnknownFeature is a device answering that it has never heard of the
// feature asked about. Not a failure: it is how a device says it has no fuel
// gauge, and the caller tries the older feature next.
var errUnknownFeature = errors.New("the device does not have that feature")

// errNoDevice is an index with nothing paired to it. The receiver answers for
// it — immediately — so this ends a discovery rather than delaying one.
var errNoDevice = errors.New("no device at that index")

// errSilent is a device that did not answer within the deadline.
//
// It is a sentinel and not a plain timeout because it is the ordinary case: a
// wireless mouse that has gone to sleep says nothing at all, every poll, for
// as long as nobody touches it. Reported as a failure it would put a line in
// the log every fifteen seconds for a mouse that is merely idle, and the real
// failures would be lost among them. The section draws the device stale; the
// caller does not hear about it.
var errSilent = errors.New("the device did not answer")

// hidppError is the device refusing a request, with the code it refused with.
type hidppError struct {
	code byte
}

func (e hidppError) Error() string { return fmt.Sprintf("hid++ error 0x%02x", e.code) }

// HID++ 1.0 error codes, which the receiver answers with for an index that
// has nothing on it.
const (
	err10UnknownDevice = 0x08
	err10ConnectFail   = 0x09
)

// HID++ 2.0 error codes.
const err20UnsupportedFeature = 0x01

// endpoint is a hidraw node, or a test's stand-in for one.
//
// The deadline is part of the interface because it is part of the protocol as
// used here: a device that is asleep does not answer, and a panel that polls
// on a timer cannot wait for it.
type endpoint interface {
	io.ReadWriter
	SetReadDeadline(time.Time) error
}

// request sends one HID++ request and returns the reply's parameters.
//
// Three things about the reply are not obvious and each of them cost a probe
// to find out:
//
//   - **A short request may be answered with a long report.** The 0x1004 reply
//     on the machine this was written on comes back as 0x11. Matching on the
//     report ID drops the answer that was asked for, and the only symptom is a
//     timeout, which reads like absent hardware.
//   - **There are two error forms.** HID++ 2.0 answers with feature index
//     0xFF; HID++ 1.0 answers with sub-id 0x8F, and the receiver uses the 1.0
//     form to say that an index is empty. A reader that knows only the 2.0
//     form waits out its timeout on every unpaired index — six of them, and a
//     discovery that should take milliseconds takes seconds.
//   - **Other traffic shares the node.** The mouse's own notifications arrive
//     on it unasked, so a reply has to be recognised rather than merely
//     received.
func request(e endpoint, timeout time.Duration, device, feature, function byte, params ...byte) ([]byte, error) {
	var err error
	for attempt := 0; attempt < requestAttempts; attempt++ {
		var out []byte
		out, err = attemptRequest(e, timeout, device, feature, function, params...)
		if !errors.Is(err, errSilent) {
			return out, err
		}
		// Silence is the one answer worth asking again for. An error reply is
		// an answer, and asking an empty index twice would double the cost of
		// a discovery to learn nothing.
	}
	return nil, err
}

// requestAttempts is how many times a request is sent before the device is
// taken to be silent.
//
// **Five, because one is not nearly enough.** Measured on the receiver this
// was written against, and the two measurements say different things:
//
//   - A mouse in continuous use answers a single request fourteen times in
//     twenty; a second attempt carries it to twenty.
//   - A mouse left alone for six seconds — which is less than half a poll
//     interval — takes up to *four* attempts. The first requests wake it and
//     are lost, which is the ordinary state of a wireless peripheral and not
//     a fault.
//
// None of this is visible from the protocol, and every test in this package
// that drives a fake endpoint passes at one attempt. At one attempt the panel
// would have shown a mouse flapping between a reading and "not answering" on
// most polls, on a desk where nothing was wrong.
//
// Only silence is retried, so this costs nothing on an index that is empty:
// the receiver refuses those, and a refusal is an answer.
const requestAttempts = 5

// attemptRequest sends one request and waits once for its reply.
func attemptRequest(e endpoint, timeout time.Duration, device, feature, function byte, params ...byte) ([]byte, error) {
	out := make([]byte, 7)
	out[0] = reportShort
	out[1] = device
	out[2] = feature
	out[3] = function<<4 | softwareID
	copy(out[4:], params)

	if _, err := e.Write(out); err != nil {
		return nil, fmt.Errorf("sending a hid++ request: %w", err)
	}

	deadline := time.Now().Add(timeout)
	if err := e.SetReadDeadline(deadline); err != nil {
		return nil, fmt.Errorf("setting a deadline on the hid++ endpoint: %w", err)
	}

	buf := make([]byte, 64)
	for time.Now().Before(deadline) {
		n, err := e.Read(buf)
		if errors.Is(err, os.ErrDeadlineExceeded) {
			return nil, errSilent
		}
		if err != nil {
			return nil, fmt.Errorf("reading a hid++ reply: %w", err)
		}
		params, matched, err := reply(buf[:n], device, feature, out[3])
		if matched {
			return params, err
		}
	}
	return nil, fmt.Errorf("device %d: %w within %s", device, errSilent, timeout)
}

// reply reads one report, saying whether it answers the request at all.
//
// The middle return is what makes this readable: a report that is not an
// answer is neither a result nor an error, it is somebody else's traffic, and
// the caller goes back to waiting.
func reply(r []byte, device, feature, function byte) (params []byte, matched bool, err error) {
	if len(r) < 4 || r[1] != device {
		return nil, false, nil
	}
	if r[0] != reportShort && r[0] != reportLong {
		return nil, false, nil
	}

	// HID++ 1.0 error: sub-id 0x8F, then the sub-id and address that failed
	// and the code. The receiver answers this way for an empty index.
	if r[2] == 0x8F {
		if len(r) < 6 {
			return nil, false, nil
		}
		return nil, true, translate(r[5])
	}

	// HID++ 2.0 error: feature index 0xFF, the failing function, then the code.
	if r[2] == 0xFF {
		if len(r) < 5 {
			return nil, false, nil
		}
		return nil, true, translate(r[4])
	}

	if r[2] != feature || r[3] != function {
		return nil, false, nil
	}
	return r[4:], true, nil
}

// translate names the error codes this package treats as answers rather than
// as failures.
func translate(code byte) error {
	switch code {
	case err10UnknownDevice, err10ConnectFail:
		return errNoDevice
	case err20UnsupportedFeature:
		return errUnknownFeature
	default:
		return hidppError{code: code}
	}
}

// featureIndex asks a device which of its own indices holds a feature.
//
// Index zero is the answer for a feature the device does not have, which is
// the 2.0 way of saying no and is not an error report.
func featureIndex(e endpoint, timeout time.Duration, device byte, feature uint16) (byte, error) {
	params, err := request(e, timeout, device, rootFeature, 0x00, byte(feature>>8&0xFF), byte(feature&0xFF), 0x00)
	if err != nil {
		return 0, err
	}
	if len(params) < 1 || params[0] == 0 {
		return 0, errUnknownFeature
	}
	return params[0], nil
}
