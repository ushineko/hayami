package view

import "time"

// The clock's formats (spec 106): 24-hour time to the second, and the day
// of the week with the date, year first. Both are a fixed length in
// characters, so a tick changes no width in the terminal.
const (
	ClockTime = "15:04:05"
	ClockDate = "Mon 2006/01/02"
)

/*
Clock is the clock section at a moment (spec 106): its heading is the reading,
the time at the left after a clock face and the date at the right after a
calendar, with nothing under it. Local time.
*/
func Clock(now time.Time) Section {
	s := ClockInfo.section()
	now = now.Local()
	s.Title = now.Format(ClockTime)
	s.TitleEnd = now.Format(ClockDate)
	s.TitleEndIcon = IconCalendar
	return s
}
