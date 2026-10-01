# 028 — the weekly window always says how long it has left

**Issue**: #102

## Context

Seen on the desk on 2026-09-30, with the Max account's weekly window about
fourteen hours from its reset. The window's card and the pane's stack and
grid said `7d: 52 %` and nothing else; the pane's row said `0%  ·  7d 52%`.
The Codex line beside them said `7d: 37 % (4d left)`, so the suffix works
and had simply gone quiet for the day it matters most.

Two causes. `view.remaining` floored the time to whole days and returned
nothing under a day, so at thirty-six hours it said `1d left` and on the last
day nothing. And spec 019 rebuilt the pane's row in the widget's words and
left out the widget's own suffix, so the row has never said it at all.

The widget this program replaces (`peripheral-battery.py`,
`get_days_until_reset`) rounds up and never goes silent before the reset:
`7d: 52% (2d left)` at thirty-six hours, `7d: 52% (<1d left)` on the last
day. That is what a reader of these numbers has been looking at for months.

## Requirements

- `remaining` rounds up to whole days and says `<1d left` under a day, as the
  widget does. A reset that has passed says nothing, as before. A window whose
  reset is the one already in the countdown column still says nothing, as
  before: the column is the answer.
- The pane's row appends ` (Nd left)` to a window figure after the bar's own,
  in the widget's form (`7d 85% (4d left)`), dim, with the same two
  exceptions.
- The widget-parity tests (`TestAStripSaysWhatTheWidgetSays`, the real-TUI
  pane test) assert the suffix.

## Acceptance Criteria

- [x] Thirty-six hours is `2d left`, a day is `1d left`, four days is
      `4d left`, a minute is `<1d left`, and a reset that has passed is
      nothing (`TestDaysLeftAreRoundedUp`)
- [x] The card and the row both say `<1d left` on the last day
      (`TestAWindowOnItsLastDaySaysSo`; failed before the fix with
      `7d: 21 %` alone)
- [x] The row's strip carries the suffix as a dim figure of its own
      (`TestAStripSaysWhatTheWidgetSays`, `TestEachFigureInAStripHasItsOwnColour`)
- [x] The real `hayami-tui` in a 200×6 tmux pane prints `7d 85% (4d left)`
      (`TestTheUsagePaneIsLaidOutLikeTheWidgets`)
- [x] A weekly window that resets with the bar's window says no more in
      either form (`TestALimitBesideTheBarSaysItsAmounts`)
- [x] `make test`, `make lint` pass

## Risks & Assumptions

- A figure five days and an hour out now reads `6d left` where it read
  `5d left`. That is the widget's reading and the change the issue asks
  for; two existing assertions were updated to say so.
- The row gains up to eleven columns of text (` (<1d left)`) before the
  reset column. Spec 019's narrow-pane rule already cuts figures with an
  ellipsis when they do not fit, and the 200-column pane test still fills
  every line exactly.
- Rollback: revert the commit. No settings, cache or payload change.

## Alternatives Considered

- Hours under a day (`13h left`): more precise, but diverges from the widget
  the pane is specified to match; the user chose parity.

## Status: COMPLETE
