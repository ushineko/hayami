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

// logitechVendor is Logitech's USB vendor ID.
const logitechVendor = 0x046D

// hidppNodes lists the hidraw nodes that speak HID++.
//
// A Logitech receiver presents three of them and only one is the HID++
// endpoint; the other two are the mouse and the keyboard interfaces and never
// answer a request. They are told apart by their report descriptor — the HID++
// one declares a vendor usage page and report 0x10 — and never by node number,
// which is the hwmon-index mistake spec 006 already refused. The numbering
// moves when a device is replugged.
func hidppNodes() ([]string, error) {
	entries, err := os.ReadDir(SysHidraw)
	if err != nil {
		if os.IsNotExist(err) {
			// A machine with no HID devices at all. Not an error: it is a
			// machine with no peripherals to report.
			return nil, nil
		}
		return nil, fmt.Errorf("listing hidraw nodes: %w", err)
	}

	var found []string
	for _, e := range entries {
		dir := filepath.Join(SysHidraw, e.Name(), "device")
		if !isLogitech(filepath.Join(dir, "uevent")) {
			continue
		}
		descriptor, err := os.ReadFile(filepath.Join(dir, "report_descriptor"))
		if err != nil {
			continue
		}
		if speaksHIDPP(descriptor) {
			found = append(found, filepath.Join(DevDir, e.Name()))
		}
	}
	return found, nil
}

// isLogitech reads a node's uevent for the vendor ID.
//
// HID_ID is written as bus:vendor:product, each zero-padded to eight hex
// digits. The vendor is read as a *number* and matched as a field: comparing
// it as text means deciding what to do about the padding, and stripping the
// padding off "0000046D" with TrimLeft takes the vendor's own leading zero
// with it and leaves "46D", which matches nothing. Matching the field as a
// substring instead would take a product ID that happens to contain 046D for
// a Logitech device.
func isLogitech(uevent string) bool {
	b, err := os.ReadFile(uevent)
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
		return err == nil && vendor == logitechVendor
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
	const (
		typeGlobal   = 1
		tagUsagePage = 0
		tagReportID  = 8
	)

	vendorPage := false
	for i := 0; i < len(descriptor); {
		prefix := descriptor[i]
		size := int(prefix & 0x03)
		if size == 3 {
			size = 4
		}
		if i+1+size > len(descriptor) {
			// A truncated item. Whatever this descriptor is, it is not one to
			// write requests into.
			return false
		}
		data := value(descriptor[i+1 : i+1+size])
		tag, itemType := prefix>>4, (prefix>>2)&0x03

		if itemType == typeGlobal {
			switch tag {
			case tagUsagePage:
				// 0xFF00 and above is the vendor-defined range.
				vendorPage = data >= 0xFF00
			case tagReportID:
				if vendorPage && data == reportShort {
					return true
				}
			}
		}
		i += 1 + size
	}
	return false
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
