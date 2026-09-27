/*
Package usage takes part in the cache the Python tools already share.

It is a protocol, not a file format. `peripheral-battery-monitor` and
`claude-usage-widget-windows` each carry a copy of `usage_cache.py`, and its
docstring says why: they "resolve the identical path and genuinely share the
cache", so the widget, every terminal pane and any one-shot line together make
about one API request per account per window. During the port all three
programs run at once, and a cache of our own would double every upstream call.

This package is the protocol and nothing else: it does not know what a payload
means, and it holds `data` opaque so a field it has never heard of survives a
write.
*/
package usage

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// DirName is the directory the cache lives in, named after the widget rather
// than after this program. Both Python programs resolve this exact name and
// that is the point; renaming it would be leaving the cache, not joining it.
const DirName = "claude-usage-widget"

// ProviderClaude and ProviderCodex are the two providers the cache holds.
// Claude is the default and has no suffix, which is what keeps every filename
// that already exists on a machine.
const (
	ProviderClaude = "claude"
	ProviderCodex  = "codex"
)

// Dir is the cache directory, resolved the way the Python resolves it on each
// platform.
func Dir() (string, error) {
	if runtime.GOOS == "darwin" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err //nolint:wrapcheck // os names the variable it could not read
		}
		return filepath.Join(home, "Library", "Caches", DirName), nil
	}
	if local := os.Getenv("LOCALAPPDATA"); local != "" {
		return filepath.Join(local, DirName, "cache"), nil
	}
	if xdg := os.Getenv("XDG_CACHE_HOME"); xdg != "" {
		return filepath.Join(xdg, DirName), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err //nolint:wrapcheck // os names the variable it could not read
	}
	return filepath.Join(home, ".cache", DirName), nil
}

// Slug is the suffix that names one account's file.
//
// Every character that is not alphanumeric, a hyphen or an underscore becomes
// an underscore. The provider suffix is empty for Claude, which is what keeps
// the filenames that already exist. An empty account has no suffix at all: the
// widget's single-account files were named that way before accounts existed
// and are left where they are.
//
// This function is a contract with code this repository does not own, and
// getting it wrong is silent: hayami would write a file nothing else reads,
// and both programs would poll on their own. It is pinned by a test against
// the Python's own results.
func Slug(account, provider string) string {
	safeProvider := sanitise(provider)
	suffix := ""
	if safeProvider != ProviderClaude {
		suffix = "-" + safeProvider
	}
	if account == "" {
		return suffix
	}
	if safe := sanitise(account); safe != "" {
		return suffix + "-" + safe
	}
	return suffix
}

// sanitise replaces every character a filename should not carry.
func sanitise(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9',
			r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	return b.String()
}

// Path is one account's cache file.
func Path(account, provider string) (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "usage"+Slug(account, provider)+".json"), nil
}

// LockPath is the sibling lock file.
func LockPath(account, provider string) (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "usage"+Slug(account, provider)+".lock"), nil
}
