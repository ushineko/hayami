package panel_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/hayami/internal/panel"
	"github.com/ushineko/hayami/internal/view"
)

// Spec 106 R2, R7. The clock's heading is the time it read, to the second,
// and the date, with nothing under it.
func TestTheClockIsItsHeading(t *testing.T) {
	at := time.Date(2026, 11, 21, 18, 25, 4, 900e6, time.Local)
	c := panel.NewClock(func() time.Time { return at })
	drawn, err := c.Poll(context.Background())
	require.NoError(t, err)
	assert.True(t, drawn)

	sec := c.Section()
	assert.Equal(t, view.ClockInfo.Key, sec.Key)
	assert.Equal(t, "18:25:04", sec.Title)
	assert.Equal(t, "Sat 2026/11/21", sec.TitleEnd)
	assert.Equal(t, view.IconClock, sec.Icon)
	assert.Equal(t, view.IconCalendar, sec.TitleEndIcon)
	assert.True(t, sec.HeadingOnly())
}

// Spec 106 R4. The clock is polled at the turn of each second, just after
// it, and a source that is not paced at its interval.
func TestTheClockIsPolledAtTheTurnOfEachSecond(t *testing.T) {
	c := panel.NewClock(nil)
	at := time.Date(2026, 11, 21, 18, 25, 4, 300e6, time.Local)
	wait := panel.Wait(c, at)
	next := at.Add(wait)
	assert.Equal(t, 5, next.Second(), "not the next second")
	assert.Less(t, next.Nanosecond(), int(20*time.Millisecond), "not just after its turn")

	assert.Positive(t, panel.Wait(c, at.Truncate(time.Second)), "a poll on the turn waited no time")
	assert.Equal(t, time.Hour, panel.Wait(hourly{}, at))
}

// hourly is a source that is not paced.
type hourly struct{ panel.Source }

func (hourly) Interval() time.Duration { return time.Hour }
