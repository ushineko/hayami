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
	fdtheme "github.com/ushineko/fynedesygn/theme"

	"github.com/ushineko/hayami/internal/desktop"
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

	// Font and Mono are the panel's own faces: the interface family and the
	// monospace one.
	//
	// The panel's own, not the preferences window's. Empty means the family
	// chosen in Appearance, which is what a settings file written before
	// these existed carries.
	//
	// They are two settings and not one because the panel uses both, for the
	// reason the design system keeps them apart: a label is read as words and
	// a reading is read as a column, and a column needs every digit the same
	// width.
	Font string `json:"font"`
	Mono string `json:"mono"`

	// FontSize is the panel's text size, in points.
	//
	// The panel's own, not the preferences window's. They are read at
	// different distances — a panel from across a desk, a settings window at
	// arm's length — and a Fyne theme is application-wide, so the panel
	// carries a theme of its own to hold this.
	//
	// Zero means whatever the appearance says, which is what a settings file
	// written before this field existed carries.
	FontSize float32 `json:"fontSize"`

	// Opacity is how opaque the desktop panel is, as a percentage.
	//
	// It is only ever applied by the compositor, through the window rule:
	// nothing this process draws can be see-through, so this is a number
	// hayami keeps and KWin acts on. Zero means the default, which is what a
	// settings file written before this field existed carries.
	Opacity int `json:"opacity"`

	// X and Y are where the panel last was, in the compositor's coordinates.
	//
	// Kept because a Wayland client cannot place itself and cannot read
	// where it is: both are the compositor's, and the only way to either is
	// a KWin script. A glance panel lives in one corner of one screen and
	// being asked to put it there again at every login is the friction that
	// makes a thing not worth running.
	//
	// Placed is the marker for "we have a position", because zero is a
	// legal coordinate -- the top-left corner of the leftmost screen -- and
	// a settings file written before these existed has to mean "no".
	X      int  `json:"x"`
	Y      int  `json:"y"`
	Placed bool `json:"placed"`

	// LHM is where LibreHardwareMonitor serves its sensor tree, which is where
	// the processor's temperature comes from on Windows (spec 036). Empty is
	// its default address, http://127.0.0.1:8085/data.json, which is what a
	// settings file written before this field existed carries. Settings file
	// only: it is set once, to match a port changed in LibreHardwareMonitor,
	// and a preferences page for one address would be a page for nobody.
	LHM string `json:"lhm,omitempty"`
}

// Position is where the panel last was, and whether it has ever been told.
func (c Config) Position() (x, y int, ok bool) { return c.X, c.Y, c.Placed }

// WithPosition is the config with a new position remembered.
func (c Config) WithPosition(x, y int) Config {
	c.X, c.Y, c.Placed = x, y, true
	return c
}

// FontSizeOr is the panel's text size, falling back to the appearance's when
// the panel has not been given one of its own.
//
// Zero is not a legal size, so it is the marker for "not set" — which is what
// every settings file written before this field existed has.
func (c Config) FontSizeOr(appearance float32) float32 {
	if c.FontSize <= 0 {
		return appearance
	}
	return c.FontSize
}

// PanelAppearance is the appearance the panel draws in: the one chosen in
// Appearance, with whatever the panel has been given of its own laid over it.
//
// A field the panel has not been given falls through to the appearance, so a
// panel that has only been given a size still follows the scheme and the faces
// the user picked for everything else.
func (c Config) PanelAppearance(a fdtheme.Appearance) fdtheme.Appearance {
	if c.Font != "" {
		a.Font = c.Font
	}
	if c.Mono != "" {
		a.Mono = c.Mono
	}
	a.TextSize = c.FontSizeOr(a.TextSize)
	return a
}

// OpacityOrDefault is the opacity to use, resolving the unset zero.
//
// Zero is not a legal opacity — an invisible panel is not a setting anyone
// chose — so it is the marker for "not set", which is what every settings
// file written before this field existed has.
func (c Config) OpacityOrDefault() int {
	if c.Opacity <= 0 || c.Opacity > 100 {
		return desktop.DefaultOpacity
	}
	return c.Opacity
}

// Default is what a machine with no settings file gets: everything that exists
// so far, stacked, and no interfaces -- the bandwidth section stays dark until
// the user names one, because guessing an interface would be this program
// deciding what is interesting about someone's network.
func Default() Config {
	return Config{
		Sections:    []string{"bandwidth", "usage", "cooler", "peripherals"},
		Arrangement: "stack",
		Opacity:     desktop.DefaultOpacity,
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

// Settings is the store underneath, for the parts of the settings file this
// package does not own.
//
// The design system keeps the appearance — scheme, fonts, text size, scale —
// in its own section of the same file, and reads and writes it through this
// type. hayami neither parses nor validates any of it; it hands the store
// over and lets the library do both.
func (s *Store) Settings() *settings.Store { return s.store }

// Flush writes any pending change now, without giving the store up.
func (s *Store) Flush() error {
	if err := s.store.Flush(); err != nil {
		return fmt.Errorf("writing the settings: %w", err)
	}
	return nil
}

/*
Close writes any pending change and stops the store writing on its own.

For a program about to exit, which is the case this was missing. A change
made in the last moments -- the position the compositor reported as the
window went away is the one that matters here -- is in the store's memory with
a write scheduled a second later, and the process does not last a second. The
panel lost the position it had just been moved to, every time, and the only
reason the setting ever reached disk was that the panel usually sat still for
longer than the timer.
*/
func (s *Store) Close() error {
	if err := s.store.Close(); err != nil {
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
