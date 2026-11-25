package api

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/windows/svc"
)

// execute runs h as the service manager would, sending reqs, and returns its
// exit code and the states it reported.
func execute(t *testing.T, h *handler, reqs ...svc.ChangeRequest) (uint32, []svc.State) {
	t.Helper()
	requests := make(chan svc.ChangeRequest, len(reqs))
	status := make(chan svc.Status, 16)
	type result struct{ code uint32 }
	done := make(chan result, 1)
	go func() {
		_, code := h.Execute(nil, requests, status)
		done <- result{code}
	}()
	var states []svc.State
	for _, r := range reqs {
		// Each request after the service has said it runs.
		for s := range status {
			states = append(states, s.State)
			if s.State == svc.Running {
				break
			}
		}
		requests <- r
	}
	select {
	case r := <-done:
		close(status)
		for s := range status {
			states = append(states, s.State)
		}
		return r.code, states
	case <-time.After(10 * time.Second):
		t.Fatal("the service did not stop")
		return 0, nil
	}
}

func quietLog() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

// R6. A stop asked for ends serving and is a clean stop: the recovery
// actions do not restart it.
func TestAStopAskedForIsAClean(t *testing.T) {
	var ended bool
	h := &handler{ctx: context.Background(), o: Options{Log: quietLog()}, run: func(ctx context.Context, _ Options) error {
		<-ctx.Done()
		ended = true
		return ctx.Err()
	}}
	code, states := execute(t, h, svc.ChangeRequest{Cmd: svc.Stop})
	assert.Equal(t, uint32(0), code)
	assert.True(t, ended, "serving was told to end")
	assert.Equal(t, []svc.State{svc.Running, svc.StopPending, svc.StopPending}, states)
}

// R6. Serving that fails ends the instance with an error, which is what the
// recovery actions restart.
func TestServingThatFailsIsAFailure(t *testing.T) {
	h := &handler{ctx: context.Background(), o: Options{Log: quietLog()}, run: func(context.Context, Options) error {
		return errors.New("the pipe went") //nolint:err113 // a test's
	}}
	code, _ := execute(t, h)
	assert.Equal(t, uint32(1), code)
	require.Error(t, h.err)
}

// R6. An instance with nothing to serve stops at once, cleanly.
func TestNothingToServeStopsCleanly(t *testing.T) {
	code, states := execute(t, &handler{ctx: context.Background(), o: Options{Log: quietLog()}})
	assert.Equal(t, uint32(0), code)
	assert.Equal(t, []svc.State{svc.StopPending}, states)
}

// R6. Under the service manager, an account that has never run hayami gets
// an instance that serves nothing; one that has gets the service.
func TestAnAccountWithoutHayamiIsServedNothing(t *testing.T) {
	t.Cleanup(func() { isService, runService = svc.IsWindowsService, svc.Run })
	isService = func() (bool, error) { return true, nil }
	var ran *handler
	runService = func(_ string, h svc.Handler) error {
		ran = h.(*handler)
		return nil
	}

	dir := t.TempDir()
	never := Options{SettingsPath: filepath.Join(dir, "absent", "settings.yaml"), Log: quietLog()}
	require.NoError(t, Serve(context.Background(), never))
	require.NotNil(t, ran)
	assert.Nil(t, ran.run, "an account with no settings directory is served nothing")

	ran = nil
	has := Options{SettingsPath: filepath.Join(dir, "settings.yaml"), Log: quietLog()}
	require.NoError(t, Serve(context.Background(), has))
	require.NotNil(t, ran)
	assert.NotNil(t, ran.run, "an account with a settings directory is served")
}
