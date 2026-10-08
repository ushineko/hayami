//go:build !linux && !windows

package core

import (
	"errors"
	"fmt"
)

// ReadCounters has no interface table to read on this system: the section
// says so rather than looking for /proc/net/dev, which only Linux has.
func ReadCounters() (map[string]Counters, error) {
	return nil, fmt.Errorf("reading the interface table: %w", errors.ErrUnsupported)
}
