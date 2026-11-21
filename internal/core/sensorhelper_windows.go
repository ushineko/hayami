package core

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"golang.org/x/sys/windows"
)

// sensorFirstWait is how long a pipe just opened is given for its first
// reading: the helper sends one a second.
const sensorFirstWait = 2500 * time.Millisecond

/*
sensorHelper reads the processor's temperature from the sensor helper (spec
103). It keeps the pipe open between polls, as the helper sends a reading a
second whether or not one is asked for; the helper stops itself a while after
its last reader goes.

The pipe is opened and checked as the network helper's is: anonymously, and
refused unless the process serving it is the one the service manager started
for hayami-sensors.
*/
type sensorHelper struct {
	pipe    string
	service string
	start   func(string) error
	verify  func(server uint32) error
	now     func() time.Time

	mu     sync.Mutex
	handle windows.Handle
	stream *sensorStream
}

// newSensorHelper is the reader of the installed helper.
func newSensorHelper() *sensorHelper {
	h := &sensorHelper{pipe: SensorHelperPipe, service: SensorHelperService, start: startHelper, now: time.Now}
	h.verify = func(server uint32) error {
		pid, err := helperProcess(h.service)
		if err != nil {
			return fmt.Errorf("asking after the sensor helper's service: %w", err)
		}
		if server == 0 || server != pid {
			return &Absence{Code: AbsenceSensorHelper, Text: "helper's pipe is taken",
				Detail: fmt.Sprintf("the pipe %s is held by process %d, which is not hayami's sensor helper; "+
					"the temperature is not read from it", h.pipe, server)}
		}
		return nil
	}
	return h
}

// CPUTemperature is the helper's latest reading. The first call opens the
// pipe, starting the service, and waits for the helper's first reading.
func (h *sensorHelper) CPUTemperature(ctx context.Context) (float64, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.stream == nil {
		if err := h.connect(ctx); err != nil {
			return 0, err
		}
	}
	select {
	case <-h.stream.got:
	case <-ctx.Done():
		return 0, ctx.Err() //nolint:wrapcheck // the context's own message is the whole story
	case <-time.After(sensorFirstWait):
	}
	c, _, ended, err := h.stream.latest()
	if ended {
		h.drop()
	}
	if err != nil {
		return 0, &Absence{Code: AbsenceSensorHelper, Detail: err.Error(), Err: err}
	}
	return c, nil
}

// connect opens the pipe, starting the service where it is not there. The
// caller holds the lock.
func (h *sensorHelper) connect(ctx context.Context) error {
	handle, err := openPipe(h.pipe)
	if errors.Is(err, windows.ERROR_FILE_NOT_FOUND) {
		if err := h.start(h.service); err != nil {
			switch {
			case errors.Is(err, windows.ERROR_SERVICE_DOES_NOT_EXIST):
				return errNoSensorHelper
			case errors.Is(err, windows.ERROR_ACCESS_DENIED):
				return &Absence{Code: AbsenceSensorHelper, Text: "helper is another's", Err: err,
					Detail: "the sensor helper is installed for another account: run `hayami-tui sensor-helper install`"}
			}
			return &Absence{Code: AbsenceSensorHelper, Detail: "the sensor helper did not start: " + err.Error(), Err: err}
		}
		for deadline := h.now().Add(helperStartWait); ; {
			if handle, err = openPipe(h.pipe); !errors.Is(err, windows.ERROR_FILE_NOT_FOUND) {
				break
			}
			if !h.now().Before(deadline) {
				return &Absence{Code: AbsenceSensorHelper, Err: err,
					Detail: "the sensor helper started and opened no pipe: its log is the Event Log's"}
			}
			select {
			case <-ctx.Done():
				return ctx.Err() //nolint:wrapcheck // the context's own message is the whole story
			case <-time.After(50 * time.Millisecond):
			}
		}
	}
	if err != nil {
		return fmt.Errorf("opening the sensor helper's pipe: %w", err)
	}
	var server uint32
	if err := windows.GetNamedPipeServerProcessId(handle, &server); err != nil {
		_ = windows.CloseHandle(handle)
		return fmt.Errorf("asking who serves the sensor helper's pipe: %w", err)
	}
	if err := h.verify(server); err != nil {
		_ = windows.CloseHandle(handle)
		return err
	}
	h.handle, h.stream = handle, newSensorStream(h.now)
	go h.stream.run(pipeReader(handle))
	return nil
}

// drop lets the pipe go; the next read opens it again. The caller holds the
// lock.
func (h *sensorHelper) drop() {
	if h.handle != 0 {
		_ = windows.CancelIoEx(h.handle, nil)
		_ = windows.CloseHandle(h.handle)
	}
	h.handle, h.stream = 0, nil
}
