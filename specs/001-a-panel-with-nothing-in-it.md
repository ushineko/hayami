# Spec 001: a panel with nothing in it

**Issue**: [#1](https://github.com/ushineko/hayami/issues/1)

## Status: INCOMPLETE

## Executive Summary

(Populated before the PR opens.)

## Context

hayami replaces two programs. `ag-scripts/peripheral-battery-monitor` is a
frameless PyQt6 panel of four sections; `ag-scripts/claude-usage-widget-windows`
is a Qt widget with two terminal modes, `--tui` (a full-width row per account,
self-refreshing, owning the pane) and `--line` (one shot). A herdr pane runs
the first of those today.

Reading both, the shapes are the same data laid out differently, which is the
finding this spec is built on: **arrangement is a property of the view, not a
mode of the program.** A terminal panel that is "the TUI version" would
duplicate every section; a terminal panel that is `row` with one section
selected is the same program.

The sections themselves are settled by the archetype. fynedesygn's
`docs/glance.md` describes the window, and as of v0.1.47 its `glance` package
has everything the window needs: the card stack, the margins, the fixed-width
formatters, the sparkline, the meter, translucency, live opacity, and the KWin
rule and script writers for the frame, the position and the geometry read-back.
Nothing in this spec needs a library change.

What is genuinely new here is the settings: the Python cannot reorder or hide
sections, and it has no arrangement at all. That is why they are in the first
spec rather than a later one — they touch the store, the panel and the terminal
equally, and retrofitting them across four sections is how that goes wrong.

Bandwidth is the first section because it needs nothing: no credentials, no
hardware that may be absent, no permission. `/proc/net/dev` is there on every
machine, and the reading has a rate, a cumulative total and two directions, so
it exercises the formatters, the stretching bar and the grid reflow on its own.

## Requirements

- R1 `internal/core` reads the interfaces named in the settings from
  `/proc/net/dev` and returns rates and cumulative totals as plain values,
  with no toolkit and no formatting. A `hayami readings --json` subcommand
  prints them, so the data layer runs without a display.
- R2 `internal/view` describes a section as data: a title, rows, and each
  row's label, value, unit and status. No shell decides what a section says.
- R3 Three arrangements — `stack`, `grid`, `row` — each rendering the same
  section. `row` stretches a reading to the pane's width; `grid` reflows
  columns to the width; `stack` is one card above another.
- R4 `cmd/hayami` draws the sections as a fynedesygn glance window: frameless,
  fixed to its content, always on top. Its only menu items are Quit and the
  opacity choices.
- R5 `cmd/hayami-tui` draws the same sections with Bubble Tea, in the
  arrangement the settings name, refreshing on the same cadence, and builds
  with `CGO_ENABLED=0`.
- R6 Settings hold which sections are shown, in what order, and the
  arrangement, in one file both shells read. A section that is hidden stops
  its poll.
- R7 A parity test holds the two shells to the same set of sections, with a
  documented allow-list for anything one shell has and the other cannot.
- R8 Values do not jitter: a rate that changes magnitude does not change the
  width of its column, in either shell (`glance.Rate` in the window, the same
  rule applied in the terminal).

## Acceptance Criteria

- [ ] AC1 `hayami readings --json` prints rates and totals for the configured interfaces on a machine with no display, and its test builds its own `/proc/net/dev` fixture. (R1)
- [ ] AC2 A section is rendered from `internal/view` in all three arrangements, and the tests assert the text of each. (R2, R3)
- [ ] AC3 `row` stretches: a section rendered at 40 and at 120 columns puts its value at the right edge of each, with the bar taking the difference. (R3)
- [ ] AC4 `grid` reflows: the same sections at two widths produce a different number of columns and drop nothing. (R3)
- [ ] AC5 The window is frameless, fixed-size and sized to its cards, asserted headlessly; and **seen on a real window**, screenshotted and looked at, per the rule in `.claude/CLAUDE.md`. (R4)
- [ ] AC6 `CGO_ENABLED=0 go build ./cmd/hayami-tui` passes in CI. (R5)
- [ ] AC7 Hiding a section removes it from both shells and stops its poll; reordering changes the order in both. Asserted from the settings, not from a shell. (R6)
- [ ] AC8 `TestFeatureParity` passes with an empty allow-list. (R7)
- [ ] AC9 A rate crossing from KiB/s to MiB/s does not move the column in either shell. (R8)

## Risks & Assumptions

- **The terminal's cadence is not the window's.** A pane that repaints on a
  timer while a person is reading a scrollback is a nuisance; Bubble Tea's
  `tea.Tick` is the mechanism and the interval is a setting, defaulting to the
  Python's.
- **`grid` has no precedent in either program.** btop is the reference for
  the shape and nothing is copied from it; the column count comes from the
  width and the widest section's minimum, and the rule is written down in
  `docs/` before it is implemented.
- **Two shells, one truth.** The parity test is the only thing stopping them
  drifting, and it is worth more than any single feature in this spec.
- **Nothing here touches credentials, hardware or the network.** That is
  deliberate: the spine is proven before anything that can damage or leak.
- Rollback: the repository has no release and no users. Revert the branch.

## Alternatives Considered

- A single binary with a `--tui` flag. Rejected: jira-viewer separates them so
  the desktop one can be linked windowed and the terminal one as a console
  application, which is a real difference on Windows and a tidy one elsewhere.
- The terminal panel as a cut-down view of the window. Rejected: it is the
  same sections, and the moment it is "cut down" it starts deciding what a
  section says, which is what `internal/view` exists to prevent.
- Starting with the usage section, since it is what the herdr pane needs
  today. Rejected: it is the section with the most protocol surface and the
  only one that writes to a store another program owns. It goes last, and by
  then the spine will have been exercised by three simpler sections.
