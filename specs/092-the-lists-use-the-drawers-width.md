# 092 — the lists use the drawer's width

**Issue**: #332

## Status: COMPLETE

## Executive Summary

A filled CPU, GPU or Memory drawer now shows, after each program's figure,
the count of its processes, the other figure of the same reading and its
share of all memory, as many as the drawer's width fits whole; an unfilled
drawer is unchanged, and `hayami-tui detail` prints and serialises every
column. Reviewers should look first at `internal/view/columns.go` (the
column model and fixed widths) and `internal/gui/columns.go` (the gate on
`Filled()` and the fit by width), then at the GPU list's join by PID in
`internal/panel/cooler.go`.

## Context

The CPU, GPU and Memory drawers (spec 060, 066) are lists of programs, one
figure each: a share of the processors, a card's busiest engine, bytes held.
A drawer is `glance.DrawerWidthInEms` (32) wide, which is a name of
`view.DetailNameWidth` characters and one figure. Filled (spec 090), a
drawer is as wide as the screen leaves it, from a few ems more at the cap of
three or four to most of a screen for one, and the list is still a name and
a figure with the rest of the width empty.

What the lists already read and throw away is enough to fill it: how many
processes share a name (`view.Processes` sums them), the other figure of
the same process reading (`core.Process` carries both the share of the
processors and the memory), and the memory's total (`coolerState`).

The summary of the machine above the lists, which needs new readers, is
issue #333 and a later spec.

## Requirements

- R1 **A detail describes its columns; a shell chooses which it shows.**
  `view.Detail` gains an ordered list of extra columns, each a key, a short
  heading and a fixed width in characters; `view.DetailLine` gains a value
  for each, formatted to that width. The order is the order a narrowing
  drawer drops them in, last first. Neither the window nor the terminal
  decides what a column says.
- R2 **The columns of each list**, in order:
  - CPU by process: processes (`×N`), memory, share of all memory.
  - Memory by process: processes, CPU share, share of all memory.
  - GPU by process: processes, CPU share, memory.
  A drill page of one name (`cpu/<name>`) has the same columns less the
  count, a process being one; a page of one process has none.
- R3 **The figures are the same reading's.** CPU share and memory come from
  the one `ProcessList.Read` a poll already makes; the GPU list joins its
  processes to a process reading by PID, and a PID that reading lacks shows
  the column's empty form, not a zero. Share of all memory is a process's
  bytes over `coolerState.MemoryTotal`; with no total the column is not
  offered.
- R4 **The list stays ordered by its own figure.** A column is read, never
  sorted by.
- R5 **A filled drawer shows as many columns as its width fits**, in order,
  at fixed positions: the name, the list's figure, then each column that
  fits whole in `Drawer.Available().Width`. A column that does not fit whole
  is left out, and so is every one after it. Within one width the columns
  shown do not change from one reading to the next (glance rule): what
  decides them is the width, never a value. *Amended during
  implementation*: the first build decided by width alone, and a drawer at
  its usual 32 ems had room for the count, so it changed the unfilled
  drawer against R7. Columns are what filling gives; the width says how
  many.
- R6 **A heading line names the columns shown**, under the switch and over
  the list, when at least one extra column is shown; with none it is not
  drawn and the list keeps the height it has today.
- R7 **A drawer that is not filled is unchanged**: no extra column, and the
  list, its height and its rows are as before.
- R8 **The width is asked again when it changes**: fill, unfill, a drawer
  opened or closed beside it, the text size.
- R9 **The terminal prints every column.** `hayami-tui detail cooler/cpu`
  prints a heading line and each line's columns after its figure; `--json`
  carries the detail's `columns` (key, heading, width) and each line's
  `cells`, one per column. The text stays within the terminal's
  sanitising (spec 076): a column holds hayami's own formatting and no
  process-supplied text.
- R10 The filter (spec 062) narrows by name as before; a drill click lands
  where it did.
- R11 README changelog line under `### Unreleased`; the drawers paragraph of
  the user guide says what a wide drawer adds. `docs/architecture.md` is
  updated if the column model changes a rule there.

## Acceptance Criteria

Headless, each falsified by breaking the code it covers:

- [x] Each list's columns, in R2's order, with the figures of one reading
  (`TestEachListHasItsColumns`, `TestAListSumsItsColumns`).
- [x] Two processes of one name: the count is 2 and the CPU share and
  memory are their sums (`TestAListSumsItsColumns`).
- [x] A GPU process with no matching PID in the process reading shows the
  empty form, not `0` (`TestTheCardsListJoinsByPID`).
- [x] No memory total: share of all memory is not among the columns
  (`TestNoTotalNoShare`).
- [x] Order unchanged by the columns: a list whose memory order differs from
  its CPU order is still sorted by CPU (`TestTheColumnsDoNotOrderTheList`).
- [x] A drill page of one name has the columns but the count; a page of one
  process has none (`TestADrillPageHasTheColumnsButTheCount`).
- [x] The window, not filled: no column, no heading line, the same rows as
  before (`TestAUsualDrawerIsUnchanged`).
- [x] The window, filled at several widths: exactly the columns that fit, in
  order, never one past a gap (`TestANarrowFillShowsOnlyWhatFits`).
- [x] A reading that changes every value at one width changes no column's
  position or the set shown (`TestAReadingMovesNoColumn`).
- [x] Fill then unfill: the columns appear, then go; each line is as wide as
  the heading over it (`TestFillingShowsTheColumns`,
  `TestTheHeadingIsAsWideAsItsLine`).
- [x] `detail` text and JSON carry every column
  (`TestDetailPrintsTheSecondReadingAsText`, `TestADetailPrintsAsText`,
  `TestTheColumnsAreInTheJSON`).

Five mutations each failed a test: columns shown unfilled, the heading
without its gaps, memory not summed across a name, the card's list joined
without the PID check, the count kept on a drill page.

On a real window (Plasma, Wayland, at 1.5; the Windows VM at 150 %), the
installed panel, geometry recovered from grabs:

- [x] Plasma: a CPU drawer, not filled: name and figure only, as 0.15.0.
- [x] Plasma: CPU, GPU and Memory at the cap, filled: the heading line and
  every column in each, as R2 orders them, values right-aligned at one x
  down each list; unfilled, gone.
- [x] Windows VM: the same (CPU and Memory; the VM has no graphics row).

## Risks & Assumptions

- Two drawers that read processes (CPU and Memory open at once) already
  each call `ProcessList.Read`, so each share is over the time since the
  other's read; adding the GPU list as a third reader adds a third. The
  shares stay correct means over shorter intervals. Unchanged in kind, noted
  because the GPU drawer starts doing it.
- `Drawer.Available()` in fynedesygn 0.1.119 reports the shown width,
  filled or not. Assumed to stay so.
- Column widths are fixed by the widest value each can take (`×999`, the
  byte and percent formatters' widths), as the formatters are now.
- **Rollback**: revert the PR; no setting or file format changes. The JSON
  gains fields and loses none.

## Threat model delta

B6 (what hayami shows and prints): new fields in the `detail` text and JSON.
They are numbers formatted by hayami, and the line's name, the one
process-supplied string, is handled as now. No new reader, no new process
attribute is read (still name, PID, CPU time and memory: spec 060 R6).
`docs/threat-model.md` B6 gets one line saying the columns are hayami's own
formatting.

## Test strategy

- `view`: the column model and each kind's formatters, table-driven.
- `panel`: each list built from a fake process reader and GPU reader with
  known PIDs and figures; the join and its gaps.
- `gui`: headless, a drawer given each width, asserting the columns shown
  and the heading line; the real-window checks above for what is drawn.
- `cli`: `detail` text and JSON on a fake source.
- No mocks past the readers' existing seams (`processes`, `gpuProcesses`).

## E2E Test Plan

On the desk, installed with fynstall (Plasma, Wayland, at 1.5):

1. Click the CPU row: the drawer as before (R7).
2. Press Fill: the heading line and columns appear; values align (R5, R6).
3. Shift-click GPU and Memory to the cap, Fill: columns as fit (R5, R8).
4. Unfill: back to name and figure (R8).
5. `hayami-tui detail cpu` and `--json`: every column (R9).

On the Windows VM, 150 %: steps 1–3 and 5.

Results, 2026-11-15, Plasma 6.8 on Wayland at 1.5, the branch's installer
(built as 0.15.0, VERSION not bumped) installed with fynstall, clicked with
ydotool, grabs in the session's scratchpad:

- Step 1: the CPU drawer opened, window 618 wide at x 3382.33; name and
  figure only, as before.
- Step 3 then 2: Shift-clicks on Hardware opened GPU and Memory, window 1276
  at x 2724.33, the outermost offered Fill, and no drawer showed a column.
  Filled: x 2560.33, 1440 wide (the screen's edge at 4000, the cards
  against it as before); each drawer showed the heading line and all three
  of its columns -- CPU: procs, Mem, of mem; GPU: procs, CPU, Mem; Memory:
  procs, CPU, of mem -- each column's values ending at one x down its list.
  At the cap of three no column was cut, so the partial case is the
  headless test's alone.
- Step 4: unfilled, back to 1276 at x 2724.33 and name and figure only. A
  plain click closed all: 289.33 at x 3711.
- Step 5: `hayami-tui detail cooler/cpu`, `cooler/gpu` and `cooler/memory`
  printed the heading line and every column, the GPU list's CPU and Mem
  joined from the process table.
- Windows VM, 2026-11-15, Windows 11 at 150 % (3840x2160), the branch's
  panel cross-built with mingw and run in place of the installed 0.15.0,
  input sent through QEMU's tablet and read from in-session grabs. The VM
  has no graphics row, so CPU and Memory carried the check:
  - Step 1: the CPU drawer opened, 863 wide; name and figure only.
  - Step 3 then 2: with the panel at x 1100, Shift-clicks opened Memory and
    Bandwidth by process and by peer; at four the outermost offered Fill
    and neither list showed a column (2344 wide). Filled: 2740 wide, the
    right edge on the screen's at 3840, the cards at x 1100; Memory showed
    procs, CPU, of mem and CPU showed procs, Mem, of mem, each heading over
    its cells and the cells ending at one x down each list. Unfilled: 2344
    and name and figure only. (At the screen's right edge, x 3470, four
    drawers left room for two more and Fill was not offered, as spec 090
    has it.)
  - Step 5: `hayami-tui detail cooler/memory` printed the heading line and
    every column; `--json cooler/cpu` carried each line's `cells`.

### Prod visibility

Not applicable: a desktop program with no telemetry.
