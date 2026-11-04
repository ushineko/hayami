package panel

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"

	"github.com/ushineko/hayami/internal/claude"
	"github.com/ushineko/hayami/internal/readings"
)

// plansFile is the memory of each credential store's plan, beside the
// section cache.
const plansFile = "claude-plans.json"

/*
seenPlan is a plan Claude Code's account record once named for a store, and
what the store said when it did.

The record describes whichever account last refreshed it, so a plan it named
for this store is lost the moment another profile uses Claude Code (#286).
The memory keeps it, for as long as the store still says what it said then:
a store whose subscriptionType or tier changes probably holds another
account, and its plan is forgotten. Nothing in the store names its account,
so a different account with the same type and tier is not told apart; the
next time the record matches, it is corrected.

Neither field is a secret: the tokens are never written here.
*/
type seenPlan struct {
	Plan             string `json:"plan"`
	SubscriptionType string `json:"subscriptionType"`
	Tier             string `json:"tier"`
}

// plansMu serialises the memory's read, change and write: a poll and the
// drawer both look the stores up.
var plansMu sync.Mutex

// loadPlans is the memory, by store name. A missing, unreadable or malformed
// file is a memory of nothing.
func loadPlans(path string) map[string]seenPlan {
	plans := map[string]seenPlan{}
	raw, err := os.ReadFile(path) //nolint:gosec // a path this package composed
	if err != nil {
		return plans
	}
	if json.Unmarshal(raw, &plans) != nil {
		return map[string]seenPlan{}
	}
	return plans
}

// savePlans writes the memory. Map keys are encoded in order, so an unchanged
// memory is the same bytes; the caller writes only when it changed.
func savePlans(path string, plans map[string]seenPlan) error {
	raw, err := json.Marshal(plans)
	if err != nil {
		return fmt.Errorf("encoding the plans: %w", err)
	}
	return readings.WriteAtomic(path, raw)
}

/*
planOf is the plan a store is called by, the record's or the remembered one,
with plans brought up to date: a plan the record names is remembered, and a
remembered one the store no longer agrees with is forgotten. It reports
whether plans changed.
*/
func planOf(name string, creds *claude.Credentials, plans map[string]seenPlan) (string, bool) {
	was, known := plans[name]
	if creds.RecordedPlan != "" {
		now := seenPlan{Plan: creds.RecordedPlan, SubscriptionType: creds.Plan, Tier: creds.Tier}
		plans[name] = now
		return creds.RecordedPlan, !known || was != now
	}
	if !known {
		return "", false
	}
	if was.Tier == "" || was.Tier != creds.Tier || was.SubscriptionType != creds.Plan {
		delete(plans, name)
		return "", true
	}
	return was.Plan, false
}

/*
rememberPlans gives each store read in found the plan it is called by,
through the memory, and drops the memory of stores that are gone. A store
that could not be read keeps its memory: it is one account absent this poll.
The memory is the panel's cache, and a panel that cannot write it still draws.
*/
func rememberPlans(found map[string]*claude.Credentials, stores map[string]claude.Store) {
	path, err := readings.File(plansFile)
	if err != nil {
		return
	}
	plansMu.Lock()
	defer plansMu.Unlock()

	plans := loadPlans(path)
	changed := false
	for name, creds := range found {
		plan, c := planOf(name, creds, plans)
		creds.RecordedPlan = plan
		changed = changed || c
	}
	for name := range plans {
		if _, ok := stores[name]; !ok {
			delete(plans, name)
			changed = true
		}
	}
	if changed {
		_ = savePlans(path, plans)
	}
}
