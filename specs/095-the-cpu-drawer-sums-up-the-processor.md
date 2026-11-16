# 095 — the CPU drawer sums up the processor

**Issue**: #333

## Status: COMPLETE

## Executive Summary

The CPU drawer now carries a summary over its list: the load with a bar and
a five-minute plot fed by the cooler's polls, a heat strip of every possible
core (fynedesygn 0.1.121's `HeatStrip`), the average clock, the load average
on Linux or the processor queue on Windows, and the process and thread
counts. It follows spec 094's all-or-nothing rule. Reviewers should look
first at `ProcessorReader` and its parsers in `internal/core/processor.go`,
then `setSummary` in `internal/gui/drawer.go`, which now takes a plot and a
strip.

## Context

Spec 094 gave the Memory drawer a summary over its list and built the area
for it. #333 asks the same of the CPU drawer: the load over time, each core,
the clocks, the load average, and how many processes and threads there are.

hayami reads the processor's load as one figure (`/proc/stat`'s `cpu` line,
`GetSystemTimes`) and keeps no trail of it: the cooler's CPU trail is the
temperature. Nothing reads a core, a clock, the load average or a count.

## Requirements

### The CPU summary

- R1 Lines, in order:
  - **load**: the whole processor's, with a meter, and a plot of it over the
    last five minutes under it;
  - **cores**: one small cell per logical processor, its colour the core's
    load (the heat strip chosen on #333);
  - **clock**: the cores' average current clock, of their maximum;
  - **load avg** (Linux): 1, 5 and 15 minutes; **queue** (Windows): the
    processor queue length, Windows's nearest figure (decision D1);
  - **tasks**: processes and threads.
- R2 The summary is a fixed shape for the machine (spec 094 R2): the strip
  has a cell for every possible processor, and one offline (this desk has two)
  is an empty cell, not a missing one, so a core going off or on reflows
  nothing.
- R3 The plot is fed by the cooler's poll, so it has its five minutes when the
  drawer opens; the cores, the clocks and the counts are read on the
  drawer's poll, only while the CPU list is shown (spec 060's rule that a
  detail costs nothing unshown).

### Readers

- R4 **Linux**: per-core load from `/proc/stat`'s `cpuN` lines (the file
  already read); clocks from `/sys/devices/system/cpu/cpuN/cpufreq/`
  `scaling_cur_freq` and `cpuinfo_max_freq`; `/proc/loadavg` for the load
  average and the thread count (its fourth field's total), and the
  numbered entries of `/proc` for the processes. The places come from
  `/sys/devices/system/cpu/possible`.
- R5 **Windows**: per-core load from
  `NtQuerySystemInformation(SystemProcessorPerformanceInformation)`; the
  clock from the `Processor Information` counters `% Processor Performance`
  times `Processor Frequency`, which is what Task Manager shows (PDH is
  already used for the GPU engines); the queue from `\System\Processor Queue
  Length`; processes and threads from `K32GetPerformanceInfo` (094's call).
  No new right, no service, no subprocess.
- R6 Each figure absent is its line's empty form and a reason (spec 094 R2).
  Which of the load average and the queue the machine has is the reader's
  (`Processor.Queues`), not the reading's, so the line keeps its name when
  its figure is not read.

### The window

- R7 **The heat strip is a library piece**: fynedesygn has no row of
  coloured cells. It is a fynedesygn spec first (a `glance.HeatStrip`: n
  cells, each a fraction drawn in the theme's load colours, a fixed height,
  the width shared out), released, then used here.
- R8 The plot is the drawer's plot (`drawPlot`), as a piece of the summary.
  The summary's meters and strip restyle with the drawer (the summary
  meters of 094 did not, and now do).
- R9 `detail cooler/cpu` prints the summary, the strip as one character per
  core (` ▁▂▃▄▅▆▇█`); `--json` carries it.

### Documentation

- R10 README changelog and drawers paragraph; the threat model's B4 for the
  new files and calls.

## Acceptance Criteria

- [x] `/proc/stat` fixtures: per-core loads from two samples; an offline core
  (no `cpuN` line) is an empty cell at its place.
- [x] cpufreq and `/proc/loadavg` fixtures; a core without cpufreq leaves the
  clock to the rest.
- [x] Windows: the per-core arithmetic from fixed structures, run on the VM;
  the clock and the queue read for real on the VM.
- [x] The CPU summary's lines in R1's order; two readings with every value
  changed, and a core gone offline, keep its shape.
- [x] The plot has the cooler's samples when the drawer opens.
- [x] fynedesygn: the heat strip draws n cells in the load colours, and its
  height does not change with n or with the values.
- [x] The window: the summary drawn by 094's rule; `detail` text and JSON.
- [x] Plasma and the VM: against the system's own figures (`/proc`, and
  Windows's performance counters, Task Manager's source). `mpstat` is not
  installed on the desk; the strip is checked by eye there.

## Decisions (2026-11-15)

- D1 Windows has no load average. Proposed: the processor queue length in
  its place, so the summary keeps its shape. Alternative: no such line on
  Windows (the shape is fixed per platform either way). **Decided: the queue.**
- D2 The clock is one average. Proposed: average of the online cores, of
  the highest maximum. On a hybrid processor (this desk's P and E cores)
  an average hides the split; the strip already shows each core's load.
  **Decided: one average.**

## Risks & Assumptions

- `scaling_cur_freq` is the governor's figure, close to but not the measured
  one (`cpuinfo_avg_freq` is newer and not everywhere).
- Windows's `Processor Frequency` counter times performance is an estimate,
  the one Task Manager shows.
- A strip of 32 cells in a drawer about 40 ems wide: about one em a cell.
  A machine of 128 shares the width, narrower cells.
- **Rollback**: revert; the library piece is additive.

## Threat model delta

B4: `/proc/loadavg` and cpufreq files are new parsed inputs, a single line
each, parsed as numbers and bounded; the Windows processor table is walked
from bytes as 094's page file table is, and fuzzed. B6: hayami's own numbers.

## Test strategy

As 094: core fixtures and a fuzz target; view shape tests; headless window
tests; the library's own tests for the strip; real windows on both systems.

## E2E Test Plan

Desk and VM as 094, with the CPU list shown; the strip compared against
`mpstat -P ALL 1 1` on the desk and Task Manager's logical-processor view on
the VM.

### Prod visibility

Not applicable: a desktop program with no telemetry.

### E2E results (2026-11-15)

Desk, Plasma 6 Wayland at 1.5, installed with fynstall against fynedesygn
at this spec's library branch:

- The CPU drawer shows load 9.9 % with its bar and plot, 32 cells with
  cores 8 and 9 (offline on this desk) as outlined gaps, clock 4.4 of
  6.0 GHz, load avg 4.9 3.7 3.6, 1405 processes and 10944 threads, over 22
  programs; the window stayed 653 px wide.
- `detail cooler/cpu` beside the system: load avg 3.8 3.5 3.5 and 10947
  threads against `/proc/loadavg`'s 3.75 3.52 3.49 and 10931 a moment
  later; 1409 processes against `/proc`'s 1409 entries; clock 4.1 GHz
  against `/proc/cpuinfo`'s average 4.6 GHz a moment later (clocks move
  within a second).

Windows VM, 150 %, cross-built:

- `TestWindowsReadsTheProcessor` passes on the VM: 14 cores, 3188 MHz,
  queue 0, 272 processes, 3411 threads.
- With the panel made taller (Peripherals added, settings restored after),
  the CPU drawer shows the summary over 14 programs: 14 cells, clock
  3.2 GHz, queue 0, 266 processes, 3407 threads; Windows's counters at the
  same moment: frequency 3187 MHz at 100 % performance, queue 0, 269
  processes, 3428 threads.

Falsified: each of twelve breakages (offline cores counted, the places one
short, the clock summed, the threads from the wrong field, Windows's busy
time with idle in it, the cells unscaled, the queue line renamed, the line's
name from the reading, no load trail, no plot, no NaN for an offline cell,
a strip drawn without one) fails its test. `FuzzParseProcessorTimes` and
`FuzzParseLoadAvg` ran clean for 10 s each.
