package core

import (
	"errors"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The trace session is read for real, once, where the desk allows one: an
// elevated shell, or a member of Performance Log Users. It makes a request
// of its own so there is traffic to see, and expects this process in the
// list. Anywhere else the refusal has to be the typed absence.
//
// Measured on Windows 11: the first events arrive about two seconds after
// the session starts, and within a quarter of a second after that, so the
// list is read until this process shows or eight seconds pass.
func TestTheNetworkTraceCountsThisProcess(t *testing.T) {
	if os.Getenv("HAYAMI_DESK_TEST") == "" {
		t.Skip("reads the live network trace; set HAYAMI_DESK_TEST=1")
	}
	r := newNetReader(systemProcesses)
	defer r.Close()

	_, err := r.Read(t.Context())
	if a, ok := AbsenceOf(err); ok {
		assert.Equal(t, AbsenceNetProcessesDenied, a.Code)
		assert.True(t, errors.Is(err, &Absence{Code: AbsenceNetProcessesDenied}))
		assert.False(t, a.Actionable, "hayami does not ask for elevation, so there is nothing to do in the panel")
		t.Logf("refused, as an unelevated user is: %s", a.Detail)
		return
	}
	require.NoError(t, err)

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get("https://example.com/")
	if err != nil {
		t.Skipf("no network to make traffic on: %v", err)
	}
	_ = resp.Body.Close()

	me := os.Getpid()
	var rx, tx float64
	for deadline := time.Now().Add(8 * time.Second); time.Now().Before(deadline) && (rx == 0 || tx == 0); {
		time.Sleep(250 * time.Millisecond)
		use, err := r.Read(t.Context())
		require.NoError(t, err)
		require.True(t, use.HasRate)
		assert.Equal(t, "TCP and UDP", use.Counts)
		for _, p := range use.Processes {
			if p.PID == me {
				rx += p.Rx
				tx += p.Tx
				assert.NotEmpty(t, p.Name)
			}
		}
	}
	assert.Positive(t, rx, "the reply was received")
	assert.Positive(t, tx, "the request was sent")
}
