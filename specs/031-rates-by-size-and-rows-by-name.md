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
  (`glance.SeriesColour(th, 3)`), the one colour of its set no verdict takes.
- **Parts.** `Row.Parts` are the four pieces of a bandwidth value (arrow, down
  rate, arrow, up rate); their texts joined are `Row.Value` exactly, and a test
  holds the value to the string it was before parts. `Row.Status` is the
  strongest part, which is what a shell that draws the value as one piece uses.
- **Label width.** `view.LabelWidth` is 15 characters, the widest short name
  on either desk ("Kraken Elite V2"). The cooler's three named rows carry
  `Row.LabelWidth`, and the pane's row arrangement reserves it in its label
  column, so a name arriving moves no other section's columns.
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
| Headless window render (Fyne test driver, scratch test not committed) | panel 288×280 with and without names and in every band; whole row in the faster rate's band |

Measured in `TestTheCoolerCardIsTheSameSizeWithNames` (test theme): on the
Kraken desk the cooler card is 280.1 px wide with names and without, because the
speeds row decides it. On the other desk it grows from 194.2 to 230.2 px, under
the bandwidth card's 335.7, so the panel does not move. The longest names the
view makes, on that desk, measure 277.9 px.

### Gaps found

For fynedesygn's `glance` package; nothing was changed there.

1. **A row value of several coloured parts.** `glance.Row` draws its value as
   one `canvas.Text` in one colour, so the window colours a bandwidth row in its
   faster rate's band (R1.1 asks for each rate in its own). Needed: a value made
   of segments, each with a status or colour, laid out as one monospace run so
   widths are unchanged.
2. **Bold on a row's value**, for `Strong`. The window has the colour and not
   the weight.
3. **A fixed pixel width for a row's label column**, as `glance.NewMeter` takes
   one. The window's no-reflow guarantee for names is a measurement against the
   bandwidth card, not a property: a label of fifteen wide capitals, or a cooler
   card shown alone, could still widen the panel when a name arrives.
4. **A tip per row.** The full names are lines of the card's tip, which is the
   whole card's hover; R2.5 asks for the row's.

Desk criteria (photographs on both desks and of `hayami-tui`) are not done.

## Status: INCOMPLETE
