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

	// readWireless describes the Wi-Fi interfaces (spec 037): ReadWireless,
	// a test's, or nil for none.
	readWireless func(context.Context) (map[string]Wireless, error)

	// wireless is the last description of each watched Wi-Fi interface, and
	// wirelessErr what the last read could not get.
	wireless    map[string]Wireless
	wirelessErr error

	// radios are the names ever seen to be Wi-Fi, which stays true of an
	// interface that drops its link: its row keeps its bars' place, blank,
	// rather than shrinking by them (glance rule). probed says the names
	// being watched have been asked about once; after that the WLAN is read
	// only while one of them is a radio, so a desk of wired interfaces pays
	// for one question and no more.
	radios map[string]bool
	probed bool
}

// NewBandwidthSection builds a section over the named interfaces. read is the
// source of counters; nil means the kernel's own table and the machine's own
// Wi-Fi, and a test passes its own counters and reads no Wi-Fi.
func NewBandwidthSection(names []string, read func() (map[string]Counters, error)) *BandwidthSection {
	var wireless func(context.Context) (map[string]Wireless, error)
	if read == nil {
		read, wireless = ReadCounters, ReadWireless
	}
	return &BandwidthSection{
		names: names, sampler: NewBandwidth(), read: read, trails: map[string]*Trail{},
		now: time.Now, readWireless: wireless, radios: map[string]bool{},
	}
}

// SetWirelessReader replaces where the Wi-Fi descriptions come from, for a
// test; nil reads none.
func (b *BandwidthSection) SetWirelessReader(read func(context.Context) (map[string]Wireless, error)) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.readWireless, b.probed = read, false
}

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

	read := b.record(names, counters)
	if read != nil {
		// Outside the lock: the Wi-Fi read is a system call or several, and a
		// shell painting meanwhile should not wait on it.
		wireless, err := read(ctx)
		b.mu.Lock()
		b.recordWireless(names, wireless, err)
		b.mu.Unlock()
	}
	return true, nil
}

// record takes one sample of the counters into the rates and trails, and
// returns the Wi-Fi reader if this poll should ask it.
func (b *BandwidthSection) record(names []string, counters map[string]Counters) func(context.Context) (map[string]Wireless, error) {
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
	return b.wirelessDue(names)
}

// wirelessDue is the Wi-Fi reader if this poll should ask it: the first poll
// of a set of names, and every poll after while one of them is a radio. The
// caller holds the lock.
func (b *BandwidthSection) wirelessDue(names []string) func(context.Context) (map[string]Wireless, error) {
	if b.readWireless == nil {
		return nil
	}
	if !b.probed {
		return b.readWireless
	}
	for _, name := range names {
		if b.radios[name] {
			return b.readWireless
		}
	}
	return nil
}

// recordWireless keeps what a Wi-Fi read said about the watched names. A read
// that failed outright keeps nothing from before: a link description is a
// statement about now. The caller holds the lock.
func (b *BandwidthSection) recordWireless(names []string, wireless map[string]Wireless, err error) {
	b.probed = true
	b.wirelessErr = err
	b.wireless = map[string]Wireless{}
	for _, name := range names {
		w, ok := wireless[name]
		if !ok {
			continue
		}
		b.radios[name] = true
		b.wireless[name] = w
	}
}

// Wireless is the last description of a watched Wi-Fi interface, and whether
// the interface is one. An interface that has been Wi-Fi and was not described
// this time is reported as a radio with nothing to say.
func (b *BandwidthSection) Wireless(name string) (Wireless, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if w, ok := b.wireless[name]; ok {
		return w, true
	}
	return Wireless{}, b.radios[name]
}

// WirelessErr is what the last Wi-Fi read could not get: ErrWirelessDenied
// where Windows withheld the connection, nil where nothing was missing.
func (b *BandwidthSection) WirelessErr() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.wirelessErr
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
	out := append([]Rates(nil), b.last...)
	for i := range out {
		if w, ok := b.wireless[out[i].Name]; ok {
			out[i].Wireless = &w
		}
	}
	return out
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
	// A newly watched name may be a radio: ask once more.
	b.probed = false
}

// Interfaces are the watched interfaces.
func (b *BandwidthSection) Interfaces() []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]string(nil), b.names...)
}
