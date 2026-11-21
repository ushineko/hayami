package nethelper

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/ushineko/hayami/internal/core"
)

const (
	// tick is how often the counts are sent. The panel's list is read every
	// two seconds, so a second keeps a reading at most one tick stale.
	tick = time.Second
	// idle is how long the helper stays with nobody reading it. A drawer
	// closed and opened again finds it still there; one closed for good costs
	// this much of a process.
	idle = 15 * time.Second

	pipeInstances = 8
	pipeBuffer    = 64 << 10
)

// Server is one run of a helper: a pipe, the readers on it, and where the
// lines come from. The network helper and the sensor helper are each one.
type Server struct {
	Pipe string
	SDDL string
	// Hello is the first line each reader is sent.
	Hello any
	// Take is the next line: the bytes by process since it was last called,
	// or a reading. Stop lets go of whatever Take holds, and Take starts
	// again after it.
	Take func() (any, error)
	Stop func()
	Tick time.Duration
	Idle time.Duration
	// Errorf is the line that tells readers why the helper stopped.
	Errorf func(err error) any
}

/*
Serve runs until ctx ends or nobody has read the pipe for the idle time.

Each reader is sent the hello and then the counts, a line a tick. A reader's
pipe is written without waiting, so one that has stopped reading is dropped
when its buffer is full and holds nobody else up. The counts are taken only
while somebody reads them, and let go of when the last reader leaves.
*/
func (s *Server) Serve(ctx context.Context) error {
	security, err := windows.SecurityDescriptorFromString(s.SDDL)
	if err != nil {
		return fmt.Errorf("the pipe's access list %q: %w", s.SDDL, err)
	}
	first, err := s.instance(security, true)
	if err != nil {
		return err
	}

	joined := make(chan windows.Handle)
	accepting, stopAccepting := context.WithCancel(ctx)
	defer func() {
		stopAccepting()
		// The accept loop is blocked waiting for a reader; be one, so it
		// sees it is done.
		if h, err := openForWake(s.Pipe); err == nil {
			_ = windows.CloseHandle(h)
		}
	}()
	go s.accept(accepting, first, security, joined)

	var readers []windows.Handle
	defer func() {
		for _, r := range readers {
			closePipe(r)
		}
		s.Stop()
	}()
	hello, _ := json.Marshal(s.Hello)
	ticker := time.NewTicker(s.Tick)
	defer ticker.Stop()
	quiet := time.Now()

	for {
		select {
		case <-ctx.Done():
			return nil
		case r := <-joined:
			if !write(r, hello) {
				closePipe(r)
				continue
			}
			if len(readers) == 0 {
				// Start counting now, so the first line is this reader's.
				if _, err := s.Take(); err != nil {
					s.fail(append(readers, r), err)
					return err
				}
			}
			readers = append(readers, r)
		case now := <-ticker.C:
			if len(readers) == 0 {
				if now.Sub(quiet) > s.Idle {
					return nil
				}
				continue
			}
			next, err := s.Take()
			if err != nil {
				s.fail(readers, err)
				return err
			}
			line, _ := json.Marshal(next)
			kept := readers[:0]
			for _, r := range readers {
				if write(r, line) {
					kept = append(kept, r)
				} else {
					closePipe(r)
				}
			}
			readers = kept
			if len(readers) == 0 {
				s.Stop()
				quiet = now
			}
		}
	}
}

// fail tells every reader why the helper is stopping.
func (s *Server) fail(readers []windows.Handle, err error) {
	var v any = core.NetHelperLine{Error: err.Error()}
	if s.Errorf != nil {
		v = s.Errorf(err)
	}
	line, _ := json.Marshal(v)
	for _, r := range readers {
		write(r, line)
	}
}

// instance makes one end of the pipe for a reader to open. It is written
// and never read, and refuses a reader on another machine.
func (s *Server) instance(security *windows.SECURITY_DESCRIPTOR, first bool) (windows.Handle, error) {
	name, err := windows.UTF16PtrFromString(s.Pipe)
	if err != nil {
		return 0, err //nolint:wrapcheck // a name with a NUL in it
	}
	flags := uint32(windows.PIPE_ACCESS_OUTBOUND)
	if first {
		// Another program holding this name would be read in our place.
		flags |= windows.FILE_FLAG_FIRST_PIPE_INSTANCE
	}
	sa := &windows.SecurityAttributes{
		Length:             uint32(unsafe.Sizeof(windows.SecurityAttributes{})), //nolint:gosec // the structure's own size
		SecurityDescriptor: security,
	}
	h, err := windows.CreateNamedPipe(name, flags,
		windows.PIPE_TYPE_BYTE|windows.PIPE_WAIT|windows.PIPE_REJECT_REMOTE_CLIENTS,
		pipeInstances, pipeBuffer, 0, 0, sa)
	if err != nil {
		return 0, fmt.Errorf("making the pipe %s: %w", s.Pipe, err)
	}
	return h, nil
}

// accept waits for a reader on each instance in turn and hands it over.
func (s *Server) accept(ctx context.Context, h windows.Handle, security *windows.SECURITY_DESCRIPTOR, joined chan<- windows.Handle) {
	for {
		err := windows.ConnectNamedPipe(h, nil)
		if err != nil && !errors.Is(err, windows.ERROR_PIPE_CONNECTED) {
			closePipe(h)
		} else {
			// From here a write returns at once, full buffer or not.
			mode := uint32(windows.PIPE_NOWAIT)
			_ = windows.SetNamedPipeHandleState(h, &mode, nil, nil)
			select {
			case joined <- h:
			case <-ctx.Done():
				closePipe(h)
				return
			}
		}
		if ctx.Err() != nil {
			return
		}
		next, err := s.instance(security, false)
		if err != nil {
			// Every instance is in use: wait for one to be given back.
			select {
			case <-ctx.Done():
				return
			case <-time.After(s.Tick):
			}
			if next, err = s.instance(security, false); err != nil {
				return
			}
		}
		h = next
	}
}

// write sends one line, whole or not at all as far as the reader can tell:
// a reader whose buffer cannot take it is reported as gone.
func write(h windows.Handle, line []byte) bool {
	b := make([]byte, 0, len(line)+1)
	b = append(append(b, line...), '\n')
	var n uint32
	err := windows.WriteFile(h, b, &n, nil)
	return err == nil && int(n) == len(b)
}

func closePipe(h windows.Handle) {
	_ = windows.DisconnectNamedPipe(h)
	_ = windows.CloseHandle(h)
}

// openForWake opens the pipe as a reader would.
func openForWake(pipe string) (windows.Handle, error) {
	name, err := windows.UTF16PtrFromString(pipe)
	if err != nil {
		return 0, err //nolint:wrapcheck // a name with a NUL in it
	}
	return windows.CreateFile(name, windows.GENERIC_READ, 0, nil, windows.OPEN_EXISTING, 0, 0) //nolint:wrapcheck // only whether it opened matters
}
