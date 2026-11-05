package core

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"strings"
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
	callStarting(ctx context.Context, dest string, path dbus.ObjectPath, method string, args []any, out ...any) error
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

func (systemBus) callStarting(ctx context.Context, dest string, path dbus.ObjectPath, method string, args []any, out ...any) error {
	conn, err := dbus.SystemBus()
	if err != nil {
		return fmt.Errorf("connecting to the system bus: %w", err)
	}
	err = conn.Object(dest, path).CallWithContext(ctx, method, 0, args...).Store(out...)
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
	var out []string
	err = n.eachVPN(ctx, func(id string, _ dbus.ObjectPath) bool {
		out = append(out, id)
		return true
	})
	return out, err
}

// eachVPN calls f with each connection of type `vpn`, until f returns false.
func (n *nmVPNs) eachVPN(ctx context.Context, f func(id string, path dbus.ObjectPath) bool) error {
	var paths []dbus.ObjectPath
	if err := n.bus.call(ctx, nmName, nmSetPath, nmSetIface+".ListConnections", &paths); err != nil {
		return err
	}
	for _, path := range paths {
		var settings map[string]map[string]dbus.Variant
		if err := n.bus.call(ctx, nmName, path, nmSettings+".GetSettings", &settings); err != nil {
			continue
		}
		c := settings["connection"]
		id := variant[string](c, "id")
		if variant[string](c, "type") == "vpn" && id != "" && !f(id, path) {
			return nil
		}
	}
	return nil
}

// vpnWaitStep is how often a press looks to see what came of it.
const vpnWaitStep = 250 * time.Millisecond

// vpnWait asks until done says so or the context ends.
func vpnWait(ctx context.Context, done func() (bool, error)) error {
	t := time.NewTicker(vpnWaitStep)
	defer t.Stop()
	for {
		ok, err := done()
		if ok || err != nil {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err() //nolint:wrapcheck // the section says what a deadline means
		case <-t.C:
		}
	}
}

// busRefusal is a refusal from the bus in the words a person reads. polkit's answer
// comes back as NetworkManager's PermissionDenied, and OpenVPN3's in its message.
func busRefusal(err error) error {
	var e dbus.Error
	if errors.As(err, &e) && strings.Contains(strings.ToLower(e.Name+" "+e.Error()), "denied") {
		return ErrVPNDenied
	}
	return err
}

// nmStateActivated and nmStateDeactivated are ActiveConnectionState values.
const (
	nmStateActivated   = 2
	nmStateDeactivated = 4
)

// connect activates a connection by its settings path, leaving authorisation to
// polkit and secrets to the desktop's agent, and waits to see it up (spec 085 R18).
func (n *nmVPNs) connect(ctx context.Context, name string, _ SignIn) error {
	var conn dbus.ObjectPath
	err := n.eachVPN(ctx, func(id string, path dbus.ObjectPath) bool {
		if id == name {
			conn = path
		}
		return conn == ""
	})
	if err != nil {
		return busRefusal(err)
	}
	if conn == "" {
		return ErrVPNNotConfigured
	}
	var active dbus.ObjectPath
	none := dbus.ObjectPath("/")
	err = n.bus.callArgs(ctx, nmName, nmPath, nmName+".ActivateConnection", []any{conn, none, none}, &active)
	if err != nil {
		return busRefusal(err)
	}
	return vpnWait(ctx, func() (bool, error) {
		p, err := n.bus.props(ctx, nmName, active, nmActive)
		if err != nil {
			return false, ErrVPNFailed // gone: it went down on the way up
		}
		switch state := variant[uint32](p, "State"); {
		case state == nmStateActivated:
			return true, nil
		case state > nmStateActivated:
			return false, ErrVPNFailed
		}
		return false, nil
	})
}

// disconnect deactivates a connection that is up, and waits to see it down.
func (n *nmVPNs) disconnect(ctx context.Context, name string) error {
	manager, err := n.bus.props(ctx, nmName, nmPath, nmName)
	if err != nil {
		return err
	}
	for _, path := range variant[[]dbus.ObjectPath](manager, "ActiveConnections") {
		p, err := n.bus.props(ctx, nmName, path, nmActive)
		if err != nil || !variant[bool](p, "Vpn") || variant[string](p, "Id") != name {
			continue
		}
		err = n.bus.callArgs(ctx, nmName, nmPath, nmName+".DeactivateConnection", []any{path})
		if err != nil {
			return busRefusal(err)
		}
		return vpnWait(ctx, func() (bool, error) {
			p, err := n.bus.props(ctx, nmName, path, nmActive)
			if err != nil {
				return true, nil //nolint:nilerr // the object going is what this waits for
			}
			return variant[uint32](p, "State") >= nmStateDeactivated, nil
		})
	}
	return nil
}

// ov3Configured lists OpenVPN3's configuration profiles. Its manager exits when idle
// and starts when called, and the first call can reach it before it has made the
// object, so that one is asked twice.
func ov3Configured(ctx context.Context, bus vpnBus) ([]string, error) {
	var paths []dbus.ObjectPath
	if err := ov3AskConfigs(ctx, bus, "FetchAvailableConfigs", nil, &paths); err != nil {
		return nil, err
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

// ov3AskConfigs asks the configuration manager, which may have to start, and is
// asked twice for the reason ov3Configured gives.
func ov3AskConfigs(ctx context.Context, bus vpnBus, method string, args []any, paths *[]dbus.ObjectPath) error {
	full := ov3Configs + "." + method
	err := bus.callStarting(ctx, ov3Configs, ov3CfgPath, full, args, paths)
	if err == nil {
		return nil
	}
	var unknown dbus.Error
	if errors.As(err, &unknown) && unknown.Name == "org.freedesktop.DBus.Error.ServiceUnknown" {
		return ErrVPNBackendAbsent
	}
	return bus.callStarting(ctx, ov3Configs, ov3CfgPath, full, args, paths)
}

// ov3Connected is the status OpenVPN3 reports for a session with a tunnel:
// major CONNECTION, minor CONN_CONNECTED.
var ov3Connected = [2]uint32{2, 7}

// The other statuses a connect acts on, as major and minor. The sign-in address is
// the message of ov3SignIn.
var (
	ov3SignIn = [2]uint32{3, 22}

	// ov3WantsInput are statuses asking for what only a person typing can give.
	ov3WantsInput = map[[2]uint32]bool{{1, 4}: true, {3, 20}: true, {3, 21}: true}

	// ov3Ended are statuses a connection does not come back from by itself:
	// disconnected, failed, and authentication failed.
	ov3Ended = map[[2]uint32]bool{{2, 9}: true, {2, 10}: true, {2, 11}: true}
)

// ov3ReadyError is what Ready answers with when a session wants a person's input, as
// against a backend process that has not started yet, which is asked again.
const ov3ReadyError = "net.openvpn.v3.error.ready"

// ov3Status is a session's major, minor and message. The message can be a sign-in
// address, so it goes nowhere but SignInURL.
func ov3Status(p map[string]dbus.Variant) (code [2]uint32, message string) {
	status := variant[[]any](p, "status")
	if len(status) < 3 {
		return code, ""
	}
	code[0], _ = status[0].(uint32)
	code[1], _ = status[1].(uint32)
	message, _ = status[2].(string)
	return code, message
}

// ov3CloseSessions ends every session of a profile, including one waiting at its
// sign-in page, as vpn-toggle does, and says how many there were.
func ov3CloseSessions(ctx context.Context, bus vpnBus, name string) (int, error) {
	var sessions []dbus.ObjectPath
	err := bus.callStarting(ctx, ov3Name, ov3Path, ov3Name+".LookupConfigName", []any{name}, &sessions)
	if err != nil {
		return 0, busRefusal(err)
	}
	for _, path := range sessions {
		if err := bus.call(ctx, ov3Name, path, ov3Name+".Disconnect"); err != nil {
			return 0, busRefusal(err)
		}
	}
	return len(sessions), nil
}

// ov3Disconnect takes a profile's sessions down.
func ov3Disconnect(ctx context.Context, bus vpnBus, name string) error {
	_, err := ov3CloseSessions(ctx, bus, name)
	return err
}

// ov3SettleTime is the pause after closing a profile's old sessions, which is
// vpn-toggle's.
const ov3SettleTime = time.Second

// ov3Connect starts a session of a profile and sees it through its sign-in page. A
// session that does not end up connected is closed, so nothing is left waiting.
func ov3Connect(ctx context.Context, bus vpnBus, name string, signIn SignIn) error {
	var configs []dbus.ObjectPath
	if err := ov3AskConfigs(ctx, bus, "LookupConfigName", []any{name}, &configs); err != nil {
		return busRefusal(err)
	}
	if len(configs) == 0 {
		return ErrVPNNotConfigured
	}
	closed, err := ov3CloseSessions(ctx, bus, name)
	if err != nil {
		return err
	}
	if closed > 0 {
		select {
		case <-ctx.Done():
			return ctx.Err() //nolint:wrapcheck // the section says what a deadline means
		case <-time.After(ov3SettleTime):
		}
	}

	var session dbus.ObjectPath
	err = bus.callStarting(ctx, ov3Name, ov3Path, ov3Name+".NewTunnel", []any{configs[0]}, &session)
	if err != nil {
		return busRefusal(err)
	}
	if err := ov3Drive(ctx, bus, session, signIn); err != nil {
		// On a context of its own: the press's may be what just ended.
		end, cancel := context.WithTimeout(context.WithoutCancel(ctx), vpnReadTimeout)
		defer cancel()
		_ = bus.call(end, ov3Name, session, ov3Name+".Disconnect")
		return err
	}
	return nil
}

// ov3Drive is a new session from its backend starting to connected.
func ov3Drive(ctx context.Context, bus vpnBus, session dbus.ObjectPath, signIn SignIn) error {
	err := vpnWait(ctx, func() (bool, error) {
		err := bus.call(ctx, ov3Name, session, ov3Name+".Ready")
		var e dbus.Error
		switch {
		case err == nil:
			return true, nil
		case errors.As(err, &e) && e.Name == ov3ReadyError:
			return false, ErrVPNNeedsCredentials
		case ctx.Err() != nil:
			return false, ctx.Err() //nolint:wrapcheck // the section says what a deadline means
		}
		return false, nil // the backend process has not started
	})
	if err != nil {
		return err
	}
	if err := bus.call(ctx, ov3Name, session, ov3Name+".Connect"); err != nil {
		return busRefusal(err)
	}

	opened := false
	return vpnWait(ctx, func() (bool, error) {
		p, err := bus.props(ctx, ov3Name, session, ov3Name)
		if err != nil {
			return false, ErrVPNFailed // gone: something closed it
		}
		code, message := ov3Status(p)
		switch {
		case code == ov3Connected:
			return true, nil
		case ov3WantsInput[code]:
			return false, ErrVPNNeedsCredentials
		case ov3Ended[code]:
			return false, ErrVPNFailed
		case code == ov3SignIn && !opened:
			u, err := SignInURL(message)
			if err != nil {
				return false, err
			}
			opened = true
			if signIn != nil {
				signIn(u)
			}
		}
		return false, nil
	})
}

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
		{Name: "NetworkManager", Active: nm.active, Configured: nm.configured, Connect: nm.connect, Disconnect: nm.disconnect, Unattended: true},
		{
			Name:       "OpenVPN3",
			Active:     func(ctx context.Context) ([]VPN, error) { return ov3VPNs(ctx, bus) },
			Configured: func(ctx context.Context) ([]string, error) { return ov3Configured(ctx, bus) },
			Connect: func(ctx context.Context, name string, signIn SignIn) error {
				return ov3Connect(ctx, bus, name, signIn)
			},
			Disconnect: func(ctx context.Context, name string) error { return ov3Disconnect(ctx, bus, name) },
		},
	}
}
