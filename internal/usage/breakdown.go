package usage

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

/*
Breakdown is everything a provider said about an account that a meter has no
room for: each limit, what the week went on, extra usage, credits (spec 060
R13). Neutral, as Window is: the two providers report different shapes and
both become this.

**Every part is optional, and a missing one is not an error** (R14). The keys
read here are the providers' own and undocumented; they were found by name
and type in one account's cache, and another account, or next month's
payload, may lack any of them. A key that is absent or null is a part left
out.
*/
type Breakdown struct {
	// Plan is the plan's name where the payload carries one.
	Plan string

	Limits []Limit

	// SharesTitle heads Shares, which are parts of one whole.
	SharesTitle string
	Shares      []Share

	Facts []Fact
}

// Limit is one quota: its name, how much is used, when it starts again, the
// provider's verdict where it gives one, and its amounts where it has them.
type Limit struct {
	Name     string
	Fraction float64
	ResetsAt time.Time
	Severity string
	Amount   string
}

// Share is one part of a whole, as a fraction of it.
type Share struct {
	Name     string
	Fraction float64
}

// Fact is a named value with nothing to fill: "Extra usage", "off".
type Fact struct {
	Name  string
	Value string
}

/*
keys splits a payload into its top-level keys, each still undecoded, and part
decodes one of them, reporting whether it was there and had the shape asked
for.

A breakdown is read a key at a time for that reason: one key whose shape has
changed costs its own lines and nothing else. Decoded as one struct, a number
that became an object anywhere in the payload would empty the whole drawer.
*/
func keys(data json.RawMessage) map[string]json.RawMessage {
	var out map[string]json.RawMessage
	if json.Unmarshal(data, &out) != nil {
		return nil
	}
	return out
}

func part(from map[string]json.RawMessage, key string, into any) bool {
	raw, ok := from[key]
	if !ok || len(raw) == 0 || string(raw) == "null" {
		return false
	}
	return json.Unmarshal(raw, into) == nil
}

// number reads a value that may be a number or a number written as a string,
// and reports whether it was either.
func number(raw json.RawMessage) (float64, bool) {
	var f float64
	if json.Unmarshal(raw, &f) == nil && len(raw) > 0 && string(raw) != "null" {
		return f, true
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		if f, err := strconv.ParseFloat(s, 64); err == nil {
			return f, true
		}
	}
	return 0, false
}

// The parts of Anthropic's reply the breakdown reads, beside what
// claudePayload reads for the meters.
type (
	claudeLimit struct {
		Kind     string   `json:"kind"`
		Percent  *float64 `json:"percent"`
		Severity string   `json:"severity"`
		ResetsAt string   `json:"resets_at"`
		Scope    *struct {
			Model *struct {
				DisplayName string `json:"display_name"`
			} `json:"model"`
		} `json:"scope"`
	}

	claudeWeek struct {
		Rows []struct {
			Key         string   `json:"key"`
			DisplayName string   `json:"display_name"`
			Percent     *float64 `json:"percent"`
		} `json:"rows"`
	}

	claudeExtra struct {
		IsEnabled     bool            `json:"is_enabled"`
		MonthlyLimit  json.RawMessage `json:"monthly_limit"`
		UsedCredits   json.RawMessage `json:"used_credits"`
		Currency      json.RawMessage `json:"currency"`
		DecimalPlaces json.RawMessage `json:"decimal_places"`
	}
)

// claudeLimitNames are the limits' kinds as a person reads them. A kind this
// table does not have is shown as the payload spells it, with its underscores
// as spaces: a new limit should appear, not vanish.
var claudeLimitNames = map[string]string{
	"session":       "Session",
	"weekly_all":    "Week",
	"weekly_scoped": "Week",
}

// ClaudeBreakdown reads the breakdown out of a Claude payload.
func ClaudeBreakdown(_ time.Time, data json.RawMessage) (Breakdown, error) {
	var out Breakdown
	if len(data) == 0 {
		return out, nil
	}
	p := keys(data)
	if p == nil {
		return out, fmt.Errorf("reading the Claude usage payload: %w", errNotAnObject)
	}

	var limits []claudeLimit
	part(p, "limits", &limits)
	for _, l := range limits {
		if l.Percent == nil {
			continue
		}
		name, ok := claudeLimitNames[l.Kind]
		if !ok {
			name = strings.ReplaceAll(l.Kind, "_", " ")
		}
		if l.Scope != nil && l.Scope.Model != nil && l.Scope.Model.DisplayName != "" {
			name += " · " + l.Scope.Model.DisplayName
		}
		out.Limits = append(out.Limits, Limit{
			Name: name, Fraction: *l.Percent / 100, ResetsAt: isoTime(l.ResetsAt), Severity: l.Severity,
		})
	}

	var week claudeWeek
	if part(p, "seven_day_breakdown", &week) {
		for _, r := range week.Rows {
			if r.Percent == nil {
				continue
			}
			name := r.DisplayName
			if name == "" {
				name = r.Key
			}
			out.Shares = append(out.Shares, Share{Name: name, Fraction: *r.Percent / 100})
		}
		if len(out.Shares) > 0 {
			out.SharesTitle = "Week by use"
		}
	}

	var extra claudeExtra
	if part(p, "extra_usage", &extra) {
		out.Facts = append(out.Facts, Fact{Name: "Extra usage", Value: extraUsage(extra)})
	}
	var sp claudeSpend
	if part(p, "spend", &sp) && sp.Used != nil {
		value := sp.Used.String()
		if sp.Limit != nil {
			value += " of " + sp.Limit.String()
		}
		out.Facts = append(out.Facts, Fact{Name: "Spend", Value: value})
	}
	return out, nil
}

// extraUsage is the state of extra usage in a few words: off, on, or the
// credits used against the limit for the month. The amounts are in minor
// units where the payload says how many decimal places they have.
func extraUsage(e claudeExtra) string {
	if !e.IsEnabled {
		return "off"
	}
	used, ok := number(e.UsedCredits)
	if !ok {
		return "on"
	}
	var currency string
	_ = json.Unmarshal(e.Currency, &currency)
	places, _ := number(e.DecimalPlaces)
	money := func(v float64) string {
		m := claudeMoney{AmountMinor: int64(v), Currency: currency, Exponent: int(places)}
		return m.String()
	}
	if limit, ok := number(e.MonthlyLimit); ok {
		return money(used) + " of " + money(limit)
	}
	return money(used)
}

// errNotAnObject is a payload that is not a JSON object at all.
var errNotAnObject = errors.New("not an object")

// codexIndividual is the allowance of a Business account, as the normalised
// payload holds it.
type codexIndividual struct {
	Utilization      *float64        `json:"utilization"`
	RemainingPercent *float64        `json:"remaining_percent"`
	ResetsAt         json.RawMessage `json:"resets_at"`
	Used             string          `json:"used"`
	Limit            string          `json:"limit"`
}

// CodexBreakdown reads the breakdown out of a Codex payload.
func CodexBreakdown(_ time.Time, data json.RawMessage) (Breakdown, error) {
	var out Breakdown
	if len(data) == 0 {
		return out, nil
	}
	p := keys(data)
	if p == nil {
		return out, fmt.Errorf("reading the Codex usage payload: %w", errNotAnObject)
	}

	part(p, "plan_type", &out.Plan)
	for _, key := range []string{"primary", "secondary"} {
		var w codexWindow
		if !part(p, key, &w) {
			continue
		}
		out.Limits = append(out.Limits, Limit{
			Name: windowName(w.WindowMinutes), Fraction: w.Utilization / 100, ResetsAt: epochTime(w.ResetsAt),
		})
	}
	var l codexIndividual
	if part(p, "individual_limit", &l) && l.Utilization != nil {
		at, _ := number(l.ResetsAt)
		limit := Limit{Name: "Individual", Fraction: *l.Utilization / 100, ResetsAt: epochTime(int64(at))}
		if l.Used != "" || l.Limit != "" {
			limit.Amount = reported(l.Used) + " / " + reported(l.Limit)
		}
		out.Limits = append(out.Limits, limit)
		if l.RemainingPercent != nil {
			out.Facts = append(out.Facts, Fact{Name: "Remaining",
				Value: strconv.FormatFloat(*l.RemainingPercent, 'f', 0, 64) + " %"})
		}
	}
	if value := credits(p["credits"]); value != "" {
		out.Facts = append(out.Facts, Fact{Name: "Credits", Value: value})
	}
	return out, nil
}

// credits is the app-server's credits object in a few words, or nothing
// where it is absent, null or a shape this does not know.
func credits(raw json.RawMessage) string {
	var c struct {
		HasCredits *bool           `json:"hasCredits"`
		Unlimited  *bool           `json:"unlimited"`
		Balance    json.RawMessage `json:"balance"`
	}
	if len(raw) == 0 || json.Unmarshal(raw, &c) != nil {
		return ""
	}
	switch {
	case c.Unlimited != nil && *c.Unlimited:
		return "unlimited"
	case len(c.Balance) > 0 && string(c.Balance) != "null":
		return strings.Trim(string(c.Balance), `"`)
	case c.HasCredits != nil && !*c.HasCredits:
		return "none"
	}
	return ""
}
