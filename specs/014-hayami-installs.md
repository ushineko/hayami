# 014 — hayami installs

**Issue**: #44

## Context

hayami has no installer, no desktop entry, no icon and no `SetIcon`. It is run
from its checkout, which is how the panel on the machine it was written for
runs: `~/git/hayami/hayami`, started by hand, gone at the next reboot.

The program it replaces is installed properly. `peripheral-battery-monitor`
has a desktop entry symlinked into `~/.config/autostart`, so it is there every
time the machine is. Until hayami can do the same it cannot take over; a panel
you have to remember to start is not a glance panel.

`~/git/ototo` is the convention for this and is followed closely: the same
layout, the same `--dry-run`, the same refusal to touch anything the user did
not ask for.

## Requirements

### Four files into ~/.local, and nothing else

```
~/.local/bin/hayami                                      the desktop panel
~/.local/bin/hayami-tui                                  the terminal panel
~/.local/share/applications/io.ushineko.hayami.desktop   the launcher entry
~/.local/share/icons/hicolor/scalable/apps/hayami.svg    its icon
```

Both binaries, because they are one program and the terminal one is not a
lesser build: it needs no CGO and works on a machine with no display, which is
the case a server or an SSH session is.

Idempotent, with `--dry-run`, and a build failure that names the packages for
Arch, Debian and Fedora rather than leaving a linker error.

### The desktop entry's name is load-bearing

The file must be `io.ushineko.hayami.desktop`, matching `gui.AppID`. Fyne sets
the Wayland `app_id` from the application's unique ID, and a Wayland
compositor resolves a window's icon by matching that `app_id` to a desktop
file of the same name. Naming it `hayami.desktop` silently costs the taskbar
icon on every Wayland session. `StartupWMClass` covers X11 and XWayland, where
the match is on `WM_CLASS`.

This is written in the file itself, because it looks like a stylistic choice
and is not.

### An icon that survives a taskbar

There is no icon in any form today. It is the panel's own shape — stacked
cards — in four shapes and no more, because anything smaller than about four
pixels at a 64-pixel viewBox turns to mush at 16. The bars are different
lengths because the readings are: equal bars read as a menu rather than as
measurements.

It exists twice, at `packaging/hayami.svg` and `internal/gui/assets`, because
`go:embed` cannot reach outside its package and the installer cannot read one
out of the binary. A test compares them so the two cannot drift.

### Autostart is asked for, not assumed

`install.sh --autostart` also writes the entry into `~/.config/autostart`;
`uninstall.sh` removes it. Off by default.

ototo leaves autostart out entirely because it changes the desktop. A glance
panel is the case where that reasoning points the other way — it is meant to
be there whenever the machine is, and the program it replaces autostarts today
— so it is offered, and still not assumed.

### Replacing the monitor is the user's move, not the installer's

`install.sh` does not touch `peripheral-battery-monitor`'s autostart entry, or
any other program's. An installer that quietly disabled something else the
user runs would be doing something it was not asked to do, and the two
programs coexist perfectly well side by side while someone decides.

The README says how to make the swap — remove the symlink, stop the process —
in the user's own hands.

### The KWin rule stays where it is

Not installed here. It is the program's own offer, from the preferences window
or `hayami window install`, because it writes into the user's `kwinrulesrc`
alongside every other rule they have. `uninstall.sh` names `hayami window
remove` rather than doing it, for the same reason.

## Acceptance criteria

- [x] `install.sh --dry-run` lists exactly the four files and changes nothing.
- [x] `install.sh` installs them, is safe to run twice, and the panel starts
      from `hayami` on the PATH.
- [x] `install.sh --autostart` also writes the autostart entry.
- [x] `uninstall.sh` removes exactly what was installed, names what it leaves,
      and is safe to run when nothing is installed.
- [x] The desktop entry's basename matches `gui.AppID`.
- [x] `packaging/hayami.svg` and the embedded copy are identical, enforced by
      a test.
- [x] The window carries the icon.
- [x] Both scripts pass shellcheck.
- [x] The README says how to replace `peripheral-battery-monitor` with this.
- [x] `go test ./...` passes; `cmd/hayami-tui` still builds with
      `CGO_ENABLED=0`.

## Risks & Assumptions

- **`~/.local/bin` may not be on the PATH.** The installer says so rather than
  editing a shell profile, which is not an installer's to edit.
- **The icon is a first attempt.** It has not been looked at in a real taskbar
  at 16 pixels, only reasoned about; that check belongs in the next sitting
  with the panel.
- **An autostart entry runs `hayami` from the PATH**, so a later
  `uninstall.sh` leaves a login entry pointing at a binary that is gone —
  which is why uninstall removes it too. A user who installs, autostarts, then
  moves the binary by hand is on their own.
- **Rollback**: `./uninstall.sh`. It removes only what was installed and
  prints where the settings and the cache are.

## Status: COMPLETE
