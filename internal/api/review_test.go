package api_test

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/hayami/internal/api"
	"github.com/ushineko/hayami/internal/api/contract"
	"github.com/ushineko/hayami/internal/panel"
	"github.com/ushineko/hayami/internal/view"
)

/*
The adversarial review of spec 113's service (threat model, 2026-11-25):
each finding fixed before merge, pinned here.
*/

// Finding 1. A drill's ID is the client's to make up, and each one read is a
// poller and a read of the source. A section reads at most MaxDetails at
// once; past that the answer is 503; and a detail nobody reads is let go,
// so made-up IDs do not pile up.
//
// The leases run on the test's clock (#413): it stands still while the
// details are asked for, so none lapses however slow the machine is, and
// then moves past every lease.
func TestMadeUpDrillsAreBoundedAndLetGo(t *testing.T) {
	f := coolerFixture()
	f.anyDrill, f.interval = true, 10*time.Millisecond
	var clock atomic.Int64
	clock.Store(polledAt.UnixNano())
	h := start(t, []panel.Source{f}, options{tune: func(hub *api.Hub, _ *api.Server) {
		hub.SetClock(func() time.Time { return time.Unix(0, clock.Load()) })
	}})

	busy := 0
	for i := range api.MaxDetails + 8 {
		status, body, header := h.get(t, fmt.Sprintf("/v1/sections/cooler/details/cpu/made-up-%d", i))
		switch status {
		case http.StatusOK:
		case http.StatusServiceUnavailable:
			busy++
			assert.Equal(t, "2", header.Get("Retry-After"))
			conforms(t, "Error", body)
		default:
			t.Fatalf("answered %d: %s", status, body)
		}
	}
	assert.Equal(t, 8, busy, "past %d details at once, the rest come back later", api.MaxDetails)

	clock.Add(int64(time.Hour))
	waitFor(t, "the details to be let go", func() bool { _, d := h.hub.Active(); return d == 0 })
	status, _, _ := h.get(t, "/v1/sections/cooler/details/cpu/made-up-again")
	assert.Equal(t, http.StatusOK, status, "a detail let go gives its place back")
}

// Finding 1. A stream names a bounded number of details.
func TestAStreamNamesABoundedNumberOfDetails(t *testing.T) {
	h := start(t, sources(fixtures()), options{})
	q := "details="
	for i := range api.MaxStreamDetails + 1 {
		if i > 0 {
			q += ","
		}
		q += fmt.Sprintf("cooler/cpu/x%d", i)
	}
	status, body, _ := h.get(t, "/v1/events?"+q)
	assert.Equal(t, http.StatusBadRequest, status)
	conforms(t, "Error", body)
}

// Finding 4. A figure that is not a number is null, and the rest of the
// reading is served: JSON has no NaN, and an encoder that failed after the
// status was sent answered an empty 200.
func TestAFigureThatIsNotANumberIsNull(t *testing.T) {
	f := coolerFixture()
	f.data = view.CoolerReading{Probes: []view.Probe{
		{ID: "gpu", Role: view.RoleGPU, Temp: view.Some(math.NaN()), Load: view.Some(math.Inf(1))},
	}}
	h := start(t, []panel.Source{f}, options{})

	status, body, _ := h.get(t, "/v1/sections/cooler")
	require.Equal(t, http.StatusOK, status, "%s", body)
	conforms(t, "Section", body)
	var s contract.Section
	require.NoError(t, json.Unmarshal(body, &s))
	require.Len(t, s.Reading.Cooler.Probes, 1)
	assert.Nil(t, s.Reading.Cooler.Probes[0].TempC)
	assert.Nil(t, s.Reading.Cooler.Probes[0].LoadPct)
}

// Finding 5. An error's text is cleaned as a reading's is: the IDs a 404
// lists come from the source.
func TestAnErrorsTextIsCleaned(t *testing.T) {
	f := coolerFixture()
	f.infos = []view.DetailInfo{{ID: "cpu\x1b]52;c;aGk=\x07", Title: "CPU"}}
	h := start(t, []panel.Source{f}, options{})

	status, body, _ := h.get(t, "/v1/sections/cooler/details/nope")
	assert.Equal(t, http.StatusNotFound, status)
	var e contract.Error
	require.NoError(t, json.Unmarshal(body, &e))
	assert.Equal(t, []string{"cpu]52;c;aGk="}, e.Error.Known)
}
