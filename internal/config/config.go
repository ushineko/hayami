/*
Package config is the program's settings: one YAML file both shells read.

It is a file a person opens, edits and copies between machines, which is why it
is YAML in the user's config directory and not a toolkit's preference blob. The
store underneath is fynedesygn's, which keeps a section this build does not
know rather than dropping it -- two binaries at different versions read this
file, and a save that quietly deleted the other one's setting is the failure
that rule exists for.
*/
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ushineko/fynedesygn/settings"
	_ "github.com/ushineko/fynedesygn/settings/yamlcodec" // registers .yaml

	"github.com/ushineko/hayami/internal/view"
)

// Key is the settings section this program owns. Everything under
// "fynedesygn." belongs to the library.
const Key = "hayami"

// Config is what the user has chosen.
//
// The json tags name the fields in the YAML too: the codec encodes through
// JSON so one set of tags governs both, rather than two sets to keep in step.
type Config struct {
	// Sections are the sections to draw, in the order to draw them. A section
	// that is not listed is not drawn and does not poll.
	Sections []string `json:"sections"`

	// Arrangement is "stack", "grid" or "row".
	Arrangement string `json:"arrangement"`

	// Interfaces are the network interfaces the bandwidth section watches.
	Interfaces []string `json:"interfaces"`
}

// Default is what a machine with no settings file gets: everything that exists
// so far, stacked, and no interfaces -- the bandwidth section stays dark until
// the user names one, because guessing an interface would be this program
// deciding what is interesting about someone's network.
func Default() Config {
	return Config{
		Sections:    []string{"bandwidth"},
		Arrangement: "stack",
	}
}

// Path is where the settings live: $XDG_CONFIG_HOME/hayami/settings.yaml, or
// ~/.config/hayami/settings.yaml.
func Path() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("finding the configuration directory: %w", err)
	}
	return filepath.Join(dir, "hayami", "settings.yaml"), nil
}

// Store is the settings file and the program's view of it.
type Store struct {
	store *settings.Store
}

// Open reads the settings at a path, creating nothing until something is set.
//
// A file that does not parse is not an error here. The library returns the
// store anyway and refuses to save over a file nobody could read; the panel
// draws on defaults and says so, because a panel that would not start could
// not be fixed from the pane it failed in. Unreadable carries the reason.
func Open(path string) (*Store, error) {
	s, err := settings.Open(path)
	var parse *settings.ParseError
	if err != nil && !errors.As(err, &parse) {
		return nil, fmt.Errorf("opening the settings: %w", err)
	}
	return &Store{store: s}, nil
}

// Config is the program's settings, with a default for every field the file
// does not carry. A file that cannot be parsed yields the defaults and the
// error is available from Unreadable, so the panel draws rather than refusing
// to start over a stray tab character.
func (s *Store) Config() Config {
	c := Default()
	s.store.Get(Key, &c)
	if c.Arrangement == "" {
		c.Arrangement = "stack"
	}
	return c
}

// SetConfig writes the settings back.
func (s *Store) SetConfig(c Config) error {
	if err := s.store.Set(Key, c); err != nil {
		return fmt.Errorf("saving the settings: %w", err)
	}
	return nil
}

// Unreadable reports a settings file that could not be parsed, so a program
// can say so rather than silently running on defaults and losing the file on
// its next save.
func (s *Store) Unreadable() error {
	//nolint:wrapcheck // the library's message names the file and the line,
	// which is the whole point of showing it to someone.
	return s.store.Unreadable()
}

// Flush writes any pending change now, for a program about to exit.
func (s *Store) Flush() error {
	if err := s.store.Flush(); err != nil {
		return fmt.Errorf("writing the settings: %w", err)
	}
	return nil
}

// ParseArrangement is the configured arrangement, or an error naming what the
// user typed when it is not one.
func (c Config) ParseArrangement() (view.Arrangement, error) {
	return view.ParseArrangement(c.Arrangement)
}

// Shows reports whether a section key is one the user asked for.
func (c Config) Shows(key string) bool {
	for _, k := range c.Sections {
		if k == key {
			return true
		}
	}
	return false
}
