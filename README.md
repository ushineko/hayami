# hayami (早見)

**Version**: 0.1.0

*a chart you read at a glance*

A panel for Linux showing peripheral battery, network bandwidth,
liquid-cooler thermals and Claude Code and Codex usage — on the desktop as a
frameless always-on-top window, and in a terminal as a pane.

> **Status**: the spine works. One section — bandwidth — draws in both panels,
> in all three arrangements, from settings both read. The program it replaces,
> `ag-scripts/peripheral-battery-monitor`, is the behavioural reference and is
> still the one to run.

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
| Peripherals | `solaar` for Logitech, `upower` and BlueZ `org.bluez.Battery1` for the rest, `headsetcontrol` for Arctis, Apple's accessory protocol for AirPods |
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

## Running it

```
hayami                                    # the desktop panel
hayami-tui                                # the terminal panel
hayami-tui --sections bandwidth --arrangement row
hayami-tui --readings                     # the numbers as JSON, no display needed
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

- The repository, its conventions and its Makefile.
- A panel with one section in it. `internal/core` reads `/proc/net/dev`,
  `internal/view` describes a section and lays it out as a stack, a grid or a
  row, `internal/config` holds the choice in YAML, and both panels arrange the
  same sections: `hayami` as a glance window and `hayami-tui` on Bubble Tea,
  the second building without cgo. `--sections` and `--arrangement` override
  the settings for one run, `--readings` prints the numbers as JSON and
  `--once` draws a single frame (spec 001, #1).
