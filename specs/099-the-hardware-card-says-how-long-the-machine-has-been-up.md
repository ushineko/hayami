# 099 — the Hardware card says how long the machine has been up

**Issue**: #348

## Status: COMPLETE

## Executive Summary

The Hardware card has an Uptime row under Speeds, `3d 04h 12m` at a fixed
eleven columns, read from `/proc/uptime` on Linux and GetTickCount64 on
Windows through a new `Host.Uptime` field. Reviewers should look first at
`view.Uptime` in `internal/view/cooler.go` (the fixed width) and the keep rule
in `record` in `internal/panel/cooler.go`.

## Context

How long the machine has been up is a glance reading with no place in the
panel: it is a trip to `uptime` or to Task Manager's Performance page (#348).
The Hardware card already reads the machine itself (processor, card, memory,
IO); its uptime is one more fact of the same kind.

Both systems give it to an unprivileged program without a subprocess:

- **Linux**: `/proc/uptime`, whose first field is seconds since boot,
  CLOCK_BOOTTIME, so time spent suspended counts, as `uptime` reports it.
- **Windows**: GetTickCount64, through `x/sys/windows.DurationSinceBoot`.
  It is Task Manager's "Up time", including its quirk: a Fast Startup boot
  resumes the kernel and does not reset it.

## Requirements

- **R1** An **Uptime** row is the last row of the Hardware card, below
  Speeds, in both shells and all three arrangements. Its ID is `uptime`.
- **R2** The value reads `3d 04h 12m`: days, then hours and minutes as two
  digits each. Under a day the days are spaces (`    04h 12m`). From 100
  days the minutes go (`123d 04h`). Every form is the width of a
  processor's load and temperature (`LoadWidth + 1 + NumberWidth`, eleven),
  so the minute changing never widens the row or moves the panel.
- **R3** The row is uncoloured (Info): an uptime has no band that is a
  verdict.
- **R4** The reader is a field of the host table, `Host.Uptime`, read on the
  card's poll: `/proc/uptime` on Linux (`ProcUptime`), `DurationSinceBoot`
  on Windows, none on other systems. A host without one draws no row.
- **R5** A poll whose read fails keeps the last value, as a row name is
  kept, so the row does not leave and come back; a machine whose uptime was
  never read has no row. The failure is an aside reason, as the memory's is.
- **R6** The reading is in the card's JSON (`hayami-tui cooler`) as
  `uptime`, in nanoseconds as every other duration the command prints.
- **R7** The uptime alone does not make the card present: a machine with no
  hardware reading still has no Hardware card.

## Acceptance Criteria

- [x] `view.Cooler` draws an Uptime row last, ID `uptime`, Info, for a
      reading with an uptime, and none without (view test).
      `TestTheUptimeIsTheLastRow`, `TestNoUptimeIsNoRow`.
- [x] The value formats as R2 for zero, minutes, hours, days, 99 days and
      100 days, every form eleven wide (view test).
      `TestAnUptimeIsTheSameWidthInEveryForm`.
- [x] `ProcUptime` parses `/proc/uptime`, and a malformed or missing file
      is an error (core test against fixtures, fuzz target).
      `internal/core/uptime_test.go`, `FuzzParseUptime`.
- [x] The panel's poll fills the uptime from the host, keeps the last value
      when a read fails, and does not count it toward the card's presence
      (panel test). `internal/panel/uptime_test.go`.
- [x] The desktop panel shows the row on the desk, in a real-window
      screenshot, matching `uptime -p` to the minute.
- [x] The terminal panel shows the row in stack, grid and row.
- [x] Windows: the row matches the system's uptime in the test VM
      (`Win32_OperatingSystem.LastBootUpTime`, the figure Task Manager's
      "Up time" is).
- [x] README changelog line under `### Unreleased`.

## Risks & Assumptions

- The card grows by one row on every Linux and Windows machine: one change
  of shape when the version is installed, not a reading's.
- The uptime moves once a minute and the card polls every five seconds; the
  minute shown lags the true one by up to a poll.
- **Rollback**: revert; no setting or file format changes.

## Threat model delta

B4: `/proc/uptime` is a new parsed input, a root-owned kernel file of two
numbers, parsed by a fuzzed function; a value that is not a number is an
absence. GetTickCount64 takes no input. No new right, no new process.
`docs/threat-model.md` B4 carries the entry.

## Test strategy

View: shape and format tests on `view.Cooler` and the formatter. Core: the
parser against fixture files under `t.TempDir()`, and a fuzz target. Panel:
the poll with a stub host reader, including a failed read after a good one.
Real window: the installed panel on the desk, screenshotted; the terminal
panel in its three arrangements; the Windows VM.

## E2E Test Plan

Desk: install with fynstall, restart the panel, screenshot the Hardware
card, and compare the row with `uptime -p` (R1, R2). Terminal: `hayami-tui`
at a narrow, wide and row width (R1). VM: the row against Task Manager's
Performance → CPU "Up time" (R4).

### Prod visibility

Not applicable: a desktop program with no telemetry.

### E2E results (2026-11-17)

- **Desk, desktop panel** (R1, R2): this branch's build, captured with
  `tools/screenshot.sh --what panel`, with the installed panel quit and
  restarted afterwards. The Uptime row is the card's last, `20d 13h 26m`,
  its value ending in the processor's temperature column, the card no wider.
  `uptime -p` read "up 2 weeks, 6 days, 13 hours, 25 minutes" a minute
  earlier.
- **Desk, terminal panel** (R1): `hayami-tui --sections cooler --once` in
  stack, grid and row: the row last in each, `20d 13h 25m`.
- **Windows VM** (R4): `hayami-tui.exe --sections cooler --once` read
  `19h 33m`; `Win32_OperatingSystem.LastBootUpTime` gave 19.56 hours.
- **Not added**: a pixel-geometry test. The row is an ordinary `Row` drawn
  by the existing row code, whose widths the window tests already cover; the
  screenshot is the evidence for this change.
