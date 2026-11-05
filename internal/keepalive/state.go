package keepalive

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/ushineko/hayami/internal/readings"
)

const (
	stateName = "vpn-keepalive.json"
	lockName  = "vpn-keepalive.lock"
	runName   = "vpn-keepalive.run"

	// maxStateBytes bounds a read: the file holds a few words per VPN.
	maxStateBytes = 1 << 20

	// lockTries and lockStep bound the wait for the file's lock, which is only held
	// for a read and a write.
	lockTries = 100
	lockStep  = 10 * time.Millisecond
)

// Retry is a re-dial waiting.
type Retry struct {
	Attempts int       `json:"attempts"`
	At       time.Time `json:"at"`
}

/*
State is what the helper and the panels tell each other (spec 085 R24), in a small
file rather than over a socket, so that a press counts when the helper is stopped.

A hold says a person took a tunnel down and it is to stay down. It lasts across a
restart of the helper, which vpn-toggle's does not; nothing is opened by that, since a
held tunnel is only left alone. A disable is a reason, and lasts until a person
re-enables the VPN, as vpn-toggle's did in its config.
*/
type State struct {
	Held     map[string]bool   `json:"held,omitempty"`
	Disabled map[string]string `json:"disabled,omitempty"`
	Retries  map[string]Retry  `json:"retries,omitempty"`
}

// IsHeld reports whether a person took this VPN down.
func (s *State) IsHeld(name string) bool { return s.Held[name] }

// IsDisabled reports whether the helper has given up on this VPN.
func (s *State) IsDisabled(name string) bool {
	_, ok := s.Disabled[name]
	return ok
}

// Hold records that a person took this VPN down, and ends any re-dial waiting.
func (s *State) Hold(name string) {
	if s.Held == nil {
		s.Held = map[string]bool{}
	}
	s.Held[name] = true
	delete(s.Retries, name)
}

// Release ends a hold.
func (s *State) Release(name string) { delete(s.Held, name) }

// Disable records that the helper gave up on this VPN, and why.
func (s *State) Disable(name, reason string) {
	if s.Disabled == nil {
		s.Disabled = map[string]string{}
	}
	s.Disabled[name] = reason
	delete(s.Retries, name)
}

// Enable ends a disable.
func (s *State) Enable(name string) { delete(s.Disabled, name) }

// SetRetry records the re-dial waiting.
func (s *State) SetRetry(name string, attempts int, at time.Time) {
	if s.Retries == nil {
		s.Retries = map[string]Retry{}
	}
	s.Retries[name] = Retry{Attempts: attempts, At: at}
}

// ClearStatus says no re-dial is waiting.
func (s *State) ClearStatus(name string) { delete(s.Retries, name) }

// Files is where a State lives and its locks.
type Files struct{ dir string }

// Here is the file set under $XDG_STATE_HOME/hayami, or ~/.local/state/hayami. Not the
// cache: deleting a cache must not bring back a VPN the helper was told to leave.
func Here() (Files, error) {
	dir := os.Getenv("XDG_STATE_HOME")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return Files{}, fmt.Errorf("finding the home directory: %w", err)
		}
		dir = filepath.Join(home, ".local", "state")
	}
	return Files{dir: filepath.Join(dir, "hayami")}, nil
}

// At is the file set in a directory of a test's choosing.
func At(dir string) Files { return Files{dir: dir} }

// Load reads the state. No file is the empty state, and so is one this program did
// not write: the worst of that is a press or a disable forgotten.
func (f Files) Load() State {
	var s State
	file, err := os.Open(filepath.Join(f.dir, stateName))
	if err != nil {
		return s
	}
	defer func() { _ = file.Close() }()
	raw, err := io.ReadAll(io.LimitReader(file, maxStateBytes))
	if err != nil || json.Unmarshal(raw, &s) != nil {
		return State{}
	}
	return s
}

// Update changes the state under its lock: both the helper and a panel write it, and
// each has only a part of what is in it.
func (f Files) Update(change func(*State)) error {
	release, err := f.lock()
	if err != nil {
		return err
	}
	defer release()
	s := f.Load()
	before, err := json.Marshal(s)
	if err != nil {
		return fmt.Errorf("encoding the keepalive state: %w", err)
	}
	change(&s)
	after, err := json.Marshal(s)
	if err != nil {
		return fmt.Errorf("encoding the keepalive state: %w", err)
	}
	// The helper calls this at every reading, and most change nothing.
	if bytes.Equal(before, after) {
		return nil
	}
	return f.write(after)
}

func (f Files) lock() (func(), error) {
	for range lockTries {
		release, ok, err := readings.TryLock(filepath.Join(f.dir, lockName))
		if err != nil {
			return nil, fmt.Errorf("locking the keepalive state: %w", err)
		}
		if ok {
			return release, nil
		}
		time.Sleep(lockStep)
	}
	return nil, errors.New("the keepalive state stayed locked") //nolint:err113 // said plainly
}

// write replaces the file in one step, so a reader never sees half of one.
func (f Files) write(raw []byte) error {
	if err := os.MkdirAll(f.dir, 0o700); err != nil {
		return fmt.Errorf("making the state directory: %w", err)
	}
	tmp, err := os.CreateTemp(f.dir, stateName+".*")
	if err != nil {
		return fmt.Errorf("writing the keepalive state: %w", err)
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	if _, err := tmp.Write(raw); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("writing the keepalive state: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("writing the keepalive state: %w", err)
	}
	if err := os.Rename(tmp.Name(), filepath.Join(f.dir, stateName)); err != nil {
		return fmt.Errorf("writing the keepalive state: %w", err)
	}
	return nil
}

// HoldRunning takes the lock that says the helper is running, for as long as it runs.
// A second helper gets ok false. The kernel lets go when the process dies.
func (f Files) HoldRunning() (release func(), ok bool, err error) {
	return readings.TryLock(filepath.Join(f.dir, runName)) //nolint:wrapcheck // says what it was doing
}

// Running reports whether a helper is running: whether that lock is held.
func (f Files) Running() bool {
	release, ok, err := readings.TryLock(filepath.Join(f.dir, runName))
	if err != nil {
		return false
	}
	if ok {
		release()
	}
	return !ok
}
