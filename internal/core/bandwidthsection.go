package core

import (
	"context"
	"fmt"
	"slices"
	"sync"
	"time"
)

// BandwidthInterval is how often the interface counters are read. Two seconds
// is the monitor's: a byte counter sampled faster reports noise, and sampled
// slower misses a burst entirely.
const BandwidthInterval = 2 * time.Second

// BandwidthTrail is how many rate samples an interface's trend holds.
//
// Sixty at BandwidthInterval is two minutes. The cooler keeps sixty too, at
// its own cadence; a byte rate moves faster than a temperature, and two
// minutes is long enough to see a transfer start and finish.
const BandwidthTrail = 60

// Trail is one interface's recent rates, oldest first, in bytes per second as
// Rates reports them.
type Trail struct {
	Rx []float64 `json:"rx"`
	Tx []float64 `json:"tx"`
}

// add records one rated sample, dropping the oldest past BandwidthTrail.
func (t *Trail) add(r Rates) {
	t.Rx = keepLast(append(t.Rx, r.RxRate), BandwidthTrail)
	t.Tx = keepLast(append(t.Tx, r.TxRate), BandwidthTrail)
}

// keepLast is the newest n of a series.
func keepLast(s []float64, n int) []float64 {
	if len(s) > n {
		return s[len(s)-n:]
	}
	return s
}

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

	// trails are each watched interface's recent rates, by name. Only a
	// poll that produced a rate adds to one: a sample that was not taken
	// is not drawn as a zero, so a gap compresses rather than dipping.
	trails map[string]*Trail

	// now is the clock a rate is measured against. A test steps its own, so
	// two polls a nanosecond apart are never read as no time at all.
	now func() time.Time
}

// NewBandwidthSection builds a section over the named interfaces. read is the
// source of counters; nil means the kernel's own table, and a test passes its
// own.
func NewBandwidthSection(names []string, read func() (map[string]Counters, error)) *BandwidthSection {
	if read == nil {
		read = ReadCounters
	}
	return &BandwidthSection{
		names: names, sampler: NewBandwidth(), read: read, trails: map[string]*Trail{},
		now: time.Now,
	}
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
	b.last = b.sampler.Sample(b.now(), names, counters)
	for _, r := range b.last {
		if !r.Present || !r.HasRate {
			continue
		}
		t, ok := b.trails[r.Name]
		if !ok {
			t = &Trail{}
			b.trails[r.Name] = t
		}
		t.add(r)
	}
	b.forget()
	return true, nil
}

// forget drops the trail of every interface that is no longer watched. The
// caller holds the lock.
func (b *BandwidthSection) forget() {
	for name := range b.trails {
		if !slices.Contains(b.names, name) {
			delete(b.trails, name)
		}
	}
}

// Trail is an interface's recent rates, oldest first, as copies. An
// interface with none, or one not watched, has an empty trail.
func (b *BandwidthSection) Trail(name string) Trail {
	b.mu.Lock()
	defer b.mu.Unlock()
	t, ok := b.trails[name]
	if !ok {
		return Trail{}
	}
	return Trail{Rx: slices.Clone(t.Rx), Tx: slices.Clone(t.Tx)}
}

// Readings are the last sample, for a shell drawing it and for a test.
func (b *BandwidthSection) Readings() []Rates {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]Rates(nil), b.last...)
}

// SetInterfaces replaces the watched interfaces, for the preferences window.
// The sampler is kept: an interface that is still watched keeps its history
// and does not lose a reading because another was added. One that is no
// longer watched loses its trail.
func (b *BandwidthSection) SetInterfaces(names []string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.names = append([]string(nil), names...)
	b.forget()
}

// Interfaces are the watched interfaces.
func (b *BandwidthSection) Interfaces() []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]string(nil), b.names...)
}
