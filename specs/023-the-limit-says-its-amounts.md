# 023 — the limit says its amounts

**Issue**: #90

## Status: COMPLETE

## Context

Spec 019 made the pane's usage row say what the usage widget's `--tui`
says: for a Codex Business account, `individual 300.5/1200 (60%)`. The
window's meter has a stats line built by `view.figure`, which prints a
window's name and percentage and nothing else, so the same account's line
in the window ends `limit: 60 %` with the amounts missing. The pane and the
window are two views of one reading and should say the same thing.

## Requirements

- R1 `view.figure` (the stats line's per-window text) says the amounts when
  the window has them: `Name: Used / Limit (pct)`, so `limit: 300.5 /
  1200 (60 %)` (R3 decides the `1200`); a window without amounts is unchanged (`7d: 46 %`). The
  remaining-days parenthetical stays after it as now.
- R2 The lead window's amounts already appear in `StatsRight` from
  `Detail`; a lead window is not in `StatsLeft`, so nothing is said twice.
- R3 A `Limit` with no decimals is printed as the pane prints it (the pane
  shows `1200`, the widget `1200.00`; follow spec 019's rule, whichever it
  chose, and say which in the spec). **Decided**: spec 019's rule is the
  widget's compact form (`internal/usage.reported`: two decimals at most,
  trailing zeros dropped), and `Used`/`Limit` already arrive in it, so the
  stats row prints them as they are: `1200`, not `1200.00`. `Detail` keeps
  its two decimals and is unchanged.
- R4 README changelog under `### Unreleased`.

## Acceptance Criteria

- [x] `make test`, `make lint`, `make build` pass.
- [x] `view` test: a Codex group with a `limit` window carrying `Used` and
  `Limit` yields a `StatsLeft` containing the amounts and the percentage; a
  group without amounts is unchanged; the existing spec 019 pane tests
  still pass. (`TestALimitBesideTheBarSaysItsAmounts`, falsified by
  dropping the amounts from `figure`.)
- [x] `hayami-tui readings` on this desk shows the Codex limit window with
  amounts, and the window's meter line says them: the desktop panel is
  launched and photographed (by PID, closed afterwards) with the usage card
  showing `limit: <used> / <limit> (<pct>)`; the amounts in the photograph
  are the desk's and are noted, not reproduced, in the spec. Photographed
  at review, 2026-09-30, beside the user's own panel: the line sits next to
  the 7d figure and fits the panel's width; the photograph is not committed.
- [x] README and changelog in the same commit; spec reconciled.

## Risks & Assumptions

- **The stats line gets longer** by the amounts. The meter's stats line
  already wraps its two halves; if it truncates at the panel's width, the
  spec records the width and the fix is fynedesygn's meter, not shorter
  text.
- **Rollback**: revert.

## Implementation notes

- The stats row and the caption were one function, `figure`. The caption is
  the lead window's text and must not gain the amounts (they are its
  `Detail`, at the right of the same row, and the caption is fixed width), so
  it is now `caption`, which is the old `figure`; `figure` is `caption` plus
  the amounts where `Used` and `Limit` are both set. `spread` is the only
  caller of `figure`.
- `figure` keys on `Used` and `Limit`, not on the name `limit`, so a spend
  beside a plan's windows also says its amounts in the stats row
  (`spend: $0.00 / $1000.00 (0 %)`). The pane's strip hides a spend with
  nothing spent; the stats row does not.

## Verification

- `make test`: every package `ok`.
- `make lint`: `0 issues.` with a fresh cache
  (`GOLANGCI_LINT_CACHE=<empty dir>`). With the shared cache it reported 8
  findings in files of a removed worktree (`feat-021-bandwidth-trend`), all
  on lines that carry `//nolint`: a stale cache keyed by content, not this
  change.
- `make build` and `CGO_ENABLED=0 go build ./cmd/hayami-tui`: build.
- `hayami-tui readings --sections usage` on this desk has one window named
  `limit` with `Used`, `Limit` and `Fraction` set; `Limit` arrives in the
  compact form (no decimals), which is what R3 needed to know. The values
  are the desk's and are not reproduced here.
- The photograph of the desktop panel is the reviewer's.
