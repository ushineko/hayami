package core

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"sync"
	"time"
)

// VPNInterval is how often the VPNs are read (spec 085). The bandwidth poll's
// two seconds would be several bus calls per process for something that
// changes a few times a day; six is soon enough to read a change at a glance.
const VPNInterval = 6 * time.Second

// vpnReadTimeout bounds one backend's read, so a daemon that has stopped
// answering costs a stale row and not a stalled poll.
const vpnReadTimeout = 2 * time.Second

// VPNState is what a VPN is doing. Unknown is not Down: it says the system
// that knows would not answer, which is a fact about the reading and not about
// the tunnel (spec 085 R4).
type VPNState int

// The states a VPN can be read in.
const (
	VPNDown VPNState = iota
	VPNConnecting
	VPNUp
	VPNUnknown
)

var vpnStateNames = map[VPNState]string{
	VPNDown: "down", VPNConnecting: "connecting", VPNUp: "up", VPNUnknown: "unknown",
}

// MarshalText is the state's name in the JSON the command line prints.
func (s VPNState) MarshalText() ([]byte, error) { return []byte(vpnStateNames[s]), nil }

// VPN is one connection as a backend reports it. It holds no address: those
// are the drawer's, and this is what the card and `readings` are built from
// (spec 085 R9).
type VPN struct {
	Name      string    `json:"name"`
	Backend   string    `json:"backend,omitempty"`
	State     VPNState  `json:"state"`
	Interface string    `json:"interface,omitempty"`
	Since     time.Time `json:"since,omitzero"`

	// Stale marks a reading kept from an earlier poll, because the system
	// that knows would not answer this one.
	Stale bool `json:"stale,omitempty"`
}

// ErrVPNBackendAbsent is a backend whose system is not running on this
// machine: nothing to read, and not a failure.
var ErrVPNBackendAbsent = errors.New("not running")

// What a press on a VPN can come to, other than working. Each is short enough for a
// line of the drawer, and none carries a sign-in address (spec 085 R19).
var (
	ErrVPNDenied           = errors.New("refused by policy")
	ErrVPNNeedsCredentials = errors.New("needs a password")
	ErrVPNTimedOut         = errors.New("timed out")
	ErrVPNNotConfigured    = errors.New("not configured")
	ErrVPNSignInAddress    = errors.New("sign-in address refused")
	ErrVPNFailed           = errors.New("did not connect")
)

// SignIn opens a sign-in page in the person's browser. A backend calls it with an
// address SignInURL has passed, at most once a connect.
type SignIn func(*url.URL)

// SignInURL is a sign-in address from a session manager, if it is one to open: https,
// with a host. It is another program's string on its way to a browser.
func SignInURL(raw string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Host == "" {
		// Not wrapped: url.Parse quotes what it was given, and that is not to be
		// shown or logged.
		return nil, ErrVPNSignInAddress
	}
	return u, nil
}

// VPNBackend is one system that keeps VPN connections, as a row in the host's
// table (spec 085 R2).
type VPNBackend struct {
	Name string

	// Active is the connections that are up or coming up. One this does not
	// list is down.
	Active func(context.Context) ([]VPN, error)

	// Configured is the name of every connection this system has, up or not, for
	// a person choosing among them. It is asked only then, and may start a
	// system's daemon, which a poll never does.
	Configured func(context.Context) ([]string, error)

	// Connect brings a connection up and returns once it is up or will not come
	// up. Disconnect takes one down and returns once it is down; a connection
	// that is already down is not a failure. Each happens only because something
	// asked, and neither sees a secret (spec 085 R18).
	Connect    func(ctx context.Context, name string, signIn SignIn) error
	Disconnect func(ctx context.Context, name string) error
}

// vpnControlTimeout bounds a connect or a disconnect, and with it a sign-in page
// left open: vpn-toggle's 60 seconds.
const vpnControlTimeout = 60 * time.Second

// vpnBounceGap is the pause between a bounce's disconnect and its connect, which is
// vpn-toggle's.
const vpnBounceGap = 2 * time.Second

// VPNSection reads the VPNs the user chose. A chosen VPN always has a reading,
// so a tunnel going down changes a row and does not take one away.
type VPNSection struct {
	mu       sync.Mutex
	names    []string
	backends []VPNBackend
	now      func() time.Time
	readAt   time.Time
	last     map[string]VPN
	failed   error
	none     bool

	// controlTimeout and bounceGap are the constants above, for a test.
	controlTimeout time.Duration
	bounceGap      time.Duration
}

// NewVPNSection builds a section over the chosen names and the host's backends.
func NewVPNSection(names []string, backends []VPNBackend) *VPNSection {
	return &VPNSection{
		names: slices.Clone(names), backends: backends,
		now: time.Now, last: map[string]VPN{},
		controlTimeout: vpnControlTimeout, bounceGap: vpnBounceGap,
	}
}

// SetNames replaces the chosen VPNs; the next poll reads them.
func (v *VPNSection) SetNames(names []string) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.names = slices.Clone(names)
	v.readAt = time.Time{}
}

// SetClock replaces the section's clock, for a test.
func (v *VPNSection) SetClock(now func() time.Time) { v.now = now }

// Chosen reports whether any VPN is chosen.
func (v *VPNSection) Chosen() bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	return len(v.names) > 0
}

// Refresh makes the next poll read, so a card follows a press rather than waiting
// out the interval.
func (v *VPNSection) Refresh() {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.readAt = time.Time{}
}

// backendFor is the system a press on this connection goes to. The one it is up on,
// as the last reading said; otherwise the first in the table that has it, which is
// the order Poll gives a name two systems share.
func (v *VPNSection) backendFor(ctx context.Context, name string) (VPNBackend, error) {
	v.mu.Lock()
	reading := v.last[name]
	v.mu.Unlock()
	for _, b := range v.backends {
		if b.Name == reading.Backend {
			return b, nil
		}
	}
	for _, b := range v.backends {
		names, err := b.Configured(ctx)
		if err == nil && slices.Contains(names, name) {
			return b, nil
		}
	}
	return VPNBackend{}, ErrVPNNotConfigured
}

// control runs one action on a connection within the timeout, says a timeout in the
// words a person reads, and has the next poll read whatever came of it.
func (v *VPNSection) control(ctx context.Context, name string, do func(context.Context, VPNBackend) error) error {
	defer v.Refresh()
	bounded, cancel := context.WithTimeout(ctx, v.controlTimeout)
	defer cancel()
	b, err := v.backendFor(bounded, name)
	if err != nil {
		return err
	}
	err = do(bounded, b)
	if err != nil && errors.Is(bounded.Err(), context.DeadlineExceeded) {
		return ErrVPNTimedOut
	}
	return err
}

// Connect brings a connection up.
func (v *VPNSection) Connect(ctx context.Context, name string, signIn SignIn) error {
	return v.control(ctx, name, func(c context.Context, b VPNBackend) error { return b.Connect(c, name, signIn) })
}

// Disconnect takes a connection down.
func (v *VPNSection) Disconnect(ctx context.Context, name string) error {
	return v.control(ctx, name, func(c context.Context, b VPNBackend) error { return b.Disconnect(c, name) })
}

// Bounce takes a connection down and brings it back, as vpn-toggle does. A failed
// disconnect stops there, with the tunnel as it was.
func (v *VPNSection) Bounce(ctx context.Context, name string, signIn SignIn) error {
	if err := v.Disconnect(ctx, name); err != nil {
		return err
	}
	select {
	case <-ctx.Done():
		return ctx.Err() //nolint:wrapcheck // a press replaced by another, which nobody reads
	case <-time.After(v.bounceGap):
	}
	return v.Connect(ctx, name, signIn)
}

// Poll reads the backends if VPNInterval has passed and a VPN is chosen.
func (v *VPNSection) Poll(ctx context.Context) {
	v.mu.Lock()
	due := len(v.names) > 0 && v.now().Sub(v.readAt) >= VPNInterval
	if due {
		v.readAt = v.now()
	}
	v.mu.Unlock()
	if !due {
		return
	}

	// Outside the lock: a bus call can take its whole timeout, and a shell
	// painting meanwhile should not wait on it.
	active := map[string]VPN{}
	var failed []error
	absent := 0
	for _, b := range v.backends {
		read, cancel := context.WithTimeout(ctx, vpnReadTimeout)
		vpns, err := b.Active(read)
		cancel()
		switch {
		case errors.Is(err, ErrVPNBackendAbsent):
			absent++
		case err != nil:
			failed = append(failed, fmt.Errorf("%s: %w", b.Name, err))
		}
		for _, c := range vpns {
			if _, taken := active[c.Name]; !taken {
				c.Backend = b.Name
				active[c.Name] = c
			}
		}
	}

	v.mu.Lock()
	defer v.mu.Unlock()
	v.failed = errors.Join(failed...)
	v.none = absent == len(v.backends)
	for _, name := range v.names {
		c, ok := active[name]
		switch {
		case ok:
		case v.failed == nil:
			c = VPN{Name: name, State: VPNDown}
		default:
			c = v.unanswered(name)
		}
		v.last[name] = c
	}
}

// unanswered is what is known of a VPN no backend vouched for this poll, while
// one would not answer: it may be that one's, so its last reading stands, dim,
// and is not turned into a tunnel going down. The caller holds the lock.
func (v *VPNSection) unanswered(name string) VPN {
	c, ok := v.last[name]
	if !ok || c.State == VPNUnknown {
		return VPN{Name: name, State: VPNUnknown, Stale: true}
	}
	c.Stale = true
	return c
}

// Readings are the chosen VPNs in the order they were chosen. Before the first
// poll each is Unknown: nothing has been asked yet.
func (v *VPNSection) Readings() []VPN {
	v.mu.Lock()
	defer v.mu.Unlock()
	out := make([]VPN, 0, len(v.names))
	for _, name := range v.names {
		c, ok := v.last[name]
		if !ok {
			c = VPN{Name: name, State: VPNUnknown}
		}
		out = append(out, c)
	}
	return out
}

// Failed is why a backend could not be read, nil when all answered.
func (v *VPNSection) Failed() error {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.failed
}

// NoSystem reports that no backend's system is running on this machine.
func (v *VPNSection) NoSystem() bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.none
}
