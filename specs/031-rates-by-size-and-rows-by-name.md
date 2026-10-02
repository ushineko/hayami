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

- [x] `go test ./...` and `make lint` pass.
- [x] View tests: a rate in each band gets that band's status, separately for
      down and up; totals carry none; the padded widths are unchanged.
- [x] Name tests: the shortening table on real names from both desks (the i9,
      the RTX 4090 and 3080, the Kraken Elite V2, and an AMD CPU and card
      from the public pci.ids); a name longer than the column is cut with an
      ellipsis and the full name is kept for the tooltip.
- [x] A test shows the cooler card's size is the same with names and without.
- [ ] **On the desk:** photographs of the panel on njv-cachyos and cachyos
      with names showing, a rate in at least two bands (a download), and the
      tooltip with a full name; the card does not change size when the panel
      starts and names arrive.
- [ ] `hayami-tui` shows the same, photographed in a terminal.
- [x] README changelog under Unreleased.

## Risks & Assumptions

- **pci.ids** is installed by `hwdata`, which Arch and CachyOS pull in. Where
  it is missing the GPU label stays "GPU".
- **nvidia-smi's name field** adds one column to the query already made each
  poll; no new process.
- **Colour in the terminal**: the TUI's palette already has these statuses.
- **Rollback**: revert.

## Verification

### Choices the requirements left open

- **Statuses.** Two emphases were added to `view.Status` after `Bad`, so every
  number a cache holds keeps its meaning: `Accent` (1–10 MiB/s, the scheme's
  info colour) and `Strong` (100 MiB/s and above). 10–100 MiB/s reuses `Warn`.
  `Info` could not be the info colour: on a row it is plain text in both
  shells. The terminal draws `Accent` blue (ANSI 4) and `Strong` magenta in
  bold (ANSI 5); the window draws `Accent` in the theme's primary colour and
  `Strong` in the design system's categorical magenta
  (`glance.SeriesColour(th, 3)`) in bold, the one colour of its set no verdict
  takes.
- **Parts.** `Row.Parts` are the four pieces of a bandwidth value (arrow, down
  rate, arrow, up rate); their texts joined are `Row.Value` exactly, and a test
  holds the value to the string it was before parts. Both shells draw them as
  parts; the row's own status stays Info.
- **Label width.** `view.LabelWidth` is 15 characters, the widest short name
  on either desk ("Kraken Elite V2"). The cooler's three named rows carry
  `Row.LabelWidth`, and the pane's row arrangement reserves it in its label
  column, so a name arriving moves no other section's columns.
- **Window label column.** Pinned to fifteen lower-case letters in the panel's
  face (111 px at the default size; "Kraken Elite V2" measures 84), not
  capitals, which would widen the card for names that are never that wide.
- **Tooltips.** `Row.Tip` holds "CPU: <full name>"; `Section.Hover` puts the
  rows' tips first, so the window shows them in the card's existing tip.
  `hayami doctor` prints them under the cooler's summary.
- **GPU names.** `nvidia-smi` is asked for `name` as a third, last column.
  A card read through hwmon is named from `pci.ids` by the vendor and device of
  the PCI function behind the hwmon chip (or behind the busy file), looked up
  once per process. The bracketed product name is used and the chip code name
  dropped; a device ID that covers several models ("RX 7900 XT/7900 XTX/…") is
  cut by the ellipsis rather than guessed.

### Checks

| Check | Result |
|---|---|
| `go vet ./...` | clean |
| `go test ./...` | all packages ok |
| `make lint` (golangci-lint v2.12.2) | 0 issues |
| `make build`, `make check-no-binaries` | both binaries built; none tracked |
| Falsified: `Row.LabelWidth` not set | `TestANameArrivingMovesNoValue` fails (row arrangement: the usage meter moved) |
| Falsified: parts not painted | `TestThePaneColoursEachRate` fails |
| Falsified: names not shortened | `TestTheCoolerCardIsTheSameSizeWithNames` fails (panel 335.7 → 344.2 and 407.9 px wide) |
| `hayami-tui doctor`, the desk with the Kraken, throwaway settings | `i9-14900K`, `RTX 4090`, `Kraken Elite V2`; full names printed under the summary |
| `hayami-tui doctor`, the other desk, throwaway settings | `i7-13700K`, `RTX 3080`, Coolant "no cooler" |
| Headless window render (Fyne test driver, scratch test not committed) | panel and cooler card the same size with and without names on both desks; down and up coloured independently |
| Falsified: label column not pinned | `TestTheCoolerCardIsTheSameSizeWithNames` and `TestTheLongestNamesDoNotWidenTheCard` fail |

Measured in `TestTheCoolerCardIsTheSameSizeWithNames` (test theme): the cooler
card is 280.1 px wide on the Kraken desk and 276.7 px on the other, the same to
the pixel with names and without. Pinning costs the other desk's card the width
of the column it reserves (it was 194.2 px with "CPU"), still inside the
bandwidth card's 335.7 px, so the panel is the width it was.

### Gaps found

Found against fynedesygn v0.1.79; three are resolved by fynedesygn spec 051
(issue #158), which this branch builds against from its worktree until
v0.1.80 is released.

1. **A row value of several coloured parts.** Resolved by 051:
   `glance.Reading.Parts` and `glance.Parted`. The window now draws each rate
   as its own part in its own band, and the "row in its faster rate's colour"
   stopgap and `Row.Status` as the strongest part are gone.
2. **Bold on a row's value**, for `Strong`. Resolved by 051: `glance.Part.Bold`
   (drawn where the theme has a bold monospace face).
3. **A fixed pixel width for a row's label column.** Resolved by 051:
   `glance.NewRowWidth`. The cooler's named rows are built with their label
   column pinned to fifteen lower-case letters in the panel's face and size,
   so a name -- any name, fifteen capitals included -- cannot change the card's
   size. One limit remains: the width is taken when the rows are built, so a
   text size changed in the preferences is not followed by the pin until the
   next start. Pinned rows are chosen by the first section's rows, so a panel
   restored from a cache written before this change pins nothing until it is
   restarted once.
4. **A tip per row.** Resolved by the card's tooltip: the rows' full names are
   its first lines, which is where the reader's pointer already is.

Bandwidth interface labels are not pinned: they come from the settings, do not
change while the panel is up, and pinning them would only move today's
columns.

Desk criteria (photographs on both desks and of `hayami-tui`) are not done.

## Status: INCOMPLETE
