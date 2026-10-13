package core

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

/*
The network by process (spec 060 R11), as far as each system gives it away.
Linux counts bytes per TCP socket and says which process holds a socket only
to that process's owner; Windows says it per process only through event
tracing, which it keeps for administrators. Neither is the whole of an
interface's traffic, and each reader says what it counts (NetUse.Counts).

What is here has no build tag: the arithmetic, and the walk of /proc that
finds a socket's process, both of which a test on any system can run.
*/

// SocketCount is one TCP socket's byte counters since it opened.
type SocketCount struct {
	// Inode names the socket in /proc/<pid>/fd, and Cookie tells a socket
	// from the next one to be given the same inode.
	Inode  uint32
	Cookie uint64

	// Rx is bytes received and Tx bytes sent and acknowledged.
	Rx, Tx uint64
}

type socketKey struct {
	inode  uint32
	cookie uint64
}

/*
socketNet turns socket counters into rates by process.

A socket is found in a process's fd directory by its inode. One this user may
not read the owner of is counted under no process, PID zero, which the panel
calls "other": the bytes are real and leaving them out would make the list
look more complete than it is.

A connection opened since the previous reading is counted whole, since all of
it happened in the interval. One opened and closed between two readings is
never seen, so a program making many short requests reads low.
*/
type socketNet struct {
	sockets func(context.Context) ([]SocketCount, error)
	owners  func(context.Context) (map[uint32]int, error)
	samples func(context.Context) ([]ProcessSample, error)
	counts  string

	mu     sync.Mutex
	last   map[socketKey]SocketCount
	lastAt time.Time
	now    func() time.Time
}

// Read is the rate of each process's sockets since the previous call.
func (s *socketNet) Read(ctx context.Context) (NetUse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	sockets, err := s.sockets(ctx)
	if err != nil {
		return NetUse{}, err
	}
	at := s.now()
	use := NetUse{Counts: s.counts}

	now := make(map[socketKey]SocketCount, len(sockets))
	for _, c := range sockets {
		now[socketKey{c.Inode, c.Cookie}] = c
	}
	was, wasAt := s.last, s.lastAt
	s.last, s.lastAt = now, at
	elapsed := at.Sub(wasAt).Seconds()
	if was == nil || elapsed <= 0 {
		return use, nil
	}
	use.HasRate = true

	owners, err := s.owners(ctx)
	if err != nil {
		owners = nil // every socket is then "other", which is true
	}
	names := map[int]string{}
	if samples, err := s.samples(ctx); err == nil {
		for _, p := range samples {
			names[p.PID] = p.Name
		}
	}

	by := map[int]*NetProcess{}
	for key, c := range now {
		before := was[key] // a new socket differences against nought
		if c.Rx < before.Rx || c.Tx < before.Tx {
			continue
		}
		rx, tx := float64(c.Rx-before.Rx)/elapsed, float64(c.Tx-before.Tx)/elapsed
		pid := owners[key.inode]
		if _, named := names[pid]; !named {
			pid = 0
		}
		p, ok := by[pid]
		if !ok {
			p = &NetProcess{PID: pid, Name: names[pid]}
			by[pid] = p
		}
		p.Rx += rx
		p.Tx += tx
	}
	for _, p := range by {
		// "other" with nothing in it is not a line worth drawing.
		if p.PID == 0 && p.Rx == 0 && p.Tx == 0 {
			continue
		}
		use.Processes = append(use.Processes, *p)
	}
	return use, nil
}

// Close has nothing to let go of.
func (s *socketNet) Close() {}

/*
ProcSocketOwners finds which process holds each socket, by walking every fd
directory under root for links of the form "socket:[12345]".

Another user's fd directory cannot be read, and its sockets stay unowned. A
socket two processes hold, a server's after a fork, is given to the first
found, which is the lower process ID.
*/
func ProcSocketOwners(root string) func(context.Context) (map[uint32]int, error) {
	return func(ctx context.Context) (map[uint32]int, error) {
		entries, err := os.ReadDir(root)
		if err != nil {
			return nil, err //nolint:wrapcheck // the path is in the error
		}
		out := map[uint32]int{}
		for _, e := range entries {
			pid, err := strconv.Atoi(e.Name())
			if err != nil {
				continue
			}
			if err := ctx.Err(); err != nil {
				return nil, err //nolint:wrapcheck // the context's own message is the whole story
			}
			dir := filepath.Join(root, e.Name(), "fd")
			fds, err := os.ReadDir(dir)
			if err != nil {
				continue
			}
			for _, fd := range fds {
				target, err := os.Readlink(filepath.Join(dir, fd.Name()))
				if err != nil {
					continue
				}
				inode, ok := socketInode(target)
				if !ok {
					continue
				}
				if _, held := out[inode]; !held {
					out[inode] = pid
				}
			}
		}
		return out, nil
	}
}

// socketInode reads the inode out of a socket's fd link, "socket:[12345]".
func socketInode(target string) (uint32, bool) {
	rest, ok := strings.CutPrefix(target, "socket:[")
	if !ok {
		return 0, false
	}
	n, err := strconv.ParseUint(strings.TrimSuffix(rest, "]"), 10, 32)
	if err != nil {
		return 0, false
	}
	return uint32(n), true
}

/*
netUse makes a reading of the bytes each process ID received and sent over
elapsed seconds. A process that is gone since it sent has no name, and its
bytes are counted under no process, which the panel calls "other". With no
time passed there is no rate, which is the first reading's answer.
*/
func netUse(ctx context.Context, samples func(context.Context) ([]ProcessSample, error),
	what string, counts map[uint32][2]uint64, elapsed float64) NetUse {
	use := NetUse{Counts: what}
	if elapsed <= 0 {
		return use
	}
	use.HasRate = true
	names := map[int]string{}
	if samples != nil {
		if got, err := samples(ctx); err == nil {
			for _, p := range got {
				names[p.PID] = p.Name
			}
		}
	}
	for pid, c := range counts {
		p := NetProcess{PID: int(pid), Name: names[int(pid)],
			Rx: float64(c[0]) / elapsed, Tx: float64(c[1]) / elapsed}
		if p.Name == "" {
			p.PID = 0
		}
		use.Processes = append(use.Processes, p)
	}
	return use
}

// noNet is a platform this build reads no network by process on.
type noNet struct{ absence *Absence }

func (n noNet) Read(context.Context) (NetUse, error) { return NetUse{}, n.absence }
func (noNet) Close()                                 {}
