package core

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"golang.org/x/sys/windows"
)

/*
The panel's side of the network helper (spec 061).

The helper is a Windows service, hayami-network, that the installer registers
when asked. It runs as Local Service, which the kernel's network provider
allows, and only while something reads it: this reader starts it, the service
stops itself when its last reader has gone. What crosses the pipe is process
IDs and byte counts and nothing else.

The service's access list lets the account that installed it start it, so
starting it is a call to the service manager and asks nobody for anything.
*/

const (
	// NetHelperService is the service's name.
	NetHelperService = "hayami-network"
	// NetHelperPipe is where it speaks.
	NetHelperPipe = `\\.\pipe\hayami-network`
)

// helperStartWait is how long a service just started is given to open its
// pipe. Measured: under half a second.
const helperStartWait = 5 * time.Second

// newHelperNet is the network by process as the helper reports it.
func newHelperNet(samples func(context.Context) ([]ProcessSample, error)) NetReader {
	return &helperNet{samples: samples, pipe: NetHelperPipe, start: startNetHelper, now: time.Now}
}

/*
NewNetHelperReader reads a helper at another pipe, started another way. It
is how the helper's own tests read a server they started, with no service
and no administrator.
*/
func NewNetHelperReader(pipe string, start func() error, samples func(context.Context) ([]ProcessSample, error)) NetReader {
	return &helperNet{samples: samples, pipe: pipe, start: start, now: time.Now}
}

type helperNet struct {
	samples func(context.Context) ([]ProcessSample, error)
	pipe    string
	// start starts the service, or says errNoNetHelper where there is none.
	start func() error
	now   func() time.Time

	mu     sync.Mutex
	handle windows.Handle
	stream *helperStream
	lastAt time.Time
}

// Read is each process's rate since the previous call. The first call opens
// the pipe, starting the service if it is stopped, and has no rate.
func (h *helperNet) Read(ctx context.Context) (NetUse, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	now := h.now()
	if h.stream == nil {
		if err := h.connect(ctx); err != nil {
			return NetUse{}, err
		}
		h.lastAt = now
		return NetUse{Counts: traceCounts}, nil
	}
	counts, what, err := h.stream.take()
	if err != nil {
		h.drop()
		return NetUse{}, err
	}
	elapsed := now.Sub(h.lastAt).Seconds()
	h.lastAt = now
	if what == "" {
		what = traceCounts
	}
	return netUse(ctx, h.samples, what, counts, elapsed), nil
}

// connect opens the pipe, starting the service first where the pipe is not
// there. The caller holds the lock.
func (h *helperNet) connect(ctx context.Context) error {
	handle, err := openPipe(h.pipe)
	if errors.Is(err, windows.ERROR_FILE_NOT_FOUND) {
		if err := h.start(); err != nil {
			return err
		}
		for deadline := h.now().Add(helperStartWait); ; {
			if handle, err = openPipe(h.pipe); !errors.Is(err, windows.ERROR_FILE_NOT_FOUND) {
				break
			}
			if !h.now().Before(deadline) {
				return fmt.Errorf("the network helper started and opened no pipe: %w", err)
			}
			select {
			case <-ctx.Done():
				return ctx.Err() //nolint:wrapcheck // the context's own message is the whole story
			case <-time.After(50 * time.Millisecond):
			}
		}
	}
	if err != nil {
		return fmt.Errorf("opening the network helper's pipe: %w", err)
	}
	h.handle, h.stream = handle, &helperStream{}
	go h.stream.run(pipeReader(handle))
	return nil
}

// openPipe opens the helper's pipe for reading. A pipe whose every instance
// is taken is waited for, briefly.
func openPipe(name string) (windows.Handle, error) {
	path, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return 0, err //nolint:wrapcheck // a name with a NUL in it
	}
	for range 3 {
		handle, err := windows.CreateFile(path, windows.GENERIC_READ, 0, nil, windows.OPEN_EXISTING, 0, 0)
		if err == nil {
			return handle, nil
		}
		if !errors.Is(err, windows.ERROR_PIPE_BUSY) {
			return 0, err //nolint:wrapcheck // the caller names what it was doing
		}
		time.Sleep(100 * time.Millisecond)
	}
	return 0, windows.ERROR_PIPE_BUSY
}

// pipeReader reads a pipe's handle.
type pipeReader windows.Handle

func (p pipeReader) Read(b []byte) (int, error) {
	var n uint32
	if err := windows.ReadFile(windows.Handle(p), b, &n, nil); err != nil {
		return int(n), err //nolint:wrapcheck // the stream names what it was doing
	}
	return int(n), nil
}

// drop lets the pipe go. The caller holds the lock.
func (h *helperNet) drop() {
	if h.handle != 0 {
		// The read in progress is on another thread and holds the handle.
		_ = windows.CancelIoEx(h.handle, nil)
		_ = windows.CloseHandle(h.handle)
	}
	h.handle, h.stream = 0, nil
}

// Close lets the pipe go, and the service stops when nobody else reads it.
// A later Read opens it again.
func (h *helperNet) Close() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.drop()
}

/*
startNetHelper starts the service. A service that is not there is
errNoNetHelper; one this account may not start is a typed absence, since it
is another account's and the installer puts that right.
*/
func startNetHelper() error {
	manager, err := windows.OpenSCManager(nil, nil, windows.SC_MANAGER_CONNECT)
	if err != nil {
		return fmt.Errorf("asking the service manager: %w", err)
	}
	defer func() { _ = windows.CloseServiceHandle(manager) }()
	name, _ := windows.UTF16PtrFromString(NetHelperService)
	service, err := windows.OpenService(manager, name, windows.SERVICE_START)
	switch {
	case errors.Is(err, windows.ERROR_SERVICE_DOES_NOT_EXIST):
		return errNoNetHelper
	case errors.Is(err, windows.ERROR_ACCESS_DENIED):
		return &Absence{Code: AbsenceNetProcessesDenied, Text: "helper is another's",
			Detail: "the network helper is installed for another account: run hayami's installer again " +
				"and choose it, or run `hayami-tui network-helper install`",
			Err: err}
	case err != nil:
		return fmt.Errorf("opening the network helper's service: %w", err)
	}
	defer func() { _ = windows.CloseServiceHandle(service) }()
	if err := windows.StartService(service, 0, nil); err != nil && !errors.Is(err, windows.ERROR_SERVICE_ALREADY_RUNNING) {
		return fmt.Errorf("starting the network helper: %w", err)
	}
	return nil
}
