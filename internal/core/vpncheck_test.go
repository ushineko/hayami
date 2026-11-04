package core_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/ushineko/hayami/internal/core"
)

func TestACheckIsTimedAndSaysHowItWent(t *testing.T) {
	checkers := core.VPNCheckers{
		"pass": func(context.Context, core.VPNCheck) error {
			time.Sleep(5 * time.Millisecond)
			return nil
		},
		"fail": func(context.Context, core.VPNCheck) error { return errors.New("no reply") },
		"never": func(context.Context, core.VPNCheck) error {
			return fmt.Errorf("%w: refused", core.ErrVPNCheckDidNotRun)
		},
	}
	ctx := context.Background()

	pass := checkers.Run(ctx, core.VPNCheck{Type: "pass"})
	assert.True(t, pass.OK)
	assert.GreaterOrEqual(t, pass.LatencyMS, 5.0)

	fail := checkers.Run(ctx, core.VPNCheck{Type: "fail"})
	assert.False(t, fail.OK)
	assert.False(t, fail.DidNotRun, "no reply is a failure")

	never := checkers.Run(ctx, core.VPNCheck{Type: "never"})
	assert.False(t, never.OK)
	assert.True(t, never.DidNotRun, "a refused socket says nothing of the tunnel")

	unknown := checkers.Run(ctx, core.VPNCheck{Type: "from a newer build"})
	assert.True(t, unknown.DidNotRun)
}

// A check is given its limit, and running out of it is a failure: that is what an
// unresponsive tunnel looks like.
func TestACheckThatTimesOutFails(t *testing.T) {
	checkers := core.VPNCheckers{"wait": func(ctx context.Context, _ core.VPNCheck) error {
		<-ctx.Done()
		return ctx.Err()
	}}

	got := checkers.Run(context.Background(), core.VPNCheck{Type: "wait", Timeout: 10 * time.Millisecond})

	assert.False(t, got.OK)
	assert.False(t, got.DidNotRun)
}

func TestAPrefixMatchesLikeVPNToggles(t *testing.T) {
	assert.True(t, core.MatchesPrefix([]string{"10.0.0.1", "100.64.1.5"}, "100."))
	assert.False(t, core.MatchesPrefix([]string{"10.0.0.1"}, "100."))
	assert.False(t, core.MatchesPrefix(nil, "100."))
}

const routeFixture = `Iface	Destination	Gateway 	Flags	RefCnt	Use	Metric	Mask		MTU	Window	IRTT
eth0	00000000	0156A8C0	0003	0	0	100	00000000	0	0	0
tun0	0000080A	0100080A	0003	0	0	50	0000FFFF	0	0	0
tun0	0000000A	00000000	0001	0	0	50	000000FF	0	0	0
tun0	nothex	00000000	0001	0	0	50	000000FF	0	0	0
tun0	0000000B	00000000	0000	0	0	50	000000FF	0	0	0
short
`

func TestTheRouteTableIsReadForOneInterface(t *testing.T) {
	gateway, routes := core.ParseRouteTable(strings.NewReader(routeFixture), "tun0")

	assert.Equal(t, "10.8.0.1", gateway)
	assert.Equal(t, []string{"10.8.0.0/16", "10.0.0.0/8"}, routes,
		"another interface's, an unparsable and a route that is not up are left out")
}

func FuzzParseRouteTable(f *testing.F) {
	f.Add(routeFixture, "tun0")
	f.Add("", "")
	f.Add("tun0 FFFFFFFF FFFFFFFF FFFFFFFF 0 0 0 FF00FF00", "tun0")
	f.Fuzz(func(_ *testing.T, table, iface string) {
		core.ParseRouteTable(strings.NewReader(table), iface)
	})
}
