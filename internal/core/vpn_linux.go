package core

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"time"

	"github.com/godbus/dbus/v5"
)

// The bus names and interfaces read here. Both systems answer on the system
// bus without privilege, and neither is asked for a secret (spec 085).
const (
	nmName       = "org.freedesktop.NetworkManager"
	nmPath       = "/org/freedesktop/NetworkManager"
	nmActive     = nmName + ".Connection.Active"
	nmDevice     = nmName + ".Device"
	nmSettings   = nmName + ".Settings.Connection"
	ov3Name      = "net.openvpn.v3.sessions"
	ov3Path      = "/net/openvpn/v3/sessions"
	ov3Configs   = "net.openvpn.v3.configuration"
	ov3CfgPath   = "/net/openvpn/v3/configuration"
	nmSetPath    = nmPath + "/Settings"
	nmSetIface   = nmName + ".Settings"
	dbusProps    = "org.freedesktop.DBus.Properties.GetAll"
	dbusHasOwner = "org.freedesktop.DBus.NameHasOwner"
)

// vpnBus is what the backends ask of the system bus.
type vpnBus interface {
	// hasOwner reports whether a name is running. It is asked first and every
	// call is made without auto-start, because both of these names are
	// activatable and a panel polling must not start a daemon.
	hasOwner(ctx context.Context, name string) (bool, error)
	props(ctx context.Context, dest string, path dbus.ObjectPath, iface string) (map[string]dbus.Variant, error)
	call(ctx context.Context, dest string, path dbus.ObjectPath, method string, out ...any) error
	callArgs(ctx context.Context, dest string, path dbus.ObjectPath, method string, args []any, out ...any) error

	// callStarting is a call that may start its service, for a person who has
	// asked for the list it gives.
	callStarting(ctx context.Context, dest string, path dbus.ObjectPath, method string, out ...any) error
}

// systemBus is the real one. It connects on first use, so a machine with no
// system bus reports that as the reading's failure and not at start.
type systemBus struct{}

func (systemBus) hasOwner(ctx context.Context, name string) (bool, error) {
	conn, err := dbus.SystemBus()
	if err != nil {
		return false, fmt.Errorf("connecting to the system bus: %w", err)
	}
	var has bool
	err = conn.BusObject().CallWithContext(ctx, dbusHasOwner, 0, name).Store(&has)
	if err != nil {
		return false, fmt.Errorf("asking the bus about %s: %w", name, err)
	}
	return has, nil
}

func (b systemBus) props(ctx context.Context, dest string, path dbus.ObjectPath, iface string) (map[string]dbus.Variant, error) {
	var out map[string]dbus.Variant
	if err := b.callWith(ctx, dest, path, dbusProps, []any{iface}, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (b systemBus) call(ctx context.Context, dest string, path dbus.ObjectPath, method string, out ...any) error {
	return b.callWith(ctx, dest, path, method, nil, out...)
}

func (b systemBus) callArgs(ctx context.Context, dest string, path dbus.ObjectPath, method string, args []any, out ...any) error {
	return b.callWith(ctx, dest, path, method, args, out...)
}

func (systemBus) callStarting(ctx context.Context, dest string, path dbus.ObjectPath, method string, out ...any) error {
	conn, err := dbus.SystemBus()
	if err != nil {
		return fmt.Errorf("connecting to the system bus: %w", err)
	}
	err = conn.Object(dest, path).CallWithContext(ctx, method, 0).Store(out...)
	if err != nil {
		return fmt.Errorf("calling %s: %w", method, err)
	}
	return nil
}

func (systemBus) callWith(ctx context.Context, dest string, path dbus.ObjectPath, method string, args []any, out ...any) error {
	conn, err := dbus.SystemBus()
	if err != nil {
		return fmt.Errorf("connecting to the system bus: %w", err)
	}
	err = conn.Object(dest, path).CallWithContext(ctx, method, dbus.FlagNoAutoStart, args...).Store(out...)
	if err != nil {
		return fmt.Errorf("calling %s: %w", method, err)
	}
	return nil
}

// variant is one property as the type it should be, and the zero value where
// it is missing or something else: a bus reply is another program's data.
func variant[T any](props map[string]dbus.Variant, name string) T {
	var zero T
	v, ok := props[name]
	if !ok {
		return zero
	}
	got, ok := v.Value().(T)
	if !ok {
		return zero
	}
	return got
}

/*
nmVPNs reads NetworkManager's VPNs.

Connections whose type is `vpn`, which is what vpn-toggle lists. The tunnel's
interface is not on the active connection, whose Devices names the parent: it
is the device holding the same Ip4Config. Connected time is the connection's
`timestamp`, which on the desk matched the tunnel process's start (spec 085).

An interface and a timestamp do not change while a connection is up, and
asking for them is a call per device, so each is kept by the object path that
is new on every activation.
*/
type nmVPNs struct {
	bus        vpnBus
	since      map[dbus.ObjectPath]time.Time
	interfaces map[dbus.ObjectPath]string
}

func newNMVPNs(bus vpnBus) *nmVPNs {
	return &nmVPNs{
		bus: bus, since: map[dbus.ObjectPath]time.Time{}, interfaces: map[dbus.ObjectPath]string{},
	}
}

// nmStates are NetworkManager's ActiveConnectionState values that have a
// tunnel in them or coming; deactivating is already going down.
var nmStates = map[uint32]VPNState{1: VPNConnecting, 2: VPNUp}

func (n *nmVPNs) active(ctx context.Context) ([]VPN, error) {
	has, err := n.bus.hasOwner(ctx, nmName)
	if err != nil {
		return nil, err
	}
	if !has {
		return nil, ErrVPNBackendAbsent
	}
	manager, err := n.bus.props(ctx, nmName, nmPath, nmName)
	if err != nil {
		return nil, err
	}

	var out []VPN
	seenActive := map[dbus.ObjectPath]bool{}
	seenConfig := map[dbus.ObjectPath]bool{}
	for _, path := range variant[[]dbus.ObjectPath](manager, "ActiveConnections") {
		p, err := n.bus.props(ctx, nmName, path, nmActive)
		if err != nil {
			// Gone between the list and the read, which is a connection
			// going down as it was asked about.
			continue
		}
		state, up := nmStates[variant[uint32](p, "State")]
		if !variant[bool](p, "Vpn") || !up {
			continue
		}
		config := variant[dbus.ObjectPath](p, "Ip4Config")
		seenActive[path], seenConfig[config] = true, true
		out = append(out, VPN{
			Name:      variant[string](p, "Id"),
			State:     state,
			Interface: n.iface(ctx, manager, config),
			Since:     n.started(ctx, path, variant[dbus.ObjectPath](p, "Connection")),
		})
	}
	maps.DeleteFunc(n.since, func(k dbus.ObjectPath, _ time.Time) bool { return !seenActive[k] })
	maps.DeleteFunc(n.interfaces, func(k dbus.ObjectPath, _ string) bool { return !seenConfig[k] })
	return out, nil
}

// iface is the interface of the device holding this Ip4Config. A connection
// still coming up has none yet, and is asked again.
func (n *nmVPNs) iface(ctx context.Context, manager map[string]dbus.Variant, config dbus.ObjectPath) string {
	if name, ok := n.interfaces[config]; ok {
		return name
	}
	if config == "" || config == "/" {
		return ""
	}
	for _, device := range variant[[]dbus.ObjectPath](manager, "Devices") {
		p, err := n.bus.props(ctx, nmName, device, nmDevice)
		if err != nil {
			continue
		}
		if variant[dbus.ObjectPath](p, "Ip4Config") == config {
			name := variant[string](p, "IpInterface")
			n.interfaces[config] = name
			return name
		}
	}
	return ""
}

// started is when the connection last came up, zero where it does not say.
// GetSettings returns no secrets; those take a separate call this never makes.
func (n *nmVPNs) started(ctx context.Context, active, connection dbus.ObjectPath) time.Time {
	if t, ok := n.since[active]; ok {
		return t
	}
	var settings map[string]map[string]dbus.Variant
	if err := n.bus.call(ctx, nmName, connection, nmSettings+".GetSettings", &settings); err != nil {
		return time.Time{}
	}
	stamp := variant[uint64](settings["connection"], "timestamp")
	if stamp == 0 || stamp > 1<<62 {
		return time.Time{}
	}
	t := time.Unix(int64(stamp), 0) //nolint:gosec // bounded just above
	n.since[active] = t
	return t
}

// configured lists NetworkManager's connections of type `vpn`, as vpn-toggle does.
// A connection's settings come without its secrets, which take a call this never
// makes.
func (n *nmVPNs) configured(ctx context.Context) ([]string, error) {
	has, err := n.bus.hasOwner(ctx, nmName)
	if err != nil {
		return nil, err
	}
	if !has {
		return nil, ErrVPNBackendAbsent
	}
	var paths []dbus.ObjectPath
	if err := n.bus.call(ctx, nmName, nmSetPath, nmSetIface+".ListConnections", &paths); err != nil {
		return nil, err
	}
	var out []string
	for _, path := range paths {
		var settings map[string]map[string]dbus.Variant
		if err := n.bus.call(ctx, nmName, path, nmSettings+".GetSettings", &settings); err != nil {
			continue
		}
		c := settings["connection"]
		if variant[string](c, "type") == "vpn" {
			if id := variant[string](c, "id"); id != "" {
				out = append(out, id)
			}
		}
	}
	return out, nil
}

// ov3Configured lists OpenVPN3's configuration profiles. Its manager exits when idle
// and starts when called, and the first call can reach it before it has made the
// object, so that one is asked twice.
func ov3Configured(ctx context.Context, bus vpnBus) ([]string, error) {
	var paths []dbus.ObjectPath
	method := ov3Configs + ".FetchAvailableConfigs"
	if err := bus.callStarting(ctx, ov3Configs, ov3CfgPath, method, &paths); err != nil {
		var unknown dbus.Error
		if errors.As(err, &unknown) && unknown.Name == "org.freedesktop.DBus.Error.ServiceUnknown" {
			return nil, ErrVPNBackendAbsent
		}
		if err := bus.callStarting(ctx, ov3Configs, ov3CfgPath, method, &paths); err != nil {
			return nil, err
		}
	}
	var out []string
	for _, path := range paths {
		p, err := bus.props(ctx, ov3Configs, path, ov3Configs)
		if err != nil {
			continue
		}
		if name := variant[string](p, "name"); name != "" {
			out = append(out, name)
		}
	}
	return out, nil
}

// ov3Connected is the status OpenVPN3 reports for a session with a tunnel:
// major CONNECTION, minor CONN_CONNECTED.
var ov3Connected = [2]uint32{2, 7}

// ov3VPNs reads OpenVPN3's sessions. A session that exists is a tunnel that is
// up or on its way, including one waiting for its sign-in page.
func ov3VPNs(ctx context.Context, bus vpnBus) ([]VPN, error) {
	has, err := bus.hasOwner(ctx, ov3Name)
	if err != nil {
		return nil, err
	}
	if !has {
		return nil, ErrVPNBackendAbsent
	}
	var sessions []dbus.ObjectPath
	if err := bus.call(ctx, ov3Name, ov3Path, ov3Name+".FetchAvailableSessions", &sessions); err != nil {
		return nil, err
	}
	var out []VPN
	for _, path := range sessions {
		p, err := bus.props(ctx, ov3Name, path, ov3Name)
		if err != nil {
			continue
		}
		c := VPN{
			Name: variant[string](p, "config_name"), State: VPNConnecting,
			Interface: variant[string](p, "device_name"),
		}
		if status := variant[[]any](p, "status"); len(status) >= 2 {
			major, _ := status[0].(uint32)
			minor, _ := status[1].(uint32)
			if [2]uint32{major, minor} == ov3Connected {
				c.State = VPNUp
			}
		}
		if stamp := variant[uint64](p, "session_created"); stamp > 0 && stamp < 1<<62 {
			c.Since = time.Unix(int64(stamp), 0) //nolint:gosec // bounded just above
		}
		out = append(out, c)
	}
	return out, nil
}

// linuxVPNs is this platform's table of VPN systems.
func linuxVPNs() []VPNBackend {
	bus := systemBus{}
	nm := newNMVPNs(bus)
	return []VPNBackend{
		{Name: "NetworkManager", Active: nm.active, Configured: nm.configured},
		{
			Name:       "OpenVPN3",
			Active:     func(ctx context.Context) ([]VPN, error) { return ov3VPNs(ctx, bus) },
			Configured: func(ctx context.Context) ([]string, error) { return ov3Configured(ctx, bus) },
		},
	}
}
