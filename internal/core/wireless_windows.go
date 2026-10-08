package core

import (
	"context"
	"errors"
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	wlanapi                  = windows.NewLazySystemDLL("wlanapi.dll")
	procWlanOpenHandle       = wlanapi.NewProc("WlanOpenHandle")
	procWlanCloseHandle      = wlanapi.NewProc("WlanCloseHandle")
	procWlanEnumInterfaces   = wlanapi.NewProc("WlanEnumInterfaces")
	procWlanQueryInterface   = wlanapi.NewProc("WlanQueryInterface")
	procWlanFreeMemory       = wlanapi.NewProc("WlanFreeMemory")
	iphlpapi                 = windows.NewLazySystemDLL("iphlpapi.dll")
	procGUIDToLuid           = iphlpapi.NewProc("ConvertInterfaceGuidToLuid")
	procLuidToAlias          = iphlpapi.NewProc("ConvertInterfaceLuidToAlias")
	errWlanServiceNotRunning = windows.Errno(1062) // ERROR_SERVICE_NOT_ACTIVE
)

// The WLAN_INTF_OPCODE values asked for.
const (
	opCurrentConnection = 7
	opChannelNumber     = 8
	opRealtimeQuality   = 19
	opRSSI              = 0x10000102
)

// wlanInterfaceInfo is WLAN_INTERFACE_INFO: the GUID, a description and the
// state.
type wlanInterfaceInfo struct {
	GUID        windows.GUID
	Description [256]uint16
	State       uint32
}

// wlanInterfaceList is WLAN_INTERFACE_INFO_LIST, with its variable-length
// tail read through unsafe.Slice.
type wlanInterfaceList struct {
	Count uint32
	Index uint32
	First wlanInterfaceInfo
}

// aliasMax is IF_MAX_STRING_SIZE + 1.
const aliasMax = 257

/*
ReadWireless asks Windows' WLAN service about every Wi-Fi interface, keyed by
the interface's alias -- "Wi-Fi 2" -- which is the name ReadCounters keys the
same interface by. That agreement is what lets the bandwidth section put the
two together, and a test holds it against the real tables.

A machine with no Wi-Fi, or with the WLAN service stopped, has nothing to say
and returns nothing, not an error.

**Location consent.** Since Windows 11 24H2 the current connection's details
are withheld from a program the user has not given location access, because
the access point's address would locate the machine. Where Windows says so,
whatever else was read is returned with ErrWirelessDenied, so the row still
gets what it can and the reason says what is missing.
*/
func ReadWireless(ctx context.Context) (map[string]Wireless, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("reading Wi-Fi: %w", err)
	}
	var version uint32
	var h windows.Handle
	if r, _, _ := procWlanOpenHandle.Call(2, 0, uintptr(unsafe.Pointer(&version)), uintptr(unsafe.Pointer(&h))); r != 0 {
		if windows.Errno(r) == errWlanServiceNotRunning {
			return nil, nil
		}
		return nil, fmt.Errorf("opening the WLAN service: %w", windows.Errno(r))
	}
	defer procWlanCloseHandle.Call(uintptr(h), 0) //nolint:errcheck // nothing to do about it

	var list *wlanInterfaceList
	if r, _, _ := procWlanEnumInterfaces.Call(uintptr(h), 0, uintptr(unsafe.Pointer(&list))); r != 0 {
		return nil, fmt.Errorf("listing Wi-Fi interfaces: %w", windows.Errno(r))
	}
	defer procWlanFreeMemory.Call(uintptr(unsafe.Pointer(list))) //nolint:errcheck // void

	out := map[string]Wireless{}
	denied := false
	for _, info := range unsafe.Slice(&list.First, list.Count) {
		alias := interfaceAlias(info.GUID)
		if alias == "" {
			continue
		}
		w := Wireless{}
		if info.State == wlanConnected {
			if wlanRead(h, info.GUID, &w) {
				denied = true
			}
		}
		out[alias] = w
	}
	if denied {
		return out, ErrWirelessDenied
	}
	return out, nil
}

// wlanRead fills w from the realtime quality, the connection, the channel and
// the RSSI, each only for what the ones before it left out. It reports whether
// Windows refused the connection for want of location consent.
func wlanRead(h windows.Handle, guid windows.GUID, w *Wireless) (denied bool) {
	w.Connected = true
	if b, err := wlanQuery(h, guid, opRealtimeQuality); err == nil {
		decodeRealtime(b, w)
	} else if errors.Is(err, windows.ERROR_ACCESS_DENIED) {
		denied = true
	}
	if b, err := wlanQuery(h, guid, opCurrentConnection); err == nil {
		decodeConnection(b, w)
	} else if errors.Is(err, windows.ERROR_ACCESS_DENIED) {
		denied = true
	}
	if !w.HasChannel {
		if b, err := wlanQuery(h, guid, opChannelNumber); err == nil && len(b) >= 4 {
			if ch := int(le32(b, 0)); ch > 0 {
				w.Channel, w.HasChannel = ch, true
			}
		}
	}
	if !w.HasRSSI {
		if b, err := wlanQuery(h, guid, opRSSI); err == nil && len(b) >= 4 {
			if rssi := int32(le32(b, 0)); rssi < 0 { //nolint:gosec // a LONG
				w.RSSI, w.HasRSSI = int(rssi), true
			}
		}
	}
	return denied
}

// wlanQuery asks one opcode of one interface and returns a copy of the answer.
func wlanQuery(h windows.Handle, guid windows.GUID, op uint32) ([]byte, error) {
	var size uint32
	var data unsafe.Pointer
	r, _, _ := procWlanQueryInterface.Call(uintptr(h), uintptr(unsafe.Pointer(&guid)), uintptr(op), 0,
		uintptr(unsafe.Pointer(&size)), uintptr(unsafe.Pointer(&data)), 0)
	if r != 0 {
		return nil, windows.Errno(r)
	}
	defer procWlanFreeMemory.Call(uintptr(data)) //nolint:errcheck // void
	return append([]byte(nil), unsafe.Slice((*byte)(data), size)...), nil
}

// interfaceAlias is the alias of the interface with a GUID: the name Network
// Connections shows, and the one ReadCounters keys by. Empty where Windows
// does not know it.
func interfaceAlias(guid windows.GUID) string {
	var luid uint64
	if r, _, _ := procGUIDToLuid.Call(uintptr(unsafe.Pointer(&guid)), uintptr(unsafe.Pointer(&luid))); r != 0 {
		return ""
	}
	buf := make([]uint16, aliasMax)
	if r, _, _ := procLuidToAlias.Call(uintptr(unsafe.Pointer(&luid)), uintptr(unsafe.Pointer(&buf[0])), aliasMax); r != 0 {
		return ""
	}
	return windows.UTF16ToString(buf)
}
