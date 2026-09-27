# Spec 008: the peripherals section

**Issue**: [#19](https://github.com/ushineko/hayami/issues/19)

## Status: COMPLETE

## Executive Summary

`internal/peripherals` speaks HID++ to a Logitech receiver over `hidraw` —
choosing the node by its report descriptor, recognising both error forms, and
retrying a request five times because a real device answers one in only
fourteen — and reads `headsetcontrol -o json` for the headset.
`internal/panel.Peripherals` remembers what each device last said so one that
goes quiet keeps its level and is drawn dim, and drops that level when the
device or its charge state changes. `view.Peripherals` draws a row per device
at the reference's bands. Reviewers should start with `requestAttempts` in
`internal/peripherals/hidpp.go`, which is the finding that only the live test
could have made.

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
  get no sysfs children and device indices have to be found over HID++. Every
  index answers: a paired one with its feature index, an empty one with a
  HID++ **1.0** error, `8f 00 08 08`, in about a millisecond. The first probe
  written against this took eight seconds, because its matcher knew only the
  2.0 error form — feature index `0xFF` — and sat out its timeout on each of
  the five empty indices. The protocol was never slow; the reader was deaf to
  half of what it was told. A HID++ reader has to recognise **both** error
  forms, and a timeout is a backstop for a device that is asleep rather than
  the mechanism discovery runs on.

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
- R3 Device discovery reads the reply it is given. Both HID++ error forms —
  1.0's sub-id `0x8F` and 2.0's feature index `0xFF` — end a request rather
  than being ignored, so an empty index costs a millisecond. A timeout remains,
  as a backstop for a device that does not answer at all, and a found index is
  remembered so the next poll asks it directly.
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

- [x] AC1 A HID++ battery reply of known bytes decodes to a known level and status, and a reply arriving as a long report decodes the same as a short one. (R1)
- [x] AC2 Given a directory of report descriptors the test writes, the vendor-usage node is chosen and the mouse and keyboard nodes are not. No descriptor is no device and no error. (R2)
- [x] AC3 A discovery against a fake endpoint that answers on one index and returns each error form on the others finds the one device without waiting, an endpoint that answers nothing at all is bounded by the timeout, and the second poll asks only the index it found. (R3)
- [x] AC4 A recorded `headsetcontrol -o json` reply yields the device's name and level; the same reply with `BATTERY_UNAVAILABLE` yields no level. A `headsetcontrol` that is absent yields no row. (R4)
- [x] AC5 A device that answers and then stops keeps its level with `Gone` set; a device that never answered is absent from the section. (R5)
- [x] AC6 A remembered level survives an unchanged poll, and is dropped both when the name changes and when charging flips to discharging. (R6)
- [x] AC7 A level is red at 20 and below, amber to 50 and green above; a charging device says so and is not coloured for it. (R7)
- [x] AC8 The parity test passes with `peripherals` in `panel.Keys()`, and the preferences window lists it. (R8)
- [x] AC9 **Against the real hardware on this machine**, `hayami-tui --sections peripherals` reports the same level `solaar show` does, and the pane is photographed with `tools/shot-tui.sh`. Skipped where the hardware is absent. (R1, R4)

## Gaps found

None. The dimming a stale device needs is `view.Dim`, which the painter
already had; `view.Section.Gone` turned out to be the wrong shape for this —
it marks a whole section, and here it is one device of several that has gone
quiet — so a stale row carries `Dim` as its own status instead. Nothing was
wanted from the design system that it does not have.

## What the tests caught

Three things, and the order they were caught in is the argument for the way
they are written.

**The vendor match, caught by a unit test before it ever ran.**
`strings.TrimLeft(fields[1], "0")` on the kernel's `0000046D` returns `46D`,
not `046D`: TrimLeft strips every leading zero, including the vendor's own.
Compared against `"046D"` it matched nothing, so **no Logitech node would ever
have been found on any machine**. The section would have drawn the headset and
silently never the mouse, and it would have looked like a hardware problem.
The field is parsed as a number now.

**A device going quiet was being reported as a failure.** A sleeping wireless
mouse answers nothing, every poll, for as long as nobody touches it — that is
its ordinary state, not an error. Returned up the stack it would have put a
line in the log every fifteen seconds and buried the real failures among them.
Silence has its own sentinel now and the section draws the device stale.

**One request is not enough, caught only by the live test.** Every test in the
package drives an endpoint the test wrote, every one of them passed, and the
real mouse answered a single request **fourteen times in twenty** — and, after
six seconds of idle, needed up to *four* attempts. Nothing in the protocol says
so and nothing in the code suggests it. At one attempt the panel would have
shown a mouse flapping between a reading and "not answering" on most polls,
on a desk where nothing was wrong. This is the case the integration-boundary
rule exists for, and it is worth being precise about what saved it: not the
decoder tests, which were green throughout, but a test that opened the device.

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
- **The retry budget is a measurement of one receiver.** Five attempts covers
  what a Lightspeed receiver and a G502 X PLUS needed, with margin. It is
  recorded at `requestAttempts` with both measurements and their conditions, so
  the next person can see it is a number that was taken rather than chosen.
  Only silence is retried, so a device that is off costs one request: the
  receiver refuses its index, and a refusal is an answer.
- **A device that is off is not a device that is asleep**, and the program
  cannot tell them apart in the moment — the receiver says "unknown device" for
  both. That is what `PeripheralsForget` settles: quiet is quiet for ten
  minutes and gone after.
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
