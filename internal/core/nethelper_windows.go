package core

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
	"unsafe"

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
	return &helperNet{
		samples: samples, pipe: NetHelperPipe, start: startNetHelper, now: time.Now,
		verify: serverIsTheService,
	}
}

/*
NewNetHelperReader reads a helper at another pipe, started another way. It
is how the helper's own tests read a server they started, with no service
and no administrator.
*/
func NewNetHelperReader(pipe string, start func() error, samples func(context.Context) ([]ProcessSample, error)) NetReader {
	return &helperNet{samples: samples, pipe: pipe, start: start, now: time.Now, verify: serverIsThisProcess}
}

type helperNet struct {
	samples func(context.Context) ([]ProcessSample, error)
	pipe    string
	// start starts the service, or says errNoNetHelper where there is none.
	start func() error
	now   func() time.Time
	// verify says whether the process serving the pipe is the helper. A
	// pipe's name is anybody's to take while the helper is stopped, which
	// is most of the time.
	verify func(serverPID uint32) error

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
	var server uint32
	if err := windows.GetNamedPipeServerProcessId(handle, &server); err != nil {
		_ = windows.CloseHandle(handle)
		return fmt.Errorf("asking who serves the network helper's pipe: %w", err)
	}
	if err := h.verify(server); err != nil {
		_ = windows.CloseHandle(handle)
		return err
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
	// Anonymous: the server may not impersonate the panel's user, at any
	// level. Without SECURITY_SQOS_PRESENT a pipe client lets its server
	// impersonate it in full, and whoever holds the name is that server.
	flags := uint32(windows.SECURITY_SQOS_PRESENT | windows.SECURITY_ANONYMOUS)
	for range 3 {
		handle, err := windows.CreateFile(path, windows.GENERIC_READ, 0, nil, windows.OPEN_EXISTING, flags, 0)
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

/*
serverIsTheService checks that the process serving the pipe is the one the
service manager started for hayami-network. Only the service manager can
make the two numbers agree, so a program that took the pipe's name while
the helper was stopped is refused before a byte of it is read. Asking the
service's state is a right the installed account has.
*/
func serverIsTheService(server uint32) error {
	manager, err := windows.OpenSCManager(nil, nil, windows.SC_MANAGER_CONNECT)
	if err != nil {
		return fmt.Errorf("asking the service manager: %w", err)
	}
	defer func() { _ = windows.CloseServiceHandle(manager) }()
	name, _ := windows.UTF16PtrFromString(NetHelperService)
	service, err := windows.OpenService(manager, name, windows.SERVICE_QUERY_STATUS)
	if err != nil {
		return fmt.Errorf("asking after the network helper's service: %w", err)
	}
	defer func() { _ = windows.CloseServiceHandle(service) }()
	var status windows.SERVICE_STATUS_PROCESS
	var needed uint32
	if err := windows.QueryServiceStatusEx(service, windows.SC_STATUS_PROCESS_INFO,
		(*byte)(unsafe.Pointer(&status)), uint32(unsafe.Sizeof(status)), &needed); err != nil {
		return fmt.Errorf("asking after the network helper's service: %w", err)
	}
	return serverMatches(server, status.ProcessId)
}

// serverIsThisProcess is the check for a server a test runs in its own
// process.
func serverIsThisProcess(server uint32) error {
	return serverMatches(server, windows.GetCurrentProcessId())
}

// serverMatches refuses a pipe served by any process but want, which is
// zero when the service is not running.
func serverMatches(server, want uint32) error {
	if server != 0 && server == want {
		return nil
	}
	return &Absence{Code: AbsenceNetProcessesDenied, Text: "helper's pipe is taken",
		Detail: fmt.Sprintf("the pipe %s is held by process %d, which is not hayami's network helper; "+
			"the list is not read from it. Ending that process lets the helper start", NetHelperPipe, server)}
}
