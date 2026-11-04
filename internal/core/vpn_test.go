package core_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/hayami/internal/core"
)

// system is a VPN system that says what a test tells it to.
type system struct {
	vpns  []core.VPN
	err   error
	calls int
}

func (s *system) backend() core.VPNBackend {
	return core.VPNBackend{Name: "fake", Active: func(context.Context) ([]core.VPN, error) {
		s.calls++
		return s.vpns, s.err
	}}
}

// polled is a section over one system, with a clock the test moves.
func polled(names []string, s *system) (*core.VPNSection, func()) {
	at := time.Unix(1_000_000, 0)
	v := core.NewVPNSection(names, []core.VPNBackend{s.backend()})
	v.SetClock(func() time.Time { return at })
	return v, func() {
		at = at.Add(core.VPNInterval)
		v.Poll(context.Background())
	}
}

// A chosen VPN keeps its reading when its tunnel is down, or connecting would
// add a row and resize the panel.
func TestAChosenVPNIsDownNotGone(t *testing.T) {
	s := &system{vpns: []core.VPN{{Name: "a", State: core.VPNUp, Interface: "tun0"}}}
	v, poll := polled([]string{"a", "b"}, s)

	poll()
	got := v.Readings()
	require.Len(t, got, 2)
	assert.Equal(t, core.VPNUp, got[0].State)
	assert.Equal(t, "tun0", got[0].Interface)
	assert.Equal(t, core.VPNDown, got[1].State)

	s.vpns = nil
	poll()
	got = v.Readings()
	require.Len(t, got, 2)
	assert.Equal(t, core.VPNDown, got[0].State)
	assert.Empty(t, got[0].Interface, "a tunnel that went down keeps no interface")
}

// A system that will not answer says nothing about the tunnel, so the tunnel
// must not read as down (spec 085 R4).
func TestASystemThatWillNotAnswerDoesNotTakeATunnelDown(t *testing.T) {
	s := &system{vpns: []core.VPN{{Name: "a", State: core.VPNUp, Interface: "tun0"}}}
	v, poll := polled([]string{"a"}, s)
	poll()

	s.vpns, s.err = nil, errors.New("the bus refused")
	poll()

	got := v.Readings()
	assert.Equal(t, core.VPNUp, got[0].State, "the last reading stands")
	assert.True(t, got[0].Stale, "and says it is old")
	require.Error(t, v.Failed())

	s.err = nil
	poll()
	got = v.Readings()
	assert.Equal(t, core.VPNDown, got[0].State, "an answer settles it")
	assert.False(t, got[0].Stale)
	assert.NoError(t, v.Failed())
}

// With no earlier reading to keep, a system that will not answer leaves the VPN
// unknown, and still not down.
func TestAVPNNeverReadIsUnknownWhenTheSystemWillNotAnswer(t *testing.T) {
	v, poll := polled([]string{"a"}, &system{err: errors.New("the bus refused")})

	poll()

	got := v.Readings()
	assert.Equal(t, core.VPNUnknown, got[0].State)
	assert.True(t, got[0].Stale)
}

// A system that is not installed is not a failure: a VPN it would have kept is
// simply not up.
func TestAnAbsentSystemIsNotAFailure(t *testing.T) {
	s := &system{err: core.ErrVPNBackendAbsent}
	v, poll := polled([]string{"a"}, s)

	poll()

	assert.NoError(t, v.Failed())
	assert.True(t, v.NoSystem())
	assert.Equal(t, core.VPNDown, v.Readings()[0].State)
}

// One system down and another up: a VPN the live one vouches for is read, and
// the absent one is not reported as no system.
func TestOneSystemAbsentLeavesTheOther(t *testing.T) {
	gone := &system{err: core.ErrVPNBackendAbsent}
	live := &system{vpns: []core.VPN{{Name: "a", State: core.VPNUp}}}
	v := core.NewVPNSection([]string{"a"}, []core.VPNBackend{gone.backend(), live.backend()})

	v.Poll(context.Background())

	assert.False(t, v.NoSystem())
	assert.Equal(t, core.VPNUp, v.Readings()[0].State)
	assert.Equal(t, "fake", v.Readings()[0].Backend)
}

// Nothing is asked until a VPN is chosen, and then only every VPNInterval: the
// bandwidth poll that drives this comes round every two seconds.
func TestTheSystemsAreAskedOnlyWhenChosenAndDue(t *testing.T) {
	s := &system{}
	v, poll := polled(nil, s)
	poll()
	assert.Zero(t, s.calls, "a machine with no VPN chosen asks nothing")

	v.SetNames([]string{"a"})
	poll()
	assert.Equal(t, 1, s.calls)
	v.Poll(context.Background())
	assert.Equal(t, 1, s.calls, "the clock has not moved")
	poll()
	assert.Equal(t, 2, s.calls)
}

// A reading is written before anything has been asked: nothing is known, which
// is not the same as down.
func TestBeforeTheFirstPollAVPNIsUnknown(t *testing.T) {
	v := core.NewVPNSection([]string{"a"}, nil)
	assert.Equal(t, core.VPNUnknown, v.Readings()[0].State)
}
