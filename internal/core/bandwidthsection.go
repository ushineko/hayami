package core

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// BandwidthInterval is how often the interface counters are read. Two seconds
// is the monitor's: a byte counter sampled faster reports noise, and sampled
// slower misses a burst entirely.
const BandwidthInterval = 2 * time.Second

// BandwidthSection reads the interfaces the user named.
//
// It holds its own last reading rather than handing it to a shell, so both
// shells ask the same object the same question and get the same answer. The
// lock is because a shell's paint and the poll are different goroutines.
type BandwidthSection struct {
	mu      sync.Mutex
	names   []string
	sampler *Bandwidth
	last    []Rates
	read    func() (map[string]Counters, error)
}

// NewBandwidthSection builds a section over the named interfaces. read is the
// source of counters; nil means the kernel's own table, and a test passes its
// own.
func NewBandwidthSection(names []string, read func() (map[string]Counters, error)) *BandwidthSection {
	if read == nil {
		read = ReadNetDev
	}
	return &BandwidthSection{names: names, sampler: NewBandwidth(), read: read}
}

// Key names the section.
func (b *BandwidthSection) Key() string { return "bandwidth" }

// Title is what it is called on screen.
func (b *BandwidthSection) Title() string { return "Bandwidth" }

// Interval is BandwidthInterval.
func (b *BandwidthSection) Interval() time.Duration { return BandwidthInterval }

// Poll reads the counters once. It reports false when the user has named no
// interfaces: a section with nothing to watch is not drawn, rather than drawn
// empty.
func (b *BandwidthSection) Poll(ctx context.Context) (bool, error) {
	b.mu.Lock()
	names := b.names
	b.mu.Unlock()
	if len(names) == 0 {
		return false, nil
	}
	if err := ctx.Err(); err != nil {
		return false, fmt.Errorf("polling the interfaces: %w", err)
	}

	counters, err := b.read()
	if err != nil {
		return false, fmt.Errorf("reading the interfaces: %w", err)
	}

	b.mu.Lock()
	defer b.mu.Unlock()
	b.last = b.sampler.Sample(time.Now(), names, counters)
	return true, nil
}

// Readings are the last sample, for a shell drawing it and for a test.
func (b *BandwidthSection) Readings() []Rates {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]Rates(nil), b.last...)
}

// SetInterfaces replaces the watched interfaces, for the preferences window.
// The sampler is kept: an interface that is still watched keeps its history
// and does not lose a reading because another was added.
func (b *BandwidthSection) SetInterfaces(names []string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.names = append([]string(nil), names...)
}

// Interfaces are the watched interfaces.
func (b *BandwidthSection) Interfaces() []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]string(nil), b.names...)
}
