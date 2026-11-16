# 094 — the Memory drawer sums up the machine

**Issue**: #333

## Status: COMPLETE

## Executive Summary

The Memory drawer now carries a summary over its list: used of total with a
bar, available, cache and swap (page file on Windows), read in the same poll
as the card's memory line. The window draws it whole where the list keeps ten
lines under it and not at all otherwise, decided from the drawer's height and
never from a reading. Reviewers should look first at `setSummary` in
`internal/gui/drawer.go` (the all-or-nothing rule) and at the Windows page
file walk in `internal/core/pagefile.go`.

## Context

A CPU, GPU or Memory drawer is a list of programs (spec 060, 066, 092,
093). What the part is doing as a whole is one line on the card, and the
drawer, which has room for more, says no more about it (#333).

#333 asks for a summary above each list. It is split by drawer: this spec
is the Memory drawer's, and builds the summary area the CPU's (095) and the
GPU's (096) will use. Memory comes first because its readers are the
smallest: one file on Linux, two calls on Windows.

hayami reads the memory as used of total (`core.Memory`, from
`MemAvailable` on Linux and `GlobalMemoryStatusEx` on Windows), and nothing
else about it.

## Requirements

### The summary area

- R1 **A detail may carry a summary**: `view.Detail.Summary`, lines (name
  and value, as a list's) and meters, drawn above the list and under the
  switch and filter. It is the detail's to say what is in it, the shell's
  to say whether it is drawn (R3).
- R2 **The summary is a fixed number of lines for its kind.** A figure that
  cannot be read keeps its line and shows the empty form, with its reason
  in the detail's reasons; a machine without swap shows the swap line as
  "none". So no reading changes the summary's height (the glance rule).
- R3 **The window draws the summary only where the list keeps at least
  `SummaryKeepsLines` (10) lines under it**, and then all of it; otherwise
  none of it, and the list is as it was. Decided when the drawer is drawn
  and again when its height changes (a card appearing or going), never by a
  reading. The lines kept count the list's reasons and note with it, so a
  reason coming or going does not draw or hide the summary.
- R3a A note that says what the summary says (the Memory list's "in use")
  is left out while the summary is drawn (`Summary.Covers`).
- R4 A drill page (one program, one process) has no summary: it is about a
  program, not the machine.

### The Memory summary

- R5 Lines, in order:
  - **used**, of the total, with a meter of the share;
  - **available**: what can be given to a program without swapping;
  - **cache**: what the system holds as cache that it can give back;
  - **swap** (Linux) or **page file** (Windows): in use, of its size.
- R6 **Linux**: from `/proc/meminfo`, the file hayami already reads: used
  is `MemTotal - MemAvailable` (as now), available is `MemAvailable`, cache
  is `Buffers + Cached + SReclaimable` (what `free` calls buff/cache), swap
  is `SwapTotal - SwapFree` of `SwapTotal`. `SwapTotal` of 0 is "none".
- R7 **Windows**: used and available from `GlobalMemoryStatusEx` (as now);
  cache from `GetPerformanceInfo` (`SystemCache` pages); the page file's
  size and use from `NtQuerySystemInformation`'s page file information,
  summed over the page files. No page file is "none". No new right, no
  service, no subprocess.
- R8 **One reading per poll**: the summary's figures come from the same
  read as the card's memory line, so the two never disagree.
- R9 **The terminal prints the summary**: `hayami-tui detail cooler/memory`
  prints its lines above the list's heading line, and `--json` carries
  `summary`. Every value is hayami's own formatting (B6).

### Documentation

- R10 README changelog line under `### Unreleased`; the user guide's
  drawers paragraph says what the Memory drawer's summary shows and when.
  `docs/architecture.md` is updated if the summary area changes a rule.

## Acceptance Criteria

Headless, each falsified by breaking the code it covers:

- [x] `/proc/meminfo` fixtures: used, available, cache and swap as R6, and
  `SwapTotal: 0` is "none", not 0 of 0.
- [x] A meminfo without `SReclaimable` (older kernels) still gives cache from
  what it has; without `MemAvailable` the reader fails as now.
- [x] The Windows reader's arithmetic from fixed structures (pages times
  page size, page files summed), built and run on the Windows VM.
- [x] The Memory detail carries its summary in R5's order; a figure that
  failed keeps its line with the empty form and a reason.
- [x] Two readings with every value changed: the same number of summary
  lines.
- [x] The window, a drawer tall enough: the summary over the list, the list
  keeping at least 10 lines; a drawer too short: no summary and the list
  as before; a reading changes neither.
- [x] A drill page has no summary.
- [x] `detail` text and JSON carry the summary.

On a real window (Plasma, Wayland, at 1.5; the Windows VM at 150 %):

- [x] Plasma: the Memory drawer shows the summary over the list; the
  figures match `free -b` taken at the same moment to within a reading.
- [x] Windows VM: the same, against Task Manager's Memory page; the VM's
  drawer, about 12 lines tall, shows no summary, and with the panel's
  sections making it taller it does.

## Risks & Assumptions

- `NtQuerySystemInformation` is documented as subject to change. The page
  file class (`SystemPageFileInformation`) has been stable since NT; if it
  fails, the line is an absence, not a crash.
- "cache" is not the same quantity on the two systems: Linux's buff/cache
  and Windows's system cache. Each is the system's own figure for what it
  can give back; the label is the same because the question a reader asks
  is.
- A summary takes about five of the list's lines. On a drawer under 15
  lines it is not drawn at all.
- **Rollback**: revert the PR; no setting or file format changes. The JSON
  gains a field.

## Threat model delta

B4 (what hayami parses): `/proc/meminfo` gains fields it was already
reading the file for; the parser is the one it has. B6 (what hayami shows
and prints): new lines of hayami's own numbers. No new reader of another
program's output, no new right. `docs/threat-model.md` B4 notes the
Windows page file query.

## Test strategy

- `core`: the meminfo parser on fixtures; the Windows arithmetic on fixed
  structures, run on the VM.
- `view`: the summary's lines and their count across readings.
- `gui`: headless, drawers of given heights; the summary drawn or not.
- `cli`: text and JSON.

## E2E Test Plan

On the desk, installed with fynstall (Plasma, Wayland, at 1.5):

1. Open the Memory drawer: the summary above the list (R1, R3, R5).
2. Run `free -b` at once: the figures agree (R6).
3. Watch a minute: no line comes or goes, the window does not resize (R2).
4. `hayami-tui detail cooler/memory` and `--json`: the summary (R9).

On the Windows VM, 150 %: steps 1, 3, 4 against Task Manager's Memory page
(R7), and the short drawer without a summary (R3).

### Prod visibility

Not applicable: a desktop program with no telemetry.

### E2E results (2026-11-15)

Desk, Plasma 6 Wayland at 1.5, installed with fynstall:

- The Memory drawer shows used 72.2 of 125.5 GiB with its bar, available
  53.2 GiB, cache 39.4 GiB, swap 50.1 of 125.5 GiB over 28 programs; the
  note is gone while the summary is drawn. `free -b` a moment later:
  available 53.3 GiB, buff/cache 39.3 GiB, swap used 50.1 GiB. The window
  stayed 653 px wide.
- `hayami-tui detail cooler/memory` prints the four lines above the heading
  line; `free -b` taken with it agrees to the tenth (53.0, 40.3, 50.3 GiB).

Windows VM, 150 %, cross-built:

- `TestWindowsReadsTheCacheAndThePageFile` passes on the VM, and the core
  memory and page file tests pass there.
- `detail cooler/memory`: page file 250.5 MiB of 2.0 GiB against
  `Win32_PageFileUsage` 249 of 2048 MB; cache 20.2 GiB against the standby
  lists' counters, 20.2 GiB together; available 20.9 GiB.
- The panel as configured (bandwidth, cooler, usage) has a Memory drawer of
  11 lines and a note: no summary, the list as before. With the Peripherals
  section added (settings restored afterwards) the drawer shows the summary
  over 19 lines, page file 235.9 MiB of 2.0 GiB.

Falsified: each of eleven mutations (the cache's SReclaimable, the swap
guard, the page size, the page file bound, "none", the lines' order, a
second memory read in the list, the ten-line rule, the note left out, a
partial summary, the summary in the text) fails its test. `FuzzPageFiles`
ran clean for 10 s.
