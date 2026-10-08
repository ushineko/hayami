# 035 — The peripherals on Windows

**Issue**: #118

## Status: IMPLEMENTED — two criteria open (a live falsification of the window test; the K800, deferred)

## Context

On Windows the peripherals section found nothing: sanshoku built there and had
no HID transport (spec 033's "Gaps found"). sanshoku v0.1.8 reads HID devices
on Windows through the HID class driver (its spec 012) and adds the AULA F75
through its 2.4 GHz receiver (its spec 013). This spec is hayami's side.

What stood in the way, as found:

1. **The vendor list was Linux's.** It asks the BlueZ and Apple drivers, which
   on Windows can only say that BlueZ is a Linux service; before v0.1.8 they
   dialled a TCP bus every poll, and `doctor` printed `dial tcp
   127.0.0.1:12434 ... actively refused` under "no Bluetooth adapter".
2. **The reasons were Linux's.** A device that may not be opened is told to
   "install the udev rule (60-sanshoku.rules)". There is no udev on Windows,
   and the check for "may not be opened", `sanshoku.IsPermission`, asks for
   Go's EACCES and EPERM, which Windows never returns: ERROR_ACCESS_DENIED
   arrives as itself and would have been reported as a device that "would not
   answer".
3. **The tests took the desk away by emptying the hidraw tree.** With sanshoku
   v0.1.8 hidraw on Windows enumerates the system's own HID device list, which
   no directory stands in for, so `cli` and `tests/structure` would have asked
   the real mouse and keyboard on every run.
4. **"a AULA device answered nothing".** The vendor reasons put "a" before
   every name.

Measured on Windows 11 with a Razer Basilisk Ultimate on its dongle, a Razer
Mouse Dock and an AULA F75 on its receiver: the F75 answers in 50–260 ms when
its link is awake; after it has been idle the first reply takes about a second
(see Gaps found).

## Requirements

- R1 The vendors are Logitech, Razer, SteelSeries and AULA everywhere, then
  Bluetooth (Apple, BlueZ) where hayami can read it: not on Windows. Leaving
  the Bluetooth drivers out, rather than keeping them and hiding their
  reasons, is the true statement: nothing on Windows reads a Bluetooth
  battery, and a reason kept as an aside still reaches `doctor`. AULA is
  `quiet`, as Razer is: its receiver is listed whether or not the keyboard is
  on it.
- R2 A device that may not be opened is named as such on both systems:
  `permitted` asks `sanshoku.IsPermission` and `fs.ErrPermission`, which an
  Errno answers for on either. The detail is the platform's: the udev rule on
  Linux; on Windows, that another program may hold it without sharing.
- R3 The vendor reasons use "an" before a vowel: "an AULA device answered
  nothing".
- R4 `panel.DeviceScan` is the scan the real device sections are built over
  (`sanshoku.Scan`), and `testenv.NoDevices` empties the hidraw tree and, off
  Linux, replaces the scan with one that finds nothing. `cli` and
  `tests/structure` use it. On Linux the drivers stay real, which the udev
  test needs.
- R5 go.mod requires sanshoku v0.1.8.
- R6 README: the "On Windows" section says what reads and what does not, the
  sources table names the AULA and Windows; a changelog under `### Unreleased`.
- R7 Spec 032's memory needs nothing new: it is keyed by the battery's name,
  which on Windows comes from the device's product string as Linux's comes
  from `HID_NAME`, the same string across restarts ("F75", "Basilisk Ultimate
  Dongle").

## Acceptance Criteria

- [x] `go test -tags migrated_fynedo ./...` passes on Windows against sanshoku
  v0.1.8, except `internal/usage`'s Python cross-check, which fails on `main`
  too on this machine (its Python lacks `structlog`).
- [x] `GOOS=linux go vet` passes for `panel`, `cli`, `testenv`, `core`,
  `view`, `cmd/hayami-tui` and `tests/structure`; `CGO_ENABLED=0` builds of
  `hayami-tui` for windows and linux; golangci-lint v2.12.2 reports 0 issues
  on the changed packages for windows and for linux.
- [x] Unit tests: on Windows no Bluetooth driver is asked and a device Windows
  will not open is "not permitted" with the Windows detail; on Linux BlueZ's
  absence is "no Bluetooth adapter" as before; a quiet AULA receiver is "an
  AULA device answered nothing". Both Windows tests fail with their fix
  broken (`permitted` reduced to `sanshoku.IsPermission`; a Bluetooth vendor
  put back).
- [x] On a real window on Windows 11 the peripherals card draws the F75 at
  100 %, discharging (`tests/window`, `HAYAMI_WINDOW_TEST=1`). Picture looked
  at.
- [x] The window test's assertion rejects a card of reasons and placeholders:
  run over the picture of the run that drew no device (6 lines) it fails,
  over the one that drew the F75 (3 lines) it passes.
- [ ] Falsified live: the window test with the AULA vendor removed, on a desk
  where the keyboard answers. Not done: by then both devices had gone to
  sleep and the test skipped (it skips when nothing answers).
- [ ] The K800 on its Unifying receiver, through the panel. Deferred: the
  keyboard is in storage.

## Risks & Assumptions

- **Linux behaviour** changes in two lines: an AULA vendor reason ("no AULA
  receiver") joins the others on a desk with nothing, and "a" becomes "an"
  before it. The Bluetooth vendor is unchanged on Linux.
- **The window test depends on the desk**: it asks the devices first and skips
  when none gives a level, then takes the picture until it holds a device, for
  three polls at most.
- **Two README hunks overlap spec 034's** (the "On Windows" bullet the old
  "find nothing yet" line was, and `### Unreleased`), as do the two
  `tests/window` helper files, which are spec 034's verbatim. Whichever
  merges second resolves the README by keeping both.
- **Rollback**: revert. sanshoku v0.1.8 is backwards compatible on Linux; the
  previous hayami ignores the AULA and Windows devices.

## Gaps found

- **The F75's first reply after idling takes about a second**, longer than
  sanshoku's two questions of roughly 400 ms each, so a poll of an idle
  keyboard reads nothing: `sanshoku-bench read --driver aula` read nothing
  three times in four when idle, and every time once a scratch probe with a
  one-second wait had woken the link (73–616 ms). The panel draws the
  keyboard from its next answer and spec 022's memory keeps it, dim, while it
  sleeps; a longer per-question timeout for the aula driver is a sanshoku
  change.
- **`sanshoku.IsPermission` misses Windows' permission errors**; hayami asks
  `fs.ErrPermission` beside it. sanshoku could answer it itself.
- The second cell on a one-device desk reads "no device", as on Linux (spec
  022); not new here.

## Verification

2026-10-07, Windows 11 Pro 26200, Go 1.26.0, MSYS2 UCRT64 gcc 16.2.0,
sanshoku v0.1.8:

- Full suite: every package ok but `internal/usage` (pre-existing, above).
- Window test: `answering: [F75]`; text lines heading, "F75", the second cell;
  the picture shows "F75 / 100 % / Discharging" and a "no device" cell.
- `hayami-tui doctor`, with the mouse and the keyboard asleep:

  ```
  peripherals   absent
                        no Logitech receiver
                        a Razer device answered nothing
                        no SteelSeries device
                        an AULA device answered nothing
  ```

  No line about BlueZ or Bluetooth.
