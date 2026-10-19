package core

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"sort"
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

	// Local and Peer are the two ends, and LocalPort the near one's port.
	// They are kept to group a process's sockets by interface and to
	// name who is at the far end; the shipped reader threw them away.
	Local, Peer         string
	LocalPort, PeerPort uint16
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

	// ifaces maps a local address to the interface that holds it, and
	// listening is the set of ports this machine answers on.
	ifaces    func() map[string]string
	listening map[uint16]bool

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

	// A reader built without an interface table -- a test's, and any
	// system that does not keep one -- puts every socket under no
	// interface rather than refusing to count it.
	ifaces := map[string]string{}
	if s.ifaces != nil {
		ifaces = s.ifaces()
	}
	type procKey struct {
		pid   int
		iface string
	}
	peers := map[procKey]map[string]*NetPeer{}
	by := map[procKey]*NetProcess{}
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
		k := procKey{pid, ifaces[c.Local]}
		p, ok := by[k]
		if !ok {
			p = &NetProcess{PID: pid, Name: names[pid], Iface: k.iface}
			by[k] = p
			peers[k] = map[string]*NetPeer{}
		}
		p.Rx += rx
		p.Tx += tx

		// A socket whose near end is a listening port is one this process
		// serves, so the far end is a client of ours and not a destination
		// we chose. The two read differently and the list says which.
		served := s.listening[c.LocalPort]
		far, ok := peers[k][c.Peer]
		if !ok {
			far = &NetPeer{Addr: c.Peer, Served: served}
			_ = far
			peers[k][c.Peer] = far
		}
		far.Rx += rx
		far.Tx += tx
		far.Sockets = append(far.Sockets, NetSocket{
			LocalPort: c.LocalPort, PeerPort: c.PeerPort, Rx: rx, Tx: tx,
		})
	}
	for k, p := range by {
		// "other" with nothing in it is not a line worth drawing.
		if p.PID == 0 && p.Rx == 0 && p.Tx == 0 {
			continue
		}
		for _, far := range peers[k] {
			p.Peers = append(p.Peers, *far)
		}
		sort.Slice(p.Peers, func(i, j int) bool {
			return p.Peers[i].Rx+p.Peers[i].Tx > p.Peers[j].Rx+p.Peers[j].Tx
		})
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

// localIfaces maps every local address to the interface that holds it, which
// is how a socket is put to an interface: the kernel's socket ID carries an
// interface index only for a bound socket, and most are not bound.
func localIfaces() map[string]string {
	out := map[string]string{}
	ifs, err := net.Interfaces()
	if err != nil {
		return out
	}
	for _, i := range ifs {
		addrs, err := i.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			if n, ok := a.(*net.IPNet); ok {
				out[n.IP.String()] = i.Name
			}
		}
	}
	return out
}

// listeningPorts is every TCP port this machine answers on, read once: a
// socket whose near end is one of them is a connection we accepted.
func listeningPorts() map[uint16]bool {
	out := map[uint16]bool{}
	for _, f := range []string{"/proc/net/tcp", "/proc/net/tcp6"} {
		b, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(b), "\n")[1:] {
			fields := strings.Fields(line)
			if len(fields) < 4 || fields[3] != "0A" {
				continue
			}
			_, port, ok := strings.Cut(fields[1], ":")
			if !ok {
				continue
			}
			if n, err := strconv.ParseUint(port, 16, 16); err == nil {
				out[uint16(n)] = true
			}
		}
	}
	return out
}
