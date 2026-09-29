# 015 — why a section is missing

**Issue**: #54

## Context

On `cachyos` — an Intel box with no AIO, no Logitech receiver, no Bluetooth
adapter and a Razer/SteelSeries desk — hayami 0.3.5 draws the bandwidth
section and nothing else. Three of the four sections are absent and the panel
says nothing about any of them.

That silence is the fault this spec is about. The three underlying bugs are
real and are fixed here, but they were only findable by ssh, a throwaway probe
binary and half an hour: the panel's own answer to "why is there no cooler
card?" was an empty space, which is the same empty space a machine with no
cooler would show. A reader cannot tell a legitimate absence from a build that
is failing, and neither could the author.

Both shells currently end a poll with

```go
drawn, err := s.Poll(ctx)
if err != nil {
    drawn = false
}
```

— the error is discarded and the section disappears. `panel.Source` has no way
to say *why* it has nothing, so there is nothing for a shell to draw even if it
wanted to.

### The three bugs

**The cooler is lost to a subprocess exit status.** `liquidctl --match kraken
status` on a machine with no Kraken prints `ERROR: no device matches available
drivers and selection criteria` and exits 1. `cooler.Cooling` turns any
non-zero exit into a generic error rather than `ErrNoCooler`, which is the one
thing `panel.Cooler.Poll` forgives. The CPU sensor is fine — `coretemp` /
`Package id 0` reads 38 °C — but the whole section is dropped.

**`readings` aborts on the first source that errors.** `cli.Readings` returns
on the first failure, so the one command meant for debugging a machine with no
display printed the liquidctl error and nothing about the other three
sections.

**The usage section was held behind a poisoned cache entry.** `usage-max.json`
held `{"next_attempt_at": …, "fetched_at": null, "data": null}` — a failed
fetch that set a ~2132 s backoff from a `Retry-After`. `usage.Claude` decodes
the literal `null` into zero windows and no error, so the failure is invisible;
and `withoutSupersededDefault` drops the nameless `usage.json` — which the
Python widget keeps full and fresh — in favour of the named account that has
nothing. A forced fetch succeeded immediately: the credentials and the
endpoint were never at fault.

Peripherals on that machine is a coverage gap rather than a bug — no adapter,
no receiver, no `headsetcontrol`, and hardware hayami does not speak to — but
from the outside it is indistinguishable from the other three, which is the
point.

## Requirements

### A section says why it has nothing to say

`view.Section` gains `Reasons []Reason`:

```go
type Reason struct {
    Label  string // "Coolant", or empty for a line across the section
    Text   string // what to show: "no cooler", "liquidctl failed"
    Detail string // the underlying error, for the tooltip and for doctor
    Status Status // Info for hardware that is not there, Warn for a failure
}
```

`Info` and `Warn` carry the whole distinction the issue asks for: hardware
this machine does not have is stated plainly and without alarm; a source that
tried and failed is marked. `Detail` is never drawn on the card — it is the
hover note in the window and a line in `doctor` — because a card is read at a
glance and an exit status is not a glance.

A reason is drawn dim, after the rows and before the meters, in both shells.

### A section with reasons is drawn

Both shells replace `if err != nil { drawn = false }` with the rule that a
reason is something to draw:

```go
drawn, err := s.Poll(ctx)
sec := s.Section()
if len(sec.Reasons) > 0 {
    drawn = true
}
```

A source turns its own errors into reasons; `Poll`'s error return is left for
what a source cannot describe — a cancelled context — and no longer decides
whether anything appears. A section that a source has no reading *and* no
reason for is still not drawn: an unconfigured section is not a fault.

Only the readings are cached, never the reasons. A reason is a statement about
this moment; restoring "liquidctl failed" from yesterday's cache would be
asserting a failure nobody has observed.

### liquidctl's own words are classified, not swallowed

`cooler.Cooling` reads `exec.ExitError.Stderr`. `no device matches available
drivers and selection criteria` is `ErrNoCooler` — the machine has no cooler,
which this program has always said is not a problem. Any other non-zero exit
stays the error it is and becomes a `Warn` reason carrying the exit status and
the first line of stderr.

The cooler's reasons:

| condition | label | text | status |
|---|---|---|---|
| no `coretemp` sensor | CPU | `no sensor` | Info |
| liquidctl absent | Coolant | `no liquidctl` | Info |
| `ErrNoCooler` | Coolant | `no cooler` | Info |
| any other failure | — | `liquidctl failed` | Warn |

### BlueZ that is not there is not a failure

`peripherals.ErrNoBluez` already covers a bus name that cannot be activated --
`bluetoothDevices` wraps every D-Bus failure in it -- so `Could not activate
remote peer 'org.bluez': unit failed` was being forgiven correctly and the
section was simply silent about it. The change is the reason, not the
classification; the classification gets the test it did not have.

That message is a machine with no adapter: `/sys/class/bluetooth` does not
exist and `bluetooth.service` skips its `ConditionPathIsDirectory`. It is Info,
not Warn.

The peripherals' reasons: `no Logitech receiver`, `headsetcontrol is not
installed`, `no Bluetooth adapter` — each Info, each omitted when that source
did find something. A source that failed for any other reason is Warn with the
error as Detail.

### A cached `null` is not a reading

`usage.Read` normalises a `data` of literal JSON `null` to no data. The read
side is the only side that needs it: a nil payload and the literal serialise to
the same four bytes, so nothing can be put in the file that a read does not
take back out. The cache file's shape is unchanged — it is shared with two
other programs — only this build's reading of it.

`withoutSupersededDefault` drops the nameless `usage.json` only when a named
Claude account **has data**. The nameless file is what the widget wrote before
profiles existed and is usually dead, but a named account that has never
fetched is deader, and preferring it cost the section entirely.

The usage section's reasons: `no Claude or Codex account` when there are none;
and, for an account whose gate is closed after a failed fetch with nothing
cached, a Warn reason saying so and when the next attempt is due. That gate is
written into a file three programs read and is not shortened here — but a
panel that is waiting 35 minutes should say that it is waiting.

### An interface that is not there says so

A configured interface the kernel does not list is an Info reason naming it,
rather than a row that is silently absent.

### `readings` reports every source

`cli.Readings` polls every source and never returns early. Each key carries
what that source has:

```json
{
  "bandwidth": { "data": [ … ] },
  "cooler": {
    "data": { "CPU": 38, … },
    "reasons": [ { "text": "no cooler", "status": "info" } ]
  },
  "peripherals": {
    "data": { "Devices": null },
    "reasons": [ { "text": "no Bluetooth adapter", "status": "info" } ],
    "error": "…"
  }
}
```

`error` appears only when `Poll` returned one. The exit status stays 0 when any
source answered, so the JSON is always usable.

### `doctor` on both binaries

`hayami doctor` and `hayami-tui doctor` print every section this build knows —
not only the configured ones — with its state and its reasons, and what each
source probed. It is plain text on stdout, one section per block:

```
bandwidth     ok       enp3s0, tailscale0
cooler        partial  CPU 38 °C from coretemp/Package id 0
                       no cooler
                       liquidctl --match "kraken": no device matches
                       available drivers and selection criteria
peripherals   absent   no Logitech receiver
                       headsetcontrol is not installed
                       no Bluetooth adapter
usage         ok       claude/max (M), fetched 12s ago
```

States: `ok` (readings, no reasons), `partial` (readings and reasons),
`absent` (reasons only), `off` (not in the settings). Detail is indented under
its reason. A section not in the settings is still probed and still reported,
because "I turned it off" is one of the answers a person needs.

No credential, token or account identifier beyond the profile name the cache
filenames already carry appears in the output. `doctor` is what gets pasted
into an issue.

## Executive Summary

A section that cannot be drawn now says why, instead of disappearing. Three
bugs made that visible on one machine -- liquidctl's exit status swallowing the
whole cooler section, `readings` aborting on the first failure, and a cached
`"data": null` that decoded to an empty Usage section with no error -- and all
three are fixed. `view.Section` gains `Reasons`, both shells draw them dim
after the rows with the detail on hover, and `hayami doctor` reports every
section's state and what each source probed.

Reviewers should look at `internal/view/view.go` first: `Reason`, `Lines` and
`Hover` are the contract both shells and `doctor` render, and everything else
follows from them. Then `internal/gui/gui.go`'s `RowSlack`, which is a
workaround for a design-system constraint and is written up under "Gaps found".

## Acceptance Criteria

- [x] `view.Reason` exists with Label, Text, Detail and Status, and
      `view.Section.Reasons` carries them
- [x] Both shells draw a section that has reasons and no readings, dim, and
      neither discards a poll error into invisibility
- [x] `readings.Save` writes no reasons, and a restored section carries none
- [x] `cooler.Cooling` returns `ErrNoCooler` for liquidctl's "no device
      matches" on stderr, and the generic error for any other non-zero exit
- [x] The cooler section on a machine with no cooler draws the CPU row and an
      Info reason, and on a machine where liquidctl fails draws a Warn reason
      carrying the exit status in Detail
- [x] `peripherals.ErrNoBluez` covers a bus name that cannot be activated, and
      the peripherals section reports absent receiver, absent headsetcontrol
      and absent adapter as Info reasons
- [x] `usage.Read` treats a `data` of JSON `null` as no data, and a payload
      written as the literal does not survive the round trip
- [x] `withoutSupersededDefault` keeps the nameless account when every named
      one has no data
- [x] A usage account whose gate is closed after a failed fetch with nothing
      cached draws a Warn reason naming the next attempt time
- [x] A configured interface the kernel does not list draws an Info reason
- [x] `cli.Readings` polls every source, returns no error when any source
      answered, and emits `data`, `reasons` and `error` per key
- [x] `hayami doctor` and `hayami-tui doctor` exist, report every section this
      build knows with a state of ok/partial/absent/off, and are covered by the
      CLI parity test
- [x] `doctor` output contains no token, no credential and no path inside a
      credential store
- [x] The whole suite passes, and `go vet` and the linter are clean

## Risks & Assumptions

- **Rollback** is `git revert` of the merge. Nothing here writes to the game,
  the desktop or a credential store, and the one file written outside this
  program's own cache — the shared usage cache — keeps its existing shape.
- **The shared usage cache is three programs' file.** Normalising a `null`
  `data` is a change to how this build *reads* it; the bytes written keep the
  same keys. The Python widget's own reader is unaffected.
- **liquidctl's error text is matched as a substring.** A future liquidctl
  that rewords it falls back to the generic error, which is a Warn reason with
  the text in Detail — visibly wrong rather than silently absent, which is the
  failure mode this spec prefers.
- **No integration boundary in the spec's sense**: no DB write path, no queue,
  no search index, no async job. The two external boundaries — the liquidctl
  subprocess and the usage endpoint — are covered by the existing live tests
  that skip when the hardware or the credentials are absent.
- The Razer Mouse Dock Pro and the SteelSeries Apex Pro TKL Wireless are **out
  of scope**. This spec makes their absence legible; speaking their protocols
  is separate work and needs its own issue.

## Gaps found

Two in the design system, neither worked around in `internal/gui`:

- **`glance.Card` adds every object to one container in order**, so a row added
  after the card was built lands *under* the sparkline rather than above it.
  The first photograph of this change showed "Coolant  no cooler" drawn through
  the plot. Worked around here the way spec 003 worked around the same
  constraint for cells: the card is built with `RowSlack` spare rows, hidden.
  The library wants an insert-before-objects, or rows and objects in separate
  containers.
- **`glance.Row` exposes its reading and not its label.** A test that wants to
  know what a row says has to walk the object tree for its `canvas.Text`, which
  `internal/gui/export_test.go` now does. A `Label()` getter beside `Reading()`
  would do.

## Alternatives Considered

Considered showing a card only for a *failure* and leaving genuinely absent
hardware hidden; rejected because an invisible section is exactly what made
this machine take half an hour to diagnose, and "legitimately absent" is a
thing the reader needs told.

Considered putting a reason's detail on the card rather than in the hover;
rejected because a card is read at a glance and a subprocess's exit status is
not, and the panel is 260 px wide.

Considered shortening the usage backoff when nothing has ever been cached;
rejected because the gate is written into a file three programs read, and a
build that set its own shorter one would be deciding for the other two. The
panel says it is waiting instead.

## Status: COMPLETE
