/*
Package claude reads the credential stores Claude Code keeps, and fetches the
usage they give access to.

**This package holds tokens.** Three rules follow from that and none of them
are negotiable: a token is never logged, never printed and never put anywhere
but back in the file it came from; a store that did not fully parse is never
written; and a write is atomic, because the file belongs to Claude Code and
leaving it with fewer fields than it had would break a program somebody needs.
*/
package claude

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// The environment variable and the directories the credential stores live in.
//
// Claude Code chooses its store with CLAUDE_SECURESTORAGE_CONFIG_DIR, which
// relocates the credential file and nothing else. That variable is
// per-process, so a running panel cannot ask the system which profiles exist;
// discovery keys on the directory convention the claude-max and claude-work
// wrappers establish instead.
const (
	// ProfileRootEnv overrides where the per-profile stores are looked for,
	// so a test need not touch a real home directory.
	ProfileRootEnv = "CLAUDE_USAGE_PROFILE_ROOT"

	// CredentialsFile is the file inside each store.
	CredentialsFile = ".credentials.json"

	// DefaultProfileName is what the default store is called on screen. It
	// keeps no name of its own and is reached by claude-max, so that is what
	// it is called.
	DefaultProfileName = "max"
)

// Store is one credential file: where it is and what it holds.
type Store struct {
	// Name is the profile, as the cache filenames spell it.
	Name string

	// Path is the credential file.
	Path string
}

// Credentials are what a store holds.
//
// The tokens are here because using them is the point; they go no further than
// the request that carries them and the file they came from. Raw keeps every
// field this build does not know, so a write puts back what it read.
type Credentials struct {
	AccessToken  string
	RefreshToken string
	ExpiresAt    int64

	// Plan is subscriptionType: "max", "enterprise", "pro", "team". The usage
	// API does not report it, and a store whose token has expired should
	// still be labelled correctly.
	Plan string

	// raw is the whole file as it was read, so a write preserves every key
	// this build has never heard of. The file is Claude Code's.
	raw map[string]json.RawMessage
}

// oauth is the object inside the credential file.
type oauth struct {
	AccessToken  string `json:"accessToken"`
	RefreshToken string `json:"refreshToken"`
	ExpiresAt    int64  `json:"expiresAt"`

	SubscriptionType string `json:"subscriptionType"`
}

// oauthKey is the one key this package reads and writes.
const oauthKey = "claudeAiOauth"

// ProfileRoot is where the per-profile stores live.
func ProfileRoot() (string, error) {
	if root := os.Getenv(ProfileRootEnv); root != "" {
		return root, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("finding the home directory: %w", err)
	}
	return filepath.Join(home, ".claude-credentials"), nil
}

// DefaultStore is the store Claude Code uses when nothing relocates it.
func DefaultStore() (Store, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return Store{}, fmt.Errorf("finding the home directory: %w", err)
	}
	return Store{
		Name: DefaultProfileName,
		Path: filepath.Join(home, ".claude", CredentialsFile),
	}, nil
}

// Stores are the credential stores on this machine, default first and the
// profiles after it in name order, so a panel's accounts do not change places
// between two runs.
//
// A directory with no credential file is skipped: the wrapper scripts create
// the directory before the login that fills it.
func Stores() ([]Store, error) {
	var out []Store

	if s, err := DefaultStore(); err == nil {
		if _, err := os.Stat(s.Path); err == nil {
			out = append(out, s)
		}
	}

	root, err := ProfileRoot()
	if err != nil {
		return out, err
	}
	entries, err := os.ReadDir(root)
	if os.IsNotExist(err) {
		return out, nil
	}
	if err != nil {
		return out, fmt.Errorf("listing the credential stores: %w", err)
	}

	var profiles []Store
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		path := filepath.Join(root, e.Name(), CredentialsFile)
		if _, err := os.Stat(path); err != nil {
			continue
		}
		profiles = append(profiles, Store{Name: e.Name(), Path: path})
	}
	sort.Slice(profiles, func(i, j int) bool { return profiles[i].Name < profiles[j].Name })
	return append(out, profiles...), nil
}

// Read returns what a store holds.
//
// The whole file is kept, not only the fields this build understands. That is
// what makes a write safe: Read and Write are two halves of one operation, and
// the half that forgets a key is the half that damages the file.
func Read(s Store) (*Credentials, error) {
	b, err := os.ReadFile(s.Path) //nolint:gosec // the store the caller named
	if err != nil {
		return nil, fmt.Errorf("reading a credential store: %w", err)
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil, fmt.Errorf("a credential store does not parse: %w", err)
	}
	body, ok := raw[oauthKey]
	if !ok {
		return nil, fmt.Errorf("a credential store holds no %s", oauthKey)
	}
	var o oauth
	if err := json.Unmarshal(body, &o); err != nil {
		return nil, fmt.Errorf("a credential store's %s does not parse: %w", oauthKey, err)
	}

	return &Credentials{
		AccessToken:  o.AccessToken,
		RefreshToken: o.RefreshToken,
		ExpiresAt:    o.ExpiresAt,
		Plan:         o.SubscriptionType,
		raw:          raw,
	}, nil
}

// Badge is the one letter a panel puts beside an account's name: M for Max, E
// for Enterprise, P for Pro, T for Team. An unknown plan has no badge rather
// than a wrong one.
func (c *Credentials) Badge() string {
	if c == nil {
		return ""
	}
	switch c.Plan {
	case "max":
		return "M"
	case "enterprise":
		return "E"
	case "pro":
		return "P"
	case "team":
		return "T"
	default:
		return ""
	}
}
