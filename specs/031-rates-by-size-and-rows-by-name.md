# 031 — rates by size, and rows by name

**Issue**: #112

## Context

Two things a glance at the panel cannot tell today.

**Which interface is busy.** The bandwidth card draws every rate in the same
colour. 12 KiB/s and 80 MiB/s differ only in their digits and their unit, and
a reader has to read both to see that one interface is moving a file. The
trend line shows it, at the size of a sparkline; the numbers do not.

**What the cooler rows are about.** The rows are labelled CPU, GPU and
Coolant. A machine has one of each, but which one is a fact the panel knows
and does not say, and the two desks it runs on differ (an i9 and an RTX 4090
with a Kraken Elite V2 on one; a different CPU, an RTX 3080 and no cooler the
panel can read on the other).

Both changes are in the view model (`internal/view`), so the window and the
terminal draw them the same way. The panel's rule that nothing reflows applies
to both: a card does not change size, and a value does not move, because a
rate grew or a name arrived.

## Requirements

### R1. Rates coloured by size

- R1.1 Each rate on a bandwidth row (down and up, separately) carries its own
  status. A row today has one `Status` for the whole line; the value gains
  per-part statuses (`Row.Parts`, each a piece of text and a status), and
  both shells colour each part. A row without parts draws as now.
- R1.2 The bands, by the rate in bytes per second:

  | Rate | Drawn as |
  |---|---|
  | below 1 MiB/s | as now (the row's normal text) |
  | 1 MiB/s to below 10 MiB/s | the scheme's info colour |
  | 10 MiB/s to below 100 MiB/s | the scheme's warning colour |
  | 100 MiB/s and above | the scheme's strongest accent, bold where the shell has bold |

  A rate is not a fault, so these are emphasis, not alarm: no band uses the
  error colour. The thresholds are constants in `view` with their reason.
- R1.3 Totals (the detail line) stay as they are: they are a record, not a
  rate.
- R1.4 Colour changes nothing about width: the parts are the same padded
  strings the row draws today.

### R2. Cooler rows named by their hardware

- R2.1 The CPU row's label is the processor's model, from `/proc/cpuinfo`
  ("model name"), shortened (R2.4).
- R2.2 The GPU row's label is the card's model: from `nvidia-smi` for a card
  on NVIDIA's driver (added to the query the panel already makes, so no second
  process), and from the PCI ID database (`/usr/share/hwdata/pci.ids`) for an
  AMD or other card found through hwmon/DRM.
- R2.3 The Coolant row's label is the cooler's model, from sanshoku's identity
  for the Kraken ("Kraken Elite V2"). With no cooler the row says what it says
  now.
- R2.4 Names are shortened to what identifies the part: vendor boilerplate and
  marketing words are removed ("Intel(R) Core(TM)", "(R)", "(TM)", "CPU @ …
  GHz", "n-Core Processor", "NVIDIA GeForce", "AMD Radeon", "NZXT"). The rules
  are a table in `view` with tests on real names from both desks.
- R2.5 **No reflow.** The label column has a fixed width, set once from the
  widest label it can hold. A name longer than that is cut with an ellipsis,
  and the full name is the row's tooltip in the window and is printed by
  `hayami doctor`. A name that arrives after the first poll replaces "CPU" in
  place without moving the values. Neither shell's card changes size when
  names appear.
- R2.6 Where a name cannot be read, the label stays "CPU", "GPU" or "Coolant".

### R3. Both shells

- R3.1 The window and `hayami-tui` both draw R1 and R2. The terminal colours
  parts with the same statuses it already maps to its palette.

## Acceptance Criteria

- [ ] `go test ./...` and `make lint` pass.
- [ ] View tests: a rate in each band gets that band's status, separately for
      down and up; totals carry none; the padded widths are unchanged.
- [ ] Name tests: the shortening table on real names from both desks (the i9,
      the RTX 4090 and 3080, the Kraken Elite V2, and an AMD CPU and card
      from the public pci.ids); a name longer than the column is cut with an
      ellipsis and the full name is kept for the tooltip.
- [ ] A test shows the cooler card's size is the same with names and without.
- [ ] **On the desk:** photographs of the panel on njv-cachyos and cachyos
      with names showing, a rate in at least two bands (a download), and the
      tooltip with a full name; the card does not change size when the panel
      starts and names arrive.
- [ ] `hayami-tui` shows the same, photographed in a terminal.
- [ ] README changelog under Unreleased.

## Risks & Assumptions

- **pci.ids** is installed by `hwdata`, which Arch and CachyOS pull in. Where
  it is missing the GPU label stays "GPU".
- **nvidia-smi's name field** adds one column to the query already made each
  poll; no new process.
- **Colour in the terminal**: the TUI's palette already has these statuses.
- **Rollback**: revert.

## Status: INCOMPLETE
