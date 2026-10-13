package nethelper

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/hayami/internal/core"
)

// fakeTrace counts what the server asks of the trace.
type fakeTrace struct {
	mu      sync.Mutex
	takes   int
	stops   int
	started bool
	err     error
}

func (f *fakeTrace) take() (map[uint32][2]uint64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.takes++
	if f.err != nil {
		return nil, f.err
	}
	if !f.started {
		f.started = true
		return nil, nil
	}
	return map[uint32][2]uint64{uint32(os.Getpid()): {1000, 500}}, nil //nolint:gosec // a process ID
}

func (f *fakeTrace) stop() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.stops++
	f.started = false
}

func (f *fakeTrace) seen() (int, int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.takes, f.stops
}

var pipes atomic.Int32

// testServer is a server on a pipe of its own, open to this account, on a
// short clock. No service and no administrator: the pipe is a real one.
func testServer(t *testing.T, trace *fakeTrace) *server {
	t.Helper()
	me, err := CurrentUser()
	require.NoError(t, err)
	return &server{
		pipe:  fmt.Sprintf(`\\.\pipe\hayami-test-%d-%d`, os.Getpid(), pipes.Add(1)),
		sddl:  "D:P(A;;GA;;;" + me + ")",
		hello: core.NetHelperLine{V: core.NetHelperProtocol, Version: "1.2.3", Counts: "TCP and UDP"},
		take:  trace.take,
		stop:  trace.stop,
		tick:  40 * time.Millisecond,
		idle:  200 * time.Millisecond,
	}
}

func readLine(t *testing.T, r *bufio.Scanner) core.NetHelperLine {
	t.Helper()
	require.True(t, r.Scan(), "the pipe closed: %v", r.Err())
	var line core.NetHelperLine
	require.NoError(t, json.Unmarshal(r.Bytes(), &line))
	return line
}

func openWhenThere(t *testing.T, pipe string) *os.File {
	t.Helper()
	var f *os.File
	require.Eventually(t, func() bool {
		var err error
		f, err = os.Open(pipe) //nolint:gosec // the test's own pipe
		return err == nil
	}, 2*time.Second, 10*time.Millisecond)
	return f
}

// A reader is told who is speaking and then sent the counts, a line a tick.
// Nothing is traced until somebody reads, the trace is let go when the last
// reader leaves, and with nobody reading the helper ends by itself.
func TestTheServerCountsOnlyWhileSomebodyReads(t *testing.T) {
	trace := &fakeTrace{}
	s := testServer(t, trace)
	done := make(chan error, 1)
	go func() { done <- s.serve(t.Context()) }()

	time.Sleep(3 * s.tick)
	takes, _ := trace.seen()
	assert.Zero(t, takes, "nothing is traced with nobody reading")

	f := openWhenThere(t, s.pipe)
	lines := bufio.NewScanner(f)
	assert.Equal(t, core.NetHelperLine{V: core.NetHelperProtocol, Version: "1.2.3", Counts: "TCP and UDP"}, readLine(t, lines))

	line := readLine(t, lines)
	require.Len(t, line.P, 1)
	assert.Equal(t, [3]uint64{uint64(os.Getpid()), 1000, 500}, line.P[0]) //nolint:gosec // a process ID

	// A second reader, on an instance of its own, gets the same.
	g := openWhenThere(t, s.pipe)
	second := bufio.NewScanner(g)
	assert.Equal(t, core.NetHelperProtocol, readLine(t, second).V)
	assert.Len(t, readLine(t, second).P, 1)

	require.NoError(t, f.Close())
	require.NoError(t, g.Close())
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(3 * time.Second):
		t.Fatal("the server did not end with nobody reading it")
	}
	_, stops := trace.seen()
	assert.Positive(t, stops, "the trace is let go")
	_, err := os.Open(s.pipe)
	assert.Error(t, err, "the pipe is gone with the server")
}

// A trace that cannot start is said to the reader in the helper's words,
// which is what the panel then shows, and the helper ends.
func TestATraceThatFailsIsSaidToTheReader(t *testing.T) {
	trace := &fakeTrace{err: errors.New("enabling the network provider: Access is denied")}
	s := testServer(t, trace)
	done := make(chan error, 1)
	go func() { done <- s.serve(t.Context()) }()

	lines := bufio.NewScanner(openWhenThere(t, s.pipe))
	assert.Equal(t, core.NetHelperProtocol, readLine(t, lines).V)
	assert.Contains(t, readLine(t, lines).Error, "Access is denied")
	require.Error(t, <-done)
}

// The panel's reader against the real server: it finds no pipe, starts the
// helper, waits for the pipe and reads this process's rate by name.
func TestThePanelReadsTheHelperItStarted(t *testing.T) {
	trace := &fakeTrace{}
	s := testServer(t, trace)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	started := 0
	start := func() error {
		started++
		go func() { _ = s.serve(ctx) }()
		return nil
	}
	samples := func(context.Context) ([]core.ProcessSample, error) {
		return []core.ProcessSample{{PID: os.Getpid(), Name: "this test"}}, nil
	}
	r := core.NewNetHelperReader(s.pipe, start, samples)
	defer r.Close()

	use, err := r.Read(t.Context())
	require.NoError(t, err)
	assert.False(t, use.HasRate, "the first reading opens the pipe and has no rate")
	assert.Equal(t, 1, started)

	var got core.NetProcess
	require.Eventually(t, func() bool {
		use, err := r.Read(t.Context())
		if err != nil || !use.HasRate {
			return false
		}
		for _, p := range use.Processes {
			if p.Rx > 0 {
				got = p
			}
		}
		return got.Rx > 0
	}, 2*time.Second, 60*time.Millisecond)
	assert.Equal(t, "this test", got.Name)
	assert.Equal(t, os.Getpid(), got.PID)
	assert.InDelta(t, got.Rx/2, got.Tx, got.Rx/100, "received and sent keep their places")
	assert.Equal(t, 1, started, "a pipe that is there is not started again")

	// Closing the reader is what lets the helper go.
	r.Close()
	require.Eventually(t, func() bool { _, stops := trace.seen(); return stops > 0 }, 2*time.Second, 20*time.Millisecond)
}

// The command line a service is registered with quotes a path with a space
// in it, and the program and the version are read back out of what the
// service manager keeps.
func TestTheServicesCommandLineKeepsItsProgram(t *testing.T) {
	line := commandLine(`C:\Program Files\hayami\hayami-network.exe`, []string{"network-helper", "run", "--allow", "S-1-5-21-1-2-3-1001"})
	assert.Equal(t, `"C:\Program Files\hayami\hayami-network.exe" network-helper run --allow S-1-5-21-1-2-3-1001`, line)
	assert.Equal(t, `C:\Program Files\hayami\hayami-network.exe`, programOf(line))
	assert.Equal(t, `C:\hayami\helper.exe`, programOf(`C:\hayami\helper.exe network-helper run`))

	m := versionInDescription.FindStringSubmatch(description("0.10.1"))
	require.NotNil(t, m)
	assert.Equal(t, "0.10.1", m[1])
}
