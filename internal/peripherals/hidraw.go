package peripherals

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// SysHidraw is where the kernel lists hidraw nodes. A variable so a test can
// point it at a tree it wrote itself and touch no device.
var SysHidraw = "/sys/class/hidraw"

// DevDir is where the nodes themselves live, for the same reason.
var DevDir = "/dev"

// The USB vendor IDs this package knows how to talk to.
const (
	logitechVendor    = 0x046D
	razerVendor       = 0x1532
	steelseriesVendor = 0x1038
)

// hidppNodes lists the hidraw nodes that speak HID++.
//
// A Logitech receiver presents three of them and only one is the HID++
// endpoint; the other two are the mouse and the keyboard interfaces and never
// answer a request. They are told apart by their report descriptor — the HID++
// one declares a vendor usage page and report 0x10 — and never by node number,
// which is the hwmon-index mistake spec 006 already refused. The numbering
// moves when a device is replugged.
func hidppNodes() ([]hidNode, error) {
	return nodes(logitechVendor, speaksHIDPP)
}

// hidNode is one hidraw node: where it is and what the kernel calls the device.
//
// The name comes from here rather than from the protocol because the kernel
// already has it -- `HID_NAME=SteelSeries Apex Pro TKL Wireless Gen 3` -- and
// asking the device for a name it may not have is a round trip for something
// already on disk.
type hidNode struct {
	Path string
	Name string

	// Phys is the kernel\'s HID_PHYS: which USB interface a node is, and for
	// a Logitech receiver\'s children the device index too -- see pairedDevice.
	Phys string

	// Product is the USB product ID. It is **not** how a node is found -- see
	// nodes() for why -- but it is how this build decides whether it knows a
	// device's protocol well enough to write to it at all (spec 017).
	Product uint64
}

/*
nodes lists the hidraw nodes of one vendor whose report descriptor satisfies
wants.

**Never by product ID.** The SteelSeries keyboard on the machine this was
written for enumerates as `1038:1644` with its keyboard on 2.4 GHz and
`1038:1646` with the same keyboard on its cable, on the same USB port, and the
hidraw numbers land on the same indices both times. A reader keyed to the
product reads whichever one it was told about and says nothing about the other;
that cost a round of measurements during spec 016's investigation before anyone
noticed the number had moved. The vendor does not move and the usage page is
what actually says "this endpoint speaks the protocol".

Nor by node number, which is the hwmon-index mistake spec 006 already refused:
the numbering changes when a device is replugged.
*/
func nodes(vendor uint64, wants func(descriptor []byte) bool) ([]hidNode, error) {
	entries, err := os.ReadDir(SysHidraw)
	if err != nil {
		if os.IsNotExist(err) {
			// A machine with no HID devices at all. Not an error: it is a
			// machine with no peripherals to report.
			return nil, nil
		}
		return nil, fmt.Errorf("listing hidraw nodes: %w", err)
	}

	var found []hidNode
	for _, e := range entries {
		dir := filepath.Join(SysHidraw, e.Name(), "device")
		uevent := filepath.Join(dir, "uevent")
		if !isVendor(uevent, vendor) {
			continue
		}
		descriptor, err := os.ReadFile(filepath.Join(dir, "report_descriptor")) //nolint:gosec // a path under the hidraw root
		if err != nil {
			continue
		}
		if !wants(descriptor) {
			continue
		}
		found = append(found, hidNode{
			Path:    filepath.Join(DevDir, e.Name()),
			Name:    hidName(uevent),
			Product: hidProduct(uevent),
			Phys:    hidField(uevent, "HID_PHYS="),
		})
	}
	return found, nil
}

// hidName is what the kernel calls the device, tidied.
//
// `HID_NAME=Razer Razer Mouse Dock Pro` is what the descriptor's manufacturer
// and product strings concatenate to when a vendor puts its own name in both.
// The doubling is dropped because a card is read by a person.
func hidName(uevent string) string {
	return undouble(hidField(uevent, "HID_NAME="))
}

/*
pairedDevice reports whether a node is one device behind a receiver rather than
the receiver itself.

`hid-logitech-dj` gives a receiver's children the receiver's own HID_PHYS with
`:index` appended:

	usb-0000:03:00.0-3/input2     Logitech USB Receiver
	usb-0000:03:00.0-3/input2:1   Logitech K800
	usb-0000:03:00.0-3/input2:2   Logitech Performance MX

The distinction is load-bearing for a *name*. A receiver node answers for every
device paired to it, so an answer arriving there carries the receiver's name and
not the device's -- which put "Logitech USB Receiver: speaks HID++ 1.0" on a
panel beside the very keyboard that had said it.
*/
func pairedDevice(phys string) bool {
	_, suffix, ok := strings.Cut(phys, "input")
	if !ok {
		return false
	}
	_, index, ok := strings.Cut(suffix, ":")
	if !ok || index == "" {
		return false
	}
	for _, c := range index {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// hidProduct is the product ID out of a node's HID_ID, which the kernel writes
// as bus:vendor:product with each field zero-padded to eight hex digits.
func hidProduct(uevent string) uint64 {
	fields := strings.Split(hidField(uevent, "HID_ID="), ":")
	if len(fields) != 3 {
		return 0
	}
	product, err := strconv.ParseUint(strings.TrimSpace(fields[2]), 16, 32)
	if err != nil {
		return 0
	}
	return product
}

// hidField reads one `KEY=value` line out of a node's uevent.
func hidField(uevent, key string) string {
	b, err := os.ReadFile(uevent) //nolint:gosec // a path under the hidraw root
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(b), "\n") {
		value, ok := strings.CutPrefix(line, key)
		if !ok {
			continue
		}
		return strings.TrimSpace(value)
	}
	return ""
}

// undouble drops a repeated first word: "Razer Razer Mouse Dock Pro".
func undouble(name string) string {
	words := strings.Fields(name)
	if len(words) > 1 && strings.EqualFold(words[0], words[1]) {
		return strings.Join(words[1:], " ")
	}
	return strings.Join(words, " ")
}

// usagePage reports whether a descriptor declares a given usage page, which is
// how a vendor's control endpoint is told from its keyboard and mouse ones.
func usagePage(want uint32) func([]byte) bool {
	return func(descriptor []byte) bool {
		found := false
		walk(descriptor, func(itemType, tag byte, data uint32) bool {
			if itemType == typeGlobal && tag == tagUsagePage && data == want {
				found = true
				return false
			}
			return true
		})
		return found
	}
}

// isVendor reads a node's uevent for the vendor ID.
//
// HID_ID is written as bus:vendor:product, each zero-padded to eight hex
// digits. The vendor is read as a *number* and matched as a field: comparing
// it as text means deciding what to do about the padding, and stripping the
// padding off "0000046D" with TrimLeft takes the vendor's own leading zero
// with it and leaves "46D", which matches nothing. Matching the field as a
// substring instead would take a product ID that happens to contain 046D for
// a Logitech device.
func isVendor(uevent string, want uint64) bool {
	b, err := os.ReadFile(uevent) //nolint:gosec // a path under the hidraw root
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(b), "\n") {
		id, ok := strings.CutPrefix(line, "HID_ID=")
		if !ok {
			continue
		}
		fields := strings.Split(id, ":")
		if len(fields) != 3 {
			return false
		}
		vendor, err := strconv.ParseUint(strings.TrimSpace(fields[1]), 16, 32)
		return err == nil && vendor == want
	}
	return false
}

// speaksHIDPP reads a report descriptor for the marks of a HID++ endpoint: a
// vendor-defined usage page, and report 0x10 declared within it.
//
// The descriptor is walked as items rather than searched as bytes. `06 00 ff`
// appears in this descriptor as a usage page and could appear in another as
// the tail of a longer item's data, and a byte search cannot tell the two
// apart.
func speaksHIDPP(descriptor []byte) bool {
	vendorPage, found := false, false
	walk(descriptor, func(itemType, tag byte, data uint32) bool {
		if itemType != typeGlobal {
			return true
		}
		switch tag {
		case tagUsagePage:
			// 0xFF00 and above is the vendor-defined range.
			vendorPage = data >= 0xFF00
		case tagReportID:
			if vendorPage && data == reportShort {
				found = true
				return false
			}
		}
		return true
	})
	return found
}

// The HID item types and tags this package reads.
const (
	typeGlobal   = 1
	tagUsagePage = 0
	tagReportID  = 8
)

/*
walk reads a report descriptor item by item, stopping when visit says so.

**Items, not bytes.** `06 00 ff` is a usage page here and could be the tail of
a longer item's data there, and a byte search cannot tell the two apart. A
truncated item ends the walk: whatever such a descriptor is, it is not one to
draw conclusions from.
*/
func walk(descriptor []byte, visit func(itemType, tag byte, data uint32) bool) {
	for i := 0; i < len(descriptor); {
		prefix := descriptor[i]
		size := int(prefix & 0x03)
		if size == 3 {
			size = 4
		}
		if i+1+size > len(descriptor) {
			return
		}
		itemType, tag := (prefix>>2)&0x03, prefix>>4
		if !visit(itemType, tag, value(descriptor[i+1:i+1+size])) {
			return
		}
		i += 1 + size
	}
}

// value reads an HID item's data, which is little-endian and one, two or four
// bytes wide, or absent.
func value(b []byte) uint32 {
	var v uint32
	for i := len(b) - 1; i >= 0; i-- {
		v = v<<8 | uint32(b[i])
	}
	return v
}
