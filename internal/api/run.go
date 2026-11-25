package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/ushineko/hayami/internal/config"
	"github.com/ushineko/hayami/internal/panel"
)

// Options are what `hayami-tui serve` runs with.
type Options struct {
	// Listen is the socket to make; empty takes the one systemd hands over,
	// or the default address (R3).
	Listen string

	// SettingsPath is the settings file the sources read and the service
	// follows (R8).
	SettingsPath string

	// Idle ends the service after this long with no request and no stream;
	// zero is the default, IdleExit when systemd started it by its socket and
	// never otherwise (R5).
	Idle time.Duration

	Version string
	Log     *slog.Logger
}

// IdleExit is how long a socket-started service waits with nobody reading
// before it exits, and leaves systemd to start it on the next connection
// (R5).
const IdleExit = 5 * time.Minute

// settingsLook is how often the settings file is looked at for a change.
const settingsLook = 2 * time.Second

// Run serves until ctx ends, or the service has been idle for its limit.
func Run(ctx context.Context, o Options) error {
	if !Supported() {
		return ErrNotHere
	}
	l, activated, err := o.listener()
	if err != nil {
		return err
	}
	idle := o.Idle
	if idle == 0 && activated {
		idle = IdleExit
	}

	store, err := config.Open(o.SettingsPath)
	if err != nil {
		_ = l.Close()
		return fmt.Errorf("reading the settings: %w", err)
	}
	settings := store.Config()
	_ = store.Close()

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	env := panel.Env{Settings: settings}
	hub := NewHub(ctx, panel.Sources(panel.Keys(), env), settings, o.Log)
	srv := NewServer(hub, o.Version, o.Log)
	hs := srv.HTTP()

	done := make(chan error, 1)
	go func() { done <- hs.Serve(Limit(l, MaxConns)) }()
	o.Log.Info("serving", "address", l.Addr().String(), "socket-started", activated)

	go followSettings(ctx, o.SettingsPath, hub, o.Log)
	stop := make(chan struct{})
	if idle > 0 {
		go exitWhenIdle(ctx, hub, idle, stop)
	}

	select {
	case err := <-done:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	case <-ctx.Done():
	case <-stop:
		o.Log.Info("idle, exiting", "after", idle)
	}
	shutdown, cancelShutdown := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelShutdown()
	cancel()
	_ = hs.Shutdown(shutdown)
	return nil
}

// listener is the socket named, the one systemd handed over, or the default.
func (o Options) listener() (net.Listener, bool, error) {
	if o.Listen != "" {
		l, err := Listen(o.Listen, o.Log)
		return l, false, err
	}
	if l, ok, err := Activated(o.Log); ok || err != nil {
		return l, ok, err
	}
	addr, err := Address()
	if err != nil {
		return nil, false, err
	}
	l, err := Listen(addr, o.Log)
	return l, false, err
}

// followSettings hands the sources the settings each time the file changes.
func followSettings(ctx context.Context, path string, hub *Hub, log *slog.Logger) {
	var seen time.Time
	if info, err := os.Stat(path); err == nil {
		seen = info.ModTime()
	}
	t := time.NewTicker(settingsLook)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		info, err := os.Stat(path)
		if err != nil || info.ModTime().Equal(seen) {
			continue
		}
		seen = info.ModTime()
		store, err := config.Open(path)
		if err != nil {
			continue
		}
		c := store.Config()
		_ = store.Close()
		hub.SetSettings(c)
		log.Info("settings changed, applied")
	}
}

// exitWhenIdle closes stop once the hub has gone limit with nobody reading.
func exitWhenIdle(ctx context.Context, hub *Hub, limit time.Duration, stop chan struct{}) {
	t := time.NewTicker(min(limit/4, 10*time.Second))
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		if idle, ok := hub.Idle(); ok && idle >= limit {
			close(stop)
			return
		}
	}
}
