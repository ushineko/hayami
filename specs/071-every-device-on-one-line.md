# 071 — every device on one line

**Issue**: #236

## Status: COMPLETE

## Executive Summary

A check on the Sections page, "Every device on one line", puts every
peripheral on one line so the panel grows wider instead of taller; off, the
default, the card wraps as before. Window only. It rests on fynedesygn
0.1.108's `CellGrid.SetOneLine` (spec 075 there). Reviewers should look first
at `Panel.setOneLine` and its call in `Panel.Apply` (`internal/gui/gui.go`).

## Context

The peripherals card fits as many devices across as the window's width
allows: two at the 260 px minimum, so four devices are a two-by-two block,
and each further device makes the card taller. Dragging the window wider
gets more across.

The operator wants to choose which way the card grows past two devices
(2026-10-31): wrapping, as now, or every device on one line, with the card
and the window growing wider. The decisions made with the operator:

- It is a setting in the preferences window, in the peripherals section's
  own settings on the Sections page, beside the bandwidth section's. It does
  not go in the right-click menu or on the card.
- **Window only.** A terminal pane cannot grow, and it already puts every
  cell on one line when it is wide enough. The pane ignores the setting.
- The grid's one-line mode is a library change: fynedesygn spec 075
  (ushineko/fynedesygn#222), `CellGrid.SetOneLine`.

## Requirements

- R1 `config.PeripheralsSettings{OneLine bool}` (`"oneLine"`, omitted when
  false) under the section's key, read and written through `config.Setting`
  like `config.Bandwidth`. Off by default, so an existing settings file
  draws what it drew.
- R2 The Sections page gets a peripherals entry in `sectionPrefs`: a check,
  "Every device on one line", with a dim line saying the panel grows wider
  instead of taller. It saves as it is changed, like every other setting.
- R3 The window applies the setting when it builds the peripherals card's
  grid and again whenever the settings change, then calls `Panel.Resize`
  once, in the same call, so the window takes the new shape at the switch
  and not on the next poll.
- R4 In the `row` arrangement the setting has no effect: the library's
  `Lines` wins (fynedesygn spec 075 R4). The check stays enabled, and its
  dim line says it applies to stack and grid.
- R5 The terminal panel ignores the setting. If the parity test compares
  settings across shells, the setting goes on its allow-list as window-only,
  with the reason.
- R6 fynedesygn is bumped to the release that carries spec 075.
- R7 README: the setting where the preferences are described, and the
  changelog line under `### Unreleased`.

## Acceptance Criteria

- [x] `config` test: the setting round-trips, and a file without it reads
      as off (`TestEveryDeviceOnOneLineIsSavedAndOffByDefault`).
- [x] `gui` test: with four devices and the setting on, the four cells are
      at the same y and the card is wider than with it off. Turning it off
      again puts them back on as many lines as before, with no restart
      (`TestEveryDeviceGoesOnOneLineAndBack`; the test window is narrower
      than two cells, so the baseline is "more than one line", not two).
- [x] `gui` test: in the `row` arrangement with the setting on, one cell per
      line (`TestRowIsOneDevicePerLineWhateverTheSetting`).
- [x] `prefs` test: ticking the check writes `"oneLine": true` for the
      peripherals section, and unticking it removes the key
      (`TestTheSectionsPageSavesEveryDeviceOnOneLine`).
- [x] The pane draws the same lines with the setting on and off. Checked
      against the real binary rather than as a unit test: `tui.Options` has
      no field the setting could reach, so a unit test would pass by
      construction. `hayami-tui --sections peripherals --once` with a
      settings file saying `oneLine: true` and one without: identical output
      (2026-10-31, this desk).
- [x] Each new test fails with its part of the change reverted (the `gui`
      test with `setOneLine` a no-op: "the window did not grow wider"; the
      `prefs` test with the registry entry removed. The `row` test guards
      the library's rule and is falsified by fynedesygn spec 075's).
- [x] A screenshot of the real window on this desk with the setting on
      shows the devices on one line and the panel wider; with it off,
      wrapped at the old width. Neither picture is committed (they are the
      desk's own). `tools/screenshot.sh --what panel`, 2026-10-31, three
      devices: off 371×867 px, two cells then one; on 434×764 px, three
      cells on one line. Built against fynedesygn's branch.
- [x] `make test`, `make lint`, `make build` pass, against fynedesygn
      v0.1.108 as released (no workspace override); `make vuln` reports
      nothing this code calls.

## Risks & Assumptions

- Every card takes the widest card's width, so with four devices on one
  line the bandwidth, cooler and usage cards widen too. That is the effect
  the operator asked for.
- A device connecting or going changes the window's width rather than its
  height. Spec 070 accepted a device changing the card's size; this moves
  the change from height to width.
- A width the user dragged is kept by the library when it is wider than the
  content (`Panel.resize`). A user who dragged the panel wide, switched to
  one line and back, keeps the dragged width.
- Depends on the fynedesygn release; nothing here merges before it.
- Rollback: revert. A settings file that says `"oneLine": true` is ignored
  by an older build, which reads only the keys it knows.

## E2E Test Plan

The operator runs the built panel with three or more devices connected:

1. Preferences → Sections → Peripherals, tick "Every device on one line".
   The devices go onto one line and the panel widens at once (R2, R3).
2. Disconnect a device: the panel narrows by one cell (R3, fynedesygn R3).
3. Switch the arrangement to row: one device per line (R4).
4. Untick: the devices wrap two across at the old width (R1, R3).
5. `hayami-tui` with the setting on draws as it did before (R5).

### Prod visibility

Not applicable: a desktop program with no telemetry. The setting is in the
settings file the user owns.
