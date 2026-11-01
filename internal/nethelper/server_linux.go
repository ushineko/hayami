package nethelper

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"time"

	"golang.org/x/sys/unix"

	"github.com/ushineko/hayami/internal/core"
	"github.com/ushineko/hayami/internal/netcount"
)

const (
	// tick is how often the counts are sent. The panel sets the bytes
	// against the interfaces' counters read at its own moments, and a
	// reading is out by up to one tick of traffic: a second put whole bursts
	// on the wrong side of a reading, a quarter of one keeps the remainder
	// line honest.
	tick = 250 * time.Millisecond
	// idle is how long the helper stays with nobody reading it. A drawer
	// closed and opened again finds it still there; one closed for good costs
	// this much of a process.
	idle = 15 * time.Second
	// writeWait is how long a line may take to reach a reader. One whose
	// buffer is still full after it has stopped reading, and is dropped.
	writeWait = 100 * time.Millisecond
)

// counter is where the counts come from: netcount's programs, or a test's.
type counter interface {
	Read() (netcount.Snapshot, error)
	Close()
}

// server is one run of the helper: a socket, the readers on it, and where
// the counts come from.
type server struct {
	listener *net.UnixListener
	// allow is the account that may read; root may too.
	allow uint32
	hello core.NetHelperLine
	// open loads the programs; the returned counter's Close detaches them.
	open func() (counter, error)
	tick time.Duration
	idle time.Duration
}

/*
serve runs until ctx ends or nobody has read the socket for the idle time.

Each reader is sent the hello and then a line a tick of what moved since the
one before. A reader that stops reading is dropped when its buffer fills and
holds nobody else up. The programs are loaded only while somebody reads, and
detached when the last reader leaves, so a helper with no reader is not in
the path of a single packet.
*/
func (s *server) serve(ctx context.Context) error {
	joined := make(chan *net.UnixConn)
	accepting, stopAccepting := context.WithCancel(ctx)
	defer stopAccepting()
	go s.accept(accepting, joined)

	var (
		readers []*net.UnixConn
		count   counter
		was     netcount.Snapshot
	)
	stop := func() {
		if count != nil {
			count.Close()
			count = nil
		}
	}
	defer func() {
		for _, r := range readers {
			_ = r.Close()
		}
		stop()
		_ = s.listener.Close()
	}()
	hello, _ := json.Marshal(s.hello)
	ticker := time.NewTicker(s.tick)
	defer ticker.Stop()
	quiet := time.Now()

	for {
		select {
		case <-ctx.Done():
			return nil
		case r := <-joined:
			if !write(r, hello) {
				_ = r.Close()
				continue
			}
			if count == nil {
				// Start counting now, so the first line is this reader's.
				c, err := s.open()
				if err == nil {
					count = c
					was, err = c.Read()
				}
				if err != nil {
					stop()
					fail(append(readers, r), err)
					return err
				}
			}
			readers = append(readers, r)
		case now := <-ticker.C:
			if len(readers) == 0 {
				if now.Sub(quiet) > s.idle {
					return nil
				}
				continue
			}
			snap, err := count.Read()
			if err != nil {
				fail(readers, err)
				return err
			}
			line, _ := json.Marshal(netcount.Line(was, snap))
			was = snap
			kept := readers[:0]
			for _, r := range readers {
				if write(r, line) {
					kept = append(kept, r)
				} else {
					_ = r.Close()
				}
			}
			readers = kept
			if len(readers) == 0 {
				stop()
				quiet = now
			}
		}
	}
}

/*
accept hands over each reader the kernel vouches for. The socket's mode
already keeps other accounts out; the peer's credentials are checked as
well, so a socket made more open by mistake is not a way in.
*/
func (s *server) accept(ctx context.Context, joined chan<- *net.UnixConn) {
	for {
		conn, err := s.listener.AcceptUnix()
		if err != nil {
			if ctx.Err() != nil || errors.Is(err, net.ErrClosed) {
				return
			}
			continue
		}
		if uid, err := peerUID(conn); err != nil || (uid != s.allow && uid != 0) {
			line, _ := json.Marshal(core.NetHelperLine{Error: "this helper serves another account"})
			write(conn, line)
			_ = conn.Close()
			continue
		}
		select {
		case joined <- conn:
		case <-ctx.Done():
			_ = conn.Close()
			return
		}
	}
}

// peerUID is the account on the far end of a Unix socket, from the kernel.
func peerUID(conn *net.UnixConn) (uint32, error) {
	raw, err := conn.SyscallConn()
	if err != nil {
		return 0, fmt.Errorf("asking who connected: %w", err)
	}
	var cred *unix.Ucred
	var credErr error
	if err := raw.Control(func(fd uintptr) {
		cred, credErr = unix.GetsockoptUcred(int(fd), unix.SOL_SOCKET, unix.SO_PEERCRED)
	}); err != nil {
		return 0, fmt.Errorf("asking who connected: %w", err)
	}
	if credErr != nil {
		return 0, fmt.Errorf("asking who connected: %w", credErr)
	}
	return cred.Uid, nil
}

// fail tells every reader why the helper is stopping.
func fail(readers []*net.UnixConn, err error) {
	line, _ := json.Marshal(core.NetHelperLine{Error: err.Error()})
	for _, r := range readers {
		write(r, line)
	}
}

// write sends one line, or reports the reader gone.
func write(conn *net.UnixConn, line []byte) bool {
	b := make([]byte, 0, len(line)+1)
	b = append(append(b, line...), '\n')
	_ = conn.SetWriteDeadline(time.Now().Add(writeWait))
	n, err := conn.Write(b)
	return err == nil && n == len(b)
}
