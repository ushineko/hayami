package core

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// recorded is two systems, each saying what it was asked. The first has "nm" and the
// second "ov3"; disconnect and connect answer as a test says.
func recorded() (s *VPNSection, calls *[]string, disconnect, connect *func() error) {
	calls = &[]string{}
	dis, con := func() error { return nil }, func() error { return nil }
	system := func(name string, has ...string) VPNBackend {
		return VPNBackend{
			Name: name,
			Active: func(context.Context) ([]VPN, error) {
				return nil, nil
			},
			Configured: func(context.Context) ([]string, error) { return has, nil },
			Connect: func(_ context.Context, vpn string, _ SignIn) error {
				*calls = append(*calls, name+" connect "+vpn)
				return con()
			},
			Disconnect: func(_ context.Context, vpn string) error {
				*calls = append(*calls, name+" disconnect "+vpn)
				return dis()
			},
		}
	}
	s = NewVPNSection([]string{"nm", "ov3"}, []VPNBackend{system("first", "nm", "both"), system("second", "ov3", "both")})
	s.bounceGap = 0
	return s, calls, &dis, &con
}

func TestAPressGoesToTheSystemThatHasTheConnection(t *testing.T) {
	s, calls, _, _ := recorded()

	require.NoError(t, s.Connect(context.Background(), "ov3", nil))
	require.NoError(t, s.Disconnect(context.Background(), "nm"))

	assert.Equal(t, []string{"second connect ov3", "first disconnect nm"}, *calls)
}

func TestAPressGoesToTheSystemItIsUpOn(t *testing.T) {
	s, calls, _, _ := recorded()
	s.last["both"] = VPN{Name: "both", State: VPNUp, Backend: "second"}

	require.NoError(t, s.Disconnect(context.Background(), "both"))

	assert.Equal(t, []string{"second disconnect both"}, *calls, "not the first in the table, which also has one")
}

func TestAConnectionNoSystemHasIsNotPressed(t *testing.T) {
	s, calls, _, _ := recorded()

	require.ErrorIs(t, s.Connect(context.Background(), "nobody's", nil), ErrVPNNotConfigured)

	assert.Empty(t, *calls)
}

func TestABounceDisconnectsThenConnects(t *testing.T) {
	s, calls, _, _ := recorded()

	require.NoError(t, s.Bounce(context.Background(), "nm", nil))

	assert.Equal(t, []string{"first disconnect nm", "first connect nm"}, *calls)
}

// A refused disconnect leaves the tunnel up, so there is nothing to bring back.
func TestABounceWhoseDisconnectFailedStops(t *testing.T) {
	s, calls, disconnect, _ := recorded()
	*disconnect = func() error { return ErrVPNDenied }

	require.ErrorIs(t, s.Bounce(context.Background(), "nm", nil), ErrVPNDenied)

	assert.Equal(t, []string{"first disconnect nm"}, *calls)
}

func TestABounceWhoseConnectFailedSaysSo(t *testing.T) {
	s, _, _, connect := recorded()
	*connect = func() error { return ErrVPNFailed }

	assert.ErrorIs(t, s.Bounce(context.Background(), "nm", nil), ErrVPNFailed)
}

func TestAPressThatRunsOutOfTimeSaysItTimedOut(t *testing.T) {
	s, _, _, _ := recorded()
	s.controlTimeout = time.Millisecond
	s.backends[0].Connect = func(ctx context.Context, _ string, _ SignIn) error {
		<-ctx.Done()
		return ctx.Err()
	}

	assert.ErrorIs(t, s.Connect(context.Background(), "nm", nil), ErrVPNTimedOut)
}

// A press replaced by another is not a timeout.
func TestAPressCancelledIsNotATimeout(t *testing.T) {
	s, _, _, _ := recorded()
	s.backends[0].Connect = func(ctx context.Context, _ string, _ SignIn) error {
		<-ctx.Done()
		return ctx.Err()
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := s.Connect(ctx, "nm", nil)

	assert.True(t, errors.Is(err, context.Canceled))
}

// The card follows a press at the next poll, and not at the end of the interval.
func TestAPressMakesTheNextPollRead(t *testing.T) {
	s, _, _, _ := recorded()
	reads := 0
	s.backends[0].Active = func(context.Context) ([]VPN, error) { reads++; return nil, nil }
	s.Poll(context.Background())
	require.Equal(t, 1, reads)
	s.Poll(context.Background())
	require.Equal(t, 1, reads, "within the interval")

	require.NoError(t, s.Connect(context.Background(), "nm", nil))
	s.Poll(context.Background())

	assert.Equal(t, 2, reads)
}
