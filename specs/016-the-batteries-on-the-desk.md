# 016 — the batteries on the desk

**Issue**: #56

## Context

Spec 015 made the peripherals section say why it is empty. On `cachyos` it says
three true things — no Logitech receiver, no `headsetcontrol`, no Bluetooth
adapter — and the desk has two devices with batteries in them the whole time:
a Razer mouse on a Mouse Dock Pro, and a SteelSeries Apex Pro TKL Wireless
Gen 3.

Both protocols were measured on that machine. Neither needs a daemon, a driver
or a Windows capture.

### The Razer mouse answers through its dock

The mouse never enumerates as its own USB device. The dock is the receiver:
its `input0` carries `mouse0`, so the mouse's HID traffic arrives through the
dock and nothing but the dock appears in `lsusb`. That is why OpenRazer sees
only "Razer Mouse Dock Pro, accessory" and offers no `razer.device.power` —
`razeraccessory_driver.c` in OpenRazer 3.12.4 has no battery code at all, and
`charge_level` lives in `razermouse_driver.c`, which nothing here binds.

Razer's own report protocol reaches the mouse through the dock's RF relay at
**transaction id `0x1f`**. Measured, with the mouse awake:

```
class 0x04 cmd 0x85  dpi            800 / 800
class 0x07 cmd 0x83  idle time      300 s
class 0x07 cmd 0x81  low threshold  25 %
class 0x07 cmd 0x80  battery        100 %
class 0x07 cmd 0x84  charging       0
class 0x00 cmd 0x82  serial         632441H26203920
```

The serial is not the dock's `PM2448U28101914`, the DPI is a real DPI and the
threshold is Razer's own default: that is the mouse answering for itself.
`0x1f` is what OpenRazer's unmerged PR #2817 uses for the same relay.

### The SteelSeries battery is rivalcfg's, and generalises

There is no published protocol for this keyboard, and the search that concluded
so (HeyOkay/HaloBattery#52) checked HeadsetControl, the kernel, OpenRGB, Solaar,
rivalcfg and SDL. It was looking in the wrong place: rivalcfg has the command in
every one of its *mouse* profiles, and it is the same command.

```python
"battery_level": {
    "command": [0x92],
    "response_length": 2,
    "is_charging": lambda data: bool(data[1] & 0x80),
    "level":       lambda data: ((data[1] & ~0x80) - 1) * 5,
}
```

Its wireless variants patch that through `_patch_command`, which ORs
`_WIRELESS_FLAG = 0x40` into the command byte: `0x92` for a device on its own
cable, `0xd2` for the same device behind a dongle. Confirmed on the keyboard:

```
0x92 -> 92 95     0x95: charging bit set, ((0x95 & 0x7f) - 1) * 5 = 100 %
0xd2 -> d2 95     the same
```

The transport is the one `SilasDaSilva/apex-pro-tkl-gen3-linux` reverse
engineered for the wired Gen 3, and the wireless dongle uses it unchanged:
usage page `0xFFC0`, 64-byte output and input reports, no report id, write
`[0x00, cmd]` and read the reply off the same node with the command echoed in
byte 0.

## Requirements

### Two readers, in the shape the existing ones have

`peripherals.Razer` and `peripherals.SteelSeries`, each with a `Batteries()
([]Battery, error)`, joining `Logitech`, `Headsets` and `Bluetooth` in
`panel.Peripherals`. Both take an injected opener so the suite touches no
device, the way `Logitech` does.

### Devices are found by vendor and usage page, never by product

The SteelSeries PID **moves with the connection**: `1038:1644` with the
keyboard on 2.4 GHz and `1038:1646` with it on its cable, on the same USB port,
with the hidraw nodes landing on the same numbers both times. A reader that
matched the product would read the wrong device and say nothing about it — this
already invalidated one round of measurements.

So a node is chosen by its vendor in `HID_ID` and by walking its report
descriptor for the usage page, which is what `speaksHIDPP` already does for
Logitech. `hidraw.go` grows one generic finder and the three readers say what
they want.

### Razer speaks feature reports, not writes

Razer's transport is `HIDIOCSFEATURE`/`HIDIOCGFEATURE` on a 90-byte report —
`status, transaction_id, remaining(2), protocol_type, data_size, class, id,
arguments[80], crc, reserved`, with the CRC an XOR over bytes 2..87. A hidraw
`write` sends an *output* report, which these devices do not have, so the
existing `endpoint` interface does not fit and this gets its own seam.

Only getters are sent: class `0x07` commands `0x80` and `0x84`, and class
`0x00` command `0x82`. Nothing in this package writes a setting to a device.

### A relay that is busy or asleep is not a failure

The dock answers `timeout` when the mouse has passed its 300-second idle timer
and `busy` when the openrazer daemon is mid-poll on the same node. Both mean no
reading this time, not a fault, and both are already what the peripherals
section handles: a device that has gone quiet keeps its last value, dim, for
`PeripheralsForget`.

Status `0x05`, "not supported", is the same answer — a dock with nothing paired
to it.

### What the cells are called

The kernel already knows: `HID_NAME` in the node's `uevent` is `SteelSeries
Apex Pro TKL Wireless Gen 3` and `Razer Razer Mouse Dock Pro`. It is used
as-is, with a doubled vendor word collapsed, because a name a person reads on
a card should be the name on the box.

The Razer relay gives the mouse a serial and no name, and the dock's name is
the only one available. A cell that says "Razer Mouse Dock Pro" while showing
the mouse's battery is the honest reading of what this build can know: the
battery is the one the dock is reporting.

### The SteelSeries level is 5 % granular

`((v & 0x7f) - 1) * 5` has a step of five, so a panel goes 95 % then 100 % with
nothing in between. That is the device's resolution and not a dropped reading,
and it is written down where the arithmetic is, because it looks like a bug.

### Nothing is claimed that was not measured

Both protocols are inferred from one machine and one session, with no vendor
documentation. The packages say so. A reply that does not have the length or
the shape this build expects yields no reading rather than a guessed one.

## Executive Summary

The peripherals section on `cachyos` was empty because hayami spoke to neither
device on that desk. It now reads both: a Razer mouse through its Mouse Dock
Pro's RF relay, and a SteelSeries Apex Pro TKL Wireless over the vendor command
rivalcfg has had all along. Neither needs a daemon, a driver or a patched
OpenRazer.

Reviewers should look at `internal/peripherals/hidraw.go` first — the node
finder is shared by all three readers now, and the rule it enforces (vendor and
usage page, never product ID) is the one whose absence invalidated a round of
measurements. Then `razer.go`, which is the only thing here that speaks ioctls.

## Acceptance Criteria

- [x] `hidraw.go` exposes one node finder taking a vendor and a descriptor
      predicate, and `speaksHIDPP` is expressed in terms of it
- [x] Nodes are matched by vendor and usage page and never by product ID, with
      a test that the same reader finds the SteelSeries device under both
      `1644` and `1646`
- [x] `peripherals.Razer` reads battery and charging through a dock at
      transaction id `0x1f`, with the CRC computed over bytes 2..87
- [x] A Razer reply with status busy, timeout or not-supported yields no
      reading and no error
- [x] A Razer reply whose CRC does not match yields no reading
- [x] `peripherals.SteelSeries` reads `0x92`, falls back to `0xd2`, and decodes
      charging from bit 7 and the level as `((v & 0x7f) - 1) * 5`
- [x] A SteelSeries reply whose first byte is not the command sent is ignored,
      so a stale packet is never read as an answer
- [x] Both readers appear in `panel.Peripherals` and contribute reasons when
      they find nothing, in the shape spec 015 established
- [x] `hayami-tui doctor` on `cachyos` shows the peripherals section reading
      both devices
- [x] The whole suite passes, and `go vet` and the linter are clean

## Risks & Assumptions

- **Rollback** is `git revert` of the merge. Both readers are additive; a
  machine without either device is unchanged.
- **Only getters are sent.** The class and command values are the documented
  Razer getters and rivalcfg's battery command. A sweep of undocumented
  commands is what changed this keyboard's lighting during the investigation,
  and nothing in the shipped code sweeps.
- **hidraw contention is real.** The openrazer daemon polls the same dock, and
  a poll that collides answers `busy`. This is why a poll with no reading must
  be ordinary rather than exceptional.
- **The permission model is the seat's ACL**, which grants the logged-in user
  read and write on `/dev/hidraw*`. A machine where it does not — a headless
  session, a different seat — reads nothing and says so, which is the existing
  behaviour for the Logitech reader.
- **No integration boundary** in the spec's sense: no DB, no queue, no index,
  no async job. The boundary here is a device, and the live test against one is
  the `live_test.go` pattern this package already has — skipped where the
  hardware is absent, which is every machine but the one.

## Gaps found

**A SteelSeries device cannot say what it is.** Its vendor control endpoint
declares a vendor usage page and nothing else, and the interfaces beside it are
ambiguous in both directions: this keyboard presents a mouse interface for its
media controls, and plenty of mice present a keyboard one for their macro
buttons. An attempt to infer the kind from the sibling interfaces read the Apex
as a mouse, so it was taken out again. The cell is `KindOther` and sorts last,
which is the only thing admitting it costs.

**The Razer cell is named for the dock.** The relay gives the mouse a serial
and no name, and the dock's is the only name to hand, so the card says "Razer
Mouse Dock Pro" over the mouse's battery. It is the honest reading of what this
build can know and it still looks odd; a name would need either a serial-to-
model table or the dock's pairing protocol, which is PR #2817's territory.

## Alternatives Considered

Considered reading the Razer battery through OpenRazer's D-Bus; rejected
because the version that can see a mouse behind a dock is an unmerged PR, and
depending on it means a DKMS build the user maintains by hand. Speaking the
protocol works on a stock machine.

Considered inferring each device's kind from the interfaces beside its control
endpoint; rejected after it read the keyboard as a mouse. It is written up
under "Gaps found" rather than shipped as a guess.

Considered matching devices by product ID, which is what every comparable
project does; rejected because this keyboard's PID changes with its connection
and the first round of measurements was invalidated by exactly that.

## Status: COMPLETE
