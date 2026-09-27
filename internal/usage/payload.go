package usage

import (
	"encoding/json"
	"fmt"
	"strconv"
	"time"
)

// Window is one quota: how much of it is used and when it starts again.
//
// Neutral on purpose. Claude and Codex report different shapes and different
// timestamp formats, and both become this at the edge, so nothing downstream
// knows or cares which provider a window came from.
type Window struct {
	// Name is what the window is called in a caption: "5h", "7d", "month".
	Name string

	// Fraction is 0..1. The providers report percentages; the conversion
	// happens here so there is one place to be wrong.
	Fraction float64

	// ResetsAt is when the window starts again. Zero means the provider did
	// not say, which is common for a limit that has never been approached.
	ResetsAt time.Time

	// Detail is anything the bar cannot carry: the used and limit values of a
	// limit that reports them. Empty for most windows.
	Detail string
}

// claudePayload is the part of Anthropic's reply this program reads.
//
// The payload carries perhaps thirty more keys, most of them null and several
// with names that are plainly internal. Decoding two of them is not laziness:
// a struct that named every field would be a struct that broke when one was
// renamed, and holding the rest opaque is what spec 002 was for.
type claudePayload struct {
	FiveHour *claudeWindow `json:"five_hour"`
	SevenDay *claudeWindow `json:"seven_day"`

	// Spend is what an account on a budget reports instead of windows. A Max
	// plan carries the windows and leaves this disabled; a Team account
	// leaves the windows null and reports here. Both shapes arrive in the
	// same field, so both are read and whichever is present is drawn.
	Spend *claudeSpend `json:"spend"`
}

// claudeSpend is a budget: how much of it is gone, in money the payload
// declares the currency of.
//
// Unlike the Codex individual limit, this one does name its currency, so a
// symbol here is reporting what the source said rather than assuming it.
type claudeSpend struct {
	Enabled bool         `json:"enabled"`
	Percent float64      `json:"percent"`
	Used    *claudeMoney `json:"used"`
	Limit   *claudeMoney `json:"limit"`
}

// claudeMoney is an amount in minor units with the exponent to place the
// point: 102371 with exponent 2 is 1023.71.
type claudeMoney struct {
	AmountMinor int64  `json:"amount_minor"`
	Currency    string `json:"currency"`
	Exponent    int    `json:"exponent"`
}

// String is the amount with its symbol, or with its code when the symbol is
// not one this program knows. An unknown currency is written out rather than
// guessed at.
func (m *claudeMoney) String() string {
	if m == nil {
		return ""
	}
	v := float64(m.AmountMinor) / pow10(m.Exponent)
	switch m.Currency {
	case "USD":
		return "$" + strconv.FormatFloat(v, 'f', m.Exponent, 64)
	case "":
		return strconv.FormatFloat(v, 'f', m.Exponent, 64)
	default:
		return strconv.FormatFloat(v, 'f', m.Exponent, 64) + " " + m.Currency
	}
}

// pow10 is ten to the power of a small non-negative exponent.
func pow10(n int) float64 {
	v := 1.0
	for range n {
		v *= 10
	}
	return v
}

type claudeWindow struct {
	Utilization float64 `json:"utilization"`
	ResetsAt    string  `json:"resets_at"`
}

// Claude reads the windows out of a Claude payload.
//
// A window the payload does not carry is absent rather than zero: a quota at
// nought per cent and a quota nobody reported are different claims, and a
// panel that drew an empty bar for the second would be making one up.
func Claude(now time.Time, data json.RawMessage) ([]Window, error) {
	if len(data) == 0 {
		return nil, nil
	}
	var p claudePayload
	if err := json.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("reading the Claude usage payload: %w", err)
	}

	var out []Window
	if w := p.FiveHour; w != nil {
		out = append(out, Window{Name: "5h", Fraction: w.Utilization / 100, ResetsAt: isoTime(w.ResetsAt)})
	}
	if w := p.SevenDay; w != nil {
		out = append(out, Window{Name: "7d", Fraction: w.Utilization / 100, ResetsAt: isoTime(w.ResetsAt)})
	}
	if sp := p.Spend; sp != nil && sp.Enabled {
		w := Window{Name: "spend", Fraction: sp.Percent / 100, ResetsAt: NextMonth(now)}
		if used := sp.Used.String(); used != "" {
			w.Detail = used
			if limit := sp.Limit.String(); limit != "" {
				w.Detail += " / " + limit
			}
		}
		out = append(out, w)
	}
	return out, nil
}

// codexPayload is the part of the Codex app-server's reply this program reads.
type codexPayload struct {
	Primary   *codexWindow `json:"primary"`
	Secondary *codexWindow `json:"secondary"`

	// IndividualLimit is a Business account's own allowance. Its used and
	// limit arrive as strings, and the app-server does not declare their unit
	// as currency, so nothing here adds a symbol.
	IndividualLimit *codexLimit `json:"individual_limit"`
}

type codexWindow struct {
	Utilization   float64 `json:"utilization"`
	WindowMinutes int     `json:"window_minutes"`
	ResetsAt      int64   `json:"resets_at"`
}

type codexLimit struct {
	Utilization float64 `json:"utilization"`
	ResetsAt    int64   `json:"resets_at"`
	Used        string  `json:"used"`
	Limit       string  `json:"limit"`
}

// Codex reads the windows out of a Codex payload.
func Codex(data json.RawMessage) ([]Window, error) {
	if len(data) == 0 {
		return nil, nil
	}
	var p codexPayload
	if err := json.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("reading the Codex usage payload: %w", err)
	}

	var out []Window
	if w := p.Primary; w != nil {
		out = append(out, Window{
			Name:     windowName(w.WindowMinutes),
			Fraction: w.Utilization / 100,
			ResetsAt: epochTime(w.ResetsAt),
		})
	}
	if w := p.Secondary; w != nil {
		out = append(out, Window{
			Name:     windowName(w.WindowMinutes),
			Fraction: w.Utilization / 100,
			ResetsAt: epochTime(w.ResetsAt),
		})
	}
	if l := p.IndividualLimit; l != nil {
		out = append(out, Window{
			Name:     "limit",
			Fraction: l.Utilization / 100,
			ResetsAt: epochTime(l.ResetsAt),
			Detail:   amount(l.Used) + " / " + amount(l.Limit),
		})
	}
	return out, nil
}

// windowName turns a duration in minutes into the short name a caption uses.
// The app-server reports the real duration rather than a label, so a window
// that changes length is named for what it is rather than for what it was.
func windowName(minutes int) string {
	switch {
	case minutes <= 0:
		return "window"
	case minutes%(60*24) == 0:
		return strconv.Itoa(minutes/(60*24)) + "d"
	case minutes%60 == 0:
		return strconv.Itoa(minutes/60) + "h"
	default:
		return strconv.Itoa(minutes) + "m"
	}
}

// amount tidies a reported number without giving it a unit.
//
// No currency symbol. The app-server does not declare these units as currency
// and the program this replaces makes the same point in its own README; a
// panel that printed a dollar sign would be asserting something the source
// never said.
func amount(s string) string {
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return s
	}
	return strconv.FormatFloat(f, 'f', 2, 64)
}

// isoTime reads a timestamp the way Claude writes one. An unparseable or empty
// value is no time rather than an error: a missing reset is ordinary.
func isoTime(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}
	return time.Time{}
}

// NextMonth is the first instant of the month after now, in local time.
//
// A derived value, and the only one in this package. The payload carries no
// reset for the monthly credit cap, so Claude Code's own /usage screen
// computes first-of-next-month locally and shows that; the program this one
// replaces does the same, and a panel that said nothing where the official
// screen says a date would look like it had lost the number.
func NextMonth(now time.Time) time.Time {
	y, m := now.Year(), now.Month()
	if m == time.December {
		return time.Date(y+1, time.January, 1, 0, 0, 0, 0, now.Location())
	}
	return time.Date(y, m+1, 1, 0, 0, 0, 0, now.Location())
}

// epochTime reads a timestamp the way Codex writes one.
func epochTime(v int64) time.Time {
	if v <= 0 {
		return time.Time{}
	}
	return time.Unix(v, 0)
}
