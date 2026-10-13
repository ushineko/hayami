package core

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

/*
The network by process on Windows is event tracing or nothing (spec 060 R11).
No counter carries it: `Process` counts I/O of every kind. The kernel's
network provider, Microsoft-Windows-Kernel-Network, raises an event for each
send and receive with the process and the size, and a real-time trace session
that listens to it is what Resource Monitor is.

**Windows keeps a trace session for administrators** and members of
Performance Log Users. hayami runs as the person at the desk, asks for no
elevation and installs nothing, so on most desks the session is refused and
the list is a typed absence that says what is needed. Where it is allowed the
list counts TCP and UDP both, which Linux's cannot.
*/

// newNetReader is Windows' network by process.
func newNetReader(samples func(context.Context) ([]ProcessSample, error)) NetReader {
	return &traceNet{samples: samples, now: time.Now}
}

var (
	advapi32          = windows.NewLazySystemDLL("advapi32.dll")
	procStartTrace    = advapi32.NewProc("StartTraceW")
	procControlTrace  = advapi32.NewProc("ControlTraceW")
	procEnableTraceEx = advapi32.NewProc("EnableTraceEx2")
	procOpenTrace     = advapi32.NewProc("OpenTraceW")
	procProcessTrace  = advapi32.NewProc("ProcessTrace")
	procCloseTrace    = advapi32.NewProc("CloseTrace")
)

// kernelNetwork is Microsoft-Windows-Kernel-Network.
var kernelNetwork = windows.GUID{
	Data1: 0x7DD42A49, Data2: 0x5329, Data3: 0x4832,
	Data4: [8]byte{0x8D, 0xFD, 0x43, 0xD9, 0x79, 0x15, 0x3A, 0x88},
}

// The provider's events, by ID: data sent and received over TCP and UDP, on
// IPv4 and IPv6. Each starts with the process ID and the size, four bytes
// each, and then the remote and local addresses.
var netEvents = map[uint16]struct{ sent, v6 bool }{
	10: {sent: true}, 11: {},
	26: {sent: true, v6: true}, 27: {v6: true},
	42: {sent: true}, 43: {},
	58: {sent: true, v6: true}, 59: {v6: true},
}

// The trace constants this uses (evntrace.h, evntcons.h).
const (
	traceSessionName = "hayami-network"

	wnodeFlagTracedGUID     = 0x00020000
	eventTraceRealTimeMode  = 0x00000100
	eventTraceControlStop   = 1
	eventControlEnable      = 1
	traceLevelInformation   = 4
	processTraceRealTime    = 0x00000100
	processTraceEventRecord = 0x10000000
	invalidTraceHandle      = ^uintptr(0)

	// The provider's keywords for the IPv4 and IPv6 events.
	keywordIPv4 = 0x10
	keywordIPv6 = 0x20
)

// traceProperties is EVENT_TRACE_PROPERTIES with room after it for the
// session name, which the structure addresses by offset.
type traceProperties struct {
	// WNODE_HEADER
	BufferSize        uint32
	ProviderID        uint32
	HistoricalContext uint64
	TimeStamp         int64
	GUID              windows.GUID
	ClientContext     uint32
	Flags             uint32

	TraceBufferSize     uint32
	MinimumBuffers      uint32
	MaximumBuffers      uint32
	MaximumFileSize     uint32
	LogFileMode         uint32
	FlushTimer          uint32
	EnableFlags         uint32
	AgeLimit            int32
	NumberOfBuffers     uint32
	FreeBuffers         uint32
	EventsLost          uint32
	BuffersWritten      uint32
	LogBuffersLost      uint32
	RealTimeBuffersLost uint32
	LoggerThreadID      uintptr
	LogFileNameOffset   uint32
	LoggerNameOffset    uint32

	Name [64]uint16
}

// eventRecord is the part of EVENT_RECORD this reads: the event's ID, out of
// its descriptor, and the data the provider attached.
type eventRecord struct {
	_        [40]byte
	ID       uint16
	_        [44]byte
	DataLen  uint16
	_        [8]byte
	UserData unsafe.Pointer
}

// traceLogfile is EVENT_TRACE_LOGFILEW, with the two structures inside it
// that nothing here reads left as their bytes.
type traceLogfile struct {
	LogFileName      *uint16
	LoggerName       *uint16
	CurrentTime      int64
	BuffersRead      uint32
	ProcessTraceMode uint32
	_                [88 + 280]byte // EVENT_TRACE, TRACE_LOGFILE_HEADER
	BufferCallback   uintptr
	BufferSize       uint32
	Filled           uint32
	EventsLost       uint32
	_                uint32
	EventCallback    uintptr
	IsKernelTrace    uint32
	_                uint32
	Context          uintptr
}

// traceIdle is how long a session is kept with nobody reading it. A drawer
// that closed stops asking, and the session goes with it.
const traceIdle = 20 * time.Second

/*
traceNet holds the trace session while the list is being read, and counts the
bytes its events report.

The session is started by the first Read and stopped by Close, or by itself
when nothing has read it for traceIdle: a session is a kernel object that
outlives the process that made it, and one left behind by a panel that was
killed would be found by the next and stopped first, by name.
*/
type traceNet struct {
	samples func(context.Context) ([]ProcessSample, error)
	now     func() time.Time

	mu      sync.Mutex
	session uintptr
	trace   uintptr
	counts  map[uint32][2]uint64
	lastAt  time.Time
	readAt  time.Time
	denied  *Absence
}

// current is the reader the callback counts into. Windows calls one function
// for every session a process has, and a process has one of these.
var (
	currentMu sync.Mutex
	current   *traceNet

	traceCallback = sync.OnceValue(func() uintptr {
		return syscall.NewCallback(func(record *eventRecord) uintptr {
			currentMu.Lock()
			t := current
			currentMu.Unlock()
			if t != nil && record != nil {
				t.event(record)
			}
			return 0
		})
	})
)

// Read is each process's rate since the previous call. The first call starts
// the session and has no rate.
func (t *traceNet) Read(ctx context.Context) (NetUse, error) {
	t.mu.Lock()
	if t.denied != nil {
		defer t.mu.Unlock()
		return NetUse{}, t.denied
	}
	use := NetUse{Counts: "TCP and UDP"}
	now := t.now()
	t.readAt = now
	if t.session == 0 {
		err := t.start()
		t.lastAt = now
		t.mu.Unlock()
		if err != nil {
			return NetUse{}, err
		}
		return use, nil
	}
	counts, elapsed := t.counts, now.Sub(t.lastAt).Seconds()
	t.counts, t.lastAt = map[uint32][2]uint64{}, now
	t.mu.Unlock()

	if elapsed <= 0 {
		return use, nil
	}
	use.HasRate = true
	names := map[int]string{}
	if samples, err := t.samples(ctx); err == nil {
		for _, p := range samples {
			names[p.PID] = p.Name
		}
	}
	for pid, c := range counts {
		p := NetProcess{PID: int(pid), Name: names[int(pid)],
			Rx: float64(c[0]) / elapsed, Tx: float64(c[1]) / elapsed}
		if p.Name == "" {
			p.PID = 0 // gone since it sent: still traffic, under "other"
		}
		use.Processes = append(use.Processes, p)
	}
	return use, nil
}

// start makes the session, turns the provider on and begins reading. The
// caller holds the lock.
func (t *traceNet) start() error {
	name, _ := windows.UTF16FromString(traceSessionName)
	props := func() *traceProperties {
		p := &traceProperties{}
		p.BufferSize = uint32(unsafe.Sizeof(*p))
		p.Flags = wnodeFlagTracedGUID
		p.ClientContext = 1 // the performance counter's clock
		p.LogFileMode = eventTraceRealTimeMode
		// Events are handed over when a buffer fills or this many seconds
		// pass, so a quiet machine's list is at most a second behind.
		p.FlushTimer = 1
		p.LoggerNameOffset = uint32(unsafe.Offsetof(p.Name))
		copy(p.Name[:], name)
		return p
	}

	// A session of this name left by a panel that did not get to stop it.
	stale := props()
	_, _, _ = procControlTrace.Call(0, uintptr(unsafe.Pointer(&name[0])),
		uintptr(unsafe.Pointer(stale)), eventTraceControlStop)

	p := props()
	var session uintptr
	r, _, _ := procStartTrace.Call(uintptr(unsafe.Pointer(&session)),
		uintptr(unsafe.Pointer(&name[0])), uintptr(unsafe.Pointer(p)))
	if r != 0 {
		err := syscall.Errno(r)
		if errors.Is(err, windows.ERROR_ACCESS_DENIED) {
			t.denied = &Absence{Code: AbsenceNetProcessesDenied, Text: "needs an administrator",
				Detail: "Windows gives the network by process only through event tracing, which it allows " +
					"an administrator or a member of Performance Log Users; hayami does not ask for elevation",
				Err: err}
			return t.denied
		}
		return fmt.Errorf("starting the network trace: %w", err)
	}

	r, _, _ = procEnableTraceEx.Call(session, uintptr(unsafe.Pointer(&kernelNetwork)),
		eventControlEnable, traceLevelInformation, keywordIPv4|keywordIPv6, 0, 0, 0)
	if r != 0 {
		t.stopSession(session)
		return fmt.Errorf("enabling the network provider: %w", syscall.Errno(r))
	}

	logfile := &traceLogfile{
		LoggerName:       &name[0],
		ProcessTraceMode: processTraceRealTime | processTraceEventRecord,
		EventCallback:    traceCallback(),
	}
	trace, _, callErr := procOpenTrace.Call(uintptr(unsafe.Pointer(logfile)))
	if trace == invalidTraceHandle {
		t.stopSession(session)
		return fmt.Errorf("opening the network trace: %w", callErr)
	}

	t.session, t.trace = session, trace
	t.counts = map[uint32][2]uint64{}
	currentMu.Lock()
	current = t
	currentMu.Unlock()

	// ProcessTrace blocks until the trace is closed, delivering events to
	// the callback on this goroutine's thread.
	go func() {
		handle := trace
		_, _, _ = procProcessTrace.Call(uintptr(unsafe.Pointer(&handle)), 1, 0, 0)
	}()
	go t.expire()
	return nil
}

// event counts one record: its size, for its process and direction.
func (t *traceNet) event(record *eventRecord) {
	kind, ok := netEvents[record.ID]
	if !ok || record.UserData == nil {
		return
	}
	need := 12
	if kind.v6 {
		need = 24
	}
	if int(record.DataLen) < need {
		return
	}
	data := unsafe.Slice((*byte)(record.UserData), need)
	pid := binary.LittleEndian.Uint32(data[0:])
	size := binary.LittleEndian.Uint32(data[4:])
	if localAddress(kind.v6, data[8:]) {
		return // this machine talking to itself is not traffic on an interface
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.counts == nil {
		return // an event delivered after the session was closed
	}
	c := t.counts[pid]
	if kind.sent {
		c[1] += uint64(size)
	} else {
		c[0] += uint64(size)
	}
	t.counts[pid] = c
}

// localAddress reports whether an event's remote address is this machine's
// own: 127.0.0.0/8, ::1, or the first written as an IPv6 address.
func localAddress(v6 bool, addr []byte) bool {
	if !v6 {
		return addr[0] == 127
	}
	for _, b := range addr[:10] {
		if b != 0 {
			return false
		}
	}
	if addr[10] == 0xff && addr[11] == 0xff {
		return addr[12] == 127
	}
	return addr[10] == 0 && addr[11] == 0 && addr[12] == 0 && addr[13] == 0 && addr[14] == 0 && addr[15] == 1
}

// expire stops a session nobody has read for traceIdle.
func (t *traceNet) expire() {
	tick := time.NewTicker(traceIdle / 4)
	defer tick.Stop()
	for range tick.C {
		t.mu.Lock()
		gone := t.session == 0
		idle := t.now().Sub(t.readAt) > traceIdle
		t.mu.Unlock()
		if gone {
			return
		}
		if idle {
			t.Close()
			return
		}
	}
}

// Close stops the session. A later Read starts another.
func (t *traceNet) Close() {
	t.mu.Lock()
	session, trace := t.session, t.trace
	t.session, t.trace, t.counts = 0, 0, nil
	t.mu.Unlock()
	if session == 0 {
		return
	}
	currentMu.Lock()
	if current == t {
		current = nil
	}
	currentMu.Unlock()
	_, _, _ = procCloseTrace.Call(trace)
	t.stopSession(session)
}

// stopSession ends a session by its handle.
func (t *traceNet) stopSession(session uintptr) {
	p := &traceProperties{}
	p.BufferSize = uint32(unsafe.Sizeof(*p))
	p.LoggerNameOffset = uint32(unsafe.Offsetof(p.Name))
	_, _, _ = procControlTrace.Call(session, 0, uintptr(unsafe.Pointer(p)), eventTraceControlStop)
}
