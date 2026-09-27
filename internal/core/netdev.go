/*
Package core produces readings. It holds no toolkit and draws nothing: a
reading is a plain value, and both shells render the same one.

Every reader here is also reachable from the command line as JSON, which is
what makes the data layer debuggable on a machine with no display. The program
this one replaces does the same and it is why its readers could be tested at
all.
*/
package core

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// NetDevPath is where the kernel keeps the interface counters.
const NetDevPath = "/proc/net/dev"

// Counters are one interface's byte totals since the machine booted.
//
// Bytes only. /proc/net/dev carries sixteen columns and a panel needs two of
// them; the rest are packets, errors and drops, which belong to a different
// program.
type Counters struct {
	Rx uint64 `json:"rx"`
	Tx uint64 `json:"tx"`
}

// ParseNetDev reads the kernel's interface table.
//
// The format has two header lines and then one line per interface, the name
// followed by a colon. The colon is not always followed by a space: an
// interface whose receive count is wide enough runs the number straight into
// it ("eth0:1234567890"), which is why the split is on the colon and not on
// whitespace. A line that cannot be parsed is skipped rather than failing the
// read: one malformed interface should not take the panel down.
func ParseNetDev(r io.Reader) (map[string]Counters, error) {
	out := make(map[string]Counters)
	sc := bufio.NewScanner(r)
	for line := 0; sc.Scan(); line++ {
		if line < 2 {
			continue // "Inter-|   Receive ..." and the column names
		}
		text := sc.Text()
		name, rest, ok := strings.Cut(text, ":")
		if !ok {
			continue
		}
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		fields := strings.Fields(rest)
		// Receive bytes is the first column after the name, transmit bytes
		// the ninth: eight receive columns come first.
		const txIndex = 8
		if len(fields) <= txIndex {
			continue
		}
		rx, err := strconv.ParseUint(fields[0], 10, 64)
		if err != nil {
			continue
		}
		tx, err := strconv.ParseUint(fields[txIndex], 10, 64)
		if err != nil {
			continue
		}
		out[name] = Counters{Rx: rx, Tx: tx}
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("reading the interface table: %w", err)
	}
	return out, nil
}
