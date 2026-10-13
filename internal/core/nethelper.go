package core

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"
)

/*
The network helper's protocol (spec 061): what a service that may trace the
network says to a panel that may not. It is lines of JSON down a pipe, one
way. The helper reads nothing a panel writes, so a panel has nothing to send
that the helper could mistake.

The first line says who is speaking. Each line after it is the bytes every
process received and sent since the line before, about a second apart. The
panel adds them up between its own readings and divides by its own clock.

What is here has no build tag: the lines, and the adding up, which a test on
any system runs.
*/

// NetHelperProtocol is the version of the lines. A panel that meets another
// says so and does not guess.
const NetHelperProtocol = 1

// NetHelperLine is one line. The first has V, Version and Counts; the rest
// have P, or Error when the helper could not go on.
type NetHelperLine struct {
	V       int    `json:"v,omitempty"`
	Version string `json:"version,omitempty"`
	// Counts is what the numbers cover, such as "TCP and UDP".
	Counts string `json:"counts,omitempty"`
	// P is one process to an entry: its ID, bytes received, bytes sent.
	P [][3]uint64 `json:"p,omitempty"`
	// Error is why the helper stopped, in its words.
	Error string `json:"error,omitempty"`
}

// errNoNetHelper is the helper not being installed, which is the ordinary
// state of a desk and sends the reader on to the next way of asking.
var errNoNetHelper = errors.New("the network helper is not installed")

/*
helperStream is the panel's end: it reads lines until the pipe closes and
keeps the bytes counted since they were last taken.
*/
type helperStream struct {
	mu     sync.Mutex
	hello  NetHelperLine
	counts map[uint32][2]uint64
	err    error
	done   bool
}

// run reads r to its end. It returns when the pipe closes or a line is not
// one this version reads.
func (s *helperStream) run(r io.Reader) {
	err := s.read(r)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.done = true
	if s.err == nil {
		s.err = err
	}
}

func (s *helperStream) read(r io.Reader) error {
	lines := bufio.NewScanner(r)
	lines.Buffer(make([]byte, 0, 64<<10), 1<<20)
	first := true
	for lines.Scan() {
		var line NetHelperLine
		if err := json.Unmarshal(lines.Bytes(), &line); err != nil {
			return fmt.Errorf("the network helper sent a line that is not one: %w", err)
		}
		if line.Error != "" {
			return fmt.Errorf("the network helper: %s", line.Error) //nolint:err113 // the helper's own words
		}
		if first {
			first = false
			if line.V != NetHelperProtocol {
				return fmt.Errorf("the network helper speaks version %d of its protocol and this panel version %d: "+ //nolint:err113 // both numbers are the message
					"install the helper again from this version's installer", line.V, NetHelperProtocol)
			}
			s.mu.Lock()
			s.hello = line
			s.mu.Unlock()
			continue
		}
		s.mu.Lock()
		if s.counts == nil {
			s.counts = map[uint32][2]uint64{}
		}
		for _, p := range line.P {
			c := s.counts[uint32(p[0])] //nolint:gosec // a process ID
			c[0] += p[1]
			c[1] += p[2]
			s.counts[uint32(p[0])] = c //nolint:gosec // a process ID
		}
		s.mu.Unlock()
	}
	if err := lines.Err(); err != nil {
		return fmt.Errorf("reading the network helper: %w", err)
	}
	return errors.New("the network helper closed its pipe") //nolint:err113 // an end, said plainly
}

// take is the bytes counted since the previous call and what they cover, or
// why the stream ended.
func (s *helperStream) take() (map[uint32][2]uint64, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.done {
		return nil, "", s.err
	}
	counts := s.counts
	s.counts = nil
	return counts, s.hello.Counts, nil
}

// NetHelperCounts is a line of counts, for the helper to send.
func NetHelperCounts(counts map[uint32][2]uint64) NetHelperLine {
	line := NetHelperLine{P: make([][3]uint64, 0, len(counts))}
	for pid, c := range counts {
		line.P = append(line.P, [3]uint64{uint64(pid), c[0], c[1]})
	}
	return line
}
