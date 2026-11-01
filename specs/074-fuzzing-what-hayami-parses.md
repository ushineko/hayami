# 074 — fuzzing what hayami parses

**Issue**: #241

## Status: INCOMPLETE

## Context

Spec 073's boundary B4 is what hayami parses from sources it does not
control. A source that is hostile, or only broken, can make a parser panic,
hang or allocate without bound, and a panic in a reader takes the panel
down with it. For the network helper's protocol the source is a privileged
process; for the usage cache it is any program that writes the file; for
LibreHardwareMonitor it is whatever answers on the loopback port.

No fuzz test exists in the repository (measured on `main` at ac5caf3). The
parsers are covered by example-based tests, which check the inputs someone
thought of.

## Requirements

- R1 A Go fuzz test (`go test -fuzz`) for each parser below. The property
  checked is the same for each: no input panics, none runs longer than a
  bound, none allocates beyond a bound. Where a parser has an inverse (a
  line the helper writes and the panel reads), a round trip is checked too.
  - `core.ParseNetDev`: `/proc/net/dev`.
  - The helper protocol stream, both versions (`core.helperStream`): lines
    from the helper's pipe or socket.
  - `core.parseInetDiag`: `sock_diag` replies.
  - LibreHardwareMonitor's `data.json`, as `core` reads it.
  - The usage cache reader (`internal/usage`).
  - The Claude and Codex response parsing (`internal/claude`,
    `internal/codex`).
  - The settings file (`internal/config`), which a person edits by hand.
- R2 Each target has a seed corpus committed under its package's
  `testdata/fuzz`, taken from the example tests' inputs and from real
  output with anything identifying replaced (the repository's
  public-repository rules).
- R3 CI runs each target for a short, fixed time on every PR, in a job of
  its own, so a slow fuzz run does not hold the unit tests up.
- R4 A crash fuzzing finds is committed as a corpus entry with its fix, so
  it is a regression test from then on.
- R5 The HID report decoders are sanshoku's, and are raised there as an
  issue on that repository rather than fuzzed here.
- R6 `docs/threat-model.md`'s B4 section lists the targets and the parsers
  not yet fuzzed.

## Acceptance Criteria

- [ ] A fuzz test exists for each parser of R1, with a seed corpus.
- [ ] Each target was seen to find a planted panic (an index past the end on
      a crafted input) before the plant was removed.
- [ ] CI runs every target on PRs in its own job, and the job's time is
      recorded in Verification.
- [ ] Every crash found while writing the targets is fixed and committed as
      a corpus entry.
- [ ] An issue on sanshoku asks for fuzzing of its HID decoders.
- [ ] B4's section of the threat model lists the targets.

## Threat model delta

B4: each parser of R1 moves from "example tests only" to "fuzzed in CI".
Fixes to crashes found change the parsers; each fix is its own commit.

## Risks & Assumptions

- **CI time.** A few seconds per target on a PR; longer runs are by hand
  (`go test -fuzz=... -fuzztime=10m`) and before a release.
- **Corpus size.** Kept to the seeds and the crashers; the cache Go keeps
  between runs is not committed.
- **A crash in a parser is a fix to code that runs on desks**, found after
  the parser shipped. Each is released like any fix.
- Rollback: the fuzz tests and the CI job are reverted on their own; fixes
  stay.

## E2E Test Plan

1. Plant a panic in `ParseNetDev` on a branch; the PR's fuzz job fails and
   names the input. (AC 2, AC 3)
2. Remove it; the job passes. (AC 3)

### Prod visibility

Not applicable: a desktop program with no production deployment.
