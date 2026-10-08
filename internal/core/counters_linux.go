package core

import (
	"fmt"
	"os"
)

// ReadCounters reads the kernel's interface table from the usual place.
func ReadCounters() (map[string]Counters, error) {
	f, err := os.Open(NetDevPath)
	if err != nil {
		return nil, fmt.Errorf("opening the interface table: %w", err)
	}
	defer func() { _ = f.Close() }() // read-only; a failed close says nothing useful
	return ParseNetDev(f)
}
