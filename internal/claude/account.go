package claude

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// AccountFileEnv overrides where Claude Code's account record is looked for,
// so a test need not touch a real home directory.
const AccountFileEnv = "CLAUDE_USAGE_ACCOUNT_FILE"

// accountRecord is the part of Claude Code's ~/.claude.json this package
// reads: what the signed-in account's plan is called, and the tier that says
// which account that is. The file holds much else, names and addresses among
// it, and none of it is decoded.
type accountRecord struct {
	OAuthAccount struct {
		PlanDisplayName           string `json:"planDisplayName"`
		OrganizationRateLimitTier string `json:"organizationRateLimitTier"`
	} `json:"oauthAccount"`
}

// accountFile is Claude Code's account record.
func accountFile() (string, error) {
	if path := os.Getenv(AccountFileEnv); path != "" {
		return path, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err //nolint:wrapcheck // an absent record is not reported
	}
	return filepath.Join(home, ".claude.json"), nil
}

/*
recordedPlan is the plan's name as Claude Code's account record spells it, in
lower case, for the store whose rate-limit tier is tier; empty when the record
cannot vouch for that store.

The credential file's subscriptionType can lag the plan: an Ultra account's
store still says "max". The record names it ("Ultra"), but there is one record
for every profile -- the wrappers relocate only the credential file -- so it
describes whichever account last refreshed it. The tier is the check that it
describes this one.
*/
func recordedPlan(tier string) string {
	if tier == "" {
		return ""
	}
	path, err := accountFile()
	if err != nil {
		return ""
	}
	b, err := os.ReadFile(path) //nolint:gosec // Claude Code's own record, or the test's
	if err != nil {
		return ""
	}
	var rec accountRecord
	if err := json.Unmarshal(b, &rec); err != nil {
		return ""
	}
	if rec.OAuthAccount.OrganizationRateLimitTier != tier {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(rec.OAuthAccount.PlanDisplayName))
}
