package core

import (
	"context"
	"net"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/godbus/dbus/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// controlBus is a bus a press acts on: it records each call in order, and each
// method's answer is a test's to give. A session reads statuses in turn, then the
// last for good.
type controlBus struct {
	fakeBus
	calls    []string
	answer   map[string]func(args []any, out []any) error
	statuses [][]any
}

func (c *controlBus) props(ctx context.Context, dest string, path dbus.ObjectPath, iface string) (map[string]dbus.Variant, error) {
	if path != "/s/new" {
		return c.fakeBus.props(ctx, dest, path, iface)
	}
	next := c.statuses[0]
	if len(c.statuses) > 1 {
		c.statuses = c.statuses[1:]
	}
	return map[string]dbus.Variant{"status": dbus.MakeVariant(next)}, nil
}

func (c *controlBus) record(method string, args []any, out []any) error {
	c.calls = append(c.calls, method)
	if f, ok := c.answer[method]; ok {
		return f(args, out)
	}
	return nil
}

func (c *controlBus) callArgs(_ context.Context, _ string, _ dbus.ObjectPath, method string, args []any, out ...any) error {
	return c.record(method, args, out)
}

func (c *controlBus) callStarting(_ context.Context, _ string, _ dbus.ObjectPath, method string, args []any, out ...any) error {
	return c.record(method, args, out)
}

func (c *controlBus) call(ctx context.Context, dest string, path dbus.ObjectPath, method string, out ...any) error {
	switch method {
	case nmSetIface + ".ListConnections", nmSettings + ".GetSettings":
		return c.fakeBus.call(ctx, dest, path, method, out...)
	}
	return c.record(method, nil, out)
}

var denied = dbus.Error{Name: "org.freedesktop.NetworkManager.PermissionDenied", Body: []any{"Not authorized"}}

// nmControlFixture has one configured VPN, and one active connection at state.
func nmControlFixture(state uint32) *controlBus {
	v := dbus.MakeVariant
	c := &controlBus{answer: map[string]func([]any, []any) error{}}
	c.configs = []dbus.ObjectPath{"/s/wired", "/s/vpn"}
	c.connections = map[dbus.ObjectPath]map[string]dbus.Variant{
		"/s/wired": {"id": v("wired"), "type": v("802-3-ethernet")},
		"/s/vpn":   {"id": v("invented"), "type": v("vpn")},
	}
	c.objects = map[dbus.ObjectPath]map[string]dbus.Variant{
		nmPath:   {"ActiveConnections": v([]dbus.ObjectPath{"/a/vpn"})},
		"/a/vpn": {"Vpn": v(true), "Id": v("invented"), "State": v(state)},
	}
	return c
}

func TestAConnectActivatesThatConnectionAndWaitsForIt(t *testing.T) {
	bus := nmControlFixture(2)
	var args []any
	bus.answer[nmName+".ActivateConnection"] = func(a, out []any) error {
		args = a
		*out[0].(*dbus.ObjectPath) = "/a/vpn"
		return nil
	}

	require.NoError(t, newNMVPNs(bus).connect(context.Background(), "invented", nil))

	assert.Equal(t, []any{dbus.ObjectPath("/s/vpn"), dbus.ObjectPath("/"), dbus.ObjectPath("/")}, args,
		"the connection, leaving the device and the specific object to NetworkManager")
}

func TestAConnectThatWentDownOnTheWayUpSaysSo(t *testing.T) {
	bus := nmControlFixture(4)
	bus.answer[nmName+".ActivateConnection"] = func(_, out []any) error {
		*out[0].(*dbus.ObjectPath) = "/a/vpn"
		return nil
	}

	err := newNMVPNs(bus).connect(context.Background(), "invented", nil)

	assert.ErrorIs(t, err, ErrVPNFailed)
}

// A press polkit refuses leaves the connection as it was, and says why.
func TestAPressPolkitRefusesIsReported(t *testing.T) {
	bus := nmControlFixture(2)
	refuse := func(_, _ []any) error { return denied }
	bus.answer[nmName+".ActivateConnection"] = refuse
	bus.answer[nmName+".DeactivateConnection"] = refuse
	nm := newNMVPNs(bus)

	require.ErrorIs(t, nm.connect(context.Background(), "invented", nil), ErrVPNDenied)
	require.ErrorIs(t, nm.disconnect(context.Background(), "invented"), ErrVPNDenied)

	assert.Equal(t, uint32(2), variant[uint32](bus.objects["/a/vpn"], "State"))
}

func TestAConnectionNobodyHasIsNotActivated(t *testing.T) {
	bus := nmControlFixture(2)

	err := newNMVPNs(bus).connect(context.Background(), "someone else's", nil)

	require.ErrorIs(t, err, ErrVPNNotConfigured)
	assert.Empty(t, bus.calls)
}

func TestADisconnectDeactivatesThatConnectionAndWaits(t *testing.T) {
	bus := nmControlFixture(2)
	bus.answer[nmName+".DeactivateConnection"] = func(a, _ []any) error {
		assert.Equal(t, []any{dbus.ObjectPath("/a/vpn")}, a)
		delete(bus.objects, "/a/vpn")
		return nil
	}

	require.NoError(t, newNMVPNs(bus).disconnect(context.Background(), "invented"))

	assert.Equal(t, []string{nmName + ".DeactivateConnection"}, bus.calls)
}

func TestDisconnectingOneThatIsDownIsNotAFailure(t *testing.T) {
	bus := nmControlFixture(2)

	require.NoError(t, newNMVPNs(bus).disconnect(context.Background(), "another"))

	assert.Empty(t, bus.calls, "and another connection is not touched")
}

func TestOnlyAnHTTPSSignInAddressIsOpened(t *testing.T) {
	for _, bad := range []string{
		"http://sign-in.invalid/", "file:///etc/passwd", "javascript:alert(1)",
		"-oProxyCommand=x", "https:///no-host", "", "not a url", "HTTP://sign-in.invalid/",
		"xdg-open https://sign-in.invalid/", "https://sign-in.invalid/\nrm",
	} {
		_, err := SignInURL(bad)
		require.ErrorIs(t, err, ErrVPNSignInAddress, "%q", bad)
		assert.NotContains(t, err.Error(), "sign-in.invalid", "the refusal does not repeat the address")
	}

	u, err := SignInURL("https://sign-in.invalid/a?b=c")
	require.NoError(t, err)
	assert.Equal(t, "sign-in.invalid", u.Host)
}

// ov3ControlFixture is a profile, and a new session of it that reads statuses.
func ov3ControlFixture(statuses ...[]any) *controlBus {
	c := &controlBus{answer: map[string]func([]any, []any) error{}}
	c.objects = map[dbus.ObjectPath]map[string]dbus.Variant{}
	c.answer[ov3Configs+".LookupConfigName"] = func(_, out []any) error {
		*out[0].(*[]dbus.ObjectPath) = []dbus.ObjectPath{"/c/profile"}
		return nil
	}
	c.answer[ov3Name+".NewTunnel"] = func(_, out []any) error {
		*out[0].(*dbus.ObjectPath) = "/s/new"
		return nil
	}
	c.statuses = statuses
	return c
}

func status(major, minor uint32, message string) []any { return []any{major, minor, message} }

func TestASignInIsOpenedOnceAndTheSessionConnects(t *testing.T) {
	const page = "https://sign-in.invalid/page"
	bus := ov3ControlFixture(status(3, 22, page), status(3, 22, page), status(2, 7, ""))
	var opened []*url.URL

	err := ov3Connect(context.Background(), bus, "profile", func(u *url.URL) { opened = append(opened, u) })

	require.NoError(t, err)
	require.Len(t, opened, 1)
	assert.Equal(t, page, opened[0].String())
	assert.Equal(t, []string{
		ov3Configs + ".LookupConfigName", ov3Name + ".LookupConfigName", ov3Name + ".NewTunnel",
		ov3Name + ".Ready", ov3Name + ".Connect",
	}, bus.calls)
}

func TestAnOldSessionIsClosedBeforeANewOneStarts(t *testing.T) {
	bus := ov3ControlFixture(status(2, 7, ""))
	bus.answer[ov3Name+".LookupConfigName"] = func(_, out []any) error {
		*out[0].(*[]dbus.ObjectPath) = []dbus.ObjectPath{"/s/old"}
		return nil
	}

	require.NoError(t, ov3Connect(context.Background(), bus, "profile", nil))

	assert.Less(t, indexOf(bus.calls, ov3Name+".Disconnect"), indexOf(bus.calls, ov3Name+".NewTunnel"))
}

func indexOf(calls []string, method string) int {
	for i, c := range calls {
		if c == method {
			return i
		}
	}
	return -1
}

func TestASessionThatDoesNotConnectIsClosed(t *testing.T) {
	const page = "http://sign-in.invalid/page"
	for name, tc := range map[string]struct {
		statuses [][]any
		want     error
	}{
		"plain http sign-in":    {[][]any{status(3, 22, page)}, ErrVPNSignInAddress},
		"asks for a password":   {[][]any{status(3, 20, "")}, ErrVPNNeedsCredentials},
		"authentication failed": {[][]any{status(2, 11, "")}, ErrVPNFailed},
	} {
		t.Run(name, func(t *testing.T) {
			bus := ov3ControlFixture(tc.statuses...)
			opened := false

			err := ov3Connect(context.Background(), bus, "profile", func(*url.URL) { opened = true })

			require.ErrorIs(t, err, tc.want)
			assert.False(t, opened)
			assert.Equal(t, ov3Name+".Disconnect", bus.calls[len(bus.calls)-1], "nothing is left waiting")
			assert.NotContains(t, err.Error(), "sign-in.invalid")
		})
	}
}

func TestReadyAskingForInputIsNotWaitedOut(t *testing.T) {
	bus := ov3ControlFixture()
	bus.answer[ov3Name+".Ready"] = func(_, _ []any) error {
		return dbus.Error{Name: ov3ReadyError, Body: []any{"Missing user credentials"}}
	}

	err := ov3Connect(context.Background(), bus, "profile", nil)

	require.ErrorIs(t, err, ErrVPNNeedsCredentials)
	assert.NotContains(t, bus.calls, ov3Name+".Connect")
}

func TestABackendStillStartingIsAskedAgain(t *testing.T) {
	bus := ov3ControlFixture(status(2, 7, ""))
	asked := 0
	bus.answer[ov3Name+".Ready"] = func(_, _ []any) error {
		asked++
		if asked == 1 {
			return dbus.Error{Name: "net.openvpn.v3.sessions.error", Body: []any{"Backend VPN process is not ready"}}
		}
		return nil
	}

	require.NoError(t, ov3Connect(context.Background(), bus, "profile", nil))

	assert.Equal(t, 2, asked)
}

// A sign-in nobody finishes ends at the deadline, and the session goes with it.
func TestAnAbandonedSignInClosesItsSession(t *testing.T) {
	bus := ov3ControlFixture(status(3, 22, "https://sign-in.invalid/page"))
	ctx, cancel := context.WithTimeout(context.Background(), 2*vpnWaitStep)
	defer cancel()

	err := ov3Connect(ctx, bus, "profile", nil)

	require.ErrorIs(t, err, context.DeadlineExceeded)
	assert.Equal(t, ov3Name+".Disconnect", bus.calls[len(bus.calls)-1])
}

func TestDisconnectingAProfileClosesEverySession(t *testing.T) {
	bus := ov3ControlFixture()
	bus.answer[ov3Name+".LookupConfigName"] = func(_, out []any) error {
		*out[0].(*[]dbus.ObjectPath) = []dbus.ObjectPath{"/s/one", "/s/two"}
		return nil
	}

	require.NoError(t, ov3Disconnect(context.Background(), bus, "profile"))

	assert.Equal(t, []string{ov3Name + ".LookupConfigName", ov3Name + ".Disconnect", ov3Name + ".Disconnect"}, bus.calls)
}

// A real connection, named by the person running this because it takes a tunnel
// down: go test -run TestARealVPN with HAYAMI_TEST_VPN set. It must start down.
func TestARealVPNIsConnectedAndDisconnected(t *testing.T) {
	name := os.Getenv("HAYAMI_TEST_VPN")
	if name == "" {
		t.Skip("HAYAMI_TEST_VPN names no connection to take up and down")
	}
	nm := newNMVPNs(systemBus{})
	ctx, cancel := context.WithTimeout(context.Background(), vpnControlTimeout)
	defer cancel()
	state := func() VPNState {
		vpns, err := nm.active(ctx)
		require.NoError(t, err)
		for _, v := range vpns {
			if v.Name == name {
				return v.State
			}
		}
		return VPNDown
	}
	require.Equal(t, VPNDown, state(), "this test starts from a tunnel that is down")
	t.Cleanup(func() { _ = nm.disconnect(context.Background(), name) })

	started := time.Now()
	require.NoError(t, nm.connect(ctx, name, nil))
	t.Logf("connected in %s", time.Since(started).Round(time.Millisecond))
	assert.Equal(t, VPNUp, state())

	// Up is the tunnel existing and carrying an address, and not only a word.
	vpns, err := nm.active(ctx)
	require.NoError(t, err)
	var tunnel string
	for _, v := range vpns {
		if v.Name == name {
			tunnel = v.Interface
		}
	}
	require.NotEmpty(t, tunnel, "an up VPN has its interface")
	iface, err := net.InterfaceByName(tunnel)
	require.NoError(t, err)
	addrs, err := iface.Addrs()
	require.NoError(t, err)
	assert.NotEmpty(t, addrs)

	started = time.Now()
	require.NoError(t, nm.disconnect(ctx, name))
	t.Logf("disconnected in %s", time.Since(started).Round(time.Millisecond))
	assert.Equal(t, VPNDown, state())
	// NetworkManager says deactivated a moment before the VPN's process has left.
	assert.Eventually(t, func() bool {
		_, err := net.InterfaceByName(tunnel)
		return err != nil
	}, 10*time.Second, 50*time.Millisecond, "the tunnel is gone")

	// A bounce, through the section and the real table, as the drawer's press goes.
	section := NewVPNSection([]string{name}, linuxVPNs())
	require.NoError(t, section.Connect(ctx, name, nil))
	require.Equal(t, VPNUp, state())
	require.NoError(t, section.Bounce(ctx, name, nil))
	assert.Equal(t, VPNUp, state(), "and it is back")
	require.NoError(t, section.Disconnect(ctx, name))
	assert.Equal(t, VPNDown, state())
}
