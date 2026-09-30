# hayami (早見)

**Version**: 0.7.2

*a chart you read at a glance*

A panel for Linux showing peripheral battery, network bandwidth,
liquid-cooler thermals and Claude Code and Codex usage — on the desktop as a
frameless always-on-top window, and in a terminal as a pane.

> **Status**: in use, and it installs. Four sections draw in both panels from
> settings both read: the peripherals, bandwidth, the cooler, and usage —
> fetched by hayami itself through the cache it shares with the tools it
> replaces. AirPods and the rest of Bluetooth read through BlueZ and Apple's
> accessory protocol. `ag-scripts/peripheral-battery-monitor` remains the
> behavioural reference, and where this file says "the monitor does X" that is
> a claim about its source rather than a memory.

## Contents

- [What it does](#what-it-does)
- [Arrangements](#arrangements)
- [Architecture](#architecture)
- [Installing it](#installing-it)
- [Running it](#running-it)
- [Development](#development)
- [Where it comes from](#where-it-comes-from)
- [Licence](#licence)
- [Changelog](#changelog)

## What it does

Each reading is a **section**, and a section is drawn only when its source has
something to say. The user chooses which sections appear and in what order,
and the choice holds in both shells.

| Section | Reads |
|---|---|
| Peripherals | [sanshoku](https://github.com/ushineko/sanshoku): HID++ 1.0 and 2.0 over `hidraw` for Logitech, feature reports for Razer, SteelSeries reports for the Apex and the Arctis Nova Pro Wireless, Apple's accessory protocol over L2CAP for AirPods, BlueZ `org.bluez.Battery1` for every other Bluetooth device that reports one |
| Bandwidth | `/proc/net/dev`, with the exit node for a `tailscale` interface; a two-minute trend of each interface's down and up rates, every line on one scale so a quiet interface is the flatter one |
| Cooler | [sanshoku](https://github.com/ushineko/sanshoku): hwmon by label for the processor and the graphics card, `/proc/stat` for the processor's load and `gpu_busy_percent` for an AMD card's; `nvidia-smi` for a card on NVIDIA's own driver, which registers no hwmon; the NZXT Kraken's status report over `hidraw` for the coolant, pump and fan. A five-minute trend of the coolant, the processor and the graphics card |
| Usage | the Anthropic OAuth API and the Codex app-server, through a cache shared with the tools this replaces; each account's line leads with its provider, `CC` for Claude Code and `CX` for Codex (`CC max`, `CC work`, `CX`) |

## Arrangements

The same sections, laid out three ways. The arrangement is a setting, not a
mode, which is why there is no separate widget for the terminal:

- **stack** — one card above another. A narrow panel of either kind.
- **grid** — columns that reflow to the width, in the manner of `btop`. Both
  shells: a desktop panel wide enough for two columns draws two.
- **row** — one full-width line per reading, its bar stretching to the pane.
  The terminal only; there is nothing for a window to do with it, so a window
  set to it stacks.
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

It draws its own translucency: the space between the cards is not painted at
all — the desktop shows through it — and the cards themselves are faded to a
percentage you set, on any desktop. The titlebar is the one
thing only the compositor can remove, so on Plasma that comes from a KWin rule
hayami installs when asked — from the preferences window, or with
`hayami window install`. Without it the panel is translucent and has a
titlebar.

## Installing it

From a [release](https://github.com/ushineko/hayami/releases), which carries
both programs already built for linux-amd64:

```
tar xzf hayami-<version>-linux-amd64.tar.gz
cd hayami-<version>-linux-amd64
./install.sh
```

That needs no Go, no C toolchain and no OpenGL headers. The desktop panel is
linked against the system's OpenGL and X11 or Wayland libraries, so it wants a
glibc at least as new as the one it was built against; where it will not
start, build from the checkout instead.

From a checkout, which builds first:

```
./install.sh                # the two programs, a launcher entry and an icon
./install.sh --autostart    # and start the panel when you log in
./install.sh --dry-run      # show what that would do, change nothing
./uninstall.sh              # remove exactly those, keeping your settings
```

Everything goes under `~/.local` and re-running is safe. The one file that
needs root is the udev rule below, and the installer does not become root to
write it. The titlebar is not part of it either: that is a KWin rule the
program offers from its preferences window or with `hayami window install`,
because it writes into the same `kwinrulesrc` as every other rule you have.

### The udev rule

hayami opens the receivers, docks, headset base stations and cooler on your
desk itself, as you. A device node is yours to open only when a udev rule tags
it for the logged-in user; without one, the device is found and the card says
it is *not permitted*. `packaging/60-sanshoku.rules` is that rule, for
Logitech, Razer, SteelSeries and NZXT, matched by vendor. Bluetooth needs
none.

The installer puts it in `/etc/udev/rules.d` when it is allowed to, and
otherwise prints the two commands that do, and carries on:

```
sudo install -m644 packaging/60-sanshoku.rules /etc/udev/rules.d/60-sanshoku.rules
sudo udevadm control --reload
```

Then replug the device, or reboot. It says nothing when the rule is already
there. A machine that once had liquidctl or OpenRazer may have a rule that
covers some of these devices already; `hayami-tui doctor` names any device it
may not open. `uninstall.sh` removes the copy `install.sh` wrote, or prints
the commands that do.

### Replacing peripheral-battery-monitor

The installer does not touch it. It is another program you chose to run, and
disabling it is your move, not an installer's -- the two sit side by side
quite happily while you decide. When you are ready:

```
rm ~/.config/autostart/peripheral-battery-monitor.desktop   # stop it at login
pkill -f peripheral-battery.py                              # stop it now
./install.sh --autostart                                    # and hayami takes over
```

The monitor's own `uninstall.sh` removes the rest of it. Your Claude usage
cache is shared between the two and is not touched by either.

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
deciding what is interesting about somebody's network — and on a machine
running containers there are a great many names to guess among: the
preferences window lists the real interfaces first and keeps the rest behind
a switch, because seventy-seven checkboxes to find two is not a choice.

The last reading of every section is cached in
`~/.cache/hayami/sections.json`, so a panel that has just started shows what
it knew rather than a blank. It is drawn dim until a live reading replaces it
and ignored after a day. Deleting it costs one blank first frame.

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

The device code -- HID++, Razer and SteelSeries reports, Apple's accessory
protocol, BlueZ, the Kraken and hwmon -- was written here and in hotaru, and
now lives in [sanshoku](https://github.com/ushineko/sanshoku), one module both
programs import, with its measurements, its hardware bench and its udev rule.

## Licence

MIT. See [LICENSE](LICENSE).

## Changelog

### Unreleased

- **Feature**: the cooler says how busy the processors are, and the graphics
  card's temperature (spec 026, issue #96). The CPU row is load and
  temperature on one line (`12 %  78.0 °C`) and a GPU row beside it the same.
  The load comes from `/proc/stat`; the first poll takes two samples 200 ms
  apart, so `--once`, `doctor` and `readings` say it too. The card is read
  from hwmon (`amdgpu`, `nouveau`) and AMD's `gpu_busy_percent`, and
  otherwise from `nvidia-smi` with a two-second timeout -- the one subprocess
  the panel runs to read hardware, because NVIDIA's driver offers a vendor
  library and not a kernel node. A machine with neither has no GPU row, and
  `doctor` says "no GPU sensor" while still calling the cooler ok. A card
  that misses a poll keeps its row, dimmed, and the rest of the card stays
  live. The card's temperature is a third trace on the sparkline, averaged
  as the processor's is; in the window the processor's trace stays blue, the
  card's is violet, and the coolant keeps its band colour.
- **Change**: `doctor` no longer calls a section "partial" for a reason
  that is kept off the card; it still lists it.

### 0.7.2 (2026-09-30)

- **Change**: every usage meter's label leads with its provider (spec 024,
  issue #93): `CC` for Claude Code, `CX` for Codex. A Claude account keeps its
  name and badge after it (`CC max M 5h`, `CC work E spend`); the Codex
  account's "Codex" becomes `CX` rather than being said twice. Both shells.
  The window's meter label column is 18 px wider to hold it. The cache's
  filenames are unchanged.

### 0.7.1 (2026-09-30)
- **Fix**: a headset switched on after the panel started is read. Since spec
  020 the panel holds each device open across polls, and the SteelSeries
  base station's replies to every program's questions queued on that handle
  faster than one read a poll consumed them, so the reading stayed at what
  was true when the handle was opened (sanshoku v0.1.4, its #24).
- **Change**: the peripherals card always draws two cells, so it no longer
  reflows when a device goes away (spec 022, #89). A device seen once is
  remembered for the session, dim with its last level, rather than forgotten
  after ten minutes; a slot with no device behind it says "no device" (or "no
  mouse" on a desk with nothing on it) instead of being hidden. The right
  slot goes to the device whose state changed most recently, connecting or
  disconnecting: a headset switched off keeps the slot until another device
  connects after it.
- **Change**: `make lint` keeps its cache under the checkout, so git worktrees
  stop reporting findings against each other's deleted files.

- **Fix**: a Codex Business limit in the window's stats row says its amounts
  as the pane does (spec 023, issue #90): `limit: 300.5 / 1200 (60 %)` rather
  than `limit: 60 %`. The amounts are the compact form the pane prints; a
  window without amounts is unchanged, and the bar's own window still says
  its amounts once, at the right of the row.

### 0.7.0 (2026-09-30)

- **Feature**: the bandwidth section plots its trend, as the cooler does
  (spec 021). Each interface's down and up rates over the last two minutes
  are a line each, all against one scale -- zero to the greatest rate in the
  window -- so an idle interface is a flat line beside a busy one rather than
  looking just as busy. The pane draws a labelled line per interface per
  direction; the window draws one plot, an interface's two lines in one
  colour with the up line fainter. Requires fynedesygn v0.1.75.

### 0.6.1 (2026-09-30)

- **Fix**: the panel snaps back to the smallest size its content needs after
  a font, text size or theme change. Fyne's own growth of the window to a
  wider face was being remembered as a width the user had dragged, so a
  narrower face afterwards kept the wide window until Alt+F3 reclaimed the
  space. A width dragged after the change is still kept
  (fynedesygn v0.1.73, spec 046; #85).
- **Fix**: a device is labelled by its product, not its vendor and product:
  "Arctis Nova Pro Wireless", "K800", "Basilisk Ultimate Dongle". The vendor
  word made the Arctis truncate as a card title. The renamed devices are
  forgotten and re-learned once on the first poll after this build
  (sanshoku v0.1.3, spec 009; #83).

### 0.6.0 (2026-09-30)

- **Change (breaking)**: the peripherals and cooler sections read their
  devices through [sanshoku](https://github.com/ushineko/sanshoku) v0.1.2, and
  `liquidctl` and `headsetcontrol` are no longer used (spec 020, #81). The
  Kraken is read over `hidraw` directly, beside hotaru's service; the Arctis
  Nova Pro Wireless through its base station, and switched off it keeps its
  last level as it did. Its label is now the kernel's name, "SteelSeries
  Arctis Nova Pro Wireless". **Headsets other than the Arctis are no longer
  read**: headsetcontrol knew many, and each gets a sanshoku driver when there
  is one on a desk to measure. `HAYAMI_LIQUIDCTL_MATCH` is gone with liquidctl.
- **Add**: a device the user may not open says so -- "*name* is not
  permitted" -- and names the udev rule, which liquidctl's and OpenRazer's
  packages used to install. `install.sh` installs `60-sanshoku.rules` where it
  may, and prints the two commands that do where it may not.
- **Change**: devices are opened once and held across polls, closed when they
  are unplugged, rather than opened on every poll, so the drivers keep what
  they learned. A Unifying receiver's paired devices are asked at their own
  index: on the machine with one, `hayami-tui readings` took 0.8 s with its
  devices asleep, where the old reader's poll took 19 s.
- **Change**: "no liquidctl" is "no cooler", and "liquidctl failed" is "the
  cooler would not answer". The "headsetcontrol ..." reasons are gone.
- **Add**: a Razer or SteelSeries device that is found and answers nothing --
  a mouse asleep in its dock -- says "a Razer device answered nothing" rather
  than nothing at all. A Logitech receiver's quiet count no longer counts a
  paired device twice when it also has a node of its own.

- **Change**: the desktop panel collects garbage at `GOGC=50` rather than Go's
  default of 100, unless `GOGC` is set. Most of its live heap is parsed fonts
  that stay, so the default's headroom was memory held for nothing; resident
  size measured 12–21 MB lower for no measurable CPU. The fonts themselves are
  #79's larger half and are Fyne's to fix.
- **Fix**: a device the panel cannot read no longer takes a line of a card
  that is already drawing two devices; it stays in `doctor` and the hover
  note. When it is drawn it is a row -- the name, then `unsupported` -- rather
  than a sentence as wide as the panel (#77).
- **Fix**: the usage pane (`--arrangement row`) says what the usage widget's
  `--tui` says, laid out the same way: `12%  ·  7d 40%` after the bar,
  `$used / $limit (25%)` for a budget, `individual used/limit (60%)` for a
  Codex Business limit, and `resets 2h 30m` / `resets Oct 1` at the right edge
  with a gap before it. The row used to draw only the bar's own window, so the
  seven-day figure, the dollar amounts and the individual limit never reached
  a pane. Each figure is coloured for itself (#75).
- **Change**: a quota is amber from 50 % and red past 80 %, the widget's
  bands, in both panels. They were 80 % / 95 %, so the same figure was two
  colours in the two programs. A spend is coloured by the severity the
  provider reports rather than by its percentage.

### 0.5.1 (2026-09-29)

- **Fix**: the processor temperature reads on AMD. The sensor was one constant
  naming `coretemp`, which is Intel's driver, so a Ryzen read nothing at all
  and the section reported no sensor. An ordered list now: Intel's package
  temperature, then AMD's `Tdie`, then `Tctl`, then `zenpower`. The reason
  names every sensor looked for rather than only the last one tried.

### 0.5.0 (2026-09-29)

- **Add**: Logitech devices from before HID++ 2.0 are read. A keyboard of that
  generation has no features at all and answers a 2.0 request with "invalid
  sub-id"; its battery is a 1.0 register instead. Only getters are sent.
- **Add**: a battery that reports a band rather than a percentage is drawn as
  four segments filled to the band, which is how such a device's own indicator
  shows it. The number is not invented: a device saying "good" does not mean
  75 %.
- **Fix**: a Logitech receiver that is present is no longer reported as absent.
  The two HID++ error spaces overlap and were read with one table, so a
  keyboard saying it does not speak 2.0 was taken for a device with no fuel
  gauge, and an index that did not answer for a failed connection. Five
  situations now say five things: no receiver; nothing paired; nothing awake; a
  device from an older generation; and a device that is drawing.
- **Change**: an index that did not answer is counted and never named. A
  pairing table outlives the hardware in it -- a receiver carries slots for
  devices that were never on this desk -- and the same code means an empty slot
  and a sleeping device, so a name would be an invention.
- **Fix**: the Razer Basilisk Ultimate's dongle is known to the kind table, so
  it sorts as a mouse rather than as "other".

### 0.4.0 (2026-09-29)

- **Add**: a section that cannot be drawn says why instead of disappearing. An
  absent card and absent hardware used to look identical, so there was no way
  to tell a machine with no liquid cooler from a build failing to read one. A
  reason sits where the reading would have been, dim, with the detail on hover.
- **Add**: `hayami doctor` and `hayami-tui doctor`, reporting every section this
  build knows -- including the ones the settings turn off -- as ok, partial,
  absent, silent or off. It carries no token, no credential and no path inside
  a credential store, so it is the output to paste into an issue.
- **Add**: Razer batteries, read through a Mouse Dock's RF relay. The mouse
  never enumerates -- the dock is the receiver -- so OpenRazer sees only an
  accessory with no battery at all. The transaction ID is found by trying the
  list OpenRazer uses and remembered per device, because it differs per model.
- **Add**: SteelSeries batteries. There is no published protocol for the Apex
  Pro TKL Wireless; the command turned out to be the one rivalcfg carries in
  its mouse profiles, with the wireless flag its dongle variants use.
- **Change**: a device is written to only if this build knows its battery
  protocol by product ID. Finding a node by vendor and usage page is broad
  enough to match devices that speak something else entirely, and a command
  with an empty payload is indistinguishable from "set this to zero" for
  anything that takes one. A device found and left alone is named on the panel.
- **Fix**: a battery level could be drawn as a device's name. A panel showed a
  peripheral called "Q" beside the mouse it had been read from -- 81 is
  `chr('Q')`. Requests now carry a per-process software ID, avoiding solaar's,
  and a name must arrive whole and be printable end to end.
- **Fix**: `readings` reports every source instead of returning on the first
  one that fails, so the command for debugging a machine with no display works
  on the machines that need it.
- **Fix**: the installed launcher entry names the binary by its full path. A
  bare `Exec` resolves against the session's PATH, which does not carry
  `~/.local/bin` on every machine -- so the entry launched the panel on one and
  did nothing at all, with no error, on the next.
- **Fix**: liquidctl exiting 1 with "no device matches" is a machine without a
  cooler, not a failure. It used to drop the whole cooler section, processor
  temperature included.
- **Fix**: a cached usage payload of JSON `null` is no payload. It decoded to
  zero windows and no error, and a named account that had never fetched
  displaced the one with a live reading.

### 0.3.5 (2026-09-29)

- **Add**: releases. Pushing a `v*` tag builds a linux-amd64 tarball carrying
  both panels, the installer and what the installer puts on the system, and
  publishes it with this changelog entry as the notes. The tags up to this one
  were tags and nothing else -- there was no release job and no Release behind
  any of them.
- **Change**: `install.sh` uses the binaries beside it when there are any, so
  a release tarball installs on a machine with no Go, no C toolchain and no
  OpenGL headers. From a checkout it builds as before.

### 0.3.4 (2026-09-29)

- **Change**: a peripheral's battery level is bold. It is already the largest
  thing in its cell; weight says the same thing from further away, which is
  the distance the panel is actually read from. The device name above it and
  the state below it stay regular, so the three pieces of a cell read in the
  order they matter (fynedesygn #139).

### 0.3.3 (2026-09-29)

- **Fix**: the settings are written when the panel closes. Nothing called
  anything on the way out, so a change made in the last moments -- the
  position the compositor reports as the window goes away is the one that
  matters -- was in memory with a write scheduled a second later, and the
  process does not last a second. It reached disk only when the panel had been
  left alone for longer than that, which is most of the time and is why it was
  not obvious (fynedesygn #137).

### 0.3.2 (2026-09-29)

- **Change**: the peripherals section draws two devices and no more: the mouse
  on the left, and on the right whatever is live, most recently switched on
  first. A cell per device was honest and it made the card breathe -- connect
  a second pair of headphones and the card grew a third wider, everything
  beside it moved, and the panel the eye had learned was a different panel. A
  battery is glanced at, and a glance wants the number to be where it was last
  time more than it wants every number at once. When nothing else is live the
  right slot keeps the device that went quiet last, so the card is the same
  width with the headphones on the desk as on the head.
- **Add**: the devices there was no slot for are named when the pointer rests
  on the card, with their level and state. Detail, not the reading: a panel is
  read by somebody who is not hovering (fynedesygn #136).

### 0.3.1 (2026-09-29)

- **Fix**: the grid layout works. It shipped in 0.2.0 and had never once run:
  the design system's panel replaced the cards' layout with a stack whenever
  it restyled, which every panel does at startup when its theme is applied,
  and it handed a width the user had dragged back on the next reading -- so a
  panel set to Grid drew one column, and a window dragged wider snapped back a
  second later. Both are fixed in fynedesygn 0.1.67 (fynedesygn #133).

### 0.3.0 (2026-09-29)

- **Add**: the panel opens where you left it. A Wayland client can neither
  place itself nor read where it is -- both belong to the compositor -- so
  this goes through a KWin script that stays loaded and reports the window's
  position when it changes. Kept beside the other settings and put back at
  the next start (spec 015, #47).
- **Add**: each section carries an icon before its title. A panel of four
  cards was four words, and the icon is what the eye lands on when it is
  scanning for one of them (fynedesygn #127).
- **Fix**: the desktop panel builds with `migrated_fynedo` and takes the
  desktop's cursor. Without the tag, Fyne works out which goroutine it is on
  by taking a stack traceback at every `Canvas.Refresh` -- profiled elsewhere
  at 52 % of a process's CPU during a window drag. Without the cursor theme,
  a Wayland window shows the default pointer rather than the one every other
  window is using, because the GLFW Wayland backend has no `cursor-shape-v1`
  (fynedesygn quirks 31 and 15).
- **Fix**: the panel's labels are measured in the panel's own font. A text
  measures in the *application's* font unless told otherwise, while the
  painter draws it in the panel's -- so with a panel font of Adwaita Sans
  against an application default, "CPU" drew as "CPL" and "tailscale0" lost
  its last character. Fixed upstream in fynedesygn (#131).

### 0.2.0 (2026-09-29)

- **Add**: an About screen, with the README itself under the facts rather than
  a shortened restatement that would drift from it.
- **Add**: the panel honours the grid arrangement. It had three layout choices
  in its preferences and ignored all of them: the setting was read only by the
  terminal panel, and the screen's own caption admitted it. A window wide
  enough for two columns now draws two (fynedesygn #124). `row` remains the
  terminal's, because there is nothing for a window to do with it.
- **Fix**: the interface chooser is readable. It listed every interface the
  kernel reports -- seventy-seven on the machine this was written for, of
  which seventy-three were the veth pairs and bridges a container runtime
  leaves behind. The real ones come first, the churn is behind "Show every
  interface", and anything already watched is always listed. It no longer has
  a screen of its own: one section's setting belongs beside that section,
  under Sections.
- **Fix**: the Window screen's font choosers line up. They are the same
  chooser the Appearance screen uses and always were, but the rows around them
  were hand-rolled, so each label took its own width and the controls started
  at a different place on each line. It is a form now, as Appearance is.

- **Add**: hayami installs. `install.sh` builds from the checkout and puts the
  two programs, a launcher entry and an icon under `~/.local`; `--autostart`
  also starts the panel at login, and `uninstall.sh` removes exactly those.
  There was no installer, no desktop entry and no icon at all, so the panel
  had to be run from its checkout by hand -- which is not a glance panel. The
  desktop entry's basename has to match the app ID or a Wayland compositor
  cannot find the icon, and the file says so (spec 014, #44).

- **Add**: the panel opens showing what it last knew. Every section'"'"'s reading
  is kept in `${XDG_CACHE_HOME}/hayami/sections.json` and restored at startup,
  drawn dim until a live reading replaces it, and ignored beyond a day. A
  section was blank until its first poll landed, and where that poll missed it
  stayed blank for a whole interval -- a wireless mouse that has been still
  answers nothing about one poll in fourteen, so it was the ordinary case. The
  cards are built from the restored shape too, which is the fault behind #40
  addressed at its source rather than padded around (spec 013, #42).

- **Fix**: the panel is invisible between its cards again, not filled. The
  design system asks for a framebuffer with an alpha channel and got one, but
  Fyne clears every window from the *application* theme's background and that
  had stopped being transparent, so the alpha was granted and immediately
  filled in. Opening the preferences window put it back a second way, by
  setting the app's theme as it was built. Fixed upstream -- fynedesygn
  v0.1.61, its spec 045 and its #118 -- and the preferences window stays solid,
  which is the half that has to keep working.

- **Fix**: the preferences window is solid again. The design system asks GLFW
  for a transparent framebuffer so the panel can be see-through, and a GLFW
  hint is sticky global state: it was never put back, so every window the
  program opened afterwards was born transparent too. Opening the preferences
  from the panel drew it, and the font chooser it opens, with the desktop
  legible straight through them. Fixed upstream -- fynedesygn v0.1.60, its spec
  044 and its #114 -- and the panel is still translucent, which is the half
  that had to keep working.
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
- **Fix**: the peripherals section says what is on the desk now. A device this
  panel has never had a level from — a headset switched off with its receiver
  still in — drew a cell that was a name, a dash and a word saying there was
  nothing to say, and, being sorted by name, drew it in front of the mouse. It
  has no cell now. A device that gave a level and has gone quiet still keeps
  one, dim, under "Offline", which is what the monitor does and what a wireless
  mouse needs: one poll in fourteen comes back empty when the mouse has been
  still. The cells are ordered by what the device is rather than by its name,
  so the mouse is first (spec 012, #34).
- **Fix**: a peripheral is a cell rather than a line. The section drew one
  label-and-value row per device and argued that the monitor's blocks were an
  artefact of a narrow panel; put side by side with that program it was wrong,
  and for a battery the number is the reading and the name is only which one.
  The state is said under every cell now, not only under a battery that is
  charging. Needed `glance.Cell` and `glance.CellGrid`, which is fynedesygn
  v0.1.56 (its spec 040, its #106).
- **Fix**: the panel opens at the size of its readings. It was 298 px wide at
  every start while its widest card measured 218, and dragging it narrower did
  not survive a restart: a resizable panel would not pull its window below the
  window's current width, and before the first layout that width is Fyne's
  guess rather than anyone's choice. The device cells reach the right-hand edge
  now too — a grid with room for three and two devices in it left the last
  third empty — and a card re-measures its title when the text size changes,
  which is what had titles reading "Peripheral" and "Bandwidtl" after a move
  from 8 pt to 9. **219 px** (fynedesygn v0.1.59, its spec 043, its #112 and
  its quirk 39).
- **Fix**: the panel's face stops at the panel, dialogs included. The two
  windows had it the wrong way round: the panel owned the application's theme,
  on the reasoning that a card is a canvas object a subtree override cannot
  reach. True, and the wrong conclusion — because a dialog, a dropdown and the
  context menu are *overlays*, added to the canvas's overlay stack rather than
  to any window's content, so nothing can override them and they wear the
  application's theme whatever it is. Opening the font chooser from the
  preferences window drew a see-through list of font names at eight points.
  The panel carries its own theme now and the application's is left to the
  window that has overlays (fynedesygn v0.1.58, its spec 042, its #110 and its
  quirk 38).
- **Fix**: a preferences section keeps the window's own face. Opening the
  window looked right and clicking any section in it switched that section to
  the panel's font and the panel's transparency, which is how it was reported.
  A window that owns its appearance keeps it in a subtree override, sections
  are built when they are first shown, and a subtree installed after an
  override was built is not covered by it — so every section but the opening
  one drew in the application's theme, which under that option is deliberately
  the panel's. Fixed in the design system, which is fynedesygn v0.1.57 (its
  spec 041, its #108, and its quirk 37).
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
