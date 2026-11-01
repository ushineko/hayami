# 078 — the Linux installer offers the network helper

**Issue**: #257

## Status: COMPLETE

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

- [x] The installer's Network page on Linux, with `pkexec`, is shown and
      offers a clear check box with Linux's text.
      (`TestTheNetworkPageIsShownWhereverTheHelperIsOffered`, falsified;
      `TestTheNetworkPageOffersACheckBoxThatStartsClear`; and the real
      wizard on this desk, below.)
- [x] Ticking it and approving the prompt installs the helper: the GUI
      installer built from the branch, on this desk, as the person at it.
- [x] Without `pkexec` the page is not offered and the log carries the sudo
      line. (`TestAHelperThatCannotBeOfferedIsStillSaid`, falsified.)
- [x] `RunElevated` runs the program through `pkexec` with its arguments, and
      says a dismissed prompt as that. (`TestInstallingAsksThroughPkexec`,
      with a fake `pkexec`.)
- [x] Without `pkexec`, `RunElevated` says the sudo line.
      (`TestInstallingWithoutPkexecSaysWhatToRun`.)
- [x] The Windows page and its tests are unchanged in what they check.
- [x] `make test` and `make lint` pass.
- [x] The docs of R6 are updated.

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
- **Review of this change** (spec 073's procedure, one reviewer, findings
  checked against the code by the author). No path to root or to the
  helper's capabilities beyond what `sudo <same path>` already gave. Fixed
  here: the uninstaller elevated the user-writable `hayami-tui` to remove
  the helper, when the helper's root-owned copy can remove itself, and now
  uses that copy on Linux (`nethelper.RemoveFromCopy`; Windows cannot delete
  a running program); and `pkexec` was taken from the `PATH` as found, so a
  fake one placed there by a program running as the user could put the page
  in front of the person and take the password, and is now used only if it
  is root's and setuid (`trustedPkexec`, tested both ways). Accepted: `Skip`
  asks systemd whether the helper runs, which is a short subprocess on the
  window's thread.
- Rollback: revert the commit; installs already made are unaffected.

## Verification

On this desk (CachyOS, KDE Plasma 6, polkit with KDE's agent), with GUI
installers built from the branch under test version numbers:

- The first build skipped the Network page in the real wizard ("skipped" in
  its list of pages). The page's own check and the headless tests were
  right; the wizard's `Skip` was "off Windows" for every offer page, ahead
  of the check. Fixed by giving each page its own answer, and a test of the
  wizard's question that fails with the old rule.
- The second build showed the page; the box ticked and the prompt approved,
  `hayami-tui network-helper` said `installed: 0.11.13-test.2 ... for
  nverenin`, the socket was active, and `hayami-tui detail bandwidth` said
  "every protocol".
- Not exercised by hand: the uninstaller's removal through `pkexec` (R3),
  which runs the same `RunElevated` and is covered by the existing removal
  tests with the call replaced.

## E2E Test Plan

On this desk (CachyOS, KDE Plasma, polkit with KDE's agent):

1. Remove the helper. (Setup.)
2. Run the GUI installer built from the branch; the Network page is there
   with Linux's text. (AC 1.)
3. Tick it, finish; approve the prompt; `hayami-tui network-helper` says it
   is installed, and the drawer says "every protocol". (AC 2.)

### Prod visibility

Not applicable: a desktop program with no production deployment.
