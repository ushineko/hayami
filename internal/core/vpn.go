package core

import (
	"context"
	"errors"
	"fmt"
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

// VPNBackend is one system that keeps VPN connections, as a row in the host's
// table (spec 085 R2).
type VPNBackend struct {
	Name string

	// Active is the connections that are up or coming up. One this does not
	// list is down.
	Active func(context.Context) ([]VPN, error)
}

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
}

// NewVPNSection builds a section over the chosen names and the host's backends.
func NewVPNSection(names []string, backends []VPNBackend) *VPNSection {
	return &VPNSection{
		names: slices.Clone(names), backends: backends,
		now: time.Now, last: map[string]VPN{},
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
