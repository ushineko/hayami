package panel

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/ushineko/hayami/internal/config"
	"github.com/ushineko/hayami/internal/core"
	"github.com/ushineko/hayami/internal/keepalive"
	"github.com/ushineko/hayami/internal/readings"
)

const (
	// keepaliveTick is how often the helper looks. The VPNs are read at
	// core.VPNInterval however often it asks.
	keepaliveTick = time.Second

	// keepaliveStartTries is how long the helper waits for a lock another process
	// has for a moment: a panel looking to see whether it runs, or finishing a cycle.
	keepaliveStartTries = 25
	keepaliveStartStep  = 100 * time.Millisecond
)

// ErrKeepaliveRunning is a second helper finding the first.
var ErrKeepaliveRunning = errors.New("the VPN keepalive helper is already running")

// ErrKeepaliveNoSystems is a platform with no VPN systems (spec 085 R3).
var ErrKeepaliveNoSystems = errors.New("this system has no VPN systems to keep alive")

// KeepaliveOptions are what the helper runs over.
type KeepaliveOptions struct {
	Systems  []core.VPNBackend
	Checkers core.VPNCheckers

	// Settings are the settings as they are now. It is asked at every look, so what
	// a person changes in preferences applies without a restart.
	Settings func() config.BandwidthSettings

	Files keepalive.Files

	// Notify tells the person something, and Log writes to the journal.
	Notify func(ctx context.Context, text string)
	Log    func(text string)

	// Tick is how often to look; zero is keepaliveTick.
	Tick time.Duration

	// Now is the clock; nil is the real one.
	Now func() time.Time
}

// keepaliveDone is what came of something the helper started, handed back to its one
// loop, which alone touches the engine.
type keepaliveDone struct {
	kind   keepalive.Kind
	vpn    string
	err    error
	point  vpnPoint
	acting bool
}

// keepaliveRun is one running helper.
type keepaliveRun struct {
	o       KeepaliveOptions
	engine  *keepalive.Engine
	health  *vpnHealth
	section *core.VPNSection
	done    chan keepaliveDone

	// checking are the VPNs with a check under way that the engine is not tracking,
	// and redials what cancels each re-dial under way.
	checking map[string]bool
	redials  map[string]context.CancelFunc
}

/*
RunKeepalive is the helper (spec 085 R21 to R26): it holds the prober's lock so that it
makes the checks, and acts on what they say for a VPN it is switched on for. A VPN
chosen and not switched on is still checked, because no panel can while this holds the
lock, and is never acted on.

Everything that takes time runs off this loop and hands back its result, so a re-dial
waiting on NetworkManager does not stop another VPN's check.
*/
func RunKeepalive(ctx context.Context, o KeepaliveOptions) error {
	if len(o.Systems) == 0 {
		return ErrKeepaliveNoSystems
	}
	if o.Tick <= 0 {
		o.Tick = keepaliveTick
	}
	if o.Now == nil {
		o.Now = time.Now
	}
	running, err := waitForLock(ctx, o.Files.HoldRunning)
	if err != nil {
		return fmt.Errorf("taking the helper's lock: %w", err)
	}
	if running == nil {
		return ErrKeepaliveRunning
	}
	defer running()

	lockPath, err := readings.File(vpnHealthLock)
	if err != nil {
		return fmt.Errorf("finding the prober's lock: %w", err)
	}
	prober, err := waitForLock(ctx, func() (func(), bool, error) { return readings.TryLock(lockPath) })
	if err != nil {
		return fmt.Errorf("taking the prober's lock: %w", err)
	}
	if prober == nil {
		return errors.New("a panel kept the prober's lock") //nolint:err113 // said plainly
	}
	defer prober()

	unattended := map[string]bool{}
	for _, b := range o.Systems {
		unattended[b.Name] = b.Unattended
	}
	r := &keepaliveRun{
		o: o, engine: keepalive.New(unattended), health: newVPNHealth(o.Checkers),
		section: core.NewVPNSection(nil, o.Systems), done: make(chan keepaliveDone, 16),
		checking: map[string]bool{}, redials: map[string]context.CancelFunc{},
	}
	r.health.now = o.Now
	r.section.SetClock(o.Now)
	o.Log("running")
	t := time.NewTicker(o.Tick)
	defer t.Stop()
	for {
		r.look(ctx)
		select {
		case <-ctx.Done():
			o.Log("stopping")
			return nil
		case result := <-r.done:
			r.finished(ctx, result)
		case <-t.C:
		}
	}
}

// waitForLock takes a lock another process may have for a moment. A nil release with
// no error is a lock that stayed taken, or a context that ended.
func waitForLock(ctx context.Context, take func() (func(), bool, error)) (func(), error) {
	for range keepaliveStartTries {
		release, ok, err := take()
		if err != nil {
			return nil, err
		}
		if ok {
			return release, nil
		}
		select {
		case <-ctx.Done():
			return nil, nil //nolint:nilnil // the two absences are said alike by the caller
		case <-time.After(keepaliveStartStep):
		}
	}
	return nil, nil //nolint:nilnil // likewise
}

// options is one VPN's settings as the engine takes them.
func keepaliveOptions(v config.VPNItem) keepalive.Options {
	return keepalive.Options{NoRedial: v.NoRedial, Checked: len(v.Checks) > 0}
}

// look reads the chosen VPNs and does what is due.
func (r *keepaliveRun) look(ctx context.Context) {
	settings := r.o.Settings()
	r.health.configure(settings)
	names := make([]string, 0, len(settings.VPNs))
	for _, v := range settings.VPNs {
		names = append(names, v.Name)
	}
	r.section.Follow(names)
	r.section.Poll(ctx)
	vpns := r.section.Readings()

	r.health.mu.Lock()
	timing := keepalive.Timing{Every: r.health.every, Grace: r.health.grace}
	r.health.mu.Unlock()

	var actions []keepalive.Action
	err := r.o.Files.Update(func(s *keepalive.State) {
		actions = nil
		for _, v := range vpns {
			item, _ := findVPNItem(settings, v.Name)
			if item.Keepalive {
				actions = append(actions, r.engine.Observe(r.o.Now(), v, keepaliveOptions(item), timing, s)...)
			}
		}
	})
	if err != nil {
		// Acting without the file could re-dial a tunnel a person just took down.
		r.o.Log(err.Error())
		return
	}
	for _, a := range actions {
		r.act(ctx, a)
	}
	r.watchOthers(ctx, vpns, settings)
}

// findVPNItem is one chosen VPN's settings.
func findVPNItem(settings config.BandwidthSettings, name string) (config.VPNItem, bool) {
	i := slices.IndexFunc(settings.VPNs, func(v config.VPNItem) bool { return v.Name == name })
	if i < 0 {
		return config.VPNItem{}, false
	}
	return settings.VPNs[i], true
}

// watchOthers checks the chosen VPNs the helper is not switched on for: it holds the
// lock, so nobody else can.
func (r *keepaliveRun) watchOthers(ctx context.Context, vpns []core.VPN, settings config.BandwidthSettings) {
	var others []core.VPN
	for _, v := range vpns {
		item, _ := findVPNItem(settings, v.Name)
		if !item.Keepalive && !r.checking[v.Name] {
			others = append(others, v)
		}
	}
	for _, v := range r.health.due(others, r.health.points()) {
		r.checking[v.Name] = true
		r.check(ctx, v.Name, false)
	}
}

// check runs one VPN's checks off the loop.
func (r *keepaliveRun) check(ctx context.Context, name string, acting bool) {
	go func() {
		point, ok := r.health.checkOne(ctx, name)
		if !ok {
			return // cancelled: the helper is stopping
		}
		r.hand(ctx, keepaliveDone{kind: keepalive.RunChecks, vpn: name, point: point, acting: acting})
	}()
}

// hand gives the loop a result, unless it has stopped listening.
func (r *keepaliveRun) hand(ctx context.Context, result keepaliveDone) {
	select {
	case r.done <- result:
	case <-ctx.Done():
	}
}

// act does what the engine asked.
func (r *keepaliveRun) act(ctx context.Context, a keepalive.Action) {
	switch a.Kind {
	case keepalive.RunChecks:
		r.check(ctx, a.VPN, true)
	case keepalive.Bounce:
		r.o.Log(a.VPN + ": bouncing after a failed check")
		go func() {
			r.hand(ctx, keepaliveDone{kind: keepalive.Bounce, vpn: a.VPN, err: r.section.Bounce(ctx, a.VPN, nil)})
		}()
	case keepalive.Redial:
		r.o.Log(a.VPN + ": down, re-dialling")
		dial, cancel := context.WithCancel(ctx)
		r.redials[a.VPN] = cancel
		go func() {
			defer cancel()
			r.hand(ctx, keepaliveDone{kind: keepalive.Redial, vpn: a.VPN, err: r.section.Connect(dial, a.VPN, nil)})
		}()
	case keepalive.CancelRedial:
		r.o.Log(a.VPN + ": a person took it down, so the re-dial stops")
		if cancel, ok := r.redials[a.VPN]; ok {
			cancel()
		}
	case keepalive.DisconnectAndDisable:
		r.o.Log(a.VPN + ": disconnecting, and leaving it until a person re-enables it")
		go func() {
			if err := r.section.Disconnect(ctx, a.VPN); err != nil {
				r.o.Log(a.VPN + ": the disconnect failed: " + err.Error())
			}
		}()
	case keepalive.Notify:
		r.o.Log(a.Text)
		r.o.Notify(ctx, a.Text)
	}
}

// finished takes what came of something started off the loop to the engine.
func (r *keepaliveRun) finished(ctx context.Context, d keepaliveDone) {
	if d.kind == keepalive.RunChecks && !d.acting {
		delete(r.checking, d.vpn)
		r.store(d.point)
		return
	}
	item, _ := findVPNItem(r.o.Settings(), d.vpn)
	var actions []keepalive.Action
	err := r.o.Files.Update(func(s *keepalive.State) {
		switch d.kind {
		case keepalive.RunChecks:
			actions = r.engine.CheckDone(d.vpn, keepaliveResult(d.point), s)
		case keepalive.Bounce:
			actions = r.engine.BounceDone(r.o.Now(), d.vpn, keepaliveOptions(item), d.err, s)
		case keepalive.Redial:
			actions = r.engine.RedialDone(r.o.Now(), d.vpn, d.err, s)
		}
	})
	if err != nil {
		r.o.Log(err.Error())
	}
	if d.err != nil && !errors.Is(d.err, context.Canceled) {
		r.o.Log(d.vpn + ": " + d.err.Error())
	}
	if d.kind == keepalive.RunChecks {
		d.point.Bounce = slices.ContainsFunc(actions, func(a keepalive.Action) bool { return a.Kind == keepalive.Bounce })
		r.store(d.point)
	}
	for _, a := range actions {
		r.act(ctx, a)
	}
}

// store keeps a point. A store that cannot be written costs the history, and says so.
func (r *keepaliveRun) store(p vpnPoint) {
	if err := appendVPNPoint(p); err != nil {
		r.o.Log(err.Error())
	}
}

// keepaliveResult is what a cycle of checks came to.
func keepaliveResult(p vpnPoint) keepalive.Result {
	switch {
	case !checksRan(p):
		return keepalive.NoResult
	case p.OK:
		return keepalive.Passed
	}
	return keepalive.Failed
}
