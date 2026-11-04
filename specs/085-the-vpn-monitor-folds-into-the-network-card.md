# 085 — the VPN monitor folds into the Network card

**Issue**: #298

## Status: INCOMPLETE

## Executive Summary

To be filled in before the last phase's PR.

## Context

`ag-scripts/vpn-toggle` is a PyQt6 program of about 5,000 lines with its own
window, tray icon and systemd user service. It lists NetworkManager and
OpenVPN3 connections, connects, disconnects and bounces them, runs health
checks on a timer, graphs the results and reconnects a tunnel that drops. It
is the behavioural reference for this spec until it is retired, as
`peripheral-battery-monitor` is for the rest of hayami.

The Network card already shows a tunnel's traffic (`tun0`, `tailscale0`). What
it cannot say is whether the tunnel is the one intended, whether it is
healthy, or how to bring it back.

## Spike findings

Run against this desk on 2026-11-04, read-only, with one NetworkManager VPN
connected and no OpenVPN3 session.

| Question | Finding | Consequence |
|---|---|---|
| Can VPN state be read without a subprocess? | Yes. `org.freedesktop.NetworkManager` answers on the system bus without root: `ActiveConnections`, `Connection.Active.{Vpn,Type,State,Ip4Config}`, `VPN.Connection.VpnState` (5 when connected) | `nmcli` is not needed. `godbus/dbus/v5` is already a direct dependency and works with `CGO_ENABLED=0` |
| Can a VPN be tied to its interface? | Yes. The VPN's `Ip4Config` path equals the `Ip4Config` of a device whose `IpInterface` is `tun0`. `Devices` on the active connection names the parent, not the tunnel, so vpn-toggle's `GENERAL.DEVICES` reading is the wrong field for this | The line can name the interface the Network card already reads |
| Can configured VPNs be listed? | `Settings.ListConnections` answers without root. Secrets are not requested | Discovery needs no credential |
| Is OpenVPN3 readable? | `net.openvpn.v3.sessions` answers `FetchAvailableSessions` (no sessions at the time) and emits `SessionManagerEvent` | A live session was read later in phase 1 (below) |
| Is native ping possible? | `net.ipv4.ping_group_range` is `0 2147483647` here, so unprivileged ICMP datagram sockets are allowed | Ping needs no subprocess on this desk. Other distributions may restrict it; that is a typed absence, not a fallback to `ping` |
| Can the card's plot carry latency? | No. `ScaleShared` fits every trace on the Network plot to one scale, in bytes per second | Latency is plotted in the drawer |
| Can a drawer draw a plot? | No. `view.Detail` has meters and lines, and no trails | `view.Detail` gains trails, and the drawer draws them |
| Can the library's sparkline mark an event? | `glance.Sparkline` has series and samples. No marker was found in its public functions | A failure or bounce marker is a fynedesygn gap (see Gaps found) |
| What does the desk's vpn-toggle use? | Three VPNs: one each of ping, DNS and geolocation. The newest metric on any is 2026-10-20, though a VPN is connected and the monitor setting is on. Cause not investigated | Import must carry all three types. The stale metrics are a question for vpn-toggle, not this spec |

NetworkManager's `connection.timestamp` is the connected time: on the desk it
matched the start of the tunnel's process, about 7.6 days back. The VPN properties
read are `Vpn`, `Id`, `State`, `Ip4Config` and `Connection` on the active connection,
and `Ip4Config` and `IpInterface` on the device.

A live OpenVPN3 session was read on 2026-11-04, and its reader confirmed: `config_name`,
`device_name`, `session_created` and `status`, on `net.openvpn.v3.sessions`. Status
major 2, minor 7 is connected; a session waiting at its sign-in page has no device yet
and reads as connecting. `session_created` is when the session started, so its
connected time includes the time spent at the sign-in page.

## Delivery

Phase 1 is three PRs, approved by the operator on 2026-11-04: 1a reading, 1b health,
1c setup.

## Decisions

Made by the operator on 2026-11-04.

- **Scope:** vpn-toggle's functionality, in three phases, each its own PR.
  vpn-toggle retires after the third.
- **Keepalive:** an opt-in helper at parity with vpn-toggle: it acts on a connection
  that is on but unresponsive, and re-dials one that dropped.
- **OpenVPN3:** reconnected manually, since its connections need a sign-in page and a
  person at the desk. The helper only notifies.
- **Checks:** ping and DNS natively. Geolocation was first to be an opt-in, and was
  then dropped (R16).
- **Graphs:** latency in the Network drawer. The card has a text line per VPN,
  and its plot keeps carrying rates.
- **Migration:** a one-time offer to import the VPN list and checks. Metrics
  are not imported.

## Requirements

### All phases

- R1 VPNs belong to the Network section. There is no fifth section, and the key
  stays `bandwidth`. A hidden section stops its VPN polling and checks, as any
  hidden section does.
- R2 Backends are a table on `core.Host`, one provider per system: NetworkManager
  and OpenVPN3 over D-Bus. No subprocess reads VPN state and none is
  `nmcli` or `openvpn3`.
- R3 Linux only for now. On Windows there is no VPN row, preferences group or
  drawer tab, by the host offering no backends.
- R4 Each absence is typed and reported by `doctor`: no backend, bus refused,
  raw socket refused, check not configured. A failure of the probe is never
  reported as the tunnel being down (vpn-toggle spec 010 learned this).
- R5 Everything renders in stack, grid and row, in both shells, with a parity
  fixture.

### Phase 1: reading and health

- R6 **The line.** One row on the Network card per VPN the person chose: name,
  state, latest latency and time connected, at fixed widths. A chosen VPN keeps
  its row while down, so connecting and disconnecting does not reflow the panel.
  Only changing the chosen list does.
- R7 **A fresh machine sees no change.** No VPN is chosen by default, so a card
  with none draws what it draws today. Interfaces work the same way.
- R8 **Traffic is not repeated.** The row names no rates; the tunnel's rates stay
  on its interface row if that interface is chosen.
- R9 **Addresses.** The tunnel's address, gateway and routes are shown in the drawer
  and `hayami-tui detail`, as the peer lists already show addresses (B6). They are not
  on the card, where space is limited, and so not in `readings`. `doctor` shows none,
  since it is what gets pasted into a bug report. The public exit address is neither
  looked up nor shown. A
  connection's name is shown because the person chose it. Everything passes the
  printable filter.
- R10 **Checks.** Each check is a table entry: `ping` (host), `dns` (hostname,
  expected prefix). A check has a timeout, is cancellable and runs after a grace
  period following a connect. Defaults are vpn-toggle's: every 120 s, 15 s grace.
  Names are looked up through systemd-resolved over the bus, past its cache, and
  through Go's resolver where it does not run. Skipping the cache departs from
  vpn-toggle, by the operator's decision: a cached answer passes on a dead tunnel.
- R11 **One prober.** Each cycle takes a non-blocking `flock` and then asks a bounded
  store in the cache directory whether a check is due, as the usage cache does. Any
  number of terminal panes cause one set of probes per interval. Nobody holds the lock
  between cycles, so a process that exits or hides the section stands in nobody's way.
- R12 **The store.** JSON lines, one point per check cycle: time, VPN, latency,
  success, per-check results, bounce. Capped and compacted; a truncated last line
  is ignored. It holds no address. It is a cache: deleting it costs the history.
- R13 **The drawer.** The Network drawer gains a `VPN` switch beside Procs and
  Peers: each chosen VPN as its own part, under a rule, with its state on its heading
  and its own plot of latency with failures marked, then interface, connected time,
  average latency, failures, uptime and last check results.
  It opens from the card whether or not the process list is available, and
  `hayami-tui detail` prints the same facts.
- R14 **Preferences.** A VPN group beside the interfaces on the Sections page:
  discovered connections to tick, and per-VPN checks. Settings are
  `config.Setting` fields under `sectionSettings.bandwidth`.
- R15 **Import.** When vpn-toggle's config exists and no VPN is configured,
  preferences offers once to import its VPN list and checks. The offer is
  remembered when declined. The file is parsed as untrusted and only read.
- R16 **Geolocation is not carried over,** by the operator's decision on 2026-11-04. It
  told a third party this machine's address, and ip-api.com's free tier is HTTP only.
  hayami makes no third-party call for a VPN. An import names the check it left out.

### Phase 2: manual control

- R17 Connect, disconnect and bounce are buttons in the drawer's VPN view, and in
  no menu. Disconnect and bounce need the two presses ending a process needs.
- R18 NetworkManager activation goes through `ActivateConnection` and
  `DeactivateConnection`, leaving authorisation to polkit. hayami registers no
  secret agent and never sees a VPN's secret.
- R19 OpenVPN3 keeps its browser sign-in. A sign-in URL from the session manager
  is opened only if its scheme is `https`, and is never logged or printed.
  Raising the browser, which vpn-toggle does by `kdotool`, is left out unless the
  flow proves unusable without it.
- R20 Control is only in the desktop panel. The terminal panel reads.

### Phase 3: the keepalive helper

The helper keeps a chosen tunnel working, at parity with vpn-toggle. It handles two
conditions: a connection that is "on" but unresponsive, which only a check can detect,
and a tunnel that dropped. NetworkManager's autoconnect acts at startup and, by the
operator's understanding, does not re-dial a dropped VPN, so re-dialling is the
helper's job (a phase 3 test confirms this).

- R21 **What it is.** A systemd user service running `hayami-tui vpn-keepalive`,
  with `Restart=on-failure`. It runs as the user with no added privilege, and
  NetworkManager and polkit decide what it may do. It is installed only when
  asked: the installer's page and `hayami-tui vpn-keepalive install`. Without it
  nothing acts on a VPN unasked, and phases 1 and 2 work as written.
- R22 **Its job.** It holds the prober lock (R11), so it runs the checks and
  writes the store, and panels read. It is switched on per VPN.
- R23 **vpn-toggle's rules for a failed check,** at parity with `_on_session_done`
  and tested one by one, at the same defaults:
  - only a connected tunnel is checked, after its grace period
  - a pass resets the failure count
  - a failure below the threshold (3) bounces the tunnel
  - a failure at the threshold disconnects the tunnel and disables the VPN for
    the helper, which reports why; a person re-enables it

  One addition, not in vpn-toggle: a probe that could not run at all (R4: bus or
  socket refused) counts as no result. A check that times out is a failure, since
  that is what an unresponsive tunnel looks like.
- R24 **vpn-toggle's rules for a tunnel that is down,** at parity with `_handle_down`
  (its spec 010) and tested one by one:
  - the helper re-dials a tunnel that dropped and never opens a new one, so a VPN it
    has not seen connected since it started is left alone
  - an outage never disables a VPN; the threshold applies only to R23's failures
  - a disconnect pressed in hayami suppresses re-dialling until a connect is pressed,
    and cancels any retry waiting. The press has to reach the helper; how is designed
    in this phase
  - a failed re-dial retries, backing off from 30 s, doubling, to 600 s, each retry
    visible, and never stops trying. A bounce whose reconnect failed joins this path
  - a successful re-dial resets the backoff and starts a new grace period
  - re-dialling can be switched off per VPN, overriding the helper's setting
- R25 **OpenVPN3 is reconnected by a person: a departure from parity, decided by the
  operator.** Every OpenVPN3 connection in use needs a browser sign-in, and with
  nobody at the desk it cannot complete. The helper still checks such a tunnel and
  graphs it, and on a failed check or a drop it notifies and takes no action. It
  makes no connect or disconnect call, because a bounce it could not finish would turn
  a degraded tunnel into a down one. The person reconnects from the drawer (R19).
  Whether a backend may act unattended is a field of the backend table, so a connection
  that needs no sign-in can be allowed later.
- R26 A hidden Network section does not stop the helper: this is the deliberate
  exception to a hidden section costing nothing, since the person installed the
  helper to keep the tunnel healthy. A panel that is hidden or closed only stops
  drawing.
- R27 Enabling the helper for a VPN says that vpn-toggle must not be monitoring the
  same VPN, since two monitors would bounce each other's tunnel. `doctor` warns when
  vpn-toggle's service is active alongside the helper.
- R28 The last PR documents retirement: stopping and uninstalling vpn-toggle, and
  what is left behind in its config directory.

## Acceptance Criteria

### Phase 1

- [x] A row per chosen VPN draws state, latency and connected time at fixed
      widths, in all three arrangements in both shells (parity fixture added).
- [x] With no VPN chosen, the Network card's rows and plot are unchanged.
      `TestWithNoVPNChosenTheCardIsUnchanged`.
- [x] Connecting and disconnecting a real VPN does not change the window's
      height, asserted from pixels on a real window. On the desk on 2026-11-04,
      one panel run across down, up and down: 516x834 each time. Falsified with a
      build that drops a down row: 434x788, 516x810, 434x788. A scripted run on the
      desk, since `tests/window` is Windows only and this is Linux only; it does not
      guard against a later regression.
- [x] State, name and interface are read from a real NetworkManager VPN on the
      system bus, by an integration test that skips where none is connected.
      `TestARealNetworkManagerVPNIsRead`.
- [x] The same is shown for a real OpenVPN3 session, and the properties it reads
      are recorded in this spec. `TestARealOpenVPN3SessionIsRead`.
- [x] `core` has no `exec` of `nmcli`, `openvpn3`, `ping` or `getent`, held by a
      test in `tests/structure`. `TestNoProgramIsRunToReadAVPN`.
- [x] With 3 processes running, a check is performed once per interval, and
      another process takes over when the lock's holder exits.
      `TestOneSetOfProbesHoweverManyPanels`, with real processes and a real `flock`.
- [x] A store with a truncated last line loads; the store never exceeds its cap.
      `TestAStoreWithATruncatedLastLineLoads`, `TestTheStoreDoesNotPassItsCap`.
- [x] A failed probe (bus refused, socket refused) is reported as that, and the
      row does not read as down. `TestAFailedProbeIsNotATunnelDown`,
      `TestACheckThatCouldNotRunIsNotAFailure`.
- [x] The drawer's VPN view plots latency with failures marked, screenshotted
      from a real window, with the test falsified first. On the desk on 2026-11-04, seen
      by the operator and in a capture: three failures as red spikes over the latency.
      The earlier build was the falsification: its capture showed every row cut off at
      the drawer's edge, which the fix removed.
- [x] The drawer and `detail` show the real tunnel's address, gateway and routes;
      the card, `readings` and `doctor` contain no address. `TestReadingsCarryNoVPNAddress`;
      `doctor` read on the desk.
- [x] Import reads a fixture of vpn-toggle's config, leaves geolocation out and
      says so, and is offered once. `TestAGeolocationCheckIsLeftOutAndNamed`,
      `TestTheImportIsOfferedOnce`.
- [x] No third-party connection is made: no VPN code dials anything but the targets in
      the settings, with no geolocation code present.
- [x] A hidden Network section performs no check and no bus read. A hidden section's
      source is not polled (spec 052), and every check and read starts from `Poll`.

### Phase 2

- [ ] A real NetworkManager VPN is connected, disconnected and bounced from the
      drawer, and the card follows.
- [ ] A real OpenVPN3 session completes browser sign-in from the drawer.
- [ ] A sign-in URL with any scheme other than `https` is refused, by test.
- [ ] A press denied by polkit is reported, and the state stays as it was.
- [ ] The terminal panel has no control path, by test.

### Phase 3

- [ ] Each rule in R23 has a test, against a fake backend and a fake clock.
- [ ] A real tunnel made to fail its check is bounced and passes again.
- [ ] A check failing through the threshold disconnects and disables, and the
      helper does nothing further until re-enabled.
- [ ] Each rule in R24 has a test, against a fake backend and a fake clock.
- [ ] A real tunnel dropped without a request to NetworkManager (its VPN process
      killed) is re-dialled by the helper, and NetworkManager's own behaviour without
      the helper is recorded in this spec.
- [ ] A disconnect pressed in hayami stays down, and cancels a retry waiting.
- [ ] A VPN down when the helper starts is not connected by it.
- [ ] On a real autoconnecting VPN, a bounce does not fight autoconnect, and what
      autoconnect does after the threshold's disconnect is recorded in this spec.
- [ ] On an OpenVPN3 tunnel, a failed check and a drop each send a notification, and
      the helper makes no connect or disconnect call, by test against a fake backend.
- [ ] Killing the helper restarts it, and closing the panel leaves it running.
- [ ] Without the helper installed, nothing acts on a VPN unasked.
- [ ] `doctor` warns when vpn-toggle is active alongside the helper.
- [ ] Uninstalling removes the unit and stops the helper.
- [ ] The README covers VPNs, the helper and the retirement of vpn-toggle.

## Risks & Assumptions

- **A disconnect outside hayami is re-dialled.** At parity: vpn-toggle knows only of
  presses in its own window, so `nmcli connection down` on a tunnel it has seen looks
  like a drop. NetworkManager's state-change reason may tell the two apart; that would
  be a later improvement, not parity.
- **A helper restart forgets which tunnels it has seen.** At parity: a tunnel that
  drops while the helper is down is not re-dialled once it returns.
- **The threshold takes a tunnel down.** At three failures the tunnel is disconnected
  and disabled, at parity with vpn-toggle, so traffic then leaves outside the tunnel.
  Stated where the helper is switched on.
- **Assumed:** NetworkManager does not re-dial a dropped VPN, on the operator's
  understanding, not yet tested here. If it does on some connections, a re-dial may
  race it.
- **Assumed:** the helper and autoconnect do not fight. Autoconnect may undo the
  disconnect at the threshold. Phase 3 tests this on a real autoconnecting VPN, and
  what happens is recorded here.
- **Assumed:** a user service can activate a connection through polkit and reach the
  desktop's secret agent while the person is logged in. vpn-toggle's unit runs this
  way, but this spec has not tested a bounce from a user service.
- **DNS on a machine without systemd-resolved.** Go's resolver reads `/etc/resolv.conf`
  and skips nsswitch, so a tunnel with its own DNS may not be asked. On the desk,
  where `/etc/resolv.conf` lists the LAN's servers, Go's resolver and `getent` agreed on
  the one tunnel compared, and resolved over the bus gives the same answer as `getent`.
- **Assumed:** OpenVPN3 session properties suffice for name, state and interface.
  Untested until a session is live.
- **Two monitors during phases 1 and 2.** vpn-toggle's probes and hayami's run side
  by side, which is harmless.
- **Rollback.** Each phase is one revertible PR. No setting is required, an unknown
  setting is ignored by an older build, and the store is a cache. vpn-toggle stays
  installed until after phase 3 has run for a while, so reverting leaves a working
  monitor.
- **Query budget.** D-Bus reads are per poll and only while the section is shown.
  Signals may replace polling if a poll proves costly, as a later change.

## Threat model delta

Update `docs/threat-model.md` in the PR of the phase named.

- **B4, what hayami parses (phase 1).** New inputs: properties from the system bus,
  and vpn-toggle's config file at import. A connection's name is free text chosen by
  whoever created it, so it passes the printable filter (`view`) before any shell.
- **B5, what hayami does outside itself (phases 1 to 3).** hayami now sends ICMP and
  DNS on a timer, and in later phases changes the state of the network. New attacker
  path: a config that aims a check at a chosen host only makes the person's machine
  probe it. A forged check result could make the helper bounce a tunnel, or disable it
  at the threshold, so the checks' answers are an input to an action taken unasked,
  bounded by R21 to R24. A re-dial acts only on a tunnel already seen connected. A bus name is trusted only because the system bus owns it. "hayami runs one program to
  read" stays true.
- **B6, what hayami shows and prints (phase 1).** The tunnel's address, gateway and
  routes are shown in the drawer and `detail`, accepted as peers' addresses are; the
  card, `readings` and `doctor` omit them (R9). Connection
  names can identify an employer or place, so the screenshot rules apply to them and
  the gallery must not show them unreviewed.
- **B3, credentials.** No boundary touched: no secret agent, no token. An OpenVPN3
  sign-in URL is treated as sensitive, and is not logged (R19).
- **B2, installing (phase 3).** A new user unit, installed on request and removed
  by uninstall. Phase 3's PR therefore gets the document's adversarial review.
- **B1, B7.** No boundary touched: the helper gains no privilege, and there is no new
  module.

## E2E Test Plan

Environment: this desk, KDE/Wayland, with a real NetworkManager VPN and an OpenVPN3
profile. Installed build through the fynstall CLI installer.

| Step | Expected | AC |
|---|---|---|
| Choose a connected VPN in preferences | Row appears with state, latency and time; window grows by one row | Phase 1 rows |
| Disconnect and reconnect it with `nmcli` | Row changes state; window height does not change | Phase 1 reflow |
| Open 3 terminal panes, watch the store | One point per interval; quit the holder and points continue | Phase 1 prober |
| Open Network drawer, VPN | Latency plot, failures marked, statistics, address, gateway and routes | Phase 1 drawer |
| Press connect, disconnect, bounce | The connection follows; the card follows | Phase 2 NM |
| Start an OpenVPN3 profile from the drawer | Browser sign-in completes; row reads connected | Phase 2 OV3 |
| Install the helper, make a check fail | Tunnel bounces, then passes | Phase 3 bounce |
| Keep the check failing | Disconnected and disabled at three, with the reason shown | Phase 3 threshold |
| Kill the tunnel's VPN process, without the helper, then with it | Without: what NetworkManager does is recorded. With: re-dialled | Phase 3 drop |
| Disconnect from hayami, helper on | Stays down | Phase 3 disconnect |
| Quit the panel; kill the helper | Helper keeps running; restarts after the kill | Phase 3 lifetime |

### Prod visibility

Not applicable: a desktop program with no service or telemetry. `hayami-tui doctor`
and `readings` are how an installation is observed.

## Gaps found

For a fynedesygn change, not `internal/gui`:

- A scrolling drawer is as wide as its widest row. One value wider than the room
  widens the content, and every row's value goes past the edge, cut off. Worked around
  by `clipMiddle` in `view`; a drawer that held its rows to its own width would be the
  fix. Seen on a real window.
- `glance.Sparkline` is not a `Piece`, so the drawer's plot goes through a small adapter
  in `internal/gui` (`plotPiece`).
- `glance.Sparkline` has no way to mark an event on a trace. Not needed: a failure is a
  spike on a second trace in the verdict colour, which reads clearly on a real window.

## Alternatives Considered

- Keep vpn-toggle as the controller and only display its files in hayami; rejected
  because the operator chose full replacement, and its files go away with it.
- A fifth `vpn` section; rejected because the request places VPNs on the Network card.
- Subprocess `nmcli`, as vpn-toggle does; rejected because the system bus gives the
  same facts and a subprocess per poll per pane is what `core` avoids.
- Each process running its own checks; rejected because of the number of panes a
  desk runs (R11).
- Recovery inside the desktop panel; rejected because it would stop when the panel
  crashes, quits or hides the section.
- A root helper; rejected because bouncing a tunnel needs no privilege hayami lacks.
