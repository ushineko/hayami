/*
Package readings keeps what each section last said, so a panel that has just
started shows it rather than a blank.

A section is empty until its first poll lands, and where that poll misses it
stays empty for a whole interval — fifteen seconds for the peripherals. A
wireless mouse that has been still answers nothing about one poll in fourteen,
so a missed first poll is the ordinary case rather than an edge.

What is kept is the rendered [view.Section], not a source's own state: a
section is already plain data that both shells know how to draw, and keeping
it this way means no source has to know the cache exists.

It is a cache and behaves like one. An absent, truncated or unreadable file is
a cold start, never an error — the panel works without it, and deleting it
costs one blank first frame.
*/
package readings

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/ushineko/hayami/internal/view"
)

// DirName is the cache directory, under whatever the platform's cache root is.
const DirName = "hayami"

// FileName is the cache itself: every section in one file, because they are
// written together and a panel reads all of them at once.
const FileName = "sections.json"

// MaxAge is how old a reading may be and still be worth drawing.
//
// A battery moves over hours, so the previous evening's reading is roughly
// true and better than a blank; a week later it is furniture, and a device
// that has since been put away would have a cell of its own. A day is the
// round number between those.
const MaxAge = 24 * time.Hour

// Entry is one section as it was last drawn, and when.
type Entry struct {
	At      time.Time    `json:"at"`
	Section view.Section `json:"section"`
}

// Cache is every section's last reading, by section key.
type Cache map[string]Entry

// File is where another file of this program's cache lives, beside the
// readings.
func File(name string) (string, error) {
	dir, err := dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, name), nil
}

// Path is where the cache lives.
func Path() (string, error) {
	dir, err := dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, FileName), nil
}

// dir is the platform's cache directory for this program, following the same
// order internal/usage does for the cache it shares with the Python tools.
func dir() (string, error) {
	if local := os.Getenv("LOCALAPPDATA"); local != "" {
		return filepath.Join(local, DirName), nil
	}
	if xdg := os.Getenv("XDG_CACHE_HOME"); xdg != "" {
		return filepath.Join(xdg, DirName), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("finding the cache directory: %w", err)
	}
	return filepath.Join(home, ".cache", DirName), nil
}

/*
Load reads the cache, dropping anything older than MaxAge.

A file that is not there, cannot be read or does not parse gives an empty
cache and no error. That is the whole contract of the thing: a panel must
start whatever state this file is in, including half-written by a previous
version or by a process that was killed.
*/
func Load(path string, now time.Time) Cache {
	raw, err := os.ReadFile(path) //nolint:gosec // a path this package composed
	if err != nil {
		return Cache{}
	}

	var c Cache
	if err := json.Unmarshal(raw, &c); err != nil {
		return Cache{}
	}

	out := make(Cache, len(c))
	for key, e := range c {
		if now.Sub(e.At) <= MaxAge {
			out[key] = e
		}
	}
	return out
}

/*
Save writes the cache atomically: a temporary file beside it and a rename.

Beside it rather than in the system temp directory, because a rename across
filesystems is not atomic. A panel killed mid-write leaves the previous cache
rather than half of one, which is the difference between a cold start and a
parse failure on every start until someone deletes the file.
*/
func Save(path string, c Cache) error {
	raw, err := json.Marshal(c)
	if err != nil {
		return fmt.Errorf("encoding the readings: %w", err)
	}
	return WriteAtomic(path, raw)
}

/*
WriteAtomic puts raw at path through a temporary file beside it and a rename,
so a reader finds the previous contents or the new ones and never half of
either. Save's reason, shared with the peripherals source's memory of devices
(spec 032), which lives in the same directory.
*/
func WriteAtomic(path string, raw []byte) error {
	// The user's own, and nobody else's business: these are readings from
	// their hardware. The temporary file below is created 0600 already.
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("making the cache directory: %w", err)
	}

	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".*")
	if err != nil {
		return fmt.Errorf("opening a temporary cache file: %w", err)
	}
	name := tmp.Name()

	if _, err := tmp.Write(raw); err != nil {
		_ = tmp.Close()
		_ = os.Remove(name)
		return fmt.Errorf("writing the readings: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(name)
		return fmt.Errorf("closing the temporary cache file: %w", err)
	}
	if err := os.Rename(name, path); err != nil {
		_ = os.Remove(name)
		return fmt.Errorf("putting the cache in place: %w", err)
	}
	return nil
}

// ErrNoEntry is a key the cache has nothing for. Not a failure: it is what a
// first run looks like.
var ErrNoEntry = errors.New("no reading cached for that section")

/*
Restore is a section's cached reading, marked as what it is.

Marked, because a restored reading is a claim about the past drawn in the
present, and the only thing distinguishing it is that both shells draw it dim.
The caller uses it when a live poll gave nothing and throws it away when one
did.
*/
func (c Cache) Restore(key string) (view.Section, error) {
	e, ok := c[key]
	if !ok {
		return view.Section{}, ErrNoEntry
	}
	s := e.Section
	s.Restored = true
	return s, nil
}
