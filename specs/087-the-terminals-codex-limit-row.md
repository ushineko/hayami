# 087 — the terminal's Codex limit row

**Issue**: #316

## Status: COMPLETE

## Executive Summary

The terminal's usage row now shows a Codex limit's amounts when the limit is on the bar, and
gives hours rather than `<1d` for a window that resets within a day. Only the pane's strip
changes; the desktop card is untouched. A reviewer should look first at `stripLeft` in
`internal/view/usage.go`.

## Context

The terminal panel's `row` arrangement draws a usage account as one strip (spec 019).
When a Codex window fills, the account's individual limit takes the bar (spec 023).
At that point the strip reads `CX  limit  15%  ·  5h 100% (<1d left)  ·  7d 30% (6d left)`,
which has two faults:

- The limit shows only its percentage. When the limit is not on the bar, the strip
  shows its amounts (`individual 300.5/1200 (60%)`).
- A window that resets within a day says `(<1d left)` (issue #102, which copied the old
  widget). For the five-hour window that is always true, so it tells the reader nothing.

The desktop card is not affected and is not changed. Its stats row already puts the
limit's amounts at the right, and its captions keep the days form.

## Requirements

- R1 When the limit is the bar's window, the strip's first figure is its amounts and
  percentage, `300.5/1200 (60%)`. The window column already says `limit`, so the figure
  is not prefixed with `individual`. A limit without amounts keeps the bare percentage.
- R2 In the strip, a window that resets in under a day says how many whole hours are
  left, rounded up: `(4h left)`. Under an hour it says `(<1h left)`. A day or more keeps
  the days form, `(6d left)`.
- R3 The desktop card's captions and stats row are unchanged.

## Acceptance Criteria

- [x] A Codex strip whose bar is the limit reads `720/1200 (60%)` as its first figure,
      and its 5h window reads `5h 100% (2h left)` for a reset two hours away
      (`internal/view/usage_test.go`).
- [x] The strip's hours are rounded up: 90 minutes is `2h`, exactly 3 hours is `3h`,
      one minute is `<1h`, and 36 hours is still `2d`.
- [x] The card's `StatsLeft` still reads `(<1d)` for a window on its last day.
- [x] Every test passes (`make test`), and `make lint` is clean.

## Risks & Assumptions

- Rollback is a revert of one commit. No stored data or settings are involved.
- The strip is not fixed width (spec 019, issue #75), so a figure that changes length
  in the terminal is expected. `(4h left)` is no wider than `(<1d left)`.

## Threat model delta

No boundary touched.

## E2E Test Plan

- Run `hayami-tui` in the `row` arrangement with the usage section shown, against an
  account whose five-hour window is full. Expected: the CX line reads
  `limit  <used>/<limit> (<pct>)  ·  5h 100% (<n>h left)  ·  7d …`. Covers AC 1 and 2.
  If no account is full at the time, the unit tests stand in and the step stays pending.
- Result (2026-11-06): `hayami-tui --sections usage --arrangement row --once` against the
  live account printed `CX  limit <used>/1200 (15%)  ·  5h 100% (2h left)  ·  7d 30% (6d left)`.
  Passed.
