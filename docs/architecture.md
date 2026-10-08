# Architecture

How hayami is put together, and the rules a change is held to. It is written
for anyone changing the code, maintainer or contributor, by hand or with an
assistant. A pull request that goes against a rule here should say which rule
and why. Changing a rule starts as a GitHub Discussion, as CONTRIBUTING.md
asks of any change in direction.

Some of the mechanisms below exist and some are being built. Each one carries
its status, and [Status](#status) at the end lists them with their issues.
Where a mechanism is not built yet, follow the rule anyway and use the nearest
existing pattern. Don't invent a third one.

## The shape

hayami reads facts about a desk: device batteries, network rates, processor
and coolant temperatures, coding-agent usage. It shows them in two shells, a
desktop window and a terminal pane. Every reading goes through the same
layers, in one direction:

```
 devices ──► sanshoku ─┐
 OS APIs ──► core ─────┼─► panel (sources) ──► view (sections) ──► gui │ tui │ cli
 web APIs ─► usage ────┘          ▲                                    ▲
                                config ◄────────────── prefs ──────────┘
```

| Layer | Package | Owns | Never |
|---|---|---|---|
| Devices | [sanshoku](https://github.com/ushineko/sanshoku), a separate module | Every device protocol, every device node (hidraw, Windows HID, BlueZ, L2CAP, hwmon), vendor IDs, report IDs, which devices a driver supports and on which platform | Formats, colours, wording for the panel |
| Readers | `internal/core` | Facts the operating system or another program gives: byte counters, processor load, the graphics card through OS APIs, LibreHardwareMonitor, Wi-Fi link state. Plain Go values, no toolkit | Drawing, status colours, panel wording |
| Usage | `internal/usage`, `internal/claude`, `internal/codex` | The usage cache shared with the Python tools, OAuth refresh, the providers' APIs | Anything about devices |
| Sources | `internal/panel` | Polling: which readers a section asks, how often, what it remembers between polls (trails, held devices, the last good reading), and turning readings and absences into a `view.Section` | Platform build tags, device protocols, toolkit calls |
| View | `internal/view` | The section vocabulary (`Section`, `Row`, `Cell`, `Meter`, `Trail`, `Reason`, `Status`), fixed-width formatting, thresholds, the text arrangements | Imports from any other hayami package; any toolkit |
| Shells | `internal/gui`, `internal/tui`, `internal/cli` | Arranging sections on a surface: the Fyne window, the Bubble Tea pane, `doctor`, `readings` | Deciding what a section says |
| Settings | `internal/config`, `internal/prefs`, `internal/desktop` | The settings file, the preferences window, window placement and the KWin rule | Reading devices or sensors |

**Dependencies point one way.** A package imports only the layers to its left
in the diagram, plus `view` and `config`. `view` imports nothing from hayami.
Current exceptions: `prefs` asks `core` for the interface list its chooser
shows, `gui` reads `core.BandwidthTrail`, and `cli` passes a `core.Counters`
reader. These are known, and a new one needs a reason in its PR.

## Rules

### 1. Device knowledge lives in sanshoku

A vendor name, a protocol, a report ID, a product ID, a transport detail
(`hidraw.PairedChild`, a Logitech receiver's presence) does not appear in
hayami. Support for a new device is a sanshoku driver, with its entry in
sanshoku's support table and a bench reading from the real device, released.
Then hayami runs `go get`.

hayami's side today is one row per vendor in `vendors()`
(`internal/panel/peripherals.go`). The aim is none: sanshoku drivers describe
themselves (vendor, whether a silent device is listed, platforms) and hayami
builds its list from them. Until then, `panel/peripherals.go` still imports
`logitech` and `hidraw` for receiver presence. Don't add more of those imports.

### 2. Tables over switches

When behaviour depends on a key, such as a section, vendor, platform, device
kind, threshold, provider or label, the key is a row in a table and the code
reads the table. A `switch` or `if` chain on a string key or a platform is a
sign that a table is missing, and a review will ask for one.

Tables that exist: the fixed-width formatters (`view/format.go`), name
shortening (`view.nameRules`), the peripherals vendor list, sanshoku's support
table, threshold bands (`view.Bands`) and the section registry.

### 3. A section is one registry entry

A section is declared once: its key, title, icon and whether it is on by
default in `view.Sections()` (`internal/view/sections.go`), and its source's
constructor in `panel.Specs()` (`internal/panel/registry.go`), which takes one
`panel.Env` and panics if the two lists disagree. The key lists, the default
settings, the preferences list, `--sections` and the icon lookup all read
them. Adding a section is an entry in each, a glyph in `gui/icons.go`, a source
and a view builder; nothing else spells its key. A setting a source needs goes
in `Env`, not in a new parameter.

Every section renders in all three arrangements (stack, grid, row), in both
shells, and a hidden section's source is not polled. What the two shells draw
is compared fact by fact (`internal/gui/parity_test.go`, spec 047): every
label, value, detail line, cell and meter a section holds must be on the
window's card and in the terminal's rendering in each arrangement, unless the
test's allow-list says why not. A new section adds a fixture there.

### 4. A sensor source is a provider in a chain

Where a quantity has more than one possible source, the sources are providers
tried in a declared order, per platform. Examples are the processor's
temperature (hwmon, LibreHardwareMonitor), the graphics card (D3DKMT, the
`GPU Engine` counters, hwmon, the AMD busy file, nvidia-smi) and a battery.
The chain merges what each provider gave and records what it tried, so the
reason for a gap is written where the attempt is made, not retold by the
panel.

A provider is `core.Provider[T]` (a name a person reads, and a `Read`), and a
chain is `core.Chain[T]` (`internal/core/chain.go`): providers in order, a
`Merge` that fills what is missing and says when the reading is complete, and
an `Outcome` recording every provider tried and the first account of absence
one gave. Each platform declares its chains in `internal/core/host_tables.go`,
which has no build tag so a test on any system checks every platform's order
and wording. The processor's temperature and the graphics card are chains;
`core.GraphicsReader.Chain` builds the card's from the routes whose fields are
set, so a Windows host never looks under `/sys`. Adding a source is a provider
and a row in its platform's chain; the reason for a gap names it by itself.

What the chains read reaches the view as a list, not a field per part. A
cooler reading is `view.CoolerReading{Probes}` (`internal/view/cooler.go`):
each probe has an ID, a role, a name and its values as `view.Opt`, and
`view.Cooler` draws whatever probes there are, in the order and with the
colours its role table (`coolerRoles`) gives each role. A second graphics card
or a motherboard temperature is another probe from its source, and nothing in
the view changes. Each row carries its probe's ID (`view.Row.ID`), and the
window matches a poll's rows to the card's by it: a row arriving is inserted
above the plot, a row leaving is removed, and a value changing moves nothing
(spec 044).

### 5. Platform code: one file per platform, in core

Platform differences are build-tagged files with the same function signatures:
`x_linux.go`, `x_windows.go`, `x_other.go`. `_other` means *absent*. It returns
nothing, or an error that says so. It never assumes Linux paths. Platform files
live in `core`, plus `desktop` for window management. `panel`, `view` and the
shells contain no build tags and no `runtime.GOOS` checks. They take what the
platform offers from `core`.

What a platform offers is one value, `core.Host`, declared by `core.NewHost`
in `host_linux.go`, `host_windows.go` and `host_other.go`: the processor and
card chains and their accounts of absence, the load and name readers, the
interface and Wi-Fi readers, the advice for a device that would not open, and
the Bluetooth drivers it can use. `panel` takes it through `panel.Env.Host`
(nil is this platform's own), and a test builds one of its own. A file that
only one platform can have, such as `lhm_other.go` (LibreHardwareMonitor is a
Windows program), uses `!windows` and returns absence.

Outside `core` and `desktop`, `internal/usage/path.go` still chooses the cache
directory by `runtime.GOOS`, and `internal/testenv` does for the device scan.

### 6. Absence is a value, not a sentence

A reading that is missing is normal: the hardware isn't there, a device is
asleep, a driver isn't installed, permission was refused. The code that finds
the gap returns a typed absence (`core.Absence`, `internal/core/absence.go`)
with a code, the text a person reads, its detail, and whether a person can act
on it. One helper, `panel.reason` (`internal/panel/reason.go`), turns any
error into a `view.Reason`. Decisions, such as whether a reason stays on the
card or what advice to give, are made on the code. Comparing reason text is
not allowed.

A reason that is the normal state of a platform, such as no processor
temperature on Windows without LibreHardwareMonitor, is an aside: it shows
on hover and in `doctor`, not as a line on the card.

### 7. Thresholds are bands

A status chosen by value (warn, bad) comes from a `view.Bands` table
(`internal/view/bands.go`): ascending thresholds mapped to statuses. It
doesn't come from comparisons written inline. A new reading with a status
adds a table, not a ladder.

### 8. The glance rules

- **Nothing transient reflows the window.** Every value is formatted to a fixed
  width. Only a section appearing or disappearing may change the window's
  size.
- **The render thread never blocks.** Readers run off it, take a
  `context.Context`, and honour cancellation and a timeout. A device or sensor
  that hangs costs its own timeout and nothing else.
- **A sleeping device is not a missing one.** A device that has answered
  before and is silent now keeps its last reading, dimmed. A device silent for
  a week is treated as put away (specs 022, 032). A driver reports silence as
  no reading and no error.

### 9. What hayami may run and reach

- **No subprocess reads a device.** The one subprocess is `nvidia-smi`, for an
  NVIDIA card on Linux, where the driver offers no kernel interface. On
  Windows it is only the fallback behind D3DKMT.
- **Network:** the usage providers' APIs, and LibreHardwareMonitor's
  `data.json` over loopback. A new network destination needs a Discussion
  first.
- **No kernel driver of its own.** Where the OS needs one, as for the
  processor's temperature on Windows, hayami reads a program that has one
  (LibreHardwareMonitor) and never installs it unasked.
- **Privacy.** The repository is public. Network names (SSID, BSSID), device
  addresses, serials, hostnames, user names and home paths are not read where
  avoidable, and never stored, logged, shown or committed. Fixtures use
  invented values.

### 10. Seams for tests

A source or reader takes its dependencies as constructor arguments or function
fields defaulted in the constructor (`Cooler.load`,
`GraphicsReader.Native`, `BandwidthSection.read`), or as a small interface. A
test replaces them. It does not reach for real hardware, the real settings
file, the real usage cache or the network. Package-level variables that tests
reassign are not added. `panel.DefaultScan` remains as the fallback when an
`Env` carries no scan, until the commands take an `Env` from their callers.

## Adding things

| To add | Where | Also |
|---|---|---|
| Support for a device | A sanshoku driver and its support-table entry, with a bench reading from the device; a sanshoku release | In hayami: `go get` the release, plus a `vendors()` row while rule 1's aim is unbuilt |
| A reading from the OS or another program | A reader in `core`, with `_linux`/`_windows`/`_other` files, a field in `core.Host`, and a fake for tests | It appears in `hayami-tui readings` and `doctor` through its section |
| Another source for an existing quantity | A `core.Provider` and a row in its platform's chain in `core/host_tables.go` (rule 4), in priority order | The absence it can report, as a typed absence |
| Another part the cooler reads (a second card, a motherboard sensor) | A `view.Probe` from its source, with an ID unique in the reading; a row in `view.coolerRoles` only for a role the table does not have | No view or window change for a known role |
| A section | An entry in `view.Sections()` and `panel.Specs()`, a glyph, a source in `panel`, a builder in `view` | Renders in stack, grid and row in both shells; a hidden section is not polled; a fixture in the parity test |
| A threshold or status | A `view.Bands` table | A test at and around each threshold |
| A setting | Today a `config.Config` field; settings by section are planned | The preferences window, if a person would change it more than once |
| A platform | `_<os>.go` files in `core` (and `desktop`), including its `core.NewHost`; every `_other` stays honest | CI builds and tests on it |

## Testing

These rules apply in addition to CONTRIBUTING.md's.

- **Tests encode behaviour.** A refactor that changes no behaviour changes no
  test assertion.
- **Falsify before trusting.** Break the change, watch its test fail, put it
  back. Say in the PR that you did.
- **A claim about what the window looks like is checked on a real window.**
  `tests/window` drives the real binary and reads geometry from the picture
  (`HAYAMI_WINDOW_TEST=1`). Headless widget tests answer a different question.
- **Hardware is read for real once, then faked.** A driver or reader change
  carries a real reading in its PR, and its tests run on fakes.
- **Tests never touch the desk.** `internal/testenv` replaces the home,
  settings, cache and device scan. A test that needs the real usage cache
  reads it and skips when it is absent.

## Status

| Mechanism | Rule | State | Issue |
|---|---|---|---|
| sanshoku as the only device layer | 1 | In place | |
| Drivers describe themselves; hayami derives its vendor list | 1 | Planned (phase 3) | |
| Section registry | 3 | In place | #126 |
| Threshold bands | 7 | In place | #127 |
| Typed absences, one reason helper | 6 | In place | #128 |
| One window-test harness, rows found by label | Testing | In place | #129 |
| `Provider`/`Chain` for sensors; one platform table in `core` | 4, 5 | In place | #137 |
| Cooler reading as a list of probes; rows matched by ID in the window | 4 | In place | #138 |
| Wi-Fi link reading with `Opt` values instead of `Has` flags | 4 | Planned | |
| Settings by section | Adding things | Planned (phase 4) | |
| Parity test that compares what the shells draw | 3 | In place | #144 |
| `row` drawn by the window | 3 | Waiting on fynedesygn#170 | #144 |

The window draws `row` as `stack` (`gui.go`, `arrangement`): the design
system has no row arrangement for a panel (fynedesygn#170), and building one in
`internal/gui` is what the design system's rules forbid. The preferences offer
`row` marked "terminal only", because the setting is shared and the terminal
pane honours it. This is the one known gap against rule 3.
