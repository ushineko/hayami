package core

import "time"

// SetClock replaces a section's clock, for a test that polls faster than the
// wall clock is guaranteed to move.
func (b *BandwidthSection) SetClock(now func() time.Time) { b.now = now }
