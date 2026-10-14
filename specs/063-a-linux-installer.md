# 063 — a Linux installer

**Issue**: #201

## Status: INCOMPLETE

## Context

Spec 059 built hayami's installer with fynstall for Windows and left Linux
out of scope. A release carries two Windows installers and, for Linux, a
tarball with `install.sh` in it.

The config was never the obstacle: `fynstall.yaml` is one config for every
platform, and `fynstall build -target linux/amd64` made a working installer
from it unchanged. Two things `install.sh` does had no place in it.

- **The udev rule.** The rule that lets a person open their devices belongs
  in `/etc/udev/rules.d`, which a per-user install may not write. An
  installer that said nothing about it would install a panel that reports
  every device as not permitted.
- **A panel that is already running.** Nothing keeps two panels from
  running. The installer starts the panel when it is done (0.10.3), so an
  install over a running one, which is every upgrade, would leave two
  windows on the desk. On Windows the uninstall hook stops the panel because
  its file cannot be replaced otherwise; on Linux nothing had to, and
  nothing did.

## Requirements

- R1 `make installer` builds `dist/hayami-<version>-linux-amd64-installer`,
  with the wizard, and `…-cli-installer`, without, and their uninstallers,
  from `fynstall.yaml`. The builder is run from the module
  (`go run github.com/ushineko/fynstall/cmd/fynstall`), so it is the version
  go.mod requires and nothing is installed to build.
- R2 The installer carries `packaging/60-sanshoku.rules` inside its own
  program (`packaging/embed.go`), not in the payload: see "Gaps found".
- R3 **The rule**, in `installer/`'s install hook, as `install.sh` does it:
  - R3.1 A rule already in `/etc/udev/rules.d` that says the same, comments
    aside, is left as it is and nothing is said.
  - R3.2 Where the directory can be written the rule is copied there, whole
    or not at all, `udevadm control --reload` is run, and the log says to
    replug.
  - R3.3 Where it cannot, the install succeeds, writes the rule to
    `share/60-sanshoku.rules` in the install directory, and the log has the
    two commands that install it from there. The installer never asks to
    become root. That copy is the hook's and not fynstall's, so the
    uninstall hook removes it, in an upgrade too.
  - R3.4 An uninstall removes the rule when it is byte for byte the copy the
    installer carries, or says the commands that do; a different file of that
    name is left and said to be. An upgrade leaves the rule.
  - R3.5 `HAYAMI_UDEV_DIR` names another directory, as it does for
    `install.sh`, for the tests.
- R4 **The running panel.** After an install, before fynstall starts the
  new panel, and before an uninstall, the hook stops each process the
  kernel calls `hayami` whose program is inside the install directory or is
  `~/.local/bin/hayami`, where `install.sh` puts it, including one whose
  file has been replaced under it. It asks (SIGTERM), waits up to five
  seconds for the process to end, and then kills. A terminal panel, a
  `hayami` from anywhere else and another person's process are left alone.
- R5 CI builds the Linux installers on every run, runs `--version` on both
  and `--dry-run --yes` on the command-line one, and installs and uninstalls
  with it in a throwaway home. The release job attaches both installers
  beside the tarball and lists them in `SHA256SUMS`.
- R6 The tarball and `install.sh` stay.
- R7 The README says how to run the installer, what it does about the rule
  and about a running panel, and the changelog has the entry.

## Acceptance Criteria

- [x] `make installer` writes the four programs to `dist/`, and the
  command-line installer's `--version` is `hayami <version> installer`.
- [x] The config is unchanged for Windows: nothing in `fynstall.yaml` is
  for one platform, and the hooks are in files built for Linux only
  (`GOOS=windows go vet -tags nogui ./installer/`).
- [x] R3, in `installer/hooks_linux_test.go`: installed where it may be; explained
  where it may not, with nothing half written; a rule in place under another
  header left alone; an uninstall removes only its own copy, an upgrade
  leaves it; the copy kept in the install directory goes with either.
- [x] R4, in `installer/hooks_linux_test.go`, against real processes: the panel in
  the install directory and the one in `~/.local/bin` have ended when the
  hook returns; a panel elsewhere and a terminal panel in the install
  directory run on; a panel whose file was replaced by a link is found.
- [x] Each of those tests fails when the behaviour it names is broken
  (falsified: the deleted-file suffix, the name check, the directory check,
  the comment-blind comparison, the upgrade guard, the kept copy's removal).
- [x] The real downstream system: `tests/structure/linux_installer_test.go`
  runs the built command-line installer in a home of its own over a running
  stand-in for `install.sh`'s panel, then the uninstaller. Both panels, the
  links, both desktop entries, the icon and the rule are in place and the
  old panel has ended; afterwards the install directory and the rule are
  gone, the replaced file is back, and the settings are untouched.
- [x] Desk check, Plasma 6 on Wayland, 2026-10-13: the wizard, over a panel
  `install.sh` installed and that was running under its autostart unit.
  One panel afterwards, running from `~/.local/share/hayami/bin/hayami`, at
  the saved position; the links in `~/.local/bin` and the autostart entry
  point into the install.
- [ ] go.mod requires a fynstall whose wizard opens with
  `ui.launch_default` (ushineko/fynstall#51). Built against 0.2.3 the
  wizard stops before its window; the desk check above used an installer
  built with `-runtime-path` against the fix.
- [ ] CI is green with the new steps, on a runner.
- [ ] A release carries both Linux installers and `SHA256SUMS` lists them.
- [ ] Desk check: the wizard's uninstaller, and an upgrade from one
  installer-installed version to the next.
- [ ] Desk check: a machine without the rule, where the wizard's log shows
  the two commands.

## Out of scope

- **A system install.** fynstall can install for everyone through `pkexec`,
  and would then own the udev rule as an ordinary file. It moves the
  programs to `/opt` and the entries to `/usr/local/share`, and is a
  different install from the one `install.sh` makes; per-user is the one
  the config names.
- **Asking for root for the rule.** The network helper on Windows is offered
  on a page and installed behind one administrator prompt. The rule could
  be offered the same way, through `pkexec`. Not done: the two commands are
  what `install.sh` has always printed.
- **Other architectures**, for the reason the tarball is amd64 only.
- **Opening the wizard in CI.** The runner has no display, so CI runs the
  wizard installer's `--version` and nothing more of it. That is how
  fynstall#51 reached a release.

## Risks & Assumptions

- **The hook stops the panel whether or not it is started again.** A person
  who clears "Launch now", or passes `--launch=false`, over a running panel
  is left with none until they start it. The hook cannot see the choice, and
  Windows behaves the same way.
- **A panel that ignores SIGTERM for five seconds is killed.** The panel
  writes its settings when they change and not at exit, so nothing is lost.
- **Matching by the kernel's name.** A program somebody names `hayami` and
  keeps in `~/.local/bin` or inside the install directory is stopped. Those
  are the two places this program is installed to.
- **The stop test scans the real `/proc`.** Its stand-ins live in
  `t.TempDir()` and `HOME` is the test's, so the code under test cannot
  match a real panel. A deliberately broken matcher can, and did once while
  the tests were being falsified on a desk with a panel running.
- **An install over `install.sh`'s keeps a backup.** fynstall moves the
  replaced `~/.local/bin/hayami`, the entries and nothing else aside, and
  the uninstaller puts them back, so an uninstall returns the desk to the
  script's install and not to nothing. `uninstall.sh` removes that.
- **Size.** The wizard installer is about 115 MB and the command-line one
  about 75 MB; the tarball is far smaller. A release grows by about 190 MB.
- **glibc.** The installers are built on the same runner as the tarball and
  carry the same panel, so the same floor applies.
- Rollback: revert the PR. The hooks are in files built only for Linux,
  `fynstall.yaml` changes by nothing, and the Windows installers are built
  by a script this does not touch.

## E2E Test Plan

- Environment: a Linux desk with a release's installers downloaded.
- `chmod +x`, run the wizard over a running panel: one panel afterwards
  (R4), entries and links into `~/.local/share/hayami` (R1).
- On a machine without the rule: the log shows the two commands (R3.3);
  after running them and replugging, `hayami-tui doctor` names no device as
  not permitted.
- `~/.local/share/hayami/uninstall`: the panel stops, the install is gone,
  `~/.config/hayami` stays (R3.4, R4).

### Prod visibility

Not applicable: a desktop program with no service side. A release's assets
and `SHA256SUMS` on GitHub are what can be observed.

## Gaps found

For fynstall:

- **The wizard is made before the app.** `launch_default: true` stopped the
  wizard with a nil pointer (ushineko/fynstall#51), and a Settings page with
  a field does the same for the same reason (ushineko/fynstall#52).
- **A payload entry for one platform cannot be built one platform at a
  time.** The rule was first a payload entry with `targets: [linux/*]`. A
  config without `targets` builds for the machine it is on, and on Windows
  the entry then "matches none of the targets this config builds", with or
  without `-target`. A config that names both targets wants every target's
  payload present for any build, and the panel cannot be cross-compiled.
  So the rule is embedded and the hook writes it out.
- **A hook cannot see whether the program will be started**, so it cannot
  leave a running panel alone when it will not be replaced by a new one.
- **Nothing in a per-user install can ask for root for one file.** The
  helper that applies a system install's plan is there; a per-user install
  has no way to borrow it.

## Verification

2026-10-13, CachyOS, Plasma 6 on Wayland, Go 1.26:

- `make installer` built the four programs: 115 MB, 75 MB, and
  uninstallers of 25 MB and 5 MB.
- The command-line installer, with `HOME` and `HAYAMI_UDEV_DIR` in a
  scratch directory: 12 files installed, the rule copied and said, both
  entries with an absolute `Exec`; the uninstaller removed all of it and
  left a settings file put there by hand. With that directory read-only:
  the five lines with the two commands, the copy at
  `share/60-sanshoku.rules`, and nothing left after the uninstall.
- `go test ./installer/` and the structure test with `HAYAMI_INSTALLERS`
  set pass.
- The wizard built against fynstall 0.2.3 stopped before its window. Built
  against the fix for fynstall#51 it opened and installed over the running
  panel, as the desk check above says.
