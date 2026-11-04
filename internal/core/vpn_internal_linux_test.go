package core

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/godbus/dbus/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeBus answers from a table of objects, and counts what it is asked.
type fakeBus struct {
	owners   map[string]bool
	objects  map[dbus.ObjectPath]map[string]dbus.Variant
	sessions []dbus.ObjectPath
	stamp    uint64
	reads    int
	settings int
}

func (f *fakeBus) hasOwner(_ context.Context, name string) (bool, error) {
	return f.owners[name], nil
}

func (f *fakeBus) props(_ context.Context, _ string, path dbus.ObjectPath, _ string) (map[string]dbus.Variant, error) {
	f.reads++
	p, ok := f.objects[path]
	if !ok {
		return nil, errors.New("no such object")
	}
	return p, nil
}

func (f *fakeBus) call(_ context.Context, _ string, _ dbus.ObjectPath, method string, out ...any) error {
	switch method {
	case nmSettings + ".GetSettings":
		f.settings++
		*out[0].(*map[string]map[string]dbus.Variant) = map[string]map[string]dbus.Variant{
			"connection": {"timestamp": dbus.MakeVariant(f.stamp)},
		}
	case ov3Name + ".FetchAvailableSessions":
		*out[0].(*[]dbus.ObjectPath) = f.sessions
	default:
		return errors.New("unexpected method " + method)
	}
	return nil
}

// nmFixture is NetworkManager with one ethernet connection, and a VPN whose
// tunnel device holds the VPN's Ip4Config while the active connection's own
// device list would name the parent.
func nmFixture(state uint32) *fakeBus {
	v := dbus.MakeVariant
	return &fakeBus{
		owners: map[string]bool{nmName: true},
		stamp:  1_700_000_000,
		objects: map[dbus.ObjectPath]map[string]dbus.Variant{
			nmPath: {
				"ActiveConnections": v([]dbus.ObjectPath{"/a/eth", "/a/vpn"}),
				"Devices":           v([]dbus.ObjectPath{"/d/eth", "/d/tun"}),
			},
			"/a/eth": {"Vpn": v(false), "Id": v("wired"), "State": v(uint32(2)), "Ip4Config": v(dbus.ObjectPath("/ip/1"))},
			"/a/vpn": {
				"Vpn": v(true), "Id": v("invented"), "State": v(state),
				"Ip4Config": v(dbus.ObjectPath("/ip/2")), "Connection": v(dbus.ObjectPath("/s/vpn")),
			},
			"/d/eth": {"Ip4Config": v(dbus.ObjectPath("/ip/1")), "IpInterface": v("eth0")},
			"/d/tun": {"Ip4Config": v(dbus.ObjectPath("/ip/2")), "IpInterface": v("tun0")},
		},
	}
}

func TestNetworkManagersVPNIsReadWithItsTunnelInterface(t *testing.T) {
	got, err := newNMVPNs(nmFixture(2)).active(context.Background())

	require.NoError(t, err)
	require.Len(t, got, 1, "an ethernet connection is not a VPN")
	assert.Equal(t, "invented", got[0].Name)
	assert.Equal(t, VPNUp, got[0].State)
	assert.Equal(t, "tun0", got[0].Interface, "the device with the VPN's Ip4Config")
	assert.Equal(t, int64(1_700_000_000), got[0].Since.Unix())
}

func TestNetworkManagersActiveStatesMapToAVPNState(t *testing.T) {
	for state, want := range map[uint32]VPNState{1: VPNConnecting, 2: VPNUp} {
		got, err := newNMVPNs(nmFixture(state)).active(context.Background())
		require.NoError(t, err)
		require.Len(t, got, 1)
		assert.Equal(t, want, got[0].State, "state %d", state)
	}
	// Going, gone and unknown are not listed, which is what reads as down.
	for _, state := range []uint32{0, 3, 4} {
		got, err := newNMVPNs(nmFixture(state)).active(context.Background())
		require.NoError(t, err)
		assert.Empty(t, got, "state %d", state)
	}
}

// Both names are activatable, so a daemon that is not running must stay that
// way: this asks whether it is, and goes no further.
func TestAnAbsentDaemonIsNotAskedAnything(t *testing.T) {
	bus := &fakeBus{owners: map[string]bool{}}

	_, err := newNMVPNs(bus).active(context.Background())
	require.ErrorIs(t, err, ErrVPNBackendAbsent)
	_, err = ov3VPNs(context.Background(), bus)
	require.ErrorIs(t, err, ErrVPNBackendAbsent)
	assert.Zero(t, bus.reads)
}

// The interface and the time do not change while a connection is up, and the
// poll comes round every few seconds.
func TestAnUpConnectionIsNotReadInFullEveryPoll(t *testing.T) {
	bus := nmFixture(2)
	n := newNMVPNs(bus)

	_, err := n.active(context.Background())
	require.NoError(t, err)
	first := bus.reads
	assert.Equal(t, 1, bus.settings)

	_, err = n.active(context.Background())
	require.NoError(t, err)
	assert.Less(t, bus.reads-first, first, "the devices were read again")
	assert.Equal(t, 1, bus.settings, "the settings were read again")
}

// A reply is another program's data: a property of the wrong type is a blank,
// not a panic.
func TestAMalformedReplyIsABlank(t *testing.T) {
	bus := nmFixture(2)
	bus.objects["/a/vpn"]["Id"] = dbus.MakeVariant(uint32(7))
	bus.objects["/a/vpn"]["Ip4Config"] = dbus.MakeVariant("not a path")

	got, err := newNMVPNs(bus).active(context.Background())

	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Empty(t, got[0].Name)
	assert.Empty(t, got[0].Interface)
}

func TestOpenVPN3SessionsAreRead(t *testing.T) {
	session := func(minor uint32) map[string]dbus.Variant {
		return map[string]dbus.Variant{
			"config_name":     dbus.MakeVariant("invented"),
			"device_name":     dbus.MakeVariant("tun1"),
			"session_created": dbus.MakeVariant(uint64(1_700_000_000)),
			"status":          dbus.MakeVariant([]any{uint32(2), minor, "message"}),
		}
	}
	bus := &fakeBus{
		owners:   map[string]bool{ov3Name: true},
		sessions: []dbus.ObjectPath{"/s/up", "/s/signin"},
		objects:  map[dbus.ObjectPath]map[string]dbus.Variant{"/s/up": session(7), "/s/signin": session(4)},
	}

	got, err := ov3VPNs(context.Background(), bus)

	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, VPNUp, got[0].State)
	assert.Equal(t, "tun1", got[0].Interface)
	assert.Equal(t, time.Unix(1_700_000_000, 0), got[0].Since)
	assert.Equal(t, VPNConnecting, got[1].State, "a session waiting for its sign-in page is not up")
}

// The real system bus: what a fake cannot say is whether these are the right
// properties. Skips where nothing is connected, which is every CI run.
func TestARealNetworkManagerVPNIsRead(t *testing.T) {
	got, err := newNMVPNs(systemBus{}).active(context.Background())
	if errors.Is(err, ErrVPNBackendAbsent) {
		t.Skip("NetworkManager is not running")
	}
	if err != nil {
		t.Skipf("no system bus: %v", err)
	}
	if len(got) == 0 {
		t.Skip("no VPN is connected")
	}

	for _, c := range got {
		assert.NotEmpty(t, c.Name)
		if c.State != VPNUp {
			continue
		}
		require.NotEmpty(t, c.Interface, "an up VPN has a tunnel interface")
		_, err := os.Stat(filepath.Join("/sys/class/net", c.Interface))
		assert.NoError(t, err, "the interface read is one the kernel has")
		assert.False(t, c.Since.IsZero(), "an up VPN says when it came up")
		assert.True(t, c.Since.Before(time.Now().Add(time.Minute)))
	}
}

// The real system bus, for OpenVPN3. Skips without a session, because starting
// one takes a person at a sign-in page.
func TestARealOpenVPN3SessionIsRead(t *testing.T) {
	got, err := ov3VPNs(context.Background(), systemBus{})
	if errors.Is(err, ErrVPNBackendAbsent) {
		t.Skip("OpenVPN3 is not running")
	}
	if err != nil {
		t.Skipf("no system bus: %v", err)
	}
	if len(got) == 0 {
		t.Skip("no OpenVPN3 session")
	}

	for _, c := range got {
		assert.NotEmpty(t, c.Name, "a session names its configuration")
		assert.False(t, c.Since.IsZero(), "a session says when it was created")
		if c.State == VPNUp {
			require.NotEmpty(t, c.Interface, "an up session has a tunnel interface")
			_, err := os.Stat(filepath.Join("/sys/class/net", c.Interface))
			assert.NoError(t, err)
		}
	}
}
