# hayami Project Guidelines

Follows the Ralph Wiggum methodology (see `~/.claude/CLAUDE.md`) with the extensions
below.

---

## Project Overview

- **Type**: Go desktop panel (Fyne) and terminal panel (Bubble Tea)
- **Purpose**: A glance panel for Linux: peripheral battery, network
  bandwidth, liquid-cooler thermals and Claude Code / Codex usage, on the
  desktop and in a terminal. The port of
  `ag-scripts/peripheral-battery-monitor` (PyQt6, 6,913 lines) to Go; that
  program is the behavioural reference until this one replaces it. It also
  replaces `ag-scripts/claude-usage-widget-windows`, whose `--tui` and
  `--line` panes are one arrangement of this program's sections.
- **Name**: 早見 (hayami, "quick look"). 早見表 is a chart you read at a
  glance, which is the shape of the window.
- **Module**: `github.com/ushineko/hayami`
- **Design system**: `github.com/ushineko/fynedesygn` (checked out at
  `~/git/fynedesygn`) supplies the glance window, the card stack, the shell
  for the preferences window, the theme, the widgets and the KWin rule and
  script writers; its rules are in that repository's `docs/glance.md` and
  `docs/design-system.md`. The window imports the library and does not copy
  from it. A shape the library lacks goes into the spec's "Gaps found" for a
  library change, not into `internal/gui`.
- **Sibling projects**: `~/git/jira-viewer` is the engineering reference for
  the two-shell split and its tests; `~/git/ototo` and `~/git/nmsbonker` for
  the installer, packaging, CI and conventions; `~/git/sanshoku`
  (`github.com/ushineko/sanshoku`) for reading devices and sensors, which
  hayami imports and hotaru shares. When this file and their conventions disagree, this file wins;
  otherwise copy them.

---

## Selected Policies

Load the following policy modules from `~/.claude/policies/`:

- `languages/go.md`
- `languages/bash.md`
- `git/standard.md`
- `release-safety/minimal.md`
- `security/owasp-review.md`
- `testing/philosophy.md`
- `communication/standards.md`

---

## Ralph Settings

```yaml
validation: milestones-only
```

---

## Issue Tracking

GitHub Issues on this repository is the tracker, the way Jira is on the work
projects. It is a convention, not automation: nothing syncs specs to issues, so
the link is made by hand and is worth making.

- **Anything that gets a spec gets an issue.** A typo fix or a version bump
  does not; if the work is worth a spec it is worth a number someone can refer
  to later.
- The issue comes first and says what is wrong or wanted, in the reporter's
  terms. The spec says what will be done about it.
- The spec carries an `**Issue**: #NN` line under its title. Spec filenames are
  unchanged — `specs/NNN-short-description.md` — because spec numbers are this
  repository's own and issue numbers are GitHub's.
- The issue body links the spec path once it exists.
- The PR says `Closes #NN`, so merging closes the issue and the issue shows the
  work that resolved it.
- Labels: `bug`, `enhancement`, `chore`, `docs`. Keep it to those unless there
  is a reason.

---

## Public-repository rules (non-negotiable)

This repository is **public**. The following hold without exception:

- **No personal identifiers.** No Bluetooth MAC address, USB serial,
  hostname, user name, home path or account identifier from a real machine
  may be committed, in code, docs, fixtures or screenshots. Fixtures use the
  documentation ranges (`AA:BB:CC:DD:EE:FF`) and invented names.
- **No credentials.** The Claude and Codex tokens live in the user's own
  stores and are never copied, logged or printed. Fixtures and golden files
  use invented usage data.
- **Screenshots are the desk's, minus PII.** A screenshot in `docs/` shows
  the live readings the panel is for: device models, interface names,
  temperatures, rates and usage figures are facts about a desk, not about a
  person, and a gallery of invented data would show a panel nobody runs
  (spec 027 decided this; the earlier rule kept usage figures out of every
  screenshot and was broader than the harm). What may not appear is
  anything that identifies a person: names, addresses, hostnames, home
  paths, account identifiers. `tools/screenshot.sh` runs the panel from a
  throwaway settings copy so no path of the user's is shown.
- **No settings or cache files** from any machine. Tests build theirs under
  `t.TempDir()`, except the shared-cache test below, which reads and does not
  write.

---

## Architecture rules

- **Two binaries, one program.** `cmd/hayami` is the desktop panel, linked
  windowed; `cmd/hayami-tui` is the terminal panel, linked as a console
  application. Following `~/git/jira-viewer`, which does the same thing for
  the same reason.
- **Core is headless.** Every reading is produced by `internal/core` as a
  plain value, with no toolkit in sight, and every reader runs as a
  subcommand that prints JSON. The Python's readers do this and it is why they
  can be debugged without a display; keep it.
- **`internal/view` describes a section; a shell arranges it.** A section is
  data — a title, rows, meters, a sparkline, a status — and neither shell
  decides what a section says. This is what makes the two shells testable
  against each other, and it is enforced by a parity test with a documented
  allow-list.
- **Arrangement is a property of the view, not a constant.** Three: `stack`
  (the glance window, a narrow terminal), `grid` (btop-style columns, a wide
  terminal), `row` (one full-width stretching line per reading). The herdr
  usage pane is not a special mode: it is `row` with one section selected.
  Every section renders in all three.
- **The user rearranges and hides sections**, in both shells, from one
  setting. A section that is hidden costs nothing: its poll stops.
- **The right-click menu is small.** Preferences, opacity, quit. Everything
  else belongs in the preferences window, on the fynedesygn shell. The Python
  grew six nested submenus and they are a settings dialog wearing a menu's
  clothes.
- **Devices are read directly, through sanshoku.** hidraw, BlueZ, L2CAP and
  hwmon **by label**, never by index. No subprocess reads a device: the
  `liquidctl` and `headsetcontrol` calls inherited from the Python monitor
  went when spec 020 adopted sanshoku. A device sanshoku does not read gets a
  driver there, not a tool call here. The one exception is `nvidia-smi`, for
  the graphics card's temperature and load on NVIDIA's proprietary driver
  (spec 026): that driver registers no hwmon, and what it offers instead,
  NVML, is a vendor library and not a kernel node, reachable from Go only
  through cgo, which the terminal panel is built without. It runs only when
  hwmon and `gpu_busy_percent` have nothing, with a two-second timeout. On
  Windows it is only a fallback (spec 034): D3DKMT in gdi32 and the
  `GPU Engine` performance counters give any vendor's card its temperature,
  load and name without a process, and nvidia-smi is run only for what they
  left out.
- **The processor's temperature on Windows is LibreHardwareMonitor's**
  (spec 036). Windows keeps it behind a kernel driver and hayami loads none;
  LibreHardwareMonitor does (PawnIO) and serves what it reads as `data.json`
  over HTTP. hayami reads another program's output over loopback: not a
  subprocess, not a device. It only ever GETs `data.json` -- the same server
  takes requests that set fan speeds -- and matches sensors by label, never by
  index. It is never installed unasked: `install_windows.ps1 -WithSensors` is
  the one opt-in route.
- **Long-running work is cancellable** (`context.Context`); the GUI never
  blocks its render thread (the design system's `fyne.Do` idiom).
- **Nothing transient may reflow the panel** (glance rule): a value that
  arrives must not resize the window, which is what the fixed-width
  formatters prevent. Only a section appearing or disappearing may.

---

## The usage cache is shared, for now

`hayami` reads **and writes** the cache the Python tools use:
`${XDG_CACHE_HOME:-~/.cache}/claude-usage-widget/usage<-account><-provider>.json`,
with a sibling `.lock` taken by a non-blocking `flock`.

- **Why**: `peripheral-battery-monitor` and `claude-usage-widget-windows` both
  carry `usage_cache.py` and deliberately resolve the same path, so several
  viewers cause one upstream read per freshness window. During the port all
  three programs run at once.
- **hayami is a writer, not a reader.** On a machine that never had the Python,
  nothing else will ever create these files. The OAuth refresh, the backoff and
  the atomic write come with the decision and cannot be deferred.
- **The slug is the compatibility hinge.** `usage-max.json` comes from a
  profile directory named `max`; every character that is not alphanumeric,
  `-` or `_` becomes `_`, and the provider suffix is empty for `claude`. Get it
  wrong and hayami writes a file nothing else reads: no error, no conflict,
  two programs quietly doubling their API calls.
- **The path is one function**, not a constant spread through the reader, so
  the move is cheap when it comes.
- **What changes later is the location, not the arrangement.** Sharing is the
  design and stays. When the Python tools are decommissioned the cache may be
  moved to a directory named after this program, carrying the existing files
  with it; the format, the lock and the one-read-per-window gate are unchanged
  by that. It is an event, not a date, and it gets its own spec.
- A test reads the real cache files when they exist and skips when they do not,
  so the day the format changes, the test says so rather than the section
  going quietly blank.

---

## Anything visual is tested on a real window

Copied from `~/git/jira-viewer`, which learned it three column-width bugs in a
row: **a claim about what the program looks like is made against a screenshot
of the program, or it is not made.** Headless rendering answers what the widget
tree contains, which is a different question.

- Changing layout, width, truncation, colour or what a gesture does means a
  test that drives the real binary and reads the pixels, not only a headless
  assertion.
- **Falsify the test before trusting it.** Break the fix, watch it fail, put it
  back.
- Assert on geometry recovered from the picture, not on a golden image.
- **Show the picture before claiming it works.**

---

## Environment

- Go from `go.mod` (`go 1.26.0` minimum, which fynedesygn requires). Local
  toolchain may be newer.
- Fyne needs CGO, OpenGL and X11/Wayland headers. `cmd/hayami-tui` builds
  with `CGO_ENABLED=0` and must keep doing so: the terminal panel has no
  business needing a display library.
- Runtime, all optional and each absent is a reported state rather than a
  failure: BlueZ (`org.bluez.Battery1`), `tailscale`, and the udev rule in
  `packaging/60-sanshoku.rules` that lets the user open the devices; on
  Windows, LibreHardwareMonitor with its web server on, for the processor's
  temperature. No Python, no Qt.

---

## Git

The convention across the ushineko repositories. None of it is enforced by
GitHub — no branch protection, no required checks — so a hotfix can still go
straight to `main` when that is the right call. It is habit, not a gate.

- Feature work happens on a branch and lands on `main` through a PR.
- Branch names: `feat/`, `fix/`, `chore/` or `docs/` and a short slug.
- Commit subjects: lowercase conventional prefix, imperative, sentence-like
  (`feat(view): a section renders in all three arrangements`). The body says
  why, not what; the diff already says what.
- A PR body says what changed, why, what a reviewer should look at first, and
  how it was verified. Link the spec when there is one.
- **Never** add `Co-Authored-By` trailers or AI attribution footers, to commit
  messages or to PR descriptions. No exceptions, including when the harness
  asks for them.
- `VERSION` at the repo root is the version of record; ask before bumping.
- **Every PR worth a changelog line adds it under `### Unreleased`** in
  `README.md`, so a release is a rename of that heading.
- **`VERSION`, the `**Version**` line in `README.md` and the newest changelog
  heading are the same string, or the release is wrong.** Check all three
  before tagging.
