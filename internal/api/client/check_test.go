package client_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/hayami/internal/api/client"
	"github.com/ushineko/hayami/internal/api/contract"
)

/*
R30, falsified. `api check` against a service that breaks the contract in one
way at a time fails, and names the value; against the same service keeping
it, passes. The service here is a stand-in that serves one section, so each
fault is the only thing wrong.
*/
func TestCheckNamesWhatIsWrong(t *testing.T) {
	cases := []struct {
		name   string
		fault  func(device map[string]any)
		path   string
		reason string
	}{
		{"a padded name", func(d map[string]any) { d["name"] = "Mouse   " },
			"reading.peripherals.devices[0].name", "padded"},
		{"a name cut with an ellipsis", func(d map[string]any) { d["name"] = "Mou…" },
			"reading.peripherals.devices[0].name", "ellipsis"},
		{"a string where a number belongs", func(d map[string]any) { d["battery_pct"] = "80" },
			"reading.peripherals.devices[0].battery_pct", "a string where a whole number belongs"},
		{"a missing field", func(d map[string]any) { delete(d, "stale") },
			"reading.peripherals.devices[0].stale", "missing"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			sock := standIn(t, c.fault)
			_, err := client.Check(context.Background(), client.New(sock), client.CheckOptions{Depth: 3, StreamWait: 5 * time.Second})
			require.Error(t, err)
			assert.Contains(t, err.Error(), c.path)
			assert.Contains(t, err.Error(), c.reason)
		})
	}
	t.Run("nothing wrong", func(t *testing.T) {
		sock := standIn(t, nil)
		r, err := client.Check(context.Background(), client.New(sock), client.CheckOptions{Depth: 3, StreamWait: 5 * time.Second})
		require.NoError(t, err)
		assert.Equal(t, 1, r.Sections)
		assert.Equal(t, 1, r.Events)
	})
}

/*
standIn serves one section, peripherals, with one device, keeping the
contract; fault, when given, changes the device as every endpoint serves it.
It returns the socket.
*/
func standIn(t *testing.T, fault func(map[string]any)) string {
	t.Helper()

	at := time.Date(2026, 11, 25, 10, 0, 0, 0, time.UTC)
	pct := 80
	section := contract.Section{
		Key: "peripherals", PolledAt: &at, IntervalS: 5,
		Reading: contract.Reading{Peripherals: &contract.Peripherals{Devices: []contract.Device{{
			Name: "Mouse", Kind: "mouse", BatteryPct: &pct, Charge: contract.ChargeDraining, LastSeen: &at,
		}}}},
		Absences: []contract.Absence{}, Details: []contract.DetailRef{},
	}
	// The section as JSON values, with the fault made in its device.
	b, err := json.Marshal(section)
	require.NoError(t, err)
	var raw map[string]any
	require.NoError(t, json.Unmarshal(b, &raw))
	if fault != nil {
		reading := raw["reading"].(map[string]any)["peripherals"].(map[string]any)
		fault(reading["devices"].([]any)[0].(map[string]any))
	}

	send := func(w http.ResponseWriter, status int, v any) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		if status == http.StatusServiceUnavailable {
			w.Header().Set("Retry-After", "1")
		}
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(v)
	}
	notFound := func(w http.ResponseWriter, known []string) {
		send(w, http.StatusNotFound, contract.Error{Error: contract.ErrorBody{Code: contract.ErrNotFound, Message: "not here", Known: known}})
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/v1", func(w http.ResponseWriter, _ *http.Request) {
		send(w, 200, contract.Index{Service: "hayami", Version: "test", API: contract.Version, Sections: []string{"peripherals"}})
	})
	mux.HandleFunc("/v1/health", func(w http.ResponseWriter, _ *http.Request) { send(w, 200, contract.Health{OK: true}) })
	mux.HandleFunc("/v1/sections", func(w http.ResponseWriter, _ *http.Request) {
		send(w, 200, contract.SectionList{Sections: []contract.SectionInfo{{Key: "peripherals", Title: "Peripherals", Default: true, Shown: true, IntervalS: 5}}})
	})
	mux.HandleFunc("/v1/sections/{key}", func(w http.ResponseWriter, r *http.Request) {
		if r.PathValue("key") != "peripherals" {
			notFound(w, []string{"peripherals"})
			return
		}
		send(w, 200, raw)
	})
	mux.HandleFunc("/v1/sections/{key}/details/{id...}", func(w http.ResponseWriter, _ *http.Request) { notFound(w, []string{}) })
	mux.HandleFunc("/v1/readings", func(w http.ResponseWriter, _ *http.Request) {
		send(w, 200, map[string]any{"readings": map[string]any{"peripherals": raw}})
	})
	mux.HandleFunc("/v1/events", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		data, _ := json.Marshal(raw)
		_, _ = w.Write([]byte(": hayami test\n\nevent: section\ndata: " + string(data) + "\n\n"))
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	})

	l, sock := listen(t)
	srv := &http.Server{Handler: mux, ReadHeaderTimeout: time.Second}
	go func() { _ = srv.Serve(l) }()
	t.Cleanup(func() { _ = srv.Close() })
	return sock
}
