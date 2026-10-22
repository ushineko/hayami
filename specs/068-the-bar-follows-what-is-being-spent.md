# 068 — the bar follows what is being spent

**Issue**: #224

## Status: COMPLETE

## Executive Summary

The usage card's stats row drops the words `limit:` and `left`, so the Codex
line reads `7d: 29 % (6d)  189.75 / 1200 (16 %) (10d)` and the panel can be
narrower. When a timed window is full and the account has an individual limit,
the bar shows the limit. Reviewers should look first at `leading` and
`spending` in `internal/view/usage.go`.

## Context

The CX meter's stats row read
`7d: 29 % (6d left)  limit: 189.75 / 1200 (16 %) (10d left)`. It was the
widest line on the card and held the panel's width. The amounts already say
what the limit is, and the parentheses already say the days are what is left.

The bar is the account's shortest window (spec 004). A Codex Business account
spends its individual limit once the five-hour window or the week is used up,
so from then on a bar at 100 % on the five hours shows a window that is no
longer the one being spent.

## Requirements

- R1 In the card's stats row, a window with amounts is written without its
  name: `189.75 / 1200 (16 %)`.
- R2 In the card, how long a window has left is written `(6d)` and `(<1d)`,
  without `left`. The terminal pane's strip keeps `(6d left)`, the form of the
  widget it replaces.
- R3 When any window with a stated length is at 100 % and the account has a
  `limit` window, the bar, the caption and the window name are the limit's.
  The full windows move to the stats row.
- R4 An account without a `limit` window keeps its shortest window on the
  bar, full or not.

## Acceptance Criteria

- [x] R1: `TestALimitBesideTheBarSaysItsAmounts` asserts
      `7d: 46 %  300.5 / 1200 (60 %) (6d)`.
- [x] R2: `TestALongerWindowSaysHowLongItHasLeft`,
      `TestDaysLeftAreRoundedUp` and `TestAWindowOnItsLastDaySaysSo` assert
      the bare form in the card and `(<1d left)` in the strip.
- [x] R3: `TestAFullWindowHandsTheBarToTheLimit` covers a full 5h and a full
      7d; falsified by disabling the rule (8 assertion failures).
- [x] R4: the same test, for a CC account with a full 5h.
- [x] The panel on a real window shows the shorter line and a narrower card
      (checked by the operator on the live panel).

## Risks & Assumptions

- A Codex utilization at 100 is taken to mean the window is exhausted; the
  app-server clamps it there (`internal/codex/normalise.go`).
- Claude's monthly spend is not given the same treatment: extra usage is
  opt-in and is not named `limit`. Out of scope.
- Rollback: revert the commit. View-only; no cache or settings format change.

## E2E Test Plan

Not applicable beyond the operator's check on the live panel: the change is a
formatting and selection rule in `internal/view`, with no integration
boundary. A full 5h window on the Business account will show the switch live
when it next happens.
