package core

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

// filterInterface is the FilterInterface bit of MIB_IF_ROW2's
// InterfaceAndOperStatusFlags: the bit after HardwareInterface.
const filterInterface = 1 << 1

// ReadCounters reads Windows' interface table, keyed by each interface's alias
// -- "Ethernet", "Wi-Fi", "Tailscale" -- which is the name Network Connections
// shows and the one a person would pick.
//
// Filter interfaces are left out. Every adapter carries a stack of them (the
// WFP and QoS lightweight filters), each a row of its own with the adapter's
// alias and a suffix, counting the same bytes again. On the machine this was
// written on they were 22 of 45 rows, and none is anything a person would
// choose to watch.
//
// The octet counts are the 64-bit ones, since boot or since the adapter came
// up, which is what Sample differences.
func ReadCounters() (map[string]Counters, error) {
	var table *windows.MibIfTable2
	if err := windows.GetIfTable2Ex(windows.MibIfTableNormal, &table); err != nil {
		return nil, fmt.Errorf("reading the interface table: %w", err)
	}
	defer windows.FreeMibTable(unsafe.Pointer(table))

	rows := unsafe.Slice(&table.Table[0], table.NumEntries)
	out := make(map[string]Counters, len(rows))
	for i := range rows {
		row := &rows[i]
		if row.InterfaceAndOperStatusFlags&filterInterface != 0 {
			continue
		}
		name := windows.UTF16ToString(row.Alias[:])
		if name == "" {
			continue
		}
		out[name] = Counters{Rx: row.InOctets, Tx: row.OutOctets}
	}
	return out, nil
}
