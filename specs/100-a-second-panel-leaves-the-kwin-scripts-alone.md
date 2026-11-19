# 100 — a second panel leaves the KWin scripts alone

**Issue**: #355

## Status: COMPLETE

## Executive Summary

A hayami panel that does not hold the bus name `io.ushineko.hayami` no
longer unloads the KWin scripts when it quits. The hold script is loaded
again at each asking instead of being remembered as loaded, and the panel of
record checks its position watch every 30 seconds and puts it back if it has
gone. Reviewers should look first at `anotherPanel` and `keep` in
`internal/desktop/position.go` and `hold` in `internal/desktop/drawer.go`.

## Context

On Plasma the panel keeps scripts loaded in KWin: the position watch
(`hayami-geometry`), the place (`hayami-place`) and the hold of an edge or
corner (`hayami-hold-edge`). KWin's script names are global, and every hayami
panel loads and unloads the same ones. A second panel on the desk (a
checkout's build, a `tools/screenshot.sh` run) unloaded all of them when it
quit, the panel of record's included.

The panel of record did not notice. `Drawer.HoldEdge` remembered the edge it
last loaded and returned early when the same one was asked for again, so the
hold never came back: Plasma 6.8 grows a window from its centre, and every
drawer opening moved the cards by half the drawer's width, off the screen at
its right edge, and left the panel that far to the left when it closed. The
position watch was gone too, so the panel stopped remembering where it was
put. A restart loaded everything again.

Seen on the desk on 2026-11-19 (#355): KWin answered `isScriptLoaded` false
for all five names while the panel ran, KWin itself had been up for 22 days,
and opening the Hardware drawer took the window from 289 to 653 px wide and
moved it 182 px, half of the 364 px added.

The installer is not a source: it waits for the old panel to exit before the
new one starts (`installer/stop_linux.go`).

## Requirements

- **R1** A panel unloads its KWin scripts when it quits (`Drawer.Stop`,
  `Position.Stop`) only when no other process holds `io.ushineko.hayami`,
  the name the panel of record takes in `Position.Watch`. A name nobody
  holds is nobody's, and the scripts are unloaded.
- **R2** `Drawer.HoldEdge` and `Drawer.HoldCorner` load their script at
  every call, in place of whatever is under `hayami-hold-edge`, and never
  take it as still loaded from an earlier call. They report what they did
  as before: HoldEdge whether the right edge is held, HoldCorner whether the
  corner is.
- **R3** While a panel watches its position, it asks KWin every 30 seconds
  whether `hayami-geometry` is loaded and loads it again when it is not,
  unless another process holds the panel's name. It stops asking when the
  watch stops.
- **R4** Nothing changes on Windows, where no script is loaded.

## Acceptance Criteria

- [x] A panel that does not hold the name leaves the hold, the watch and
      the place loaded when it quits (`TestASecondPanelLeavesThePanelOfRecordsScripts`,
      with the name held by a second process).
- [x] The panel of record still unloads all three when it quits
      (`TestThePanelOfRecordTakesItsScriptsOut`).
- [x] A hold taken away is loaded again at the next HoldEdge and the next
      HoldCorner (`TestAHoldThatWasTakenAwayIsLoadedAgain`,
      `TestACornerHoldThatWasTakenAwayIsLoadedAgain`).
- [x] A watch taken away is put back (`TestAWatchThatWasTakenAwayIsPutBack`).
- [x] Each test above fails against the code before this spec, or with
      `anotherPanel` and `loaded` broken (falsified 2026-11-19).
- [x] On the desk, after a 0.18.1 second panel quit and wiped the scripts,
      the first drawer opening held the right edge and the watch came back
      (E2E results).
- [x] On the desk, a second panel of this build quitting left the hold and
      the watch loaded (E2E results).
- [x] `GOOS=windows go vet ./internal/desktop/` passes (R4).

## Risks & Assumptions

- A load at each first opening and each unfolding is a round trip of a
  millisecond or two, off the UI thread, as the room's already is.
- The ownership test is the bus name's process ID
  (`GetConnectionUnixProcessID`). A panel without a store takes no name; it
  unloads on quitting, as before, unless a panel of record runs.
- Two panels running at once still share one hold, as before: a second
  panel's opening loads its edge's hold, which acts on both windows by class
  and title. Unchanged by this spec.
- A build from before this spec quitting still takes the scripts. The hold
  comes back at the next opening, the watch within 30 seconds; the place
  only matters at a start.
- **Rollback**: revert; no setting or file format changes.

## Threat model delta

B5 (what hayami does outside itself): whether a quitting panel unloads its
KWin scripts now depends on which process holds the session-bus name
`io.ushineko.hayami`. A process of the same account that takes the name
first keeps the panel from unloading them; the scripts left act only on the
panel's window, and such a process can already load and unload KWin scripts
itself. No new right, no new process. `docs/threat-model.md` B5 carries the
entry.

## Test strategy

Integration tests in `internal/desktop/scripts_test.go` against a real
`dbus-daemon` started for each test, with a fake KWin answering
`loadScript`, `unloadScript`, `isScriptLoaded` and `run` on it, and a second
process (the test binary itself) holding the panel's name. The tests refuse
to run unless the program's session bus is the test's own, so no script
reaches the desk's compositor. On CI a missing `dbus-daemon` fails them
instead of skipping. Real window: the desk, measured with `kdotool`.

## E2E Test Plan

Desk (Plasma 6.8, panel at the right edge of a screen): run this build as the
panel of record, run the installed 0.18.1 as a second panel with throwaway
settings and quit it, then open a drawer and read the window's geometry and
the scripts KWin has (R2, R3). Run this build as the second panel and quit
it, and read the scripts again (R1).

### Prod visibility

Not applicable: a desktop program with no telemetry.

### E2E results (2026-11-19)

- **Before** (installed 0.18.1, the panel that showed the bug): KWin had none
  of the five scripts. Opening the Hardware drawer: x 3710.7 → 3528.7, width
  289.3 → 653.3, right edge 4182 against the screen's 4000. Closing it left
  the panel at x 3528.7.
- **Old second panel** (R2, R3): this build as the panel of record at x 3711;
  0.18.1 run with throwaway settings and quit with TERM. KWin then had none
  of hold, watch and place. The watch was back within the keeper's 30
  seconds, before the drawer was opened. Opening the Hardware drawer: x
  3711 → 3347, width 289.3 → 653.3, right edge 4000.3; the cards whole in
  the screenshot, the drawer to their left.
- **New second panel** (R1): this build run with throwaway settings and quit
  with TERM: hold and watch still loaded.
- The installed panel was restarted afterwards, back at x 3711.
