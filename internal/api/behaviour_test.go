package api_test

import (
	"bufio"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/hayami/internal/api"
	"github.com/ushineko/hayami/internal/api/contract"
	"github.com/ushineko/hayami/internal/config"
	"github.com/ushineko/hayami/internal/panel"
	"github.com/ushineko/hayami/internal/view"
)

// The contract's checker refuses what it does not allow: a test that could
// not fail would pass every response.
func TestTheContractRefusesWhatItDoesNotAllow(t *testing.T) {
	for name, body := range map[string]string{
		"Health":    `{"ok":"yes"}`,
		"Index":     `{"service":"hayami","version":"1","api":"1.0","sections":[],"extra":1}`,
		"DetailRef": `{"id":"cpu"}`,
		"Section":   `{"key":"clock","polled_at":"yesterday","interval_s":1,"reading":{"clock":null,"bandwidth":null,"usage":null,"cooler":null,"peripherals":null},"absences":[],"error":null,"details":[],"truncated":false}`,
	} {
		v := mustValue(t, body)
		assert.Error(t, schema(t, name).Validate(v), "%s accepted %s", name, body)
	}
}

func mustValue(t *testing.T, body string) any {
	t.Helper()
	var v any
	require.NoError(t, json.Unmarshal([]byte(body), &v))
	return v
}

// waitFor polls cond until it holds or a second passes.
func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("waited for %s", what)
}

// R9. A service nobody reads reads nothing.
func TestNothingIsReadThatNobodyReads(t *testing.T) {
	fakes := fixtures()
	h := start(t, sources(fakes), options{})
	time.Sleep(100 * time.Millisecond)
	_, _, _ = h.get(t, "/v1/health")
	_, _, _ = h.get(t, "/v1/sections")
	for _, f := range fakes {
		assert.Zero(t, f.polls.Load(), "%s was polled with nobody reading it", f.key)
	}
}

// R10, R31. Any number of clients cause one set of polls, and a source is
// never polled twice at once.
func TestManyClientsShareOnePoll(t *testing.T) {
	f := coolerFixture()
	f.delay = 100 * time.Millisecond
	h := start(t, []panel.Source{f}, options{})

	var wg sync.WaitGroup
	for range 30 {
		wg.Go(func() {
			status, body, _ := h.get(t, "/v1/sections/cooler")
			assert.Equal(t, http.StatusOK, status)
			var s contract.Section
			assert.NoError(t, json.Unmarshal(body, &s))
			assert.NotNil(t, s.PolledAt, "a first request waits for a reading (R11)")
		})
	}
	wg.Wait()
	assert.Equal(t, int32(1), f.polls.Load(), "thirty clients, one poll")
	assert.False(t, f.twice.Load(), "a source polled twice at once")
}

// R9. A section's lease lapses with its last request, and its poller stops;
// a detail's too, and its CloseDetail is called.
func TestALeaseLapsesAndTheReadingStops(t *testing.T) {
	f := coolerFixture()
	f.interval = 10 * time.Millisecond
	h := start(t, []panel.Source{f}, options{tune: func(hub *api.Hub, _ *api.Server) { hub.MinLease = 80 * time.Millisecond }})

	status, _, _ := h.get(t, "/v1/sections/cooler/details/cpu")
	require.Equal(t, http.StatusOK, status)
	waitFor(t, "a few polls", func() bool { return f.polls.Load() >= 3 && f.reads.Load() >= 3 })

	waitFor(t, "the leases to lapse", func() bool { s, d := h.hub.Active(); return s == 0 && d == 0 })
	assert.Equal(t, 1, f.closes("cpu"), "a detail let go is closed, once")
	polls, reads := f.polls.Load(), f.reads.Load()
	time.Sleep(60 * time.Millisecond)
	assert.Equal(t, polls, f.polls.Load(), "nothing is polled once nobody reads")
	assert.Equal(t, reads, f.reads.Load())

	status, _, _ = h.get(t, "/v1/sections/cooler")
	require.Equal(t, http.StatusOK, status)
	assert.Greater(t, f.polls.Load(), polls, "a new request reads it again")
}

// R18, R20. What is not there is 404, and says what is.
func TestWhatIsNotThereSaysWhatIs(t *testing.T) {
	h := start(t, sources(fixtures()), options{})

	status, body, _ := h.get(t, "/v1/sections/nope")
	assert.Equal(t, http.StatusNotFound, status)
	var e contract.Error
	require.NoError(t, json.Unmarshal(body, &e))
	assert.Equal(t, contract.ErrNotFound, e.Error.Code)
	assert.Equal(t, []string{"clock", "bandwidth", "usage", "cooler", "peripherals"}, e.Error.Known)

	status, body, _ = h.get(t, "/v1/sections/cooler/details/gpu")
	assert.Equal(t, http.StatusNotFound, status)
	require.NoError(t, json.Unmarshal(body, &e))
	assert.Equal(t, []string{"cpu"}, e.Error.Known, "the details the section offers")

	status, _, _ = h.get(t, "/v1/sections/clock/details/anything")
	assert.Equal(t, http.StatusNotFound, status, "a section with no details offers none")
}

// R12, R13. Names come back whole; text from outside is cleaned of control
// and bidirectional characters and nothing else; a string past 4 KiB is cut
// to it and the response says so.
func TestTextIsCleanedNotCut(t *testing.T) {
	f := peripheralsFixture()
	huge := strings.Repeat("é", 6000)
	f.data = view.PeripheralsReading{Devices: []view.PeripheralReading{
		{Name: longDevice, Level: 50},
		{Name: "Mouse\x1b]52;c;aGk=\x07 \u202eevil\u202c", Level: 10},
		{Name: huge, Level: 1},
	}}
	h := start(t, []panel.Source{f}, options{})

	_, body, _ := h.get(t, "/v1/sections/peripherals")
	conforms(t, "Section", body)
	var s contract.Section
	require.NoError(t, json.Unmarshal(body, &s))
	devices := s.Reading.Peripherals.Devices
	assert.Equal(t, longDevice, devices[0].Name, "a name is whole")
	assert.Equal(t, "Mouse]52;c;aGk= evil", devices[1].Name, "control and override characters go, the rest stays")
	assert.Len(t, devices[2].Name, contract.MaxText, "cut at 4 KiB, on a character's boundary")
	assert.True(t, strings.HasPrefix(huge, devices[2].Name))
	assert.True(t, s.Truncated)
}

// R31. A panic while reading a section is that section's error; the service
// keeps answering.
func TestAPanicIsAnErrorAndTheServiceStaysUp(t *testing.T) {
	bad := coolerFixture()
	bad.panics = true
	h := start(t, []panel.Source{bad, clockFixture()}, options{})

	status, body, _ := h.get(t, "/v1/sections/cooler")
	assert.Equal(t, http.StatusOK, status)
	var s contract.Section
	require.NoError(t, json.Unmarshal(body, &s))
	require.NotNil(t, s.Error)
	assert.Equal(t, "the reading failed unexpectedly", *s.Error)
	assert.NotContains(t, string(body), "/home/", "no path under a home in a response")

	status, _, _ = h.get(t, "/v1/sections/clock")
	assert.Equal(t, http.StatusOK, status)
}

// R31. A request that panics is a 500 in the error shape, and the next is
// served.
func TestAPanickingRequestIsA500(t *testing.T) {
	f := &panicky{fakeSource: coolerFixture()}
	h := start(t, []panel.Source{f}, options{})

	status, body, _ := h.get(t, "/v1/sections/cooler/details/cpu")
	assert.Equal(t, http.StatusInternalServerError, status)
	conforms(t, "Error", body)
	status, _, _ = h.get(t, "/v1/health")
	assert.Equal(t, http.StatusOK, status)
}

// panicky is a source whose offer of details fails as no source should.
type panicky struct{ *fakeSource }

func (p *panicky) Details() []view.DetailInfo { panic("the offer fell over") }

// R8. The settings the service follows are the person's, changed while it
// runs.
func TestAChangeToTheSettingsIsFollowed(t *testing.T) {
	h := start(t, sources(fixtures()), options{settings: config.Config{Sections: []string{"clock"}}})
	shown := func() map[string]bool {
		_, body, _ := h.get(t, "/v1/sections")
		var l contract.SectionList
		require.NoError(t, json.Unmarshal(body, &l))
		out := map[string]bool{}
		for _, s := range l.Sections {
			out[s.Key] = s.Shown
		}
		return out
	}
	assert.Equal(t, map[string]bool{"clock": true, "bandwidth": false, "usage": false, "cooler": false, "peripherals": false}, shown())
	h.hub.SetSettings(config.Config{Sections: []string{"usage", "cooler"}})
	assert.Equal(t, map[string]bool{"clock": false, "bandwidth": false, "usage": true, "cooler": true, "peripherals": false}, shown())
}

// stream opens an event stream and reads its events as they come.
type stream struct {
	resp   *http.Response
	events chan streamEvent
}

type streamEvent struct{ name, data string }

func openStream(t *testing.T, h *harness, query string) (*stream, int) {
	t.Helper()
	resp, err := fetch(context.Background(), h.client, "/v1/events?"+query) //nolint:bodyclose // closed by the test's cleanup
	require.NoError(t, err)
	if resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close()
		return nil, resp.StatusCode
	}
	s := &stream{resp: resp, events: make(chan streamEvent, 64)}
	go func() {
		defer close(s.events)
		r := bufio.NewReader(resp.Body)
		var ev streamEvent
		for {
			line, err := r.ReadString('\n')
			if err != nil {
				return
			}
			line = strings.TrimRight(line, "\n")
			switch {
			case strings.HasPrefix(line, "event: "):
				ev.name = strings.TrimPrefix(line, "event: ")
			case strings.HasPrefix(line, "data: "):
				ev.data = strings.TrimPrefix(line, "data: ")
			case strings.HasPrefix(line, ":"):
				s.events <- streamEvent{name: "comment", data: line}
			case line == "" && ev.name != "":
				s.events <- ev
				ev = streamEvent{}
			}
		}
	}()
	t.Cleanup(func() { _ = resp.Body.Close() })
	return s, http.StatusOK
}

// next is the stream's next event other than a comment, or fails.
func (s *stream) next(t *testing.T) streamEvent {
	t.Helper()
	timeout := time.After(3 * time.Second)
	for {
		select {
		case ev, ok := <-s.events:
			require.True(t, ok, "the stream ended")
			if ev.name != "comment" {
				return ev
			}
		case <-timeout:
			t.Fatal("no event")
		}
	}
}

// R21. A stream sends each named section's and detail's reading at once and
// each new one after, keeps an idle stream alive with a comment, and holds
// its lease while open.
func TestAStreamSendsEachReading(t *testing.T) {
	clock := clockFixture()
	clock.interval = 30 * time.Millisecond
	cooler := coolerFixture()
	h := start(t, []panel.Source{clock, cooler}, options{tune: func(_ *api.Hub, srv *api.Server) {
		srv.KeepAlive = 20 * time.Millisecond
	}})

	s, status := openStream(t, h, "sections=clock&details=cooler/cpu")
	require.Equal(t, http.StatusOK, status)
	seen := map[string]int{}
	for range 6 {
		ev := s.next(t)
		seen[ev.name]++
		switch ev.name {
		case contract.EventSection:
			conforms(t, "Section", []byte(ev.data))
		case contract.EventDetail:
			conforms(t, "Detail", []byte(ev.data))
		}
	}
	assert.GreaterOrEqual(t, seen[contract.EventSection], 4, "the clock's readings, one a poll")
	assert.GreaterOrEqual(t, seen[contract.EventDetail], 1, "the detail's reading")

	comments := 0
	timeout := time.After(time.Second)
	for comments == 0 {
		select {
		case ev := <-s.events:
			if ev.name == "comment" {
				comments++
			}
		case <-timeout:
			t.Fatal("no keepalive comment")
		}
	}

	_ = s.resp.Body.Close()
	waitFor(t, "the stream's lease to end", func() bool {
		_, open := h.hub.Idle()
		return open
	})
}

// R21, R31. Over 16 streams the answer is 503 with Retry-After, never a
// hang; a stream that closes gives its place back.
func TestOverTheStreamLimitIsA503(t *testing.T) {
	h := start(t, sources(fixtures()), options{})
	var open []*stream
	for range api.MaxStreams {
		s, status := openStream(t, h, "sections=clock")
		require.Equal(t, http.StatusOK, status)
		s.next(t)
		open = append(open, s)
	}
	resp, err := fetch(context.Background(), h.client, "/v1/events?sections=clock")
	require.NoError(t, err)
	assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)
	assert.Equal(t, "2", resp.Header.Get("Retry-After"))
	_ = resp.Body.Close()

	_ = open[0].resp.Body.Close()
	waitFor(t, "a place to free", func() bool {
		r, err := fetch(context.Background(), h.client, "/v1/events?sections=clock")
		if err != nil {
			return false
		}
		defer func() { _ = r.Body.Close() }()
		return r.StatusCode == http.StatusOK
	})
}

// R31. Over 64 connections a connection is answered 503 and closed, not
// left waiting.
func TestOverTheConnectionLimitIsA503(t *testing.T) {
	h := start(t, sources(fixtures()), options{})
	var conns []net.Conn
	for range api.MaxConns {
		c, err := dial(t, h.sock)
		require.NoError(t, err)
		conns = append(conns, c)
	}
	t.Cleanup(func() {
		for _, c := range conns {
			_ = c.Close()
		}
	})
	waitFor(t, "the connections to be taken", func() bool {
		c, err := dial(t, h.sock)
		if err != nil {
			return false
		}
		defer func() { _ = c.Close() }()
		_ = c.SetDeadline(time.Now().Add(time.Second))
		_, _ = c.Write([]byte("GET /v1/health HTTP/1.1\r\nHost: hayami\r\n\r\n"))
		resp, err := http.ReadResponse(bufio.NewReader(c), nil)
		if err != nil {
			return false
		}
		_ = resp.Body.Close()
		return resp.StatusCode == http.StatusServiceUnavailable && resp.Header.Get("Retry-After") == "2"
	})
}

// R22. Health polls nothing.
func TestHealthPollsNothing(t *testing.T) {
	fakes := fixtures()
	h := start(t, sources(fakes), options{})
	status, body, _ := h.get(t, "/v1/health")
	assert.Equal(t, http.StatusOK, status)
	assert.JSONEq(t, `{"ok":true}`, string(body))
	for _, f := range fakes {
		assert.Zero(t, f.polls.Load())
	}
}
