# 059 — a fynstall installer

**Issue**: #187

## Status: COMPLETE

## Context

hayami installs from a checkout. `install.sh` and
`scripts/install_windows.ps1` each build the two programs and copy them into
place, so installing needs Go, a C compiler and the repository. There is
nothing to hand someone who only wants to run the panel on Windows; the Linux
release is a tarball with the same script in it.

fynstall (`github.com/ushineko/fynstall`) builds a self-contained installer
from a config: a wizard from the desktop, a command line from a terminal, a
launcher entry, an entry in Settings > Apps, and an uninstaller that reverses
what the install did. This spec builds hayami's installer with it, on Windows
first, and is also the first test of whether fynstall meets a program's
needs.

It did not, in two places, and both were added to fynstall for this:

- **A program's own pages and hooks** (fynstall spec 003, ushineko/fynstall#11).
  The LibreHardwareMonitor check and offer is a wizard page with its own
  behaviour and a step after the install, and a config file cannot say
  either.
- **Start at login** (fynstall spec 005, ushineko/fynstall#35). The panel
  starts with its user, and fynstall had only services for that.

## Requirements

- R1 `fynstall.yaml` at the repository's root: both programs under `bin/`,
  on `PATH`; one launcher entry, with the app ID, which starts at login; the
  settings directory kept on uninstall; per-user only. Its `app.version` is
  `VERSION`'s, and a test holds the two together.
- R2 `installer/`, the Go package fynstall links into the installer and the
  uninstaller.
  - R2.1 **The check.** `Sensors` is what the installer finds of
    LibreHardwareMonitor: whether its program is where winget puts it,
    whether it runs and starts with Windows (`core.ProbeLHMHost`), and
    whether hayami reads the processor's temperature from it now
    (`core.FetchLHM`, `core.LHMCPUTemperature`). It reads and changes
    nothing, and needs no administrator.
  - R2.2 **The page**, `sensors`, before the summary. It says what the check
    found. Where there is something to set up it offers one check box,
    clear, with what ticking it does and what a person is agreeing to under
    it. Where LibreHardwareMonitor already works, or off Windows, it offers
    nothing and the parameter is `no`. The check runs each time the page is
    shown.
  - R2.3 **The command line.** The same text and one yes-or-no question.
    `--sensors=yes` answers it, and `--yes` without it leaves the default,
    `no`.
  - R2.4 **After the install**, when `sensors` is `yes`, the hook runs
    `install_windows.ps1 -SensorsOnly` from a copy the installer carries:
    the steps `-WithSensors` has always taken, from the same file. Its lines
    go to the installer's log. A set-up that fails is a warning and never
    fails the install.
  - R2.5 **Before an uninstall**, which is also the first half of an
    upgrade, the hook stops `hayami.exe` and `hayami-tui.exe` that run from
    the install directory, so Windows lets their files go. A program of the
    same name from elsewhere is left alone.
- R3 LibreHardwareMonitor stays opt-in: nothing sets it up but a ticked box,
  a `yes` at the prompt, or `--sensors=yes`.
- R4 `install_windows.ps1` takes `-SensorsOnly` (R2.4) and `-BuildOnly`.
  `scripts/installer_windows.ps1` builds the programs, draws the icon and
  runs `fynstall build`.
- R5 The scripts stay. `install_windows.ps1` and `install.sh` install from a
  checkout as before; this adds a second way and removes none.

## Acceptance Criteria

- [x] `scripts\installer_windows.ps1` builds
  `dist\hayami-<version>-windows-amd64-installer.exe` and its command-line
  variant, and each passes fynstall's check of its extension.
- [x] `--dry-run` lists the plan and changes nothing: both programs, the
  Start menu and Startup shortcuts, the Settings > Apps entry, the `PATH`
  entry, and that hayami's own set-up code runs.
- [x] An install and an uninstall in a temporary profile, with its own
  registry root: the installed `hayami-tui --version` answers, and the
  uninstall leaves no file and no registry key.
- [x] Unit tests: the check's words for each state; the page on the command
  line asks once, takes the answer, and asks nothing where there is nothing
  to offer; the page's window offers a clear check box, lists what it does,
  and sets the parameter; the hook runs the set-up only when asked and never
  fails the install; the version is `VERSION`'s; the extension keeps
  fynstall's rules.
- [x] The check, live on the Windows desk: LibreHardwareMonitor set up,
  "hayami reads the processor's temperature from it", nothing offered.
- [x] The set-up, live on the same desk, through the hook's own code: every
  step already done, the temperature read back, no prompt.
- [x] Desk check: the wizard, from Explorer, on the real profile, after the
  script install was removed: the pages, the install, the panel started from
  the finish page, the Start menu, Settings > Apps, and the panel there
  after a new login.
- [x] Desk check: the offer taken on a machine without LibreHardwareMonitor.
- [x] Desk check: the uninstaller from Settings > Apps, with the panel
  running: the panel is stopped and its files are gone; the settings stay.

## Out of scope

- **Linux.** The config is one for every platform and the page skips itself
  there, but no Linux installer has been built or run. The udev rule, which
  needs root and goes outside the install directory, has no fynstall action
  in a per-user install; `install.sh` still installs it.
- **Choosing start at login.** fynstall's key is in the config, so every
  install starts at login. The scripts' `-Autostart` is a choice; Windows'
  own Startup settings turn it off.
- **Signing.** The installer is not signed, so Windows asks before running
  one that was downloaded.

## Risks & Assumptions

- **The builder and go.mod name one fynstall.** go.mod requires fynstall
  0.2.1: 0.2.0 was the first release with extensions, entries that start at
  login and `--windowed`, and 0.2.1 opens the wizard for an installer run
  as an administrator. CI installs the builder at the version go.mod names, so the
  two cannot drift; `-RuntimePath` is for a fynstall that is not released.
- **The module gains a dependency.** `installer/` imports fynstall, so
  hayami's go.mod requires it. Neither panel imports the package, and the
  terminal panel still builds without cgo.
- **The hook stops the panel without asking.** An uninstall that leaves
  `hayami.exe` behind has not removed hayami, and the uninstaller says,
  before it starts, that hayami's own code runs.
- **A set-up from a hidden window.** The hook runs PowerShell with no
  console, and Windows' administrator prompt is the only thing a person
  sees until the log shows the script's lines.
- **A fresh machine is asked twice, not once.** The set-up's elevated step
  is one prompt; the PawnIO driver, which LibreHardwareMonitor offers on its
  first start, raises a second. The scripts' "Windows asks once" counts only
  hayami's own.
- Rollback: delete `fynstall.yaml`, `installer/` and
  `scripts/installer_windows.ps1`; the two switches of
  `install_windows.ps1` change nothing unless given.

## Verification

2026-10-11, Windows 11, fynstall at its `feat/extensions` branch:

- `scripts\installer_windows.ps1 -WithCliOnly` built both installers, 127 MB
  with the wizard and 107 MB without.
- `--dry-run` on the real profile listed the plan above and changed nothing.
- In a temporary profile, `--cli --yes` installed 4 files (86 MB); the
  installed `hayami-tui --version` said 0.9.9; `uninstall.exe --cli` said
  "Before removing anything, it runs hayami's own clean-up code", removed
  it, and left no file and no registry key.
- The check on this desk: "LibreHardwareMonitor is set up: hayami reads the
  processor's temperature from it (Core (Tctl/Tdie) 53.4 °C)".
- The set-up through the hook's code on this desk: installed, settings,
  startup task and running each "[ok]", and "hayami can read it".

2026-10-11, the wizard from Explorer on the real profile, with neither
hayami nor LibreHardwareMonitor installed:

- The Sensors page made the offer and it was taken. Windows asked twice:
  once for LibreHardwareMonitor and once for the PawnIO driver.
- Read off the machine afterwards: the receipt has `sensors: yes`; the two
  programs, the uninstaller and the icon are in the install directory and
  both programs say 0.9.9; one panel runs from the installed copy; the Start
  menu and Startup shortcuts point at it; Settings > Apps has hayami 0.9.9
  with `uninstall.exe --gui`; the user `Path` has `bin`; the settings file
  is the one from before the install.
- LibreHardwareMonitor runs, its startup task is registered, `data.json`
  answers, and the installed `hayami-tui doctor` reads the processor at
  54.1 °C.
- After signing out and in again, the panel was there.
- The uninstaller from Settings > Apps, with the panel running: no
  `hayami.exe` is left running; the install directory, both shortcuts, the
  Settings > Apps key, the `Path` entry and fynstall's index entry are gone;
  the settings file is still there, unchanged. LibreHardwareMonitor and its
  startup task stay, as the uninstall does not own them.
- Left behind in `%TEMP%`: `fynstall-removed-<n>.exe`, the uninstaller's
  moved-aside copy of itself, 26 MB, one per uninstall. fynstall's, not this
  spec's.
- Installed again after that uninstall: it works.
