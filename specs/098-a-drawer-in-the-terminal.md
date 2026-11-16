# 098 — a drawer in the terminal

**Issue**: #346

## Status: COMPLETE

## Executive Summary

The terminal pane has the window's drawer, read-only: Tab chooses a card,
Enter opens what is behind it, and the lists' keys, the filter, drilling into
a line, pause and Esc work as the window's do. The same `view.Detail` from
the same poll is drawn as text by `view.RenderDetail`, beside the cards where
the pane is wide enough and in their place where it is not. Reviewers should
look first at `internal/tui/drawer.go` (keys and polling) and
`internal/view/drawertext.go` (the layout and the summary rule).

## Context

The terminal panel draws the same cards as the window (spec 047's parity
test holds it), and nothing behind them: its one key is quit. The window's
drawer (specs 060, 062, 066, 092–097) shows the process lists with their
columns, a summary of the part over each, a program's processes, the usage
breakdown and the VPNs' detail. The terminal prints each of these once,
through `hayami-tui detail`; spec 060 R21 left a view in the pane out of
scope "as its own decision". This is that decision (#346): the drawer in the
pane, read-only.

## Requirements

### Choosing and opening

- R1 **A card is chosen with Tab and Shift+Tab** (or ↓/↑ when no drawer is
  open), among the drawn sections that offer a detail; the chosen card's
  title is marked. Enter opens its detail: the list the section opens on, or
  the last one shown, as the window's card does (spec 062).
- R2 **Esc goes back a step**: up from a drill page to its list, then out of
  the list's filter, then the drawer closes; with nothing open it quits, as
  it did. `q` still quits from anywhere but a filter being typed.

### Inside the drawer

- R3 **The switch**: the section's lists by label on one line, the shown one
  marked; each list's key (c g m i, p e) shows it.
- R4 **The filter**: `f` starts it, typed text narrows the list by name (the
  window's regular expression), Enter keeps it, Esc clears it. Keys typed
  faster than the terminal is read arrive as one message of several
  characters; each is taken as it would be alone.
- R5 **Lines**: ↓/↑ (and j/k) move a marked line; Enter on a line with
  somewhere to go drills into it (a program's processes, an account); the
  drill page's crumbs head it, as the window's do. The list's filter is
  kept through a drill and the way back.
- R6 **Space pauses the reading** and again resumes it, as the window's
  Pause does: the lines stop moving while a person reads them.

### What it shows

- R7 **The same detail the window draws**, from the same `view.Detail` and
  the same poll: the summary (spec 094's rule: drawn only where at least
  `SummaryKeepsLines` lines of the list stay under it, all or nothing), its
  meters as a bar of block characters, the plot as a sparkline, the heat
  strip a character per cell (spec 095's `Heat.Text`); the heading line and
  as many columns as the width fits, whole, left to right (spec 092); the
  lines cut to the height; the note and the reasons under them. A part
  made of parts (the usage breakdown) scrolls with PgUp/PgDn, as the window's
  drawer does.
- R8 **Layout by width**: where the pane is wide enough for the cards and a
  drawer of the width the window would give it (`ColumnsWant`, at least the
  usual 32 ems, taken as 52 columns), the drawer stands to the right of the
  cards; otherwise it takes the pane until Esc. In `row` the drawer always
  takes the pane. No reading moves the split: it is decided by the width.
- R9 **Polling**: a detail is polled at its interval while it is shown and
  let go when it is not (`DetailInterval`, `CloseDetail`), as the window's
  drawer polls; a section not shown is not asked.
- R10 **Nothing acts**: no VPN control, no ending of processes (decided on
  #346); a drill page shows the processes and says nothing of ending them.
- R11 `--once` draws no drawer.

### Also

- R12 The parity test grows a detail comparison: for each kind of detail,
  the pane's drawer and the window's drawer show the same facts (title,
  switch labels, summary lines, list names, note, reasons) at the same
  height.
- R13 README: the terminal section's keys; changelog; `docs/architecture.md`
  where it says the pane has no detail view.

## Acceptance Criteria

- [x] Tab, Shift+Tab, Enter and Esc choose a card, open and close its drawer.
- [x] The switch shows each list by its key; the filter narrows and clears.
- [x] A marked line drills into its program and Esc comes back.
- [x] Space pauses and resumes the reading.
- [x] Wide pane: the drawer beside the cards; narrow: in their place; the
  split does not move on a reading.
- [x] The summary follows 094's rule by height; columns by width, whole.
- [x] A detail is polled only while shown and let go after.
- [x] Parity: the pane's and the window's drawers show the same facts.
- [x] A real terminal (tmux, captured with `capture-pane`) at two widths:
  the cards with a drawer beside them, and a drawer in their place.

## Risks & Assumptions

- Keys the pane did not take before (Tab, Enter, f, space, letters) now do
  something; `q`, Ctrl+C and Esc (with nothing open) still quit, so a
  script or habit that relied on them is unchanged.
- A herdr pane in `row` is a few lines tall: the drawer there has room for
  little, and the summary rule leaves it out.
- **Rollback**: revert; no setting or file format changes.

## Threat model delta

No boundary touched: the pane draws details already printed by
`hayami-tui detail` (B6, through the same printable filter), and asks for
nothing new.

## Test strategy

- `tui`: model tests driving key and poll messages, asserting the model's
  state and the rendered text (not a golden image).
- `gui`: the parity test, extended to details.
- A real terminal: tmux at two widths, the screen captured and the layout
  asserted from it.

## E2E Test Plan

On the desk, `hayami-tui` in a tmux pane at 200 and 80 columns: choose the
Hardware card, open it, switch to IO, filter, drill into a program, pause,
back out; captured with `tmux capture-pane -p`. The same on the Windows VM
over ssh in Windows Terminal is not possible headless; `hayami-tui`'s model
tests run on Windows CI.

### Prod visibility

Not applicable: a desktop program with no telemetry.

### E2E results (2026-11-16)

Desk, the built `hayami-tui` in tmux:

- 200×50: Tab chose Hardware (the Peripherals card, with nothing behind it,
  was passed over), Enter opened the CPU list beside the cards: the switch
  `[CPU] GPU Mem IO`, the filter line, the summary (the load's bar and plot,
  32 core cells with the two offline as `··`, clock, load average, tasks) and
  the list with every column. `f hay`, Enter, ↓, Enter drilled into hayami's
  processes; Esc came back to the list still filtered; Esc again cleared it.
  41 lines carried the divider.
- The first run sent `f h a y` faster than the pane read them and the filter
  never opened: Bubble Tea delivers such a burst as one message. Fixed (R4)
  and tested; the same burst then filtered.
- 80×40: the drawer took the pane (no divider); `i` showed the IO list with
  its summary and the procs column; Esc gave the cards back.

Windows: the `tui` and `view` tests pass on the VM; a pane there is not
driven headless.

Falsified: each of fourteen breakages (the summary rule, the covered note,
every column drawn, actions kept, no scroll, no summary, Esc's filter step,
the old list not let go, the side-by-side rule, no read on resume, cards with
nothing behind them chosen, the filter not typed into, no drill, no mark on
the chosen card) fails its test, and the burst handling's test fails without
it.
