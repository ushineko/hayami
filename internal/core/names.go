package core

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// CPUInfoPath is where the kernel describes the processor.
const CPUInfoPath = "/proc/cpuinfo"

// PCIIDsPath is the PCI ID database, which hwdata installs on Arch and on
// CachyOS. Where it is missing a card found through the kernel keeps the
// label "GPU" (spec 031).
const PCIIDsPath = "/usr/share/hwdata/pci.ids"

/*
CPUName is the processor's model as the kernel gives it -- the first "model
name" in /proc/cpuinfo, "Intel(R) Core(TM) i9-14900K" -- or empty where there
is none.

The first, because every core of a desktop processor carries the same one.
A hybrid part's performance and efficiency cores are one model name. An ARM
machine has no such field at all, and empty is the answer that keeps the row's
label "CPU".
*/
func CPUName(path string) string {
	f, err := os.Open(path) //nolint:gosec // procfs, or a test's file
	if err != nil {
		return ""
	}
	defer func() { _ = f.Close() }()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		key, value, ok := strings.Cut(sc.Text(), ":")
		if ok && strings.TrimSpace(key) == "model name" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

/*
PCIName is the product name the PCI ID database has for a vendor and device,
"Navi 31 [Radeon RX 7900 XT/7900 XTX/7900 GRE/7900M]", or empty where the
database is missing or does not list it.

The database is a text file of vendors at the margin and their devices one tab
in:

	1002  Advanced Micro Devices, Inc. [AMD/ATI]
		744c  Navi 31 [Radeon RX 7900 XT/7900 XTX/7900 GRE/7900M]
		  1002 0e3b  RX 7900 GRE

A subsystem line, two tabs in, would name the board more exactly, but the
board's subsystem is a second lookup for a name that is shortened to the model
anyway; the device line is the model.

Read once per card and remembered by the caller, never on every poll: the file
is a megabyte and a half and the answer does not change while the machine is
up.
*/
func PCIName(path string, vendor, device uint16) string {
	f, err := os.Open(path) //nolint:gosec // the hwdata file, or a test's
	if err != nil {
		return ""
	}
	defer func() { _ = f.Close() }()

	wantVendor := fmt.Sprintf("%04x", vendor)
	wantDevice := fmt.Sprintf("%04x", device)
	inVendor := false
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if line == "" || line[0] == '#' {
			continue
		}
		switch {
		case line[0] != '\t':
			// A vendor line. The list of classes at the end of the file
			// begins with "C ", which matches no vendor and ends the search
			// as well as any.
			id, _, _ := strings.Cut(line, " ")
			if inVendor {
				return ""
			}
			inVendor = strings.EqualFold(id, wantVendor)
		case inVendor && !strings.HasPrefix(line, "\t\t"):
			id, name, ok := strings.Cut(strings.TrimPrefix(line, "\t"), " ")
			if ok && strings.EqualFold(id, wantDevice) {
				return strings.TrimSpace(name)
			}
		}
	}
	return ""
}

// pciID is the vendor and device of the PCI function a sysfs device directory
// is, from its vendor and device files ("0x1002").
func pciID(dir string) (vendor, device uint16, ok bool) {
	v, okV := hexFile(filepath.Join(dir, "vendor"))
	d, okD := hexFile(filepath.Join(dir, "device"))
	return v, d, okV && okD
}

// hexFile is a sysfs file holding one hexadecimal number.
func hexFile(path string) (uint16, bool) {
	body, err := os.ReadFile(path) //nolint:gosec // sysfs, or a test's file
	if err != nil {
		return 0, false
	}
	var v uint16
	if _, err := fmt.Sscanf(strings.TrimSpace(string(body)), "0x%x", &v); err != nil {
		return 0, false
	}
	return v, true
}

/*
chipDevice is the device directory behind the hwmon chip named chip under root:
the PCI function of the card whose temperature it reports.

By name, as hwmon is read everywhere here, and never by index: the numbers move
between boots.
*/
func chipDevice(root, chip string) string {
	dirs, err := os.ReadDir(root)
	if err != nil {
		return ""
	}
	for _, d := range dirs {
		dir := filepath.Join(root, d.Name())
		body, err := os.ReadFile(filepath.Join(dir, "name")) //nolint:gosec // sysfs, or a test's file
		if err != nil || strings.TrimSpace(string(body)) != chip {
			continue
		}
		return filepath.Join(dir, "device")
	}
	return ""
}
