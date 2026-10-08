# 049 — The window draws row

**Issue**: #153

## Status: COMPLETE

## Context

hayami has three arrangements, and its rule is that every section renders in
all three in both shells (docs/architecture.md, rule 3). The window drew two:
`arrangement()` in `internal/gui/gui.go` mapped `row` to `glance.Stack`,
because the design system had no panel arrangement for one line per reading,
and building one in `internal/gui` from glance's pieces is what the design
system's rules forbid. Spec 047 recorded the gap: the preferences marked `row`
"terminal only", and the parity test allowed five differences in row (titles,
detail lines, and a meter's caption, reset and stats).

fynedesygn v0.1.86 (its spec 057, #170) adds `glance.Lines`: card headings,
plots and any object that is not a row, meter or cell grid hidden; rows
unchanged, so their IDs (spec 044) survive a switch; a meter one line (label,
bar, caption, trailing value; no stats row); a cell grid one cell per line
with aligned bars. `Panel.SetArrangement` hands the arrangement to every card
and resizes once.

## Requirements

- R1 Require fynedesygn v0.1.86. `arrangement()` maps `view.ArrangeRow` to
  `glance.Lines`.
- R2 **Detail lines.** The window flattens a row's detail lines (a Wi-Fi
  interface's totals and link line) into rows of their own. `glance.Lines`
  draws rows, so it would draw them; the terminal's row is one line per
  reading and does not. The window drops them in row, so the two shells
  agree, which is what the row arrangement is for (spec 019's pane). The
  panel keeps each card's rows before flattening (`card.lines`), and a change
  of arrangement rebuilds every card's rows by ID before the arrangement is
  set, so the arrangement's one resize covers both.
- R3 The preferences no longer mark `row` "terminal only".
- R4 The parity test draws the window in each arrangement, by applying the
  setting as the preferences do, and compares it with the terminal in the
  same arrangement. In stack and grid every fact must be in both shells; in
  row the two must agree, a fact both leave out being the arrangement's
  shape. Keys name the shell that lacks a fact and the arrangement
  ("terminal row: meter caption"). The stale-entry check covers both shells.
- R5 A real-window test that the window draws row as a line per reading.
- R6 README "Arrangements", docs/architecture.md (rule 3, the Status row and
  the known-gap paragraph), changelog.

## Acceptance Criteria

- [x] In row the window draws no card heading and no plot, and the cooler's
  card is as many lines as the section has rows; in stack it is those and a
  heading (real window, `tests/window/row_windows_test.go`).
- [x] In row the window leaves out a reading's detail lines and they come
  back on leaving it (headless, `TestRowLeavesOutDetailLinesAndStackBringsThemBack`).
- [x] The parity allow-list is what is true now:

  | Before (spec 047) | After |
  |---|---|
  | `row: title` | gone: both shells omit headings in row |
  | `row: detail` | gone: both omit detail lines in row (R2) |
  | `row: meter stats` | gone: both omit a meter's stats in row |
  | `row: meter caption` | `terminal row: meter caption` |
  | `row: meter reset` | `terminal row: meter reset` |

  The two that remain are spec 019's: in a row the terminal spells a usage
  meter as the usage widget's line ("10%", "resets Oct 7") where the window
  keeps the card's ("5h: 10 %", "in 2h 0m").
- [x] Falsified: with row mapped back to stack, the real-window test fails
  (the row picture has the heading) and three headless tests fail (the
  arrangement, the detail lines, and parity: `terminal row: title`,
  `terminal row: detail`, `terminal row: meter stats` reappear); with detail
  lines kept in row, the detail test and parity (`terminal row: detail`)
  fail. Restored each time.
- [x] Screenshots looked at: the panel in row and in stack with all four
  sections, and the cooler alone in each.

## Risks & Assumptions

- **Plots.** In row the window hides its plots (`glance.Lines`), where the
  terminal draws each trend as a named line. A plot is drawn, not written,
  and the parity test compares text, so it is not an allow-list entry; it is
  recorded here and in the guide.
- **Switching while shown** is checked headlessly: the rows are rebuilt
  before `SetArrangement`, whose single resize the library tests. A
  real-window check would need to drive the preferences window; the panel
  applies a changed setting only through it, not by watching the file.
- **Rollback**: revert; v0.1.86 is additive in the library, so requiring it
  changes nothing else.

## Gaps found

- The real-window row test has its own launcher (`startArranged`), because
  the shared `panelSettings` has no arrangement and the harness was being
  changed in parallel (#149). Fold it in when that lands.
- The terminal spells a usage meter differently in row (spec 019); making the
  window's row use the same spelling is a choice for a later spec.

## Verification

2026-10-08, Windows 11, Go 1.26.0, fynedesygn v0.1.86:

- `go test -tags migrated_fynedo ./...`: every package ok but
  `internal/usage`'s Python cross-check, which fails on `main` on this desk
  too.
- Window tests with `HAYAMI_WINDOW_TEST=1`: the row test passes (stack: four
  text lines, heading and three rows; row: three); the others as noted in the
  PR.
- Pictures: in row the four sections are a line per reading with no
  headings or plots; Wi-Fi shows its bars and rates and no totals or link
  line; the peripherals are one cell per line; in stack the same panel shows
  headings, detail lines and plots as before.
