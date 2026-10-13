package core

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// socketFixture is a socketNet over counters, owners and names a test sets
// between readings, with a clock it moves.
type socketFixture struct {
	sockets []SocketCount
	owners  map[uint32]int
	names   map[int]string
	clock   time.Time
	net     *socketNet
}

func newSocketFixture() *socketFixture {
	f := &socketFixture{clock: time.Unix(500, 0)}
	f.net = &socketNet{
		sockets: func(context.Context) ([]SocketCount, error) { return f.sockets, nil },
		owners:  func(context.Context) (map[uint32]int, error) { return f.owners, nil },
		samples: func(context.Context) ([]ProcessSample, error) {
			var out []ProcessSample
			for pid, name := range f.names {
				out = append(out, ProcessSample{PID: pid, Name: name})
			}
			return out, nil
		},
		counts: "TCP only",
		now:    func() time.Time { return f.clock },
	}
	return f
}

func byPID(use NetUse) []NetProcess {
	out := append([]NetProcess(nil), use.Processes...)
	sort.Slice(out, func(i, j int) bool { return out[i].PID < out[j].PID })
	return out
}

// Spec 060 R11. A rate is a socket's counters differenced over the time
// between two readings, summed over each process's sockets. The first
// reading has nothing to difference against and says so.
func TestARateIsSocketCountersOverTheInterval(t *testing.T) {
	f := newSocketFixture()
	f.owners = map[uint32]int{100: 7, 101: 7, 102: 8}
	f.names = map[int]string{7: "browser", 8: "sync"}
	f.sockets = []SocketCount{{Inode: 100, Rx: 1000, Tx: 100}, {Inode: 101, Rx: 0, Tx: 0}, {Inode: 102, Rx: 50, Tx: 50}}

	first, err := f.net.Read(t.Context())
	require.NoError(t, err)
	assert.False(t, first.HasRate)
	assert.Empty(t, first.Processes)
	assert.Equal(t, "TCP only", first.Counts)

	f.clock = f.clock.Add(2 * time.Second)
	f.sockets = []SocketCount{{Inode: 100, Rx: 5000, Tx: 300}, {Inode: 101, Rx: 2000, Tx: 0}, {Inode: 102, Rx: 50, Tx: 50}}
	use, err := f.net.Read(t.Context())
	require.NoError(t, err)

	assert.True(t, use.HasRate)
	assert.Equal(t, []NetProcess{
		{PID: 7, Name: "browser", Rx: 3000, Tx: 100},
		{PID: 8, Name: "sync", Rx: 0, Tx: 0},
	}, byPID(use))
}

// R11. A socket whose process this user may not read is counted under no
// process, which the panel calls "other": the bytes are real.
func TestASocketWithNoReadableProcessIsOther(t *testing.T) {
	f := newSocketFixture()
	f.owners = map[uint32]int{100: 7, 200: 99}
	f.names = map[int]string{7: "mine"}
	f.sockets = []SocketCount{{Inode: 100}, {Inode: 200}, {Inode: 300}}
	_, err := f.net.Read(t.Context())
	require.NoError(t, err)

	f.clock = f.clock.Add(time.Second)
	// 200's process is not in the table, 300 is in nobody's fd directory.
	f.sockets = []SocketCount{{Inode: 100, Rx: 10}, {Inode: 200, Rx: 400, Tx: 4}, {Inode: 300, Rx: 600, Tx: 6}}
	use, err := f.net.Read(t.Context())
	require.NoError(t, err)

	assert.Equal(t, []NetProcess{
		{PID: 0, Rx: 1000, Tx: 10},
		{PID: 7, Name: "mine", Rx: 10},
	}, byPID(use))
}

// "other" with nothing in it is not listed.
func TestOtherWithNoTrafficIsNotListed(t *testing.T) {
	f := newSocketFixture()
	f.sockets = []SocketCount{{Inode: 300, Rx: 5}}
	_, _ = f.net.Read(t.Context())
	f.clock = f.clock.Add(time.Second)

	use, err := f.net.Read(t.Context())

	require.NoError(t, err)
	assert.Empty(t, use.Processes)
}

// A connection opened since the last reading is counted whole; a counter
// that went backwards, an inode given to a new socket with the same cookie,
// is not counted at all.
func TestANewConnectionIsCountedWhole(t *testing.T) {
	f := newSocketFixture()
	f.owners = map[uint32]int{100: 7, 101: 7}
	f.names = map[int]string{7: "browser"}
	f.sockets = []SocketCount{{Inode: 100, Cookie: 1, Rx: 9000}}
	_, _ = f.net.Read(t.Context())

	f.clock = f.clock.Add(time.Second)
	f.sockets = []SocketCount{{Inode: 100, Cookie: 1, Rx: 100}, {Inode: 101, Cookie: 2, Rx: 700, Tx: 70}}
	use, err := f.net.Read(t.Context())
	require.NoError(t, err)

	assert.Equal(t, []NetProcess{{PID: 7, Name: "browser", Rx: 700, Tx: 70}}, byPID(use))

	// The same inode with a new cookie is a new socket.
	f.clock = f.clock.Add(time.Second)
	f.sockets = []SocketCount{{Inode: 100, Cookie: 5, Rx: 40}}
	use, err = f.net.Read(t.Context())
	require.NoError(t, err)
	assert.Equal(t, []NetProcess{{PID: 7, Name: "browser", Rx: 40}}, byPID(use))
}

// Owners that cannot be read leave every socket under "other", which is
// true, and the list is still drawn.
func TestOwnersThatCannotBeReadLeaveEverythingOther(t *testing.T) {
	f := newSocketFixture()
	f.net.owners = func(context.Context) (map[uint32]int, error) { return nil, errors.New("no /proc") }
	f.sockets = []SocketCount{{Inode: 1}}
	_, _ = f.net.Read(t.Context())
	f.clock = f.clock.Add(time.Second)
	f.sockets = []SocketCount{{Inode: 1, Rx: 8, Tx: 2}}

	use, err := f.net.Read(t.Context())

	require.NoError(t, err)
	assert.Equal(t, []NetProcess{{PID: 0, Rx: 8, Tx: 2}}, use.Processes)
}

// A socket's process is found by its inode in an fd directory, the lower
// process ID where two hold it, and a directory that cannot be read is
// passed over.
func TestASocketsProcessIsFoundByItsInode(t *testing.T) {
	root := t.TempDir()
	link := func(pid, fd int, target string) {
		dir := filepath.Join(root, strconv.Itoa(pid), "fd")
		require.NoError(t, os.MkdirAll(dir, 0o755))
		if err := os.Symlink(target, filepath.Join(dir, strconv.Itoa(fd))); err != nil {
			t.Skipf("this user cannot make a symbolic link: %v", err)
		}
	}
	link(30, 3, "socket:[555]")
	link(30, 4, "/dev/null")
	link(20, 9, "socket:[555]")
	link(40, 3, "socket:[777]")
	link(40, 4, "pipe:[12]")
	require.NoError(t, os.MkdirAll(filepath.Join(root, "50"), 0o755)) // no fd directory: another user's

	got, err := ProcSocketOwners(root)(t.Context())

	require.NoError(t, err)
	assert.Equal(t, map[uint32]int{555: 20, 777: 40}, got)
}

// A link is a socket only in the kernel's own form.
func TestASocketLinkIsReadForItsInode(t *testing.T) {
	inode, ok := socketInode("socket:[12345]")
	assert.True(t, ok)
	assert.Equal(t, uint32(12345), inode)
	for _, not := range []string{"/dev/null", "pipe:[1]", "socket:[x]", "socket:[]", "anon_inode:[eventfd]"} {
		_, ok := socketInode(not)
		assert.False(t, ok, not)
	}
}

// A platform with no reader says so as a typed absence.
func TestNoNetworkReaderIsATypedAbsence(t *testing.T) {
	n := noNet{&Absence{Code: AbsenceNetProcesses, Detail: "none here"}}

	_, err := n.Read(t.Context())

	a, ok := AbsenceOf(err)
	require.True(t, ok)
	assert.Equal(t, AbsenceNetProcesses, a.Code)
	n.Close()
}
