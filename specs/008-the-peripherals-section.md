# Spec 008: the peripherals section

**Issue**: [#19](https://github.com/ushineko/hayami/issues/19)

## Status: INCOMPLETE

## Executive Summary

Populated before the PR is opened.

## Context

The section the program is named after, and the last one missing. Until it
draws, `hayami` cannot replace `peripheral-battery-monitor` on the desktop
that program runs on, whatever else it has learned to do.

`battery_reader.py` is the behavioural reference and it reads Logitech through
solaar's **Python library**, `logitech_receiver`. Two hundred lines of that
file exist to survive doing so: `gi` and `evdev` are mocked before the import,
because solaar's `diversion` module creates a real kernel input device as a
module-level side effect, and a reader that runs as a fresh subprocess every
poll would register a new one each time. A Go program cannot import that
library, and the CLI that wraps it takes three and a half seconds to answer and
prints a Wayland warning to stderr while doing it.

So this section speaks HID++ to the device itself. That was measured before it
was specified: on the machine this was written on, the root feature lookup for
`0x1004 UNIFIED BATTERY` and one `get_status` returned **86 %, the same figure
solaar reports, in about five milliseconds**. The protocol is a request and a
reply of seven bytes, not a library.

Three things that probe found, which the implementation has to know:

- The HID++ endpoint is one of three `hidraw` nodes the receiver presents, and
  it is the one whose report descriptor opens with a vendor usage page
  (`06 00 ff`) and declares report `0x10`. The other two are the mouse and the
  keyboard interfaces and never answer. Picking by node number would be the
  hwmon-index mistake spec 006 already refused.
- **A reply to a short request may arrive as a long one.** The `0x1004` reply
  came back as report `0x11`. A matcher that keys on the report ID drops the
  answer it asked for.
- The driver here is `hid-generic`, not `hid-logitech-dj`, so paired devices
  get no sysfs children and device indices have to be found over HID++.
  Receiver register `0x02` gives the connected count; an index with nothing
  paired to it does not answer **at all**. Discovery is therefore bounded by a
  timeout rather than ended by a reply, and the first probe — written with a
  1.5 second one — took eight seconds to find a mouse that answers in five
  milliseconds. The timeout is the design.

`headsetcontrol` is the opposite story: it has gained `-o json` and an
`api_version` since the reference was written, and it now prints
`Warning: short output deprecated` when asked the way the reference asks. The
JSON carries the device's real name, its capabilities and a battery status
distinct from its level, and it answers in twenty-four milliseconds. hayami
reads the JSON.

What comes across from the reference unchanged is the part that took it
releases to get right. A peripheral is not a sensor: it goes away, and the
question a reader has about a headset that has stopped answering is not "what
is it now" but "is what it said last still true". The reference answers by
keeping the last level and dimming it, and by dropping that level in the two
cases where it is not stale but wrong — the device on the other end changed,
or the battery crossed between charging and discharging. `view.Section.Gone`
already exists for the first half of this; it was built for bandwidth and has
not had a second user until now.

The reference's two fixed left/right slots do not come across. They are an
artefact of a PyQt panel 260 pixels wide, configured through a submenu, and
hayami's view is a list of rows laid out three ways. A row per device found,
appearing and disappearing as the hardware does, is what the rest of this
program already does.

## Requirements

- R1 `internal/peripherals` reads a Logitech battery over HID++ on `hidraw`,
  with no subprocess and no dependency on solaar being installed.
- R2 The HID++ endpoint is chosen by its report descriptor, never by node
  number. A machine with no Logitech receiver is a machine with no Logitech
  row, not an error.
- R3 Device discovery is bounded. An index with nothing paired to it costs a
  short timeout, not a long one, and a found index is remembered so the next
  poll asks it directly.
- R4 The headset is read from `headsetcontrol -o json`. `headsetcontrol`
  missing, failing, or reporting a device that is offline is a row that is not
  drawn or is drawn stale, never a section that fails.
- R5 A device that stops answering keeps its last level and is drawn dim. A
  device that has never answered is not drawn at all.
- R6 The remembered level is dropped when the device name changes, or when the
  reading crosses between charging and discharging. Then it is not stale, it
  is wrong.
- R7 Levels are coloured at the reference's bands: 20 % and 50 %. Charging is
  said, not coloured.
- R8 The section is in both panels, in all three arrangements, in the settings
  and in the preferences window, as the other three are. The parity test holds.

## Acceptance Criteria

- [ ] AC1 A HID++ battery reply of known bytes decodes to a known level and status, and a reply arriving as a long report decodes the same as a short one. (R1)
- [ ] AC2 Given a directory of report descriptors the test writes, the vendor-usage node is chosen and the mouse and keyboard nodes are not. No descriptor is no device and no error. (R2)
- [ ] AC3 A discovery against a fake endpoint that answers on one index and ignores the rest completes within its bound, and the second poll asks only the index it found. (R3)
- [ ] AC4 A recorded `headsetcontrol -o json` reply yields the device's name and level; the same reply with `BATTERY_UNAVAILABLE` yields no level. A `headsetcontrol` that is absent yields no row. (R4)
- [ ] AC5 A device that answers and then stops keeps its level with `Gone` set; a device that never answered is absent from the section. (R5)
- [ ] AC6 A remembered level survives an unchanged poll, and is dropped both when the name changes and when charging flips to discharging. (R6)
- [ ] AC7 A level is red at 20 and below, amber to 50 and green above; a charging device says so and is not coloured for it. (R7)
- [ ] AC8 The parity test passes with `peripherals` in `panel.Keys()`, and the preferences window lists it. (R8)
- [ ] AC9 **Against the real hardware on this machine**, `hayami-tui --sections peripherals` reports the same level `solaar show` does, and the pane is photographed with `tools/shot-tui.sh`. Skipped where the hardware is absent. (R1, R4)

## Risks & Assumptions

- **One receiver, one mouse, one headset.** Every finding above is from one
  machine. The report-descriptor rule and the long-reply rule are protocol
  facts and should travel; the device indices and the timeout are measurements
  and are recorded where they are used.
- **HID++ is spoken to hardware, not to a file.** The tests drive an endpoint
  the test writes, so the suite touches no device; AC9 is the one that touches
  the real one, and it skips when there is none. This is the
  integration-boundary criterion: the decoder passing on recorded bytes is not
  evidence that the exchange works, which is exactly the shape the policy
  warns about.
- **Writing to `hidraw` needs the node writable.** It is here, by a seat ACL.
  Where it is not, the section reports no Logitech row rather than failing, and
  that case is worth a log line because it is a permission problem wearing the
  clothes of absent hardware.
- **Battery percentages from HID++ are coarse.** `0x1004` reports a state of
  charge and a level band; the band is what a device without a fuel gauge
  actually knows. Drawing the band as a percentage would invent precision, so
  the row says what the device said.
- Rollback: revert. The section is drawn only when the settings name it, and
  nothing else in the program changes.

## Alternatives Considered

- Running `solaar show` and parsing it. Rejected: three and a half seconds per
  poll, a human-readable format that is not an API, and a Python runtime
  dependency for the program written to remove one.
- Trying both, HID++ first and the CLI as a fallback. Rejected for now: it is
  two code paths plus the logic that chooses, and the slow one would almost
  never run here, which is the condition under which a fallback rots.
- `upower` and BlueZ for keyboards, and AirPods over Apple's accessory
  protocol. Both are in the README's table and neither is in this spec: there
  is no hardware here to hold them to, and the reference's AirPods path is an
  L2CAP protocol implementation that deserves a spec of its own.
- Drawing each battery as a meter rather than a row. Rejected: `view.Meter`'s
  own argument is that a bar needs a limit and a caption that earns its width,
  and a battery's caption would repeat the number beside it.
