# 106 — a clock

**Issue**: #372

## Status: COMPLETE

## Executive Summary

A clock section, on by default at the top: the card's heading is the time to
the second at the left and the date at the right, in the title's style, with
nothing under it, drawn with fynedesygn's `Card.SetTitle` and
`SetTitleEnd`. It is polled at the turn of each second through a new
optional `panel.Paced`, and existing settings files are offered it once
(`config.Offered`). Review `internal/panel/clock.go`, the bare card in
`internal/gui/gui.go`, and `Config.offer` first.

## Context

The panel says nothing about when it is. A glance at the desk for the
network, the temperatures or the usage windows is also, often, a glance for
the time, and the usage windows reset at times of day. Spec 105 gives back
the height the Network card spent on down VPNs; a clock can take one line of
it.

A section rather than a part of another card, because the section registry
is how the panel is arranged (`docs/architecture.md`): the person can hide it
or move it, it works in all three arrangements and in the terminal, and a
hidden clock costs nothing.

## Requirements

- R1 A section, key `clock`, title "Clock" in the preferences and on the
  command line (`--sections clock`). **On by default, at the top of the
  stack.** A saved order that does not name `clock` (every settings file
  written before this) gets it at the top once; a person who then hides or
  moves it keeps their choice.
- R2 The clock is the card's heading, in the title's style (bold, the
  title's grey), with nothing under it: a clock-face icon and `HH:MM:SS` at
  the left, where every card's title starts, and a calendar icon and
  `DoW YYYY/MM/DD` at the right, where the figures end. e.g.
  `◷ 18:25:04 ............ ▦ Sat 2026/11/21`. Local time, **24-hour**, with
  no 12-hour setting. The icons are `theme.HistoryIcon` and
  `theme.CalendarIcon`, coloured as the title is. (Chosen from three styles
  mocked on the desk, 2026-11-21.)
- R3 It is drawn with fynedesygn spec 086's `Card.SetTitle` (the time, with
  `SetIcon`) and `Card.SetTitleEnd` (the date and its icon). A card that is
  only its heading keeps it in `Lines`.
- R4 It ticks at the turn of each second, from a timer aligned to the second,
  not on a poll interval. Each tick sets the title in place; nothing else
  repaints. Hidden, its timer stops.
- R5 The title's face is not monospace, so the time's width varies with its
  digits; fynedesygn holds a title at the widest it has been, so no tick
  narrows or reflows the card, and the date, pinned to the right, never
  moves.
- R6 `view.Section` gains what the shells need to draw a heading at both
  ends (a title end and its icon, by `IconName`), so the view says it and
  both shells draw it. The terminal panel prints the time at the left and
  the date at the right of the section's title line, with no glyphs -- an
  emoji's width is the terminal's guess -- and the parity test's allow-list
  says so, as it does for the section icons. `row` is the one line; `grid`
  is a column one line high.
- R7 The core reads the time through an injected clock, so a test sets the
  time.

## Acceptance Criteria

- [x] `make test` (`go test -race ./...`), `make lint` (0 issues);
      `CGO_ENABLED=0` build of `hayami-tui`; `go vet -tags nogui` for
      Windows on the changed packages.
- [x] Tests: the section for a fixed time, its title, end and icons
      (`TestTheClockIsItsHeading`); the next poll at the next second's turn,
      and an unpaced source at its interval
      (`TestTheClockIsPolledAtTheTurnOfEachSecond`); one length on every day
      and hour (`TestTheClockIsOneLengthEveryDay`); one line in the terminal
      in stack and row (`TestTheClockIsOneLineInTheTerminal`); the window's
      card ticks in place at one height and turns the date
      (`TestTheClockCardIsItsHeadingAndTicks`) and keeps its heading in row
      (`TestTheClockKeepsItsHeadingInRow`); a file from before the clock gets
      it at the top once, and one that hid or moved it keeps that
      (`TestAnOldFileIsOfferedTheClockAtTheTop`, `TestAHiddenClockStaysHidden`);
      hidden, it is not polled, as any section
      (`TestAHiddenSectionIsNotPolledUntilShown`); the parity test passes
      with the date as a fact and no allow-list entry.
- [x] Real terminal: `hayami-tui --once --sections clock` prints the time and
      the date on one line in stack and in row.
- [x] Real window: see E2E.
- [x] Falsified: the card not bare (lost in row), the title not updated on a
      tick, the clock not offered to an old file, and the pacing ignored
      each fail a test.
- [x] README changelog under `### Unreleased` and its sections table;
      `docs/architecture.md`'s adding-things and status tables (the guide
      has no list of sections).

## Threat model delta

No boundary touched: the system clock is read, nothing is parsed or run.

## Risks & Assumptions

- 24-hour and year/month/day are fixed; there is no setting for either.
- A tick a second is a repaint a second of one row. The Network card already
  repaints at about that rate; the real-window check watches the panel's CPU
  for a minute with the clock on and off.
- Depends on fynedesygn spec 086: `Card.SetTitle` and `Card.SetTitleEnd`.
- Rollback is reverting the PR; a settings file naming `clock` in its order
  ignores an unknown key as it does today.

## E2E Test Plan

Done on the desk (Plasma 6.8 Wayland, scale 1.5), this branch's build on a
copy of the settings, which did not name the clock: the clock was at the top
(R1), its heading the time and the date in the title's bold grey, with grey
icons (R2, R3); three captures read 20:45:01, 20:45:04 and 20:45:07 (R4),
the window 621 px tall and the clock line's ink from x 21 to 361 in each
(R5). `hayami-tui --once --sections clock` in stack and row (R6).

### Prod visibility

Not applicable: a desktop program with no telemetry.
