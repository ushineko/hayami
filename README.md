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

It draws its own translucency: the space between cards is clear and the cards
are faded to a percentage you set, on any desktop. The titlebar is the one
thing only the compositor can remove, so on Plasma that comes from a KWin rule
hayami installs when asked — from the preferences window, or with
`hayami window install`. Without it the panel is translucent and has a
titlebar.

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

- **Fix**: the usage bar is the window that bites today. It was the window
  furthest along, which sounds like the same thing and is not: an account a
  tenth of the way through five hours and three quarters of the way through a
  week put the week on the bar, and read down a panel of three accounts it gave
  three bars about three different windows — seven days, a monthly spend, a
  Business limit. It is the shortest window an account has, and the providers
  now carry each window's own duration because a name is not a length
  (spec 012, #34).
- **Fix**: the cooler keeps what it knew, and plots the processor. liquidctl
  was opening every HID device on a bus with a documented history of
  contention, so a poll came back empty several times an hour — and a poll that
  failed replaced the reading, so the coolant row went away and the panel
  changed height, at random. It is narrowed with `--match` now, and a failed
  poll keeps its last values dimmed. The plot carries the processor as a
  trailing mean over a minute beside the coolant, each scaled to its own range.
- **Fix**: a peripheral is a cell rather than a line. The section drew one
  label-and-value row per device and argued that the monitor's blocks were an
  artefact of a narrow panel; put side by side with that program it was wrong,
  and for a battery the number is the reading and the name is only which one.
  The state is said under every cell now, not only under a battery that is
  charging. Needed `glance.Cell` and `glance.CellGrid`, which is fynedesygn
  v0.1.56 (its spec 040, its #106).
- **Fix**: the preferences window is not faded. The card opacity was being
  applied to the theme that window draws itself in, so its every button and
  separator was drawn at ninety-five per cent over a framebuffer the panel had
  already asked GLFW to make transparent, and the desktop showed through the
  controls. The reasoning behind it was sound while that window owned the
  application's theme and stopped being sound the moment it took its own; the
  fade belongs to the cards, and the cards are the panel's.
- **Fix**: the panel's face stays in the panel. The preferences window is meant
  to keep the appearance on its own screen and drew at the panel's size
  instead — 8 pt against 12, or 20 against 12, always the panel's. The shell
  asks for a theme every time it lays itself out, the hook answered by telling
  the panel, the panel set the application's theme, and that rebuilds every
  window and takes the subtree override with it. So the separation was undone
  by the act of building the window that wanted it.

- The panel has its own faces and size, chosen with the design system's own
  font choosers: an interface family, a monospace family and a size, all
  separate from the preferences window's. The panel owns the application's
  theme, because its cards are canvas objects that cannot be themed per
  subtree, and the preferences window draws in its own appearance instead
  (#33).
- **Fix**: the meters drew their labels in whatever size the application had
  when they were built. A card restyles its title and its rows; a meter goes in
  as a plain canvas object and has to be told, so the usage section ignored the
  panel's size while every other section followed it.

- The panel has a text size of its own, separate from the preferences window's.
  A Fyne theme is application-wide, so the two shared one — and they are read at
  different distances: a panel from across a desk, a settings window at arm's
  length. The panel carries a theme of its own over its own subtree, taking the
  scheme and the faces from Appearance and departing from it in the one respect
  that was asked for. It goes down to 8 pt (#33).
- The navigation's shape is the user's: icons and labels, icons alone or no
  navigation at all, down the left or across the top. The shell draws the
  control and binds the shortcut for whichever shapes a program lists, and this
  one lists them all — four sections is few enough that icons alone are
  legible.
- The preferences window has no Refresh button. It rebuilt the current screen,
  and every screen here holds settings and saves as it is changed, so there was
  nothing to re-fetch and the button visibly did nothing.

- **Fix**: closing the preferences window no longer closes the program. The
  design system marked every shell window as the application's master, and
  closing a master window exits the application — so dismissing the
  preferences took the panel with it. It is a secondary window now, and its
  close button puts it away rather than destroying it, because a closed Fyne
  window cannot be shown again and the panel's menu offers it every time (#33).

- **Fix**: the frameless toggle takes effect on the panel in front of you. KWin
  applies a window rule to the windows it creates *afterwards* and leaves the
  ones already on screen alone, so turning it on changed a file and nothing
  visible — a control that appeared to do nothing. It now also asks KWin to set
  the property on the running window (#33).
- Turning it **off** still waits for the next start, and the interface says so
  rather than claiming otherwise: a window that has lost its titlebar cannot be
  given one back while it is open.

- **Fix**: the preferences window keeps its titlebar. The KWin rule matched the
  app ID, and every window in the program carries the same one. It matches the
  panel's title as well now — and takes out any rule an older version wrote,
  because a remove keyed on the new match could not see one written under the
  old key, which left two rules installed and made the toggle look dead (#33).
- **Fix**: the text size reaches the Usage section. A card restyles its title
  and its rows; a meter goes in as a plain canvas object, so the card cannot
  know it has a restyle of its own. Usage was the only section made of meters,
  which is why it was the only one that did not follow.
- **Fix**: the cooler draws its trend in the window, as it has in a pane since
  spec 006. The parity test could not see this one: it compares which sections
  each shell draws, not what they draw in them.
- **Fix**: the network card is half the height. Four lines per interface became
  two — the name and both rates on one line, both totals under them, as the
  program this replaces draws them.
- **Fix**: the window no longer keeps its high-water mark. Nothing re-measured
  it when a card's contents shrank, so a row that went away left a band of
  panel background below the last card — eighteen pixels of it — that never
  closed again.

- The panel draws in the appearance you chose. It hard-coded its scheme and
  its face, so the Appearance screen in the preferences changed the
  preferences window and nothing else — a font chooser with no effect on the
  panel sitting beside it. Scheme, interface font, monospace font and text
  size now reach the panel, as they are changed, without a restart (#32).

- The window manager can resize the panel. It was fixed to its content, which
  told the window manager it would not take a resize at all — so a frameless
  panel's own Resize menu item was greyed out and there was no way to ask for a
  wider one. It still cannot be made *narrower* than its readings, which is
  Fyne's floor and is why the panel had to stop being wide in the first place
  (#30).

- **Fix**: the panel fits its corner. It was 655 px wide and 268 px without its
  usage section, so one section was more than doubling the width of the whole
  window: every figure went in one caption, and a caption sets the width of the
  meter, the panel and the window. The figures are spread across columns now,
  as the program this replaces spreads them — the window the bar is about in
  the caption, the others under it, the amounts opposite them, the reset in a
  column of its own. **340 px** (spec 011, #28).
- Nothing is lost by it. A pane still draws one line per meter with every
  figure in reading order, and a window whose reset is not the one counting
  down still says how long it has left.

- **Fix**: the panel is a glance window at last. It has had a titlebar and no
  opacity since spec 001, and the menu's own doc comment has been describing an
  opacity item nobody had built (spec 010, #26).
- The translucency is the toolkit's own: GLFW grants a framebuffer with an
  alpha channel and the window's background is drawn clear, so the desktop
  shows through the space between cards **on any desktop**, with no compositor
  involved. The cards are faded separately, to a percentage the user sets, so
  the readings stay legible over whatever is behind them.
- The titlebar is the one thing only the compositor can take away, so that —
  and always-on-top beside it — is all the KWin rule carries. Installing it is
  something the user asks for: it writes into `~/.config/kwinrulesrc`, which
  holds every window rule on the machine.
- `hayami window install`, `remove` and `status` do the same from a shell,
  because a panel with no titlebar and no controls of its own should not be
  undoable only from System Settings.

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
