# hayami (早見)

**Version**: 0.1.0

*a chart you read at a glance*

A panel for Linux showing peripheral battery, network bandwidth,
liquid-cooler thermals and Claude Code and Codex usage — on the desktop as a
frameless always-on-top window, and in a terminal as a pane.

> **Status**: four sections draw in both panels, in all three arrangements,
> from settings both read: bandwidth, usage — fetched by hayami itself, through
> the cache it shares with the tools it replaces — the cooler, and the
> peripherals. The program it replaces,
> `ag-scripts/peripheral-battery-monitor`, is the behavioural reference and is
> still the one to run: its keyboard and AirPods readings have no equivalent
> here yet.

## Contents

- [What it will do](#what-it-will-do)
- [Arrangements](#arrangements)
- [Architecture](#architecture)
- [Development](#development)
- [Where it comes from](#where-it-comes-from)
- [Licence](#licence)
- [Changelog](#changelog)

## What it will do

Each reading is a **section**, and a section is drawn only when its source has
something to say. The user chooses which sections appear and in what order,
and the choice holds in both shells.

| Section | Reads |
|---|---|
| Peripherals | HID++ over `hidraw` for Logitech, `headsetcontrol` for Arctis, Apple's accessory protocol for AirPods, BlueZ `org.bluez.Battery1` for every other Bluetooth device that reports one |
| Bandwidth | `/proc/net/dev`, with the exit node for a `tailscale` interface |
| Cooler | hwmon by label for the processor; `liquidctl` for the pump and the coolant, where the kernel has no driver |
| Usage | the Anthropic OAuth API and the Codex app-server, through a cache shared with the tools this replaces |

## Arrangements

The same sections, laid out three ways. The arrangement is a setting, not a
mode, which is why there is no separate widget for the terminal:

- **stack** — one card above another. The desktop panel, and a narrow terminal.
- **grid** — columns that reflow to the width, in the manner of `btop`.
- **row** — one full-width line per reading, its bar stretching to the pane.
  This is what a `herdr` pane wants, and it replaces
  `claude-usage-widget-windows`'s `--tui` and `--line`:

  ```
  hayami-tui --sections usage
  ```

  `--sections` and `--arrangement` override the settings file for that run and
  never write back to it, so a pane's arguments are the pane's own.

## Architecture

Two binaries and one program. `cmd/hayami` is the desktop panel and
`cmd/hayami-tui` is the terminal panel; `internal/core` produces readings with
no toolkit in sight, `internal/view` describes a section as data, and each
shell only arranges it. A parity test holds the two shells to the same
capabilities.

The window is a glance window from
[fynedesygn](https://github.com/ushineko/fynedesygn) — frameless, fixed to its
content, always on top, read without being touched. Its rules are that
repository's `docs/glance.md`.

Frameless, on top and translucent are the compositor's to grant, not the
toolkit's, so on Plasma they come from a KWin rule that hayami installs when
asked — from the preferences window, or with `hayami window install`. Without
it the panel is an ordinary window with a titlebar, which is what another
desktop gets.

## Running it

```
hayami                                    # the desktop panel
hayami --preferences                      # …and its preferences window
hayami-tui                                # the terminal panel
hayami-tui --sections bandwidth --arrangement row
hayami-tui readings                       # the numbers as JSON, no display needed
hayami-tui arrangements                   # what --arrangement takes
hayami-tui --once                         # one frame, for a prompt or a status line
```

`--sections` and `--arrangement` override the settings file for that run and
never write back to it. The window takes neither: a desktop panel is
configured from its own settings, and a flag that changed what it drew for one
run would be a setting nobody could find again.

Settings live in `~/.config/hayami/settings.yaml`:

```yaml
hayami:
    sections:
        - bandwidth
    arrangement: stack
    interfaces:
        - eno2
```

No interface is watched until one is named. Guessing would be this program
deciding what is interesting about somebody's network.

## Development

```
make setup     # install the pinned linter
make test      # race detector
make lint
make build     # both panels
make vuln      # govulncheck, before every tagged release

tools/shot-tui.sh out.png 150 6 ./hayami-tui --sections usage
               # photograph a pane in a real terminal (KDE/Wayland)
```

## Where it comes from

`ag-scripts/peripheral-battery-monitor`, 6,913 lines of PyQt6, which has run
this shape through its 1.x releases. Two things change in the port: the
context menu shrinks to almost nothing and its settings move to a preferences
window, and every section gains a terminal equivalent.

`ag-scripts/claude-usage-widget-windows` is replaced too. Its terminal panes
are this program in its `row` arrangement with one section selected.

## Licence

MIT. See [LICENSE](LICENSE).

## Changelog

### Unreleased

- **Fix**: the panel is a glance window at last. It has had a titlebar and no
  opacity since spec 001, and the menu's own doc comment has been describing an
  opacity item nobody had built. Neither want is the toolkit's to grant — Fyne
  never asks for a transparent framebuffer, and KWin decorates its undecorated
  windows anyway — so both come from a KWin rule, and the menu gains its third
  item at last (spec 010, #26).
- `hayami window install`, `remove` and `status` do the same from a shell,
  because a panel with no titlebar and no controls of its own should not be
  undoable only from System Settings.
- The opacity is in two places on purpose. The preferences' value is written
  into the rule and survives a restart; the menu's asks KWin to fade the
  running window and saves nothing, which is what trying a value should do.
  Plasma offers two mechanisms and the difference is said rather than hidden.
- Installing the rule is something the user asks for. It is a write into
  `~/.config/kwinrulesrc`, which holds every window rule on the machine, so it
  does not happen on a first run.

- AirPods, over Apple's accessory protocol. BlueZ exposes no battery for them
  — the code that would is behind its `Experimental` setting — so the device
  is asked directly on an L2CAP channel, which needs no system configuration
  and gives a level per ear and for the case. A pair of earbuds is one device
  on the desk and is one row on the panel: it carries the lower of the two
  ears, because that is the one that stops working first, with the cells on a
  quiet line beneath it (spec 009, #23).
- Every other Bluetooth device that reports a battery is read from
  `org.bluez.Battery1`, generically — a device with that interface gets a row
  with its own name, and one without gets none. The program this replaces
  parses `upower -i` instead; BlueZ carries the same numbers with the device's
  name and state beside them, and is an interface rather than a report.
- Two findings that no test away from the hardware could make: `SockaddrL2`
  takes an address in written order and reverses it itself, so reversing it
  first dials nothing and the kernel calls that `ECONNREFUSED`; and Go
  preempts goroutines with signals, so `poll(2)` returns `EINTR` as a matter
  of course and a reader that treats it as a failure calls a working device
  broken.

- The peripherals section, which is the one the program is named after. The
  Logitech mouse is read by speaking **HID++ over `hidraw`** rather than by
  running `solaar`: the CLI takes three and a half seconds to answer and wraps
  a Python library that cannot be imported from Go, and the protocol under
  both is a seven-byte request answered in about five milliseconds. The
  headset comes from `headsetcontrol -o json`, which has replaced the short
  output the program this succeeds still parses. A row per device, appearing
  and disappearing as the hardware does; a device that stops answering keeps
  its last level and is drawn dim, because the question about a headset that
  has gone quiet is whether what it said last is still true (spec 008, #19).
- The live test is why this works. A single HID++ request is answered
  fourteen times in twenty, and after a few seconds of idle it takes up to
  four attempts — a fact the protocol does not mention and no test against a
  fake endpoint can see. Every one of those was green while the real mouse
  would have flickered in and out of the panel on most polls.
- Keyboards over `upower` and AirPods over Apple's accessory protocol are not
  in this: there is no hardware here to hold them to.

- **Fix**: `hayami-tui --once` drew only whichever section polled first. It
  batches a poll per section and quit on the first answer, so the frame held
  whichever won the race — and with the default sections that was bandwidth
  answering "nothing yet", which made `--once` print nothing at all. It waits
  for every section now, bounded, because a prompt that hangs is worse than a
  prompt missing a reading (#21).

- Preferences, in a window rather than a menu. A fynedesygn shell window —
  Sections, Bandwidth, Appearance — opened from a panel menu of three items,
  which is the difference from the program this replaces: its own menu is six
  nested submenus and is a settings dialog wearing a menu's clothes. Which
  sections are drawn, in what order, in which arrangement, and which
  interfaces the bandwidth section watches, all saved as they are changed. The
  interfaces are listed from the kernel's own table, so nobody has to know
  that a virtual network is called `wg0` to be able to watch it (spec 007,
  #17).
- `hayami --preferences` opens that window as well as the panel, for a desktop
  entry's second action and for a panel somewhere a person cannot right-click.

- A cooler section: the processor's temperature from hwmon **by label**, and
  the coolant, pump and fan from `liquidctl`. The approach is hotaru's — read
  by label, never by hwmon index, because the numbers move between boots — and
  it means no daemon is needed for what the kernel already has. liquidctl
  reports every device it can see, so the cooler is chosen as the one that
  reports a *liquid* temperature: a power supply's case temperature under a
  heading that says coolant would be the wrong number under the right label.
  The coolant is coloured by the hardware's own bands and the processor is not
  coloured at all, because a high boost temperature is normal and a colour
  that is always on is not a signal (spec 006, #15).
- `view.Sparkline` plots a series as one line of block runes, so a pane gets
  the trend the window has had all along. Scaled to the series' own range,
  with a floor under it so a steady reading is drawn steady rather than
  amplified into noise.

- Both panels are cobra command trees, as the other ushineko programs are.
  `hayami-tui readings` and `hayami-tui arrangements` are subcommands where
  `--readings` was a flag, both binaries answer `--version`, and a misuse of
  the command line exits 2 rather than 1, so a script can tell it from a
  command that ran and failed.

- A meter's name is three columns rather than one string — the account, its
  plan letter and the window its bar is about — so the badges line up under
  each other down a pane instead of landing wherever the name before them
  ended. The name is drawn white: it says which account and which window, and
  a reader who cannot tell two lines apart has no use for either.
- A window whose reset is not the one in the countdown column says how long it
  has left, as the monitor does: `7d: 21 % (3d left)`. One reset goes in the
  column and it is the soonest, so the weekly window would otherwise say
  nothing about when it turns over — and that is the one worth planning
  around.

- **Fix**: a pane's bar is a rule rather than a slab. `█` on `░` was a band of
  colour the width of the pane, next to which the numbers — the actual reading
  — looked like a caption. It is `━` on `─` now, which is the weight the
  program this replaces draws. Heavy against light rather than one glyph in two
  colours, so the bar still says something in a pipe and under `NO_COLOR`,
  which rich's own bar does not.

- A pane has colour and columns. `view.Render` takes a painter — a function
  from text and a verdict to text — so the terminal can colour through lipgloss
  while `internal/view` stays free of any toolkit and the window keeps
  colouring through the design system's own widgets. In `row` the line is four
  columns: the label, a bar taking what is left, the figures ranged left and
  the reset hard right. A meter's label names the window its bar is about, and
  a section's title is no longer repeated on every line (spec 005, #10).
- `tools/shot-tui.sh` photographs a terminal program in a real terminal of a
  known size. It found the bug above: `lipgloss.ColorProfile()` answers 0 in a
  terminal that plainly has colour, and gating on it had switched colour off
  everywhere while every headless test passed.

- hayami fetches its own usage. Credential stores are found by the convention
  the `claude-max` and `claude-work` wrappers establish, an expired token is
  refreshed and **written back** to Claude Code's own store, and Codex is asked
  through its app-server with the reply reshaped into what the shared cache
  holds. A machine that never ran the Python tools now shows its usage.
- The usage section is denser: one meter per account rather than one per
  window, with the bar showing the window nearest its limit and the caption
  carrying the rest. A plan badge beside the name, and a date rather than a
  countdown for a window that resets more than a day away (spec 004, #8).

- The usage section. `hayami-tui --sections usage` shows what the shared cache
  holds — every Claude profile, its five-hour and seven-day windows or its
  spend, and Codex with its allowance windows and individual limit — as meters
  with a bar, a percentage and a countdown. `view.Meter` draws in all three
  arrangements and stretches to the pane in `row`, which is what replaces the
  widget's own terminal modes. Nothing is fetched yet: the section shows what
  the Python tools keep fresh (spec 003, #6).

- hayami joins the usage cache the Python tools already share, rather than
  keeping one of its own: the same directory, filenames, gate, non-blocking
  lock and the rule that a failed fetch never clobbers good data. The payload
  stays opaque, so a field this build has never heard of survives a write. A
  test runs the Python's own slug function and compares, and another reads the
  real cache files on the machine and skips where there are none (spec 002,
  #4).

- The repository, its conventions and its Makefile.
- A panel with one section in it. `internal/core` reads `/proc/net/dev`,
  `internal/view` describes a section and lays it out as a stack, a grid or a
  row, `internal/config` holds the choice in YAML, and both panels arrange the
  same sections: `hayami` as a glance window and `hayami-tui` on Bubble Tea,
  the second building without cgo. `--sections` and `--arrangement` override
  the settings for one run, `--readings` prints the numbers as JSON and
  `--once` draws a single frame (spec 001, #1).
