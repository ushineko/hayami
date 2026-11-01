# 078 — the Linux installer offers the network helper

**Issue**: #257

## Status: INCOMPLETE

## Context

Installing hayami on Linux with the GUI installer did not install the
network helper of spec 072, and said nothing about it (#257, found installing
on a second machine). The installer's Network page was Windows' alone: on
Windows the helper is installed through UAC, and the Linux helper was to be
installed only by a sudo line, because hayami's installers never became
root. `install.sh --with-network` printed that line; the GUI installer had no
equivalent, so a person who did not already know of the helper never heard
of it, and the bandwidth list stayed TCP only.

The project's owner chose to offer it through polkit: the page with a check
box, as on Windows, and `pkexec` asking the desktop for an administrator once
when the box is ticked.

## Requirements

- R1 On Linux the Network page is offered when `pkexec` is on the `PATH`,
  with Linux's account of the helper: why it exists, the steps installing
  it takes (the copy in `/usr/lib/hayami`, the systemd units, the socket only
  the account may open, the dynamic user with three capabilities), and that
  the desktop asks for an administrator once. The text is `nethelper`'s for
  each system.
- R2 Ticking the box runs the installed `hayami-tui network-helper install
  --allow <uid>` through `pkexec` after the install. A prompt dismissed or
  refused is a warning in the log; hayami is installed either way.
- R3 An uninstall removes the helper the same way, through `pkexec`, before
  the program that removes it is gone; an upgrade leaves it.
- R4 Without `pkexec`, the page is not offered, and the install log says the
  sudo line instead: the installer is never silent about the helper.
- R5 `hayami-tui network-helper install`, run without root, asks through
  `pkexec` as well (a text prompt in a terminal); without `pkexec` it says
  the sudo line, as before. Under `pkexec`, the account the helper is for is
  read from `PKEXEC_UID`, as it is from `SUDO_UID` under sudo.
- R6 The rule changes from "hayami never asks to become root" to "hayami
  asks to become root only when the person chose an action that needs it,
  and through the system's own prompt". `docs/architecture.md`, `CLAUDE.md`,
  the README and `install.sh` say so.
- R7 No test asks the machine it runs on for an administrator: `pkexec` is
  replaced by a script of the test's in every test that would reach it.

## Acceptance Criteria

- [ ] The installer's Network page on Linux, with `pkexec`, offers a clear
      check box and says what installing does on Linux.
- [ ] Ticking it and approving the prompt installs the helper (checked on
      this desk with the GUI installer built from the branch).
- [ ] Without `pkexec` the page is not offered and the log carries the sudo
      line. (`TestAHelperThatCannotBeOfferedIsStillSaid`, falsified.)
- [ ] `RunElevated` runs the program through `pkexec` with its arguments, and
      says a dismissed prompt as that. (`TestInstallingAsksThroughPkexec`,
      with a fake `pkexec`.)
- [ ] Without `pkexec`, `RunElevated` says the sudo line.
      (`TestInstallingWithoutPkexecSaysWhatToRun`.)
- [ ] The Windows page and its tests are unchanged.
- [ ] `make test` and `make lint` pass.
- [ ] The docs of R6 are updated.

## Threat model delta

B2: an install step on Linux may now become root, through polkit, when the
person ticks the box and approves the prompt. The program `pkexec` runs is the
installed `hayami-tui` under `~/.local/share/hayami`, which the user can
write: code already running as the user could replace it before the prompt,
the same as with the sudo line it replaces. polkit's prompt names the program.
`docs/threat-model.md`'s B2 section records this once it is on `main` (#252).

## Risks & Assumptions

- **A polkit agent is needed.** KDE, GNOME and the other desktops run one; a
  session without one makes `pkexec` fail, which is a warning in the log and
  the sudo line still works.
- **The CLI now prompts.** `network-helper install` typed without sudo asks
  for a password through polkit's text agent where it used to print the sudo
  line.
- Rollback: revert the commit; installs already made are unaffected.

## E2E Test Plan

On this desk (CachyOS, KDE Plasma, polkit with KDE's agent):

1. Remove the helper. (Setup.)
2. Run the GUI installer built from the branch; the Network page is there
   with Linux's text. (AC 1.)
3. Tick it, finish; approve the prompt; `hayami-tui network-helper` says it
   is installed, and the drawer says "every protocol". (AC 2.)

### Prod visibility

Not applicable: a desktop program with no production deployment.
