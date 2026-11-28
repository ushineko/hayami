package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/ushineko/hayami/internal/api/tcpkey"
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

	// KeyPath is the TCP port's key (spec 115 R6); empty is its usual place,
	// api-tcp.json in the settings directory. Without a key the service may
	// serve with, there is no port.
	KeyPath string

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
	limiter := NewLimiter(MaxConns)

	done := make(chan error, 2)
	go func() { done <- hs.Serve(limiter.Wrap(l)) }()
	o.Log.Info("serving", "address", l.Addr().String(), "socket-started", activated)

	// The TCP port, beside the socket, on the same hub and the same limits
	// (spec 115 R5). A service nothing handed its sockets to -- the Windows
	// instance, or serve run by hand -- watches for a key to appear and
	// serves the port from then on (#423, spec 117): the key is written after
	// the instance has started.
	tcp := &tcpServer{srv: srv, done: done, log: o.Log}
	if tl, key := tcpListener(o.keyPath(), o.Log); tl != nil {
		tcp.start(ctx, tl, key, o.keyPath())
	} else if !activated {
		go tcp.waitForKey(ctx, o.keyPath())
	}

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
	tcp.shutdown(shutdown)
	return nil
}

// tcpServer is the TCP port's server, once there is one.
type tcpServer struct {
	srv  *Server
	done chan error
	log  *slog.Logger

	mu sync.Mutex
	ht *http.Server
}

// start serves the port on tl with key, and follows the key from then on.
func (t *tcpServer) start(ctx context.Context, tl net.Listener, key tcpkey.Key, keyPath string) {
	auth := newTCPAuth(key)
	ht := t.srv.HTTP()
	ht.Handler = t.srv.TCPHandler(auth, tcpPort(tl))
	t.mu.Lock()
	t.ht = ht
	t.mu.Unlock()
	// Its own limit: connections from another account to the port never
	// take the socket's places (review F2).
	go func() { t.done <- ht.Serve(NewLimiter(MaxTCPConns).Wrap(tl)) }()
	go followKey(ctx, keyPath, tcpPort(tl), auth, t.log)
	t.log.Info("serving", "address", tl.Addr().String())
}

/*
waitForKey looks at the key every keyLook until there is one the service may
serve with and its port can be listened on, then serves it (spec 117). A
refusal is logged once for each reason, not at every look.
*/
func (t *tcpServer) waitForKey(ctx context.Context, keyPath string) {
	tick := time.NewTicker(keyLook)
	defer tick.Stop()
	said := ""
	say := func(msg string, args ...any) {
		if line := fmt.Sprint(msg, args); line != said {
			said = line
			t.log.Info(msg, args...)
		}
	}
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
		key, err := tcpkey.Read(keyPath)
		if err != nil {
			if !errors.Is(err, tcpkey.ErrNoFile) {
				say("no TCP port yet: its key is refused", "error", err)
			}
			continue
		}
		tl, err := listenTCP(key.Address)
		if err != nil {
			say("no TCP port yet: it could not be listened on", "address", key.Address, "error", err)
			continue
		}
		if err := loopbackOnly(tl); err != nil {
			_ = tl.Close()
			say("no TCP port yet", "error", err)
			continue
		}
		t.log.Info("the TCP port's key appeared; serving the port")
		t.start(ctx, tl, key, keyPath)
		return
	}
}

func (t *tcpServer) shutdown(ctx context.Context) {
	t.mu.Lock()
	ht := t.ht
	t.mu.Unlock()
	if ht != nil {
		_ = ht.Shutdown(ctx)
	}
}

// keyPath is the TCP port's key: the one named, or its usual place.
func (o Options) keyPath() string {
	if o.KeyPath != "" {
		return o.KeyPath
	}
	p, err := tcpkey.Path()
	if err != nil {
		return ""
	}
	return p
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
