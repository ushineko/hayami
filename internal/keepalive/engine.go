// Package keepalive is what keeps a chosen VPN working when nobody is looking: the
// rules of spec 085 R23 to R25, and the small file a panel and the helper share.
package keepalive

import (
	"fmt"
	"time"

	"github.com/ushineko/hayami/internal/core"
)

// vpn-toggle's defaults (spec 085 R23, R24).
const (
	// FailureThreshold is the failed check that disconnects and disables.
	FailureThreshold = 3

	// BackoffInitial and BackoffMax bound the wait between re-dials.
	BackoffInitial = 30 * time.Second
	BackoffMax     = 600 * time.Second
)

// Kind is something the engine wants done.
type Kind int

// What the engine can ask for.
const (
	// RunChecks runs a VPN's checks, and CheckDone is told what they said.
	RunChecks Kind = iota
	// Bounce and Redial act on the tunnel, and BounceDone and RedialDone are told
	// how it went.
	Bounce
	Redial
	// DisconnectAndDisable takes the tunnel down for good: the disable is already
	// recorded when this is asked for. Nobody needs telling how it went.
	DisconnectAndDisable
	// CancelRedial stops a re-dial under way, because a person took the tunnel down.
	CancelRedial
	// Notify tells the person, with Text.
	Notify
)

// Action is one thing to do, about one VPN.
type Action struct {
	Kind Kind
	VPN  string
	Text string
}

// Options are one VPN's settings, as the helper reads them.
type Options struct {
	// NoRedial switches re-dialling off for this VPN (R24).
	NoRedial bool

	// Checked says it has checks to run. One with none is only re-dialled.
	Checked bool
}

// Timing is how often a VPN is checked and how long a new tunnel is left alone.
type Timing struct {
	Every time.Duration
	Grace time.Duration
}

// Result is what a cycle of checks came to.
type Result int

// A cycle passed, failed, or could not run at all.
const (
	Passed Result = iota
	Failed
	// NoResult is a probe that could not run: it says nothing of the tunnel (R23).
	NoResult
)

// tracked is what the engine knows of one VPN since it started. Forgetting it on a
// restart is at parity (the spec's risks).
type tracked struct {
	// seen says it has been connected since the engine started: the helper
	// re-dials a tunnel that dropped and never opens a new one.
	seen    bool
	backend string

	failures  int
	connected time.Time
	lastCheck time.Time

	checking  bool
	bouncing  bool
	redialing bool

	// attempts is the failed re-dials in this outage, and retryAt when the next is
	// due; zero when none is waiting.
	attempts int
	retryAt  time.Time

	// wasDown is that it has been read down while held, so reading it up again means
	// somebody brought it back and the hold is over.
	wasDown bool

	// wasDisabled is that it has been read disabled, so reading it enabled means a
	// person has re-enabled it.
	wasDisabled bool

	// downSaid and failSaid keep a notification to one per event.
	downSaid bool
	failSaid bool
}

/*
Engine is vpn-toggle's rules as a state machine, with no bus, clock or file of its
own. It is told what was read and what came of what it asked for, and answers with
what to do. That is what lets each rule have a test (spec 085).

It reads and writes a State, which is shared with the panels: a hold written by a
press, and a disable that outlives a restart.
*/
type Engine struct {
	unattended map[string]bool
	vpns       map[string]*tracked
}

// New builds an engine. unattended says, by backend name, which systems may be acted
// on with nobody at the desk (R25).
func New(unattended map[string]bool) *Engine {
	return &Engine{unattended: unattended, vpns: map[string]*tracked{}}
}

func (e *Engine) vpn(name string) *tracked {
	t, ok := e.vpns[name]
	if !ok {
		t = &tracked{}
		e.vpns[name] = t
	}
	return t
}

// busy is an operation under way, during which a reading says nothing new.
func (t *tracked) busy() bool { return t.checking || t.bouncing || t.redialing }

// Observe takes one reading of a VPN the helper is on for, and says what to do. It is
// called at every read and asks for a check when one is due.
func (e *Engine) Observe(now time.Time, v core.VPN, o Options, timing Timing, s *State) []Action {
	t := e.vpn(v.Name)
	if v.Backend != "" {
		t.backend = v.Backend
	}
	if s.IsDisabled(v.Name) {
		t.wasDisabled = true
		return nil
	}
	if t.wasDisabled {
		// A person re-enabled it, and it starts its count again.
		t.wasDisabled, t.failures = false, 0
	}
	// A system that would not answer says nothing of the tunnel (R4), and one coming
	// up is neither.
	if v.Stale || v.State == core.VPNUnknown || v.State == core.VPNConnecting {
		return nil
	}
	if v.State == core.VPNUp {
		return e.up(now, v, t, o, timing, s)
	}
	return e.down(now, v.Name, t, o, s)
}

// up is a connected tunnel: the outage, if there was one, is over, and it is checked
// past its grace period.
func (e *Engine) up(now time.Time, v core.VPN, t *tracked, o Options, timing Timing, s *State) []Action {
	if s.IsHeld(v.Name) && (t.wasDown || !t.seen) {
		// Somebody brought it back without a press, or it was up when this started.
		// A hold on a tunnel still read up is a press on its way down, and stands.
		s.Release(v.Name)
	}
	t.wasDown = false
	if !t.seen || t.attempts > 0 || !t.retryAt.IsZero() {
		s.ClearStatus(v.Name)
	}
	if !t.seen && t.connected.IsZero() {
		t.connected = now
	}
	t.seen = true
	t.attempts, t.retryAt, t.downSaid = 0, time.Time{}, false

	grace := t.connected
	if v.Since.After(grace) {
		grace = v.Since
	}
	if t.busy() || !o.Checked || now.Sub(grace) < timing.Grace || now.Sub(t.lastCheck) < timing.Every {
		return nil
	}
	if s.IsHeld(v.Name) {
		return nil
	}
	t.checking = true
	t.lastCheck = now
	return []Action{{Kind: RunChecks, VPN: v.Name}}
}

// down is vpn-toggle's _handle_down.
func (e *Engine) down(now time.Time, name string, t *tracked, o Options, s *State) []Action {
	if s.IsHeld(name) {
		t.wasDown = true
		t.attempts, t.retryAt, t.downSaid = 0, time.Time{}, false
		s.ClearStatus(name)
		if t.redialing {
			t.redialing = false
			return []Action{{Kind: CancelRedial, VPN: name}}
		}
		return nil
	}
	if !t.seen || t.bouncing || t.redialing {
		return nil
	}
	if !e.unattended[t.backend] {
		if t.downSaid {
			return nil
		}
		t.downSaid = true
		return []Action{{Kind: Notify, VPN: name, Text: name + " dropped. It needs you to reconnect it."}}
	}
	if o.NoRedial {
		return nil
	}
	if !t.retryAt.IsZero() && now.Before(t.retryAt) {
		return nil
	}
	t.retryAt = time.Time{}
	t.redialing = true
	return []Action{{Kind: Redial, VPN: name}}
}

// CheckDone is what a cycle of checks came to: vpn-toggle's _on_session_done.
func (e *Engine) CheckDone(name string, r Result, s *State) []Action {
	t := e.vpn(name)
	t.checking = false
	switch {
	case r == NoResult:
		return nil
	case r == Passed:
		t.failures, t.failSaid = 0, false
		return nil
	case s.IsHeld(name) || s.IsDisabled(name):
		return nil // taken down while the checks ran, which is why they failed
	}
	if !e.unattended[t.backend] {
		if t.failSaid {
			return nil
		}
		t.failSaid = true
		return []Action{{Kind: Notify, VPN: name, Text: name + " failed its check. It needs you to reconnect it."}}
	}
	t.failures++
	if t.failures < FailureThreshold {
		t.bouncing = true
		return []Action{{Kind: Bounce, VPN: name}}
	}
	reason := fmt.Sprintf("failed %d checks in a row", t.failures)
	s.Disable(name, reason)
	return []Action{
		{Kind: DisconnectAndDisable, VPN: name},
		{Kind: Notify, VPN: name, Text: name + " " + reason + ", so it was disconnected and will stay down until you re-enable it."},
	}
}

// BounceDone is how a bounce went. One that left the tunnel down joins the re-dials.
func (e *Engine) BounceDone(now time.Time, name string, o Options, err error, s *State) []Action {
	t := e.vpn(name)
	t.bouncing = false
	if err == nil {
		t.connected = now
		t.attempts, t.retryAt = 0, time.Time{}
		return nil
	}
	if o.NoRedial || s.IsHeld(name) {
		return nil
	}
	e.retryLater(now, name, t, s)
	return nil
}

// RedialDone is how a re-dial went: a new grace period, or a longer wait.
func (e *Engine) RedialDone(now time.Time, name string, err error, s *State) []Action {
	t := e.vpn(name)
	t.redialing = false
	if err == nil {
		t.connected = now
		t.attempts, t.retryAt = 0, time.Time{}
		s.ClearStatus(name)
		return nil
	}
	if s.IsHeld(name) {
		return nil
	}
	e.retryLater(now, name, t, s)
	return nil
}

// retryLater schedules the next re-dial, and says so where the panels can read it. An
// outage never stops the trying and never disables a VPN.
func (e *Engine) retryLater(now time.Time, name string, t *tracked, s *State) {
	t.attempts++
	t.retryAt = now.Add(backoff(t.attempts))
	s.SetRetry(name, t.attempts, t.retryAt)
}

// backoff is the wait after a number of failed attempts: 30 s, doubling, to 600 s.
func backoff(attempts int) time.Duration {
	d := BackoffInitial
	for i := 1; i < attempts && d < BackoffMax; i++ {
		d *= 2
	}
	return min(d, BackoffMax)
}

// Failures is how many checks in a row have failed, for the point the store keeps.
func (e *Engine) Failures(name string) int { return e.vpn(name).failures }
