# 024 — the provider at a glance

**Issue**: #93

## Status: COMPLETE

## Context

The usage card stacks one meter per account. Its label is the account's
own name (`usage.Account.Label`): a Claude profile's name, "max" or
"work", with its badge and the lead window after it, and for Codex the
word "Codex", because it is the other provider and says which it is. Read
down a column of three accounts, the provider is not something the eye
picks out: two names and a word, none of them the same shape.

Every label leads with a two-letter provider shorthand instead: `CC` for
Claude Code, `CX` for Codex. A Claude account keeps its name and badge
after it; the Codex account's "Codex" is replaced by the shorthand rather
than said twice. Both shells, since both draw `Meter.Name()`.

## Requirements

- R1 `usage.Account.Label` returns the shorthand and then the name: `CC
  max`, `CC work`, `CC` for a Claude account with no name; `CX`, or `CX
  <name>` for a Codex account with one. Two exported constants in `usage`
  hold the shorthands with a comment on the choice (`CC` is the product's
  own abbreviation; `CX` is Codex's).
- R2 Nothing else changes: `Meter.Name()` still joins label, badge and
  window, so the lines read `CC max M 5h`, `CC work E spend`, `CX 5h`. The
  pane's label column and the window's `MeterLabelWidth` take the two extra
  characters; if the window's label width truncates, the spec records the
  measurement and `MeterLabelWidth` grows by what the shorthand needs.
- R3 The usage cache shared with the Python widget is unchanged; the
  account filenames are the same.
- R4 README: the usage row in "What it does" and the changelog under
  `### Unreleased`; any README or docs text that quotes a meter label is
  updated.

## Acceptance Criteria

- [x] `make test`, `make lint`, `make build` and the cgo-free `hayami-tui`
  build pass.
- [x] `usage` test: the four label cases in R1.
- [x] `view` and `gui` tests that quote a meter label are updated and
  pass; the parity test passes.
- [x] `hayami-tui --sections usage --arrangement row` and the column pane
  on this desk show `CC` and `CX` leading every line (pasted with the
  figures replaced by `<n>`).
- [x] The desktop panel is launched and photographed with the usage card
  showing the shorthands and no truncated label (by PID, closed
  afterwards; the photograph is not committed, it carries spend figures).
  Photographed at review, 2026-09-30, beside the user's own panel: `CC max M
  5h`, `CC work E spend`, `CX 5h`, no label truncated, window 283 px wide as
  before. Not committed (spend figures).
- [x] README and changelog in the same commit; spec reconciled.
  usage row and the `### Unreleased` entry are written; the commit and the
  final reconciliation are the reviewer's.

## Risks & Assumptions

- **Three characters more per label** on a 260 px panel. R2 says what to do
  if it truncates.
- **Rollback**: revert.

## Verification

**Label.** `usage.Account.Label` is the shorthand, then the name when there
is one; the shorthands are `usage.ShortClaude` (`CC`) and `usage.ShortCodex`
(`CX`). `TestALabelLeadsWithTheProvider` holds the four cases of R1: `CC max`,
`CC`, `CX`, `CX team`. The cache's filenames come from `Slug` and
`account()`, neither of which changed (R3).

**Tests quoting a label.** `internal/usage/accounts_test.go`,
`internal/panel/reasons_test.go`, `internal/view/{usage,strip,paint}_test.go`
and `tests/structure/pane_test.go` (the tmux pane against the real binary)
now quote `CC max`, `CC work`, `CX`. No `internal/gui` test quoted a meter
label. The strip's bar-width test subtracts the wider name column, 11
(`CC max  M  `) rather than 8. `TestFeatureParity` and
`TestTheUsagePaneIsLaidOutLikeTheWidgets` pass.

**The window's label width.** Measured with `fyne.MeasureText` at the panel
theme's text size (fynedesygn BreezeDark, 12), in a throwaway test not
committed:

| Label | Width |
|---|---|
| `work E spend` (widest before) | 75.7 |
| `CC max M 5h` | 73.8 |
| `CC work E spend` (widest now) | 93.7 |
| `CX limit` | 42.6 |
| `CC ` (the shorthand and its space) | 18.0 |

`MeterLabelWidth` was 84, 8.3 of air past the widest label. `CC work E spend`
at 93.7 overflows it by 9.7; the fixed width is a floor, not a clip, so the
label would not be cut but its caption would be pushed out of line with the
others. `MeterLabelWidth` grows by the 18 the shorthand needs, to 102, which
restores the same 8.3 of air. A usage meter built with invented figures
(headless, `Object().MinSize()`) measures 18 wider as a result: the widest,
the `work` spend meter, 235.8 before the shorthand and 253.8 after. Whether
that moves the window past the rest of the panel is the photograph's to say.

**Checks.**

```
$ make test
ok  	github.com/ushineko/hayami/internal/usage
ok  	github.com/ushineko/hayami/internal/view
ok  	github.com/ushineko/hayami/tests/structure
... (every package ok)
$ make lint
0 issues.
$ make build
CGO_ENABLED=1 go build ... -o hayami ./cmd/hayami
CGO_ENABLED=0 go build ... -o hayami-tui ./cmd/hayami-tui
$ CGO_ENABLED=0 go build ./cmd/hayami-tui
(ok)
```

**The panes on this desk**, 120 columns, every figure, amount, time and bar
length replaced (`<n>`, `<bar>`):

```
$ hayami-tui --sections usage --arrangement row --once
CC max  M  5h <bar> <n>%  ·  7d <n>%                                               resets <n>
CC work E     <bar> <n> / <n> (<n>%)                                  resets <n>
CX         5h <bar> <n>%  ·  7d <n>%  ·  individual <n> / <n> (<n>%)            resets <n>
```

```
$ hayami-tui --sections usage --once
Usage
CC max M 5h     5h: <n>%  7d: <n>% · in <n>
<bar>
CC work E spend spend: <n>%  <n> / <n> · in <n>
<bar>
CX 5h           5h: <n>%  7d: <n>% (<n> left)  limit: <n> / <n> (<n>%) · in <n>
<bar>
```
