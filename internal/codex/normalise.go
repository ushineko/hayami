package codex

import "encoding/json"

/*
Normalise reduces the app-server's reply to the shape the shared cache holds.

This is not tidying. The cache is shared with two Python programs and they
write *this* shape, not the app-server's: a hayami that cached the raw reply
would leave the widget unable to read what it wrote, and the cache exists so
that neither of them has to ask twice.

It also drops what the panel does not need. The app-server reports account
identifiers alongside the limits; nothing here carries them into a file that
lives in a cache directory.
*/
func Normalise(result json.RawMessage) (json.RawMessage, error) {
	var reply struct {
		ByID     map[string]rateLimits `json:"rateLimitsByLimitId"`
		Limits   *rateLimits           `json:"rateLimits"`
		PlanType string                `json:"planType"`
	}
	if err := json.Unmarshal(result, &reply); err != nil {
		return nil, wrap(err)
	}

	limits := reply.Limits
	if got, ok := reply.ByID["codex"]; ok {
		limits = &got
	}
	if limits == nil {
		limits = &rateLimits{}
	}

	out := normalised{
		Provider:             "codex",
		Primary:              window(limits.Primary),
		Secondary:            window(limits.Secondary),
		PlanType:             first(limits.PlanType, reply.PlanType),
		Credits:              limits.Credits,
		RateLimitReachedType: limits.RateLimitReachedType,
	}
	if l := limits.IndividualLimit; l != nil {
		remaining := percent(l.RemainingPercent)
		var utilization *float64
		if remaining != nil {
			v := 100 - *remaining
			utilization = &v
		}
		out.IndividualLimit = &individual{
			Utilization:      utilization,
			RemainingPercent: remaining,
			ResetsAt:         l.ResetsAt,
			Used:             l.Used,
			Limit:            l.Limit,
		}
	}

	b, err := json.Marshal(out)
	if err != nil {
		return nil, wrap(err)
	}
	return b, nil
}

// rateLimits is the part of the app-server's reply this package reads.
type rateLimits struct {
	Primary              *appWindow      `json:"primary"`
	Secondary            *appWindow      `json:"secondary"`
	PlanType             string          `json:"planType"`
	Credits              json.RawMessage `json:"credits"`
	RateLimitReachedType json.RawMessage `json:"rateLimitReachedType"`
	IndividualLimit      *appIndividual  `json:"individualLimit"`
}

type appWindow struct {
	UsedPercent        *float64 `json:"usedPercent"`
	WindowDurationMins *float64 `json:"windowDurationMins"`
	ResetsAt           *float64 `json:"resetsAt"`
}

type appIndividual struct {
	RemainingPercent *float64 `json:"remainingPercent"`
	ResetsAt         *float64 `json:"resetsAt"`
	Used             string   `json:"used"`
	Limit            string   `json:"limit"`
}

// normalised is the shape the cache holds, named as the Python names it.
type normalised struct {
	Provider             string          `json:"provider"`
	Primary              *cacheWindow    `json:"primary"`
	Secondary            *cacheWindow    `json:"secondary"`
	PlanType             string          `json:"plan_type"`
	Credits              json.RawMessage `json:"credits"`
	RateLimitReachedType json.RawMessage `json:"rate_limit_reached_type"`
	IndividualLimit      *individual     `json:"individual_limit"`
}

type cacheWindow struct {
	Utilization   *float64 `json:"utilization"`
	WindowMinutes *float64 `json:"window_minutes"`
	ResetsAt      *float64 `json:"resets_at"`
}

type individual struct {
	Utilization      *float64 `json:"utilization"`
	RemainingPercent *float64 `json:"remaining_percent"`
	ResetsAt         *float64 `json:"resets_at"`
	Used             string   `json:"used"`
	Limit            string   `json:"limit"`
}

// window converts one window, or nothing when the app-server said nothing
// about it. A window with no figure at all is absent rather than zero.
func window(w *appWindow) *cacheWindow {
	if w == nil {
		return nil
	}
	if w.UsedPercent == nil && w.WindowDurationMins == nil && w.ResetsAt == nil {
		return nil
	}
	return &cacheWindow{
		Utilization:   percent(w.UsedPercent),
		WindowMinutes: w.WindowDurationMins,
		ResetsAt:      w.ResetsAt,
	}
}

// percent clamps a reported percentage into the range one can be.
func percent(v *float64) *float64 {
	if v == nil {
		return nil
	}
	out := *v
	if out < 0 {
		out = 0
	}
	if out > 100 {
		out = 100
	}
	return &out
}

// first is the first of two strings that is not empty.
func first(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func wrap(err error) error {
	return &normaliseError{err}
}

type normaliseError struct{ err error }

func (e *normaliseError) Error() string { return "reading the app-server's limits: " + e.err.Error() }
func (e *normaliseError) Unwrap() error { return e.err }
