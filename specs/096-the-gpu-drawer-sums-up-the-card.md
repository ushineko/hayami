# 096 — the GPU drawer sums up the card

**Issue**: #333

## Status: COMPLETE

## Executive Summary

The GPU drawer now carries a summary over its list: the card's memory used of
its total with a bar, its graphics and memory clocks, and the power it draws
of its limit. An NVIDIA card is read through nvidia-smi on Linux and Windows,
an AMD card on Linux through its driver's files; any other card keeps the
lines in their empty form. Reviewers should look first at `internal/core/card.go`
(the routes and parsers) and at the AMD readers' limit: no AMD card was in
reach, so they are tested on files laid out as the kernel documents them.

## Context

The last of #333's three drawers (094 Memory, 095 CPU). hayami reads a
card's temperature, load and name through the graphics chain (spec 043):
hwmon and `gpu_busy_percent` on Linux, D3DKMT and the `GPU Engine` counters
on Windows, nvidia-smi last on both. Nothing read its memory, its clocks or
its power.

## Requirements

- R1 Lines, in order:
  - **VRAM**: used of total, with a meter;
  - **clock**: the graphics clock, in MHz;
  - **mem clock**: the memory clock, in MHz;
  - **power**: drawn, of the card's limit where it is known, in watts.
- R2 The shape is fixed (094 R2): a figure the card's route does not give
  is its line's empty form and a reason, and a card that cannot be read at
  all keeps every line.
- R3 **Read on the drawer's poll**, only while the GPU list is shown, by the
  route the card's reading came from (`gpuVia`, as the GPU list is). The
  card's chain is unchanged: the summary does not make nvidia-smi run on
  the card's poll.
- R4 **NVIDIA (Linux and Windows)**: one nvidia-smi query,
  `memory.used,memory.total,clocks.gr,clocks.mem,power.draw,power.limit`,
  constant, with SMITimeout, while the GPU list is shown. A field it does
  not have (`[N/A]`, `[Not Supported]`) is that part absent.
- R5 **AMD (Linux)**: beside the busy file, `mem_info_vram_used` and
  `mem_info_vram_total` (bytes); in the card's hwmon, the clocks by their
  labels, `sclk` and `mclk` (Hz), and `power1_average` or `power1_input`
  with `power1_cap` (microwatts). These are the files btop and nvtop read.
- R6 **Windows, any other card**: not read (decision D3); its lines are
  empty, with their reasons.
- R7 `detail cooler/gpu` prints the summary; `--json` carries it.
- R8 README changelog and drawers paragraph; threat model B4 and B5.

## Acceptance Criteria

- [x] nvidia-smi answer fixtures, units scaled; `[N/A]` fields absent; more
  memory used than there is, no memory.
- [x] The card is read by its route: nvidia-smi for a card read through it,
  the AMD files otherwise on Linux, nvidia-smi on Windows; an AMD card runs
  no nvidia-smi.
- [x] AMD fixtures, laid out as the kernel's amdgpu documentation describes:
  VRAM, both clocks by label, power and cap; the newer `power1_input`.
- [x] The GPU summary's lines in R1's order, the same shape with nothing
  read; the card is not read for another list.
- [x] `FuzzParseSMICard` runs clean.
- [x] Plasma (this desk's RTX 4090): the drawer's figures against nvidia-smi
  at the same moment; `detail cooler/gpu`.

## Decisions (2026-11-15)

- D1 Power as a share of the limit, for Windows's perf data: moot, Windows
  reads only nvidia-smi (D3).
- D2 nvidia-smi while the GPU list is shown, every drawer poll. **Decided.**
- D3 Coverage, decided after the Windows VM's card (an Intel UHD 770) was
  found to have no GPU row at all, so a Windows-only reader could not be
  seen in a window: **NVIDIA through nvidia-smi on both systems, and AMD on
  Linux through its driver's files, tested on fixtures only and said so.**
  D3DKMT for other Windows cards is left for a later spec, when there is a
  Windows card hayami reads.

## Risks & Assumptions

- **The AMD readers have not met an AMD card.** They read the files btop
  and nvtop read, as the kernel documents them; a card that names them
  otherwise shows "--" with a reason, not a wrong figure.
- nvidia-smi now runs on the drawer's poll as well as the card's while the
  GPU list is shown: one more process per drawer poll, bounded by
  SMITimeout.
- The Windows VM has no GPU row, so the Windows path (nvidia-smi on an
  NVIDIA card) shares its parser with Linux's and is not seen in a Windows
  window.
- **Rollback**: revert; no setting or file format changes.

## Threat model delta

B4: nvidia-smi's CSV gains a second query, parsed by `ParseSMICard` (fuzzed);
the AMD files are single numbers and labels, read by name under the busy
file's device. B5: nvidia-smi, a constant command line with SMITimeout, runs
on the drawer's poll while the GPU list is shown. `docs/threat-model.md`
B4 and B5 say so.

## Test strategy

As 094 and 095: core fixtures and a fuzz target; view shape tests; panel
tests of the route and of when the card is read. The window's summary
pieces (a meter and lines) are 094's, covered by its window tests.

## E2E Test Plan

The desk's GPU drawer against `nvidia-smi --query-gpu=...` at the same
moment; `detail cooler/gpu`. Windows: not applicable in a window (no GPU
row on the VM).

### Prod visibility

Not applicable: a desktop program with no telemetry.

### E2E results (2026-11-15)

Desk, Plasma 6 Wayland at 1.5, RTX 4090 on NVIDIA's driver:

- `detail cooler/gpu`: VRAM 11.1 of 24.0 GiB, clock 1680 MHz, mem clock
  810 MHz, power 62.8 of 400.0 W; nvidia-smi at once: 11343 of 24564 MiB,
  1680 MHz, 810 MHz, 62.79 of 400.00 W.
- The GPU drawer in the window: VRAM 11.0 of 24.0 GiB with its bar, clock
  750 MHz, mem clock 810 MHz, power 57.6 of 400.0 W over 26 programs;
  nvidia-smi a moment later: 11302 MiB, 900 MHz, 810 MHz, 58.73 W (the
  clock and power move within a second). The window kept its width.

Falsified: each of ten breakages (MiB unscaled, the used-of-total check, the
AMD clock labels swapped, the power's unit, no `power1_input`, the route
ignored, the power limit dropped, the lines' order, the card read by another
route, the card read for every list) fails its test. `FuzzParseSMICard`
found a negative field kept as the figure of an absent part; fixed, its
input kept in `testdata/fuzz`, then clean for 10 s.
