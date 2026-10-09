# 054 — A stale usage reading stays in place

**Issue**: #172

## Status: COMPLETE

## Context

Once the usage cache was more than `UsageStale` (five minutes) old, the usage
section added a row, "read 12m ago", above its meters, and took it away when
the cache was refreshed. That breaks the glance rule that nothing transient
reflows the panel: the window grew and shrank by a line, and in a herdr pane
one line tall (`row` with only usage selected) the row was all there was.

## Requirements

- R1 `view.Section` gains `Stale bool` (json `stale,omitempty`): a reading of
  this run that its source considers out of date. `Dimmed()` includes it, so
  the terminal draws a stale section dim, as it does a restored one.
- R2 `view.Usage` adds no row for age. A stale reading sets `Stale` and
  carries an aside reason, Label "Usage", Text "stale", Detail "read 12m ago":
  off the card, in the hover note (built from the details) and in `doctor`.
- R3 The usage source puts the view's reasons after its own.
- R4 The window tells a stale card it is last-known (`SetLastKnown`), with
  none of Restored's other meanings: the source still counts as heard and its
  reading is still written to the window's cache.

## Acceptance Criteria

- [x] A stale reading has no rows, is `Stale` and `Dimmed`, and has the age
  in an aside reason and the hover note; a fresh one has neither
  (`internal/view/usage_test.go`).
- [x] Stack, grid and row at width 80 take the same number of lines fresh
  and stale, and no line says "read" (`TestAStaleReadingTakesNoMoreLines`).
- [x] Headless window: fresh is not dim, stale is dim but not marked gone,
  with the rows of the fresh card and the age in the tip, and a refresh
  clears it (`TestAStaleUsageReadingIsDimInPlaceWithItsAgeInTheTip`).
- [x] The parity fixture's usage section is stale, and both shells still draw
  the same facts.
- [x] `doctor` prints "Usage: stale" and "read 12m ago".
- [x] Real window: the panel run over an invented cache read 1 and 12 minutes
  ago is the same height (132 px) with the same four bands of text, and the
  usage source polled over the same cache confirms the second run is stale
  (`TestAStaleUsageReadingTakesTheLinesAFreshOneTakes`).
- [x] Falsified: with the row put back, the window test fails (132 px against
  162 px, four bands against five) and so do the view tests.

## Risks & Assumptions

- **The window's card does not look dim.** See the gaps.
- **Other sections**: no other section adds or removes a row on age. The
  cooler's and the peripherals' stale readings already keep their place and
  dim (`Row.Stale`, `Cell.Stale`), and the reasons that come and go (a fetch
  that failed, a gate that is closed) are drawn only when an account has no
  meters to draw.
- **Rollback**: revert; the row comes back.

## Gaps found

- **fynedesygn's `glance.Meter` has no dim state**, and `Card.SetLastKnown`
  dims rows only. A usage card is all meters, so the window marks it
  last-known (`Card.Stale()` is true) but paints it exactly as it paints a
  fresh one. A restored usage card has always looked like this. The fix
  belongs in the library: a meter that takes the card's dimming, the way a
  cell takes `Reading.Stale`.
- With the old row put back, the "read" row was drawn bright on a card told
  to dim. A row inserted after `SetLastKnown` is not dimmed. Nothing draws
  rows that way after this change, but a restored card that gains a row
  would.

## Verification

2026-10-08, Windows 11 Pro 26200, Go 1.26.0: the full suite passes except
`internal/usage`'s cross-check against the Python, which fails on `main` too.
Window tests pass with `HAYAMI_WINDOW_TEST=1`. golangci-lint v2.12.2 reports
nothing on Windows, or on Linux for the packages that build without cgo. Linux
vet passes, gofmt lists nothing, and `hayami-tui` builds with `CGO_ENABLED=0`
for Windows and Linux. The window pictures were looked at: fresh and stale
are the same; the falsified stale picture shows the extra "read 12m ago" line.
