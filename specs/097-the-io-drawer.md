# 097 — the IO drawer

**Issue**: #340

## Status: COMPLETE

## Executive Summary

IO is a fourth hardware drawer: an IO row on the Hardware card with the
disks' read and write rates, and an IO list of each program's reads and
writes under a summary of the disks (the rates with a plot, a cell per disk
by how busy it is, the IO pressure or Windows's queue, the swap). Nothing
privileged is read: on Linux what the account cannot see is "other", on
Windows each figure is all of a program's IO and says so. Reviewers should
look first at `internal/core/disks.go` and `ioUse` in
`internal/panel/cooler.go`.

## Context

The Hardware card has three drawers, CPU, GPU and Mem, each a list of
programs with a summary over it (specs 060, 092–096). Disk IO has none (#340).

### What iotop reports, and how (iotop-c 1.33, the desk's)

- **Per process or thread**: DISK READ and DISK WRITE (bytes per second over
  the sample), SWAPIN % and IO % (the share of the sample spent waiting to
  swap in and waiting on IO), and the IO priority (class/level).
- **At the top**: Total DISK READ/WRITE, between processes and the block
  layer (the sum of the per-task figures), and Current DISK READ/WRITE,
  between the block layer and the disks (`/proc/vmstat`'s `pgpgin` and
  `pgpgout`). The two differ by the page cache and write-back.
- **How**: the kernel's taskstats over a generic netlink socket, per task:
  `read_bytes`, `write_bytes`, `blkio_delay_total`, `swapin_delay_total`.
  taskstats needs `CAP_NET_ADMIN`, which is why iotop runs as root. The two
  delay percentages also need `kernel.task_delayacct`, off by default since
  5.14 (off on this desk) and switched on only as root.

### What an unprivileged panel can read

- **Linux, per process**: `/proc/<pid>/io` (`read_bytes`, `write_bytes`, the
  same counters taskstats gives, and `rchar`/`wchar` including the cache),
  readable only where ptrace would be allowed: a person's own, ordinary
  processes. On this desk 477 of 1391 processes are readable; root's, the
  kernel's (`btrfs-transaction`, `kworker`) and a privileged program of the
  person's own (`kwin_wayland`) are not.
- **Linux, the machine**: `/proc/diskstats`, world-readable, per device: reads
  and writes completed, sectors, and the milliseconds spent doing IO, which
  is a disk's busy share; `/proc/vmstat` (`pgpgin`/`pgpgout`,
  `pswpin`/`pswpout`); `/proc/pressure/io` (PSI), the share of time some or
  all tasks waited on IO, which is iotop's IO % for the whole machine and
  needs no delay accounting.
- **Windows, per process**: the process table hayami already reads
  (`NtQuerySystemInformation`) carries `ReadTransferCount` and
  `WriteTransferCount` for every process, no administrator needed; they are
  all IO, files, network and devices, not the disk alone.
- **Windows, the machine**: the `PhysicalDisk` counters, per disk and
  `_Total`: `Disk Read Bytes/sec`, `Disk Write Bytes/sec`, `% Idle Time`
  (100 less it is Task Manager's active time), `Avg. Disk Queue Length`.

## Requirements

### The card and the drawer

- R1 **An IO row on the Hardware card**: the disks' read and write rates,
  ↓ and ↑ as a network row draws them, at fixed width, read on the card's
  poll. The card opens on the list it last showed, as it does; the switch
  is **CPU GPU Mem IO** (key `i`).
- R2 **The IO list**: one line per program name, its read and write rates
  as one figure (the bandwidth list's form), greatest total first,
  filterable, with the process count, the one column of spec 092 that fits
  beside two rates without widening the drawer the other lists set; each
  line drills to its processes as the other lists do.
- R3 **What the list cannot see is a line of its own**: the machine's IO
  less what the readable processes account for is "other", as the bandwidth
  list's unattributed traffic is, so the list never claims to be all of it.

### The summary (spec 094's area and rule)

- R4 Lines, in order:
  - **read** and **write**: the disks' rates, with a plot of both over the
    last five minutes;
  - **disks**: one cell per disk, coloured by its busy share (095's heat
    strip), in `/proc/diskstats` order; a disk that arrives has no share
    until its second reading, and one that is plugged in or out changes the
    number of cells but not the strip's height;
  - **pressure** (Linux): PSI's `some` over 10 s, a percent; **queue**
    (Windows): the disks' average queue length, Windows's nearest figure
    (decision D3);
  - **swap**: the rates swapped in and out.

### Readers

- R5 **Linux**: `/proc/<pid>/io`'s `read_bytes` and `write_bytes`, read with
  the process list's own pass over `/proc`; `/proc/diskstats` for the rates
  and each disk's busy share, whole disks only (no partitions, no `loop`,
  `ram`, `zram`, `dm-` or `md` device counted twice); `/proc/pressure/io`;
  `/proc/vmstat`'s `pswpin`/`pswpout` in pages.
- R6 **Windows**: the process table's transfer counts; the `PhysicalDisk`
  counters for the rates, each disk's busy share and the queue; swap from
  `\Memory\Pages Input/sec` and `Pages Output/sec`.
- R7 No new right, service or subprocess (decision D1).

### Also

- R8 `detail cooler/io` prints it all; `--json` carries it.
- R9 README changelog and drawers paragraph; threat model B4 for the new
  files and counters; `docs/architecture.md` if the cooler's details table
  changes a rule.

## Acceptance Criteria

- [x] Fixtures: `/proc/<pid>/io` two samples to rates; an unreadable one is
  left out; diskstats to rates and busy shares, partitions and virtual
  devices not counted; PSI and vmstat lines.
- [x] "other" is the machine's less the listed, never negative.
- [x] Windows: the transfer counts and the disk counters read for real on
  the VM.
- [x] The IO row's width does not change with its values; the switch has IO.
- [x] The summary's shape is fixed across readings; a disk leaving keeps its
  cell.
- [x] The window: the summary by 094's rule; `detail` text and JSON.
- [x] Plasma against `/proc/diskstats` (iotop needs root, which this
  session does not take); the VM against Windows's PhysicalDisk counters,
  Task Manager's source.

## Decisions (2026-11-15)

- D1 **Per-process scope on Linux.** Proposed: the processes a person may
  read, with the rest as "other" (R3); nothing privileged. Alternative: have
  the network helper (spec 072, which already holds `CAP_NET_ADMIN`) read
  taskstats for every process, which is B1 work with the threat model's
  adversarial review, as a later spec. **Decided: readable plus "other".**
- D2 **Windows's per-process figure is all IO**, not the disk alone.
  Proposed: show it, with the list's note saying so ("files, network and
  devices"). Alternative: no IO list on Windows, the summary only.
  **Decided: show it, labelled.**
- D3 **Windows has no PSI.** The disks' queue length in its place, as 095
  put the processor queue in the load average's.
- D4 **The card row**: proposed, read and write rates (R1); alternative,
  the busiest disk's busy share. **Decided: the rates.**

## Risks & Assumptions

- The card grows by one row on every machine with a disk reader: one change
  of shape when the version is installed, not a reading's.
- Busy share from diskstats' IO milliseconds is the kernel's figure; on an
  NVMe with deep queues it reads 100 % well before the disk is saturated, as
  iostat's %util does.
- **Rollback**: revert; no setting or file format changes.

## Threat model delta

B4: `/proc/<pid>/io`, `/proc/diskstats`, `/proc/pressure/io` and
`/proc/vmstat` are new parsed inputs, line formats parsed by fuzzed
functions; device names from diskstats are drawn and pass the printable
filter. No new right (D1 as proposed).

## Test strategy

As 094–096: core fixtures and fuzz targets; view shape tests; panel tests
of the list, "other" and the summary; headless window tests of the row and
the switch; real windows on both systems.

## E2E Test Plan

Desk: the IO row and drawer while copying a large file, against
`sudo iotop -b -o -n 3` and `/proc/diskstats` deltas. VM: against Task
Manager's Performance → Disk page and its Processes tab.

### Prod visibility

Not applicable: a desktop program with no telemetry.

### E2E results (2026-11-15)

Desk, Plasma 6 Wayland at 1.5, installed with fynstall, while a loop wrote
50 MiB a second in bursts to a file under `~/.cache` (removed after):

- The Hardware card gained the IO row (the window 587 to 602 px tall, its
  width unchanged): ↓ 819 B/s ↑ 9.4 MiB/s. The IO drawer's summary agreed;
  seven disk cells (three NVMe, four SATA), pressure 0.0 %, swap 64.0 KiB/s
  in. `/proc/diskstats` over the next five seconds: 1.1 MiB/s read, 2.0 MiB/s
  written (bursts; the windows differ).
- The list put the writing shell first, ↑ 25.0 MiB/s: a reaped child's IO is
  its parent's in `/proc/<pid>/io`. 477 of 1391 processes were readable.

Windows VM, 150 %, cross-built:

- `TestWindowsReadsTheDisks` passes there: one disk, "0 C:", busy 11 %,
  queue 0.11, and the test's own process with its reads.
- With the panel made taller (Peripherals added, settings restored after):
  the IO row, the switch CPU Mem IO, and the IO drawer: one disk cell, read
  0 B/s, write 369.7 KiB/s, queue 0.0, swap 30.5 KiB/s in, the list of all
  IO under "all IO: files, network and devices". Windows's counters at the
  moment: 99.7 % idle, queue 0, writes in the hundreds of KiB/s.

Falsified: thirteen of fourteen breakages fail their tests (partitions
counted, virtual devices counted, the sector size, the busy share's unit, a
new disk given a share, Windows's busy from idle, rchar for read_bytes, a
rate not divided by time, Windows given an "other", the IO list offered
without disks, a column more, the queue line's name from the reading, an IO
row without rates). The fourteenth, "other" not clamped at zero, is
equivalent: the rate formatter draws a negative rate as zero.
`FuzzParseDiskStats`, `FuzzParseProcIO` and `FuzzParsePressure` ran clean
for 10 s each.
