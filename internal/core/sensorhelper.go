package core

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"
)

/*
The sensor helper's lines, and the panel's end of its pipe (spec 103).

The helper is a Windows service, hayami-sensors, that reads the processor's
temperature through PawnIO (sanshoku's cpu package) and writes it down a
one-way pipe: a hello, then a reading a second. It replaces
LibreHardwareMonitor as where the panel gets that temperature on Windows;
LibreHardwareMonitor stays the next provider in the chain for a desk that
has it and not the helper.
*/

const (
	// SensorHelperService is the service's name.
	SensorHelperService = "hayami-sensors"
	// SensorHelperPipe is where it speaks.
	SensorHelperPipe = `\\.\pipe\hayami-sensors`
	// SensorHelperProtocol is the version of its lines.
	SensorHelperProtocol = 1
)

// SensorHelperLine is one line. The first has V and Version; the rest have
// C and Label, or Error when the helper could not go on.
type SensorHelperLine struct {
	V       int    `json:"v,omitempty"`
	Version string `json:"version,omitempty"`
	// C is the processor's temperature in degrees.
	C *float64 `json:"c,omitempty"`
	// Label is what the temperature is, in LibreHardwareMonitor's words:
	// "Core (Tctl/Tdie)", "Core (Tdie)", "CPU Package".
	Label string `json:"label,omitempty"`
	Error string `json:"error,omitempty"`
}

// SensorHelperReading is a reading line.
func SensorHelperReading(c float64, label string) SensorHelperLine {
	return SensorHelperLine{C: &c, Label: label}
}

// errNoSensorHelper is the helper not being installed, which sends the chain
// on to LibreHardwareMonitor. It is not an Absence: the reason a person is
// given is the next provider's.
var errNoSensorHelper = fmt.Errorf("the sensor helper is not installed: %w", errNotHere)

// sensorStale is how old the latest reading may be before it is no reading.
const sensorStale = 5 * time.Second

/*
sensorStream is the panel's end: it reads lines until the pipe closes and
keeps the latest reading.
*/
type sensorStream struct {
	now func() time.Time

	mu      sync.Mutex
	got     chan struct{} // closed at the first reading, or the end
	hello   bool
	reading float64
	label   string
	at      time.Time
	err     error
	waited  bool
}

func newSensorStream(now func() time.Time) *sensorStream {
	return &sensorStream{now: now, got: make(chan struct{})}
}

// run reads r until it ends, and records why.
func (s *sensorStream) run(r io.Reader) {
	err := s.read(r)
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err == nil {
		s.err = err
	}
	if s.err == nil {
		s.err = errors.New("the sensor helper closed its pipe") //nolint:err113 // said plainly
	}
	s.finish()
}

// finish ends the wait for the first reading. The caller holds the lock.
func (s *sensorStream) finish() {
	if !s.waited {
		s.waited = true
		close(s.got)
	}
}

func (s *sensorStream) read(r io.Reader) error {
	lines := bufio.NewScanner(r)
	for lines.Scan() {
		var line SensorHelperLine
		if err := json.Unmarshal(lines.Bytes(), &line); err != nil {
			return fmt.Errorf("the sensor helper's line: %w", err)
		}
		s.mu.Lock()
		switch {
		case !s.hello:
			if line.V != SensorHelperProtocol {
				s.mu.Unlock()
				return fmt.Errorf("the sensor helper speaks version %d, and this panel reads %d: install this hayami's helper", //nolint:err113 // the versions are the message
					line.V, SensorHelperProtocol)
			}
			s.hello = true
		case line.Error != "":
			s.err = fmt.Errorf("the sensor helper: %s", line.Error) //nolint:err113 // the helper's words
			s.mu.Unlock()
			return s.err
		case line.C != nil:
			s.reading, s.label, s.at = *line.C, line.Label, s.now()
			s.finish()
		}
		s.mu.Unlock()
	}
	return lines.Err() //nolint:wrapcheck // the caller says it was the pipe
}

// latest is the newest reading, or why there is none. ended reports a
// stream that will give no more.
func (s *sensorStream) latest() (c float64, label string, ended bool, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	switch {
	case s.err != nil:
		return 0, "", true, s.err
	case !s.at.IsZero() && s.now().Sub(s.at) <= sensorStale:
		return s.reading, s.label, false, nil
	default:
		return 0, "", false, errors.New("the sensor helper has sent no reading lately") //nolint:err113 // said plainly
	}
}
