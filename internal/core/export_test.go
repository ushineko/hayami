package core

import "time"

// SetClock replaces a section's clock, for a test that polls faster than the
// wall clock is guaranteed to move.
func (b *BandwidthSection) SetClock(now func() time.Time) { b.now = now }

// SetWait replaces the pause between a CPULoad's first two samples.
func (c *CPULoad) SetWait(wait func()) { c.wait = wait }
