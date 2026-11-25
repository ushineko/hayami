package client_test

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/hayami/internal/api/client"
)

// R29. A stream is read into typed events, and what the contract does not
// have in one is an error rather than an event.
func TestAStreamIsReadIntoTypedEvents(t *testing.T) {
	const clock = `{"key":"clock","polled_at":"2026-11-25T10:00:00Z","interval_s":1,"reading":{"clock":` +
		`{"time":"2026-11-25T10:00:00Z","zone":"UTC","utc_offset_s":0},"bandwidth":null,"usage":null,"cooler":null,` +
		`"peripherals":null},"absences":[],"error":null,"details":[],"truncated":false}`
	cases := []struct {
		name, body string
		check      func(t *testing.T, s *client.Stream)
	}{
		{"a comment, then a section", ": hayami 1.0\n\nevent: section\ndata: " + clock + "\n\n", func(t *testing.T, s *client.Stream) {
			ev, err := s.Next()
			require.NoError(t, err)
			assert.Equal(t, client.EventComment, ev.Name)
			assert.Equal(t, "hayami 1.0", ev.Text)
			ev, err = s.Next()
			require.NoError(t, err)
			require.Equal(t, client.EventSection, ev.Name)
			assert.Equal(t, "clock", ev.Section.Key)
			_, err = s.Next()
			assert.ErrorIs(t, err, io.EOF)
		}},
		{"an event the contract does not have", "event: surprise\ndata: {}\n\n", func(t *testing.T, s *client.Stream) {
			_, err := s.Next()
			var ce *client.ContractError
			require.ErrorAs(t, err, &ce)
			assert.Contains(t, ce.Reason, "surprise")
		}},
		{"a line no event has", "retry: 10\n\n", func(t *testing.T, s *client.Stream) {
			_, err := s.Next()
			var ce *client.ContractError
			require.ErrorAs(t, err, &ce)
		}},
		{"a section off the contract", "event: section\ndata: " + strings.Replace(clock, `"zone":"UTC"`, `"zone":3`, 1) + "\n\n",
			func(t *testing.T, s *client.Stream) {
				_, err := s.Next()
				var ce *client.ContractError
				require.ErrorAs(t, err, &ce)
				assert.Equal(t, "events.section.reading.clock.zone", ce.Path)
			}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			sock := serveStream(t, strings.NewReader(c.body))
			s, err := client.New(sock).Events(context.Background(), []string{"clock"}, nil)
			require.NoError(t, err)
			t.Cleanup(func() { _ = s.Close() })
			c.check(t, s)
		})
	}
}

// B4. A stream is read no further than a response: a line that does not end
// is an error at MaxBody, not memory without end.
func TestAStreamsLineIsBounded(t *testing.T) {
	sock := serveStream(t, io.MultiReader(strings.NewReader("event: section\ndata: "), endless{}))
	s, err := client.New(sock).Events(context.Background(), []string{"clock"}, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = s.Close() })
	_, err = s.Next()
	require.Error(t, err)
	assert.False(t, errors.Is(err, io.EOF))
}

// endless is a reader of x without end.
type endless struct{}

func (endless) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = 'x'
	}
	return len(p), nil
}

// serveStream answers /v1/events with body, as an event stream.
func serveStream(t *testing.T, body io.Reader) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "hy")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	sock := filepath.Join(dir, "api.sock")
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/events", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		buf := make([]byte, 1<<20)
		for r.Context().Err() == nil {
			n, err := body.Read(buf)
			if _, werr := w.Write(buf[:n]); werr != nil || err != nil {
				return
			}
		}
	})
	var lc net.ListenConfig
	l, err := lc.Listen(context.Background(), "unix", sock)
	require.NoError(t, err)
	srv := &http.Server{Handler: mux, ReadHeaderTimeout: time.Second}
	go func() { _ = srv.Serve(l) }()
	t.Cleanup(func() { _ = srv.Close() })
	return sock
}
