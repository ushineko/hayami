package view

import "time"

// UsageWindow is one quota, already measured and not yet formatted. It mirrors
// what the cache decodes to without importing it: the view takes plain values.
type UsageWindow struct {
	// Account is what the caption calls the owner: "Claude max", "Codex".
	Account string

	// Name is the window: "5h", "7d", "limit".
	Name string

	Fraction float64
	ResetsAt time.Time

	// Detail is anything the bar cannot carry, such as a limit's used and
	// limit values.
	Detail string
}

// UsageStale is how old a reading may be before its age is worth saying.
//
// Five minutes, which is longer than any pane's poll and shorter than the
// shortest quota window. Below it the age is noise; above it, a reader
// deserves to know the numbers are not live.
const UsageStale = 5 * time.Minute

// Usage turns windows into a section of meters.
//
// One meter per window per account. The caption carries the percentage, the
// countdown and any detail, in that order, and each is formatted at a fixed
// width so the section does not change size as the numbers do.
func Usage(now time.Time, windows []UsageWindow, fetchedAt time.Time) Section {
	s := Section{Key: "usage", Title: "Usage"}

	for _, w := range windows {
		caption := Percent(w.Fraction) + " · " + resets(now, w.ResetsAt)
		if w.Detail != "" {
			caption += " · " + w.Detail
		}
		s.Meters = append(s.Meters, Meter{
			Label:    label(w),
			Caption:  caption,
			Fraction: w.Fraction,
			Status:   quota(w.Fraction),
		})
	}

	if age := now.Sub(fetchedAt); !fetchedAt.IsZero() && age > UsageStale {
		s.Rows = append(s.Rows, Row{
			Label:  "read",
			Value:  ago(age),
			Status: Info,
		})
	}
	return s
}

// label names a meter: the account and its window, or just the window when
// there is one account.
func label(w UsageWindow) string {
	if w.Account == "" {
		return w.Name
	}
	return w.Account + " " + w.Name
}

// resets is the countdown, or a fixed-width blank when the provider did not
// say when the window starts again.
func resets(now, at time.Time) string {
	if at.IsZero() {
		return NoUntil()
	}
	return Until(now, at)
}

// quota is the verdict on a proportion of something with a limit.
//
// A quota is one of the few readings with a true threshold, so its colour is a
// signal rather than decoration. The bands are the monitor's: amber at four
// fifths, red at nineteen twentieths.
func quota(fraction float64) Status {
	switch {
	case fraction >= 0.95:
		return Bad
	case fraction >= 0.80:
		return Warn
	default:
		return Good
	}
}

// ago is how old a reading is, at a fixed width.
func ago(d time.Duration) string {
	switch {
	case d >= 24*time.Hour:
		return pad(itoa(int(d.Hours())/24) + "d ago")
	case d >= time.Hour:
		return pad(itoa(int(d.Hours())) + "h ago")
	default:
		return pad(itoa(int(d.Minutes())) + "m ago")
	}
}

// itoa is strconv.Itoa, named here to keep the import list of this file to the
// one thing it is about.
func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for v > 0 {
		i--
		b[i] = byte('0' + v%10)
		v /= 10
	}
	return string(b[i:])
}
