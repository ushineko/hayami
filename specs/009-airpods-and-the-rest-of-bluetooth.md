# Spec 009: AirPods and the rest of Bluetooth

**Issue**: [#23](https://github.com/ushineko/hayami/issues/23)

## Status: INCOMPLETE

## Executive Summary

Populated before the PR is opened.

## Context

Spec 008 read Logitech and SteelSeries and said the rest would follow. This is
the rest, and it is two different problems wearing one label.

**Everything except AirPods is easy.** A Keychron keyboard, and anything else
that reports a battery over Bluetooth, appears in `upower` or carries
`org.bluez.Battery1`, and either is one percentage per device with no protocol
to implement. The reference reaches for `upower -i` and parses its output;
BlueZ's own D-Bus interface is the better source, because it is an API rather
than a human-readable report and it carries the device's name and connection
state beside the level.

**AirPods are not there at all.** There is no `org.bluez.Battery1` on the
device and `bluetoothctl info` shows no battery line, because the code that
would populate it — BlueZ's battery provider, reading Apple's HFP
`AT+IPHONEACCEV` — is behind the `Experimental` setting that is off by
default. A panel that needed a line added to `/etc/bluetooth/main.conf` before
it could draw would be a panel that is blank on every machine nobody has
prepared, which is the same argument spec 006 made against needing a daemon.

So AirPods are read the way the reference reads them and the way LibrePods
does: Apple's accessory protocol, on an L2CAP channel at PSM `0x1001`.
Handshake, set features, request notifications, and the device starts
reporting. It is confirmed working from Go against the AirPods on this
machine, per ear and for the case.

Two findings from getting there, both of which cost real time and neither of
which is visible from the code afterwards:

- **`unix.SockaddrL2.Addr` takes the address in the order it is written.**
  x/sys reverses it on the way to the kernel. Reversing it first — which is
  what the raw `sockaddr_l2` struct wants, and what every C example does —
  dials an address nothing answers on, and the kernel's answer to that is
  `ECONNREFUSED`. That reads like the device refusing the channel, which sent
  the first investigation after contention, audio profiles and BlueZ settings,
  none of which had anything to do with it.
- **An L2CAP connect completes asynchronously** and reports `EINPROGRESS`. The
  real result is in `SO_ERROR` once the socket is writable, and code that
  treats `EINPROGRESS` as the failure reports a working device as broken.

The protocol's own shape matters for what is drawn. A battery packet carries
one record per cell — left, right, case, or a single "headset" for a device
that is one piece — each with a level and a status, and a cell that is not
present says so with status `0x04` rather than by being absent. The case on
its own is not the reading somebody wants when they glance at a panel; the
ears are, and the lower of the two is the one that will run out first. So the
row carries that, and the cells go on a quiet line beneath it. One row per
device is what the section already does, and three rows for one pair of
earbuds in a panel 260 px wide is not.

## Requirements

- R1 `internal/peripherals` reads AirPods battery over AAP on an L2CAP
  channel, with no dependency on BlueZ's `Experimental` setting.
- R2 The device to talk to is found over BlueZ's D-Bus: connected, bonded, and
  an Apple audio device. A machine with none is a machine with no AirPods row,
  not an error.
- R3 A cell reporting "not present" is not drawn and is not read as zero. A
  case left behind is absent, not flat.
- R4 The row carries the lower of the two ears; the cells go on a detail line
  beneath it. A device reporting a single cell is one number and no detail
  line.
- R5 Bluetooth devices that do report a battery are read from
  `org.bluez.Battery1` over D-Bus, giving one row each with their own name.
- R6 A device carrying both — AAP and a BlueZ battery — is drawn once, from
  AAP, because that is the reading with the cells in it.
- R7 The AAP exchange is bounded. A device that connects and never reports is
  a row that is not drawn, not a poll that hangs.
- R8 Neither source is a section that fails: no BlueZ, no D-Bus, a refused
  channel or a device that has gone are all rows that are absent or stale, as
  spec 008 established.

## Acceptance Criteria

- [ ] AC1 A recorded AAP battery packet decodes to the known levels and statuses for left, right and case. (R1)
- [ ] AC2 A packet whose case reports status "not present" yields no case reading, and not a zero. (R3)
- [ ] AC3 A packet reporting a single cell yields one level and no per-cell detail. (R4)
- [ ] AC4 A truncated or malformed packet is an error rather than a partial reading. (R1)
- [ ] AC5 Against a fake D-Bus object tree, an Apple audio device that is connected and bonded is selected, and a disconnected one and a non-Apple one are not. (R2)
- [ ] AC6 A device carrying a BlueZ battery and reachable over AAP appears once, with the AAP reading. (R6)
- [ ] AC7 A BlueZ battery device yields a row with the device's own name and level; a device with no `Battery1` yields no row. (R5)
- [ ] AC8 The AAP exchange gives up at its deadline rather than waiting on a device that never reports. (R7)
- [ ] AC9 **Against the real hardware on this machine**, the AirPods' levels match what the reference monitor reports, and the pane is photographed with `tools/shot-tui.sh`. Skipped where no AirPods are connected. (R1, R4)
- [ ] AC10 **Against real hardware**, a Bluetooth keyboard reporting a battery is drawn with its level. Skipped where none is connected. (R5)

## Risks & Assumptions

- **No addresses, no device names, no fixtures from this machine.** The
  repository is public. The AirPods' address and the name their owner gave
  them do not appear in code, tests or the spec; recorded packets are
  hand-checked and carry no identifier. The live tests read whatever is
  connected at the time and skip when nothing is.
- **AAP is an undocumented protocol.** The constants come from LibrePods and
  from the program this succeeds, and they are what one firmware answered to
  on one pair of AirPods Pro. A firmware that changes them shows up as a
  device that connects and never reports, which R7 already treats as a row
  that is not drawn rather than as a failure.
- **One AAP channel at a time.** Whether a second reader is refused while the
  reference monitor holds the channel is not established — the two ran
  together during this investigation without trouble, but that is one
  observation and not a guarantee. If it turns out to be a constraint, a
  refused channel is already a row that is not drawn.
- **`upower` is not used.** BlueZ's D-Bus carries the same numbers with the
  device's name and state beside them, and the reference's `upower -i` parsing
  exists because it was easier in a shell, not because it is better.
- Rollback: revert. The rows are additions to a section that already draws.

## Alternatives Considered

- BlueZ's `Experimental` battery provider, which would give AirPods a
  `org.bluez.Battery1` through Apple's HFP extension and remove the need for
  AAP entirely. Rejected: it is a line in a system configuration file and a
  daemon restart, and a panel that draws nothing until somebody has done that
  is a panel that looks broken on a new machine. It also gives one percentage
  where AAP gives three.
- Parsing `upower -i`, as the reference does. Rejected for R5: it is a
  human-readable report being read by a program, and BlueZ's own interface is
  right there.
- Reading AirPods from their BLE advertisements, which the reference keeps as
  a fallback. Not implemented: it yields data only when the device happens to
  be broadcasting battery, which is why the reference stopped relying on it.
