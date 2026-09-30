# 019 — the usage pane matches the widget

**Issue**: #75

## Context

`hayami-tui --sections usage --arrangement row` exists to replace
`claude-usage-widget-windows --tui` in a herdr pane. Set beside it at 200
columns, the two did not say the same thing:

| | widget `--tui` | hayami `row` before |
|---|---|---|
| name | `max  M  5h`, a budget's window blank | `max   M 5h`, a budget named `spend` |
| figures | `12%  ·  7d 40%` | `5h: 12 %` |
| budget | `$250.00 / $1000.00 (25%)` | `spend: 25 %` |
| Codex | `0%  ·  individual 300.5/1200 (60%)` | `5h: 0 %` |
| colour | each figure its own, separators dim | the whole caption one colour |
| reset | `resets 2h 30m`, `resets Oct 1` | `in   2h 30m`, `1 Oct` |
| spacing | slack split 3:1 between bar and a gap before the reset | slack all to the bar |

The row drew `Meter.Caption` alone. The seven-day figure, the amounts and the
individual limit were computed into `StatsLeft`/`StatsRight` for the window's
card and never reached a pane. (The figures in this table are invented, in the
shapes each program prints.)

## Requirements

### A meter carries its row form

- `view.Meter` gains `Strip`: the window beside the name, the figures after the
  bar as pieces each with its own `Status`, and the reset text.
- `view.Usage` builds it in the widget's words: the bar's own percentage bare
  (`12%`), each other window after a dim `  ·  ` (`7d 40%`), a budget as
  `$used / $limit (N%)` with no window name, a Codex limit as
  `individual used/limit (N%)`, a spend beside a plan's windows as its amount
  only when non-zero.
- The reset is `resets Xh Ym` / `resets Ym` / `resets Xd Yh` for a window with a
  length and `resets Mon D` for an allowance without one.
- Percentages are not padded (`12%`), as the widget prints them. This form only;
  the window's captions stay fixed width.

### The row lays a strip out the way the widget does

- Name column: account names padded to each other where there is a badge, then
  the badge; an account with no badge is its bare name. Two spaces, the window
  column, a space, the bar.
- The slack after the fixed columns is split three to one between the bar and a
  gap before the reset, rounded up as rich rounds it. The reset is right-aligned
  at the edge.
- The bar is filled in the lead window's status and its track dim; the name is
  plain; the reset is dim.
- Too narrow for an 8-column bar: the bar goes, the figures stay, the reset
  goes if it does not fit, then the figures are cut with an ellipsis. Every line
  is exactly the pane's width.
- A meter without a strip is drawn as before.

### Colour bands are the widget's

- `quota`: amber from 50 %, red past 80 % (was 80 % / 95 %), in both shells.
- A Claude spend is coloured by the payload's `spend.severity`
  (`normal` / `warning`, `elevated` / `critical`, `exceeded`), falling back to
  the bands when absent or unknown. This applies to the meter's status too, so
  the window's bar follows it.

### The payload keeps what the pane needs

- `usage.Window` gains `Used`, `Limit` and `Severity`. A Claude spend's amounts
  are as `Detail` already formats them (`$250.00`); a Codex limit's are the
  widget's compact form (`300.5`, `1200`). `Detail` is unchanged.

## Acceptance Criteria

- [x] The row form's figures, window and reset match the widget for a plan, a
      budget and a Codex limit (`TestAStripSaysWhatTheWidgetSays`)
- [x] Each figure is coloured for itself and separators are dim
      (`TestEachFigureInAStripHasItsOwnColour`)
- [x] A budget is coloured by its severity (`TestABudgetIsColouredByItsSeverity`)
- [x] A spend beside a plan shows its amount only once one exists
      (`TestASpendBesideAPlanIsItsAmountOnceThereIsOne`)
- [x] Name, badge and window columns match the widget
      (`TestAStripLineBeginsTheWayTheWidgetsDoes`)
- [x] Lines fill the pane, bars end in one column, resets end at the edge
      (`TestAStripLineIsLaidOutInColumns`)
- [x] The bar is `ceil(3/4)` of the slack at every remainder
      (`TestTheBarTakesThreeQuartersOfTheSpareWidth`, falsified with
      round-to-nearest: fails at 203 columns)
- [x] A narrow pane keeps the figures (`TestANarrowPaneKeepsAStripsFigures`)
- [x] Quota bands are 50 / 80 (`TestAQuotaNearItsLimitIsMarked`, falsified by
      restoring 80 / 95)
- [x] The payload keeps amounts and severity
      (`TestALimitsAmountsArriveApartInTheWidgetsForm`,
      `TestASpendKeepsItsAmountsAndItsSeverity`)
- [x] The real `hayami-tui` in a 200×6 tmux pane, reading an invented cache
      under `t.TempDir()`, draws the three lines in this layout
      (`TestTheUsagePaneIsLaidOutLikeTheWidgets`; falsified by disabling the
      strip: 9 assertions fail)
- [x] Side by side with the widget on a live cache at 200 columns, the two panes
      agree column for column apart from the deviations listed below (checked
      by hand; not recorded, the figures are an account's)

## Deviations kept

- **Codex's secondary window is shown** (`0%  ·  7d 18%  ·  individual …`). The
  widget reads only the primary; dropping a window hayami already reads would
  hide a weekly limit near its cap.
- **The track is `─`, not grey `━`.** Spec 005's decision: the bar has to read
  without colour. Rich's half-cell caps (`╸`, `╺`) are not drawn either.
- **Staleness stays a `read … ago` line** for the section rather than the
  widget's per-account `(cached 3m ago)` in the reset column. hayami knows one
  age for the section, the oldest.
- **No per-model breakdown** (`opus 12% sonnet 19%`). hayami does not decode
  `seven_day_opus` / `seven_day_sonnet`; the widget shows them only when
  non-zero.

## Risks & Assumptions

- The colour bands change the window as well as the pane. It is a threshold,
  not a layout change: nothing moves, a 50–80 % quota turns amber.
- The strip's percentages are not fixed width, so a figure gaining a digit moves
  the end of the bar by one column. Chosen to match the widget; confined to the
  row arrangement.
- `strip` keys on the window names `spend` and `limit`, which `internal/usage`
  assigns. A rename there is caught by `TestAStripSaysWhatTheWidgetSays`.
- Rollback: revert the commit. No settings, cache or format change.

## Status: COMPLETE
