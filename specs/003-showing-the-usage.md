# Spec 003: showing the usage

**Issue**: [#6](https://github.com/ushineko/hayami/issues/6)

## Status: COMPLETE

## Executive Summary

`view.Meter` is a bar with a caption, drawn in all three arrangements; in a
pane it stretches, which is the shape the widget being replaced draws.
`internal/usage` decodes both providers' payloads into one neutral Window and
finds accounts by listing the cache, so nothing here opens a credential store.
`hayami-tui --sections usage` now shows what the shared cache holds, in both
shells. Reviewers should start with `internal/usage/payload.go`: two providers,
three payload shapes and two timestamp formats meet there and become one thing.

## Context

Spec 002 joined the cache and nothing draws from it. This spec draws from it
and still fetches nothing: the Python widget and the monitor are both running
and both keep the cache fresh, so reading is enough to make
`hayami-tui --sections usage` replace the pane in a herdr session today.

Splitting it here rather than doing the providers in one go is deliberate.
Fetching means the Anthropic OAuth refresh, which *writes* to
`~/.claude-credentials` — the only code in this project that can damage
something outside it. It should land on its own, against a section that
already works, rather than being debugged at the same time as a layout.

The intermediate state is honest and worth saying out loud: on a machine that
never had the Python, the usage section will be empty until spec 004. On the
machine doing the port it works immediately.

### What the payload holds

Read from this machine's own cache files, shapes only:

- **Claude**: `five_hour` and `seven_day`, each an object with `utilization`
  as a float percentage and `resets_at` as an ISO timestamp. A `limits` list
  carries `kind`, `group`, `percent`, `severity` and `resets_at`. Many other
  keys are present and null; several have obviously internal names. This
  decodes the two windows and ignores the rest, which is what holding `data`
  opaque in spec 002 was for.
- **Codex**: `primary` and `secondary`, each `utilization`, `window_minutes`
  and `resets_at` as an epoch integer, plus `individual_limit` with `used` and
  `limit` as *strings*. The app-server does not declare those units as
  currency, so nothing here adds a dollar sign — the program this replaces
  makes the same point in its own README.

Two payloads, two timestamp formats, one section. The decoding is per provider
and the drawing is not.

### Accounts without credentials

The cache filenames name the accounts: `usage-max.json` is the profile `max`.
So this spec discovers accounts by listing the cache directory and never opens
a credential store at all. Discovery from `~/.claude-credentials` arrives with
fetching, where it is actually needed.

## Requirements

- R1 `view.Meter`: a label, a caption, and a bar whose fill is a fraction and
  whose colour follows a status. It renders in all three arrangements; in
  `row` the bar stretches to the pane, as the widget's own terminal mode does.
- R2 A meter's caption obeys the no-jitter rule: it sets the section's width
  and must not change it as the numbers change.
- R3 `internal/usage` decodes the Claude payload into windows: a name, a
  fraction and a reset time.
- R4 `internal/usage` decodes the Codex payload the same way, including the
  individual limit's used and limit as reported units, without a currency
  symbol.
- R5 Accounts are discovered by listing the cache directory. No credential
  store is opened by this spec.
- R6 A usage section draws every discovered account and Codex, in both
  shells, labelled so two accounts cannot be mistaken for each other.
- R7 A reading's age is shown when it is stale, from `fetched_at`.
- R8 A section with no cache at all is not drawn, rather than drawn empty.

## Acceptance Criteria

- [x] AC1 A meter renders in stack, grid and row; in row its bar is the width the pane gives it. (R1)
- [x] AC2 A caption's width does not change between 5 % and 100 %, nor between one digit of hours and two. (R2)
- [x] AC3 The Claude payload decodes to its two windows from a fixture, and an unknown key does not break it. (R3)
- [x] AC4 The Codex payload decodes, its epoch resets read as times, and its used and limit carry no currency symbol. (R4)
- [x] AC5 Accounts come from the cache directory; the test builds its own directory and no credential store is read. (R5)
- [x] AC6 The usage section draws in both shells with one meter per window per account, and the parity test still passes. (R6)
- [x] AC7 A reading older than its window says its age. (R7)
- [x] AC8 No cache means no section. (R8)

## Gaps found

- **A card takes its objects at build time.** `glance.Card.AddObject` has no
  counterpart, so a card built from an empty section stays empty: the window
  drew no meters at all until `gui.Start` was changed to poll every source
  once before building. That fixed it for the shape a section has when the
  window opens, and a section that *gains* a meter later — an account
  appearing while the panel is running — still needs a restart. A card that
  could be given its pieces after it exists belongs in the library.

## What the payload taught us

Three things were found by drawing the real files rather than by reading the
Python, and each changed the code:

- **A Team account reports no windows at all.** `five_hour` and `seven_day`
  are null and `spend` carries the reading instead. The account was simply
  missing from the panel until that was decoded.
- **Claude declares its currency and Codex does not.** `spend.used` carries
  `"currency": "USD"` with an exponent; the Codex individual limit carries two
  strings and no unit. So one gets a symbol and the other does not, and that
  asymmetry is the source being reported faithfully rather than an
  inconsistency.
- **The credit cap has no reset in the payload.** Claude Code's own /usage
  screen derives first-of-next-month locally, and the widget does the same.
  `usage.NextMonth` is the only derived value in the package and says so.

## Risks & Assumptions

- **The payload is not ours.** Only the fields this spec names are decoded,
  and a missing one leaves its meter absent rather than failing the section.
  The canary from spec 002 is what reports a format change.
- **A usage figure is a fact about a person's account.** No fixture, golden
  file or screenshot in this repository carries a real one; the fixtures are
  invented, as `.claude/CLAUDE.md` requires.
- **Two timestamp formats.** Claude's resets are ISO strings and Codex's are
  epoch integers. Both become `time.Time` at the edge, so nothing downstream
  knows which provider it came from.
- **The pre-profile `usage.json` is dropped once named profiles exist.** The
  widget's own docstring says that file is "left unused" after the upgrade,
  and on this machine it was three weeks staler than the named ones —
  it was being drawn beside them, under the same heading, until this was
  fixed. It is still the only reading on a machine that never upgraded, so it
  is kept when it is all there is.
- **A meter label is a profile's own name**, not "Claude max". The label
  shares its line with a window name, a percentage, a countdown and sometimes
  an amount, in a panel 260 px wide.
- Rollback: revert. The section is drawn only when the settings name it.

## Alternatives Considered

- Fetching in this spec. Rejected: the credential write-back deserves to land
  alone, against a section that already works.
- Discovering accounts from `~/.claude-credentials`. Deferred to spec 004: the
  cache filenames already name them, and a spec that reads no credentials can
  be reviewed as one that cannot damage a credential store.
- Reusing `glance.Meter`'s formatter for the terminal. Rejected: it draws with
  Fyne, and the terminal panel builds with cgo off. The rule the two share is
  the width, and that lives in `internal/view`.
