# 039 — Thresholds as tables

**Issue**: #127

## Status: COMPLETE

## Context

Every reading that has a verdict chose it with a hand-written ladder of
comparisons and constants of its own. The architecture review after spec 037
counted five; converting them found a sixth. On `main` at 06a82a8:

| Reading | Where | Rule |
|---|---|---|
| A rate's emphasis | `internal/view/bandwidth.go:278` `RateBand` | Info below 1 MiB/s, Accent from it, Warn from 10, Strong from 100 (`>=`) |
| The Wi-Fi bars | `internal/view/bandwidth.go:140` `SignalLevel` | 1–4 bars from the RSSI at −75/−67/−55 dBm, or from Windows' percentage at 25/50/75 where there is no RSSI (`>=`) |
| The bars' colour | `internal/view/bandwidth.go:172` `signalStatus` | Warn for one bar, Info otherwise |
| Coolant | `internal/view/cooler.go:178` `coolant` | Good, Warn from 50 °C, Bad from 55 (`>=`) |
| A battery's level | `internal/view/peripherals.go:392` (in `peripheral`) | Bad at 20 % and below, Warn to 50, Good above (`<=`) |
| A battery's band | `internal/view/peripherals.go:451` (in `bandCell`) | Bad at one segment, Warn at two, Good from three |
| A quota | `internal/view/usage.go:275` `quota` | Good, Warn from 0.50, Bad past 0.80 (`>=` then `>`) |

They used three different comparisons (`>=`, `<=`, `>`), sometimes in one
ladder. Each new reading would have added another ladder.

Searched for others: every function in `internal` that returns a `Status`
(`trailStatus`, `cellStatus` and `verdict` choose by state, not by a number,
and `verdict` falls through to `quota`), and every `Status:` set in `panel`
(fixed statuses on reasons, not thresholds). None.

## Requirements

- R1 `view.Bands[T]`: a floor and ascending steps, each step `{From, Above,
  To}`. A value takes the `To` of the last step it reaches, and the floor below
  the first. A step is reached at `From`, or only above it when `Above` is set.
  One ascending form then says `>=`, `>` and, with the bad verdict as the floor,
  "at most". `T` is the verdict: a `Status`, or an `int` for a count of bars.
- R2 Each ladder above becomes one exported table: `RateBands`,
  `SignalRSSIBands`, `SignalPercentBands`, `SignalEmphasis`, `CoolantBands`,
  `BatteryBands`, `SegmentBands`, `QuotaBands`. The functions that held the
  ladders now read the table. The threshold constants stay (`RateNotable`,
  `CoolantWarm`, `BatteryLow`, `SignalGood` and the rest); the tables are made
  from them.
- R3 Nothing draws differently. Every status at, below and above every
  threshold is pinned by a test written against the ladders before they
  changed.
- R4 Every table ascends, and a test holds them to it. `Of` stops at the first
  step a value does not reach, so a step out of order would be skipped.

## Acceptance Criteria

- [x] `internal/view/bands_boundary_test.go` passes against the ladders on
  `main` (committed first, 3f5511b) and unchanged against the tables.
- [x] `TestEveryTableAscends` and `TestBandsReachAStepAtItsFromOrAboveIt` pass.
- [x] Falsified:
  - `BatteryBands`' first step without `Above` fails at 20 %.
  - `QuotaBands`' last step without `Above` fails at 0.80.
  - `SegmentBands`' first step from 1 fails at one segment.
  - A step out of order in `SignalRSSIBands` fails the ascending test.
  Each passes again once restored.
- [x] `go test -tags migrated_fynedo ./...` on Windows; Linux vet and
  golangci-lint on both platforms.

## Risks & Assumptions

- **No behaviour change**, which is what R3's tests are for. The battery
  ladder said "at most 20"; the table says "above 20 is Warn", the same
  boundary for integer levels and for any value.
- **The tables are exported variables**, like `SparkRunes`. A later spec can
  read them from settings; nothing changes them today.
- **Rollback**: revert the commit. The tests that pin the boundaries stay
  valid against the ladders.

## Gaps found

None for the library. Spec 037's signal percentage thresholds (25/50/75) have
no named constants, unlike the RSSI's; they live in `SignalPercentBands` only.

## Verification

2026-10-07, Windows 11, Go 1.26.0:

- The boundary tests passed against the ladders before any change, then
  against the tables.
- `go test -tags migrated_fynedo ./...`: every package ok except
  `internal/usage`'s Python cross-check, which fails on `main` on this machine
  too (its Python lacks `structlog`).
- `GOOS=linux CGO_ENABLED=0 go vet ./internal/view/...`: clean.
- golangci-lint v2.12.2, `GOOS=windows` and `GOOS=linux`: 0 issues.
