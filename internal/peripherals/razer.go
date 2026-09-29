package peripherals

import (
	"errors"
	"fmt"
	"os"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"
)

/*
Razer batteries, through whatever is in front of them.

**The mouse is not the device this talks to.** On the desk spec 016 was written
for, a Razer mouse sits on a Mouse Dock Pro and never enumerates at all: the
dock is the receiver, its `input0` carries `mouse0`, and `lsusb` shows the dock
alone. OpenRazer sees the same thing and offers no battery for it, because its
accessory driver has none -- `charge_level` lives in the mouse driver, and no
mouse is bound.

So the mouse is asked through the dock's RF relay, which is a transaction ID
and nothing more exotic. What comes back is the mouse's own answer: a serial
that is not the dock's, a real DPI, and Razer's 25 % low-battery default.

Only getters are sent. The class and command values below are OpenRazer's, read
from its driver source; nothing here writes a setting to a device.
*/

// The Razer report protocol, which is 90 bytes with a checksum.
const (
	// razerReportSize is the report, without the leading report number.
	razerReportSize = 90

	// relayTransaction addresses a device behind a dock rather than the dock
	// itself. OpenRazer's unmerged dock support uses the same value.
	relayTransaction = 0x1F

	// The classes and commands. 0x07 is power; 0x00 is the standard class
	// every Razer device answers.
	classPower    = 0x07
	classStandard = 0x00

	commandBatteryLevel = 0x80
	commandCharging     = 0x84
	commandSerial       = 0x82
)

// Razer's status byte, which is the difference between "no reading" and
// "broken".
const (
	statusBusy         = 0x01
	statusOK           = 0x02
	statusFail         = 0x03
	statusTimeout      = 0x04
	statusNotSupported = 0x05
)

// RazerTimeout is how long one exchange may take. The device answers in
// milliseconds when it answers at all.
const RazerTimeout = 300 * time.Millisecond

// razerSettle is how long the device needs between the request and the reply
// being readable. Measured: below about fifty milliseconds the read returns
// the *previous* answer, which is worse than no answer.
const razerSettle = 70 * time.Millisecond

// Razer reads Razer batteries.
type Razer struct {
	// open is how a node is obtained, replaced by a test so the suite touches
	// no device.
	open func(path string) (featureDevice, error)

	// settle is razerSettle, a field so a test need not wait it out.
	settle time.Duration
}

// NewRazer builds the reader.
func NewRazer() *Razer {
	return &Razer{open: openFeatureDevice, settle: razerSettle}
}

/*
Batteries reads every Razer device that answers.

**A poll that gets nothing is the ordinary case, not a fault.** The relay says
`timeout` once the mouse has passed its idle timer -- five minutes on this one
-- and `busy` when another program is mid-exchange on the same node, which the
openrazer daemon is whenever it is running. Both mean no reading this time.
The section above keeps the last one, dim, which is what it already does for a
Logitech mouse that has gone quiet.
*/
func (r *Razer) Batteries() ([]Battery, error) {
	found, err := nodes(razerVendor, speaksRazer)
	if err != nil {
		return nil, err
	}

	var (
		out  []Battery
		errs []error
	)
	for _, n := range found {
		b, err := r.read(n)
		if err != nil {
			if !errors.Is(err, errSilent) {
				errs = append(errs, err)
			}
			continue
		}
		out = append(out, b)
	}
	return out, errors.Join(errs...)
}

// read asks one node for a battery.
func (r *Razer) read(n hidNode) (Battery, error) {
	d, err := r.open(n.Path)
	if err != nil {
		return Battery{}, err
	}
	defer func() { _ = d.Close() }()

	level, err := r.exchange(d, classPower, commandBatteryLevel, 0x02)
	if err != nil {
		return Battery{}, err
	}

	b := Battery{
		Name: n.Name,
		Kind: KindMouse,
		// The level is a byte over full scale, not a percentage: 0xFF is a
		// full battery. Rounded rather than truncated, so 0xFF is 100 and not
		// 99.
		Level:    (int(level[1])*100 + 127) / 255,
		HasLevel: true,
	}

	// The charge state is a second exchange and an optional one: a device that
	// will not answer it still has a level worth drawing, and a missing answer
	// must not be read as "not charging".
	if charging, err := r.exchange(d, classPower, commandCharging, 0x02); err == nil {
		switch {
		case charging[1] == 0:
			b.State = Discharging
		case b.Level == 100:
			b.State = Full
		default:
			b.State = Charging
		}
	}
	return b, nil
}

/*
exchange sends one request and returns its arguments.

The reply comes back in the same shape as the request, with the status byte
filled in and the arguments replaced. Its checksum is verified: this is a radio
link with a dock in the middle, and a corrupted battery level is a number a
panel would draw without hesitating.
*/
func (r *Razer) exchange(d featureDevice, class, command, size byte) ([]byte, error) {
	req := razerReport(relayTransaction, class, command, size)
	if err := d.SetFeature(req); err != nil {
		return nil, fmt.Errorf("asking a Razer device for %#02x/%#02x: %w", class, command, err)
	}
	time.Sleep(r.settle)

	reply := make([]byte, len(req))
	if err := d.GetFeature(reply); err != nil {
		return nil, fmt.Errorf("reading a Razer device's answer: %w", err)
	}

	body := reply[1:] // drop the report number
	switch body[0] {
	case statusOK:
	case statusBusy, statusTimeout, statusNotSupported:
		// Asleep, contended, or a dock with nothing paired to it. No reading,
		// and nothing to report.
		return nil, errSilent
	case statusFail:
		return nil, fmt.Errorf("a Razer device refused %#02x/%#02x", class, command)
	default:
		return nil, fmt.Errorf("a Razer device answered status %#02x", body[0])
	}

	if body[razerReportSize-2] != razerCRC(body) {
		return nil, errors.New("a Razer reply's checksum does not match")
	}
	return body[8 : 8+int(size)], nil
}

// razerReport builds a request, with its leading report number.
func razerReport(transaction, class, command, size byte) []byte {
	out := make([]byte, razerReportSize+1)
	body := out[1:]
	body[1] = transaction
	body[5] = size
	body[6] = class
	body[7] = command
	body[razerReportSize-2] = razerCRC(body)
	return out
}

// razerCRC is an XOR over the report's addressed and argument bytes, which is
// everything but the status, the transaction ID and the two trailing bytes.
func razerCRC(body []byte) byte {
	var crc byte
	for _, b := range body[2 : razerReportSize-2] {
		crc ^= b
	}
	return crc
}

/*
speaksRazer picks the node that answers the protocol.

A Razer dock presents three interfaces and only the first answers a feature
report; the others are the relayed mouse and keyboard. They are told apart by
the vendor usage page their control interface declares, for the reason every
other reader here matches on a descriptor and not on a number: the numbering
moves when a device is replugged, and the product ID is not a device's
identity either.
*/
func speaksRazer(descriptor []byte) bool {
	return usagePage(0xFF00)(descriptor) || usagePage(0xFF01)(descriptor)
}

// featureDevice is a hidraw node spoken to with feature reports.
//
// Its own interface rather than [endpoint], because Razer devices declare no
// output report: a hidraw write would have nowhere to go, and the exchange is
// a pair of ioctls instead.
type featureDevice interface {
	SetFeature([]byte) error
	GetFeature([]byte) error
	Close() error
}

// hidrawFeature is a real node.
type hidrawFeature struct{ f *os.File }

func (h hidrawFeature) Close() error { return h.f.Close() } //nolint:wrapcheck // the close of a file this type opened

func (h hidrawFeature) SetFeature(b []byte) error {
	return h.ioctl(hidiocSetFeature, b)
}

func (h hidrawFeature) GetFeature(b []byte) error {
	return h.ioctl(hidiocGetFeature, b)
}

/*
The hidraw feature ioctls, composed here because x/sys/unix does not name them.

They are `_IOC(READ|WRITE, 'H', 0x06|0x07, len)`: the length is part of the
request number, so the constant is a function of the buffer and not a constant
at all. Getting the direction bits the wrong way round is the classic mistake
and shows up as EINVAL rather than as anything informative.
*/
const (
	iocWrite uint64 = 1
	iocRead  uint64 = 2

	iocTypeShift uint64 = 8
	iocSizeShift uint64 = 16
	iocDirShift  uint64 = 30

	// iocSizeMax is the widest report a request number can carry: the size
	// field is fourteen bits. A longer buffer would silently wrap into the
	// type field and address some other driver's ioctl entirely, so it is
	// refused instead.
	iocSizeMax uint64 = 1<<14 - 1

	hidrawMagic uint64 = 'H'
)

func hidiocSetFeature(n uint64) uint64 { return ioc(iocRead|iocWrite, hidrawMagic, 0x06, n) }
func hidiocGetFeature(n uint64) uint64 { return ioc(iocRead|iocWrite, hidrawMagic, 0x07, n) }

func ioc(dir, typ, nr, size uint64) uint64 {
	return dir<<iocDirShift | size<<iocSizeShift | typ<<iocTypeShift | nr
}

func (h hidrawFeature) ioctl(request func(uint64) uint64, b []byte) error {
	if len(b) == 0 {
		return errors.New("an empty hidraw feature report")
	}
	size := uint64(len(b))
	if size > iocSizeMax {
		return fmt.Errorf("a hidraw feature report of %d bytes", len(b))
	}
	// The buffer's address is the ioctl's third argument, which is what a
	// hidraw feature report *is*; there is no other way to make this call.
	// It is kept alive by b being live across the call.
	if _, _, errno := unix.Syscall(unix.SYS_IOCTL, h.f.Fd(),
		uintptr(request(size)), uintptr(unsafe.Pointer(&b[0]))); errno != 0 { //nolint:gosec // the documented hidraw ioctl
		return fmt.Errorf("hidraw ioctl: %w", errno)
	}
	return nil
}

// openFeatureDevice opens a node for the pair of ioctls this protocol is.
func openFeatureDevice(path string) (featureDevice, error) {
	f, err := os.OpenFile(path, os.O_RDWR, 0) //nolint:gosec // a hidraw node the kernel named
	if err != nil {
		return nil, fmt.Errorf("opening %s: %w", path, err)
	}
	return hidrawFeature{f: f}, nil
}
