# Spec 010: the panel is a glance window

**Issue**: [#26](https://github.com/ushineko/hayami/issues/26)

## Status: COMPLETE

## Executive Summary

`internal/desktop` installs, removes and reads hayami's KWin rule — frameless,
always on top, translucent — and sets the running window's opacity live over
KWin's D-Bus. The menu gains the opacity item its own documentation has
claimed since spec 001, the preferences gain a Window section that installs
the rule, and `hayami window install|remove|status` does the same from a
shell, so a person whose panel has no titlebar can undo it without one.
Reviewers should start with the package comment in `internal/desktop`, which
says why a rule and a script are two mechanisms and not one.

## Context

The panel is described everywhere in this repository as a glance window:
frameless, above other windows, read without being touched. On Plasma it has a
titlebar with window buttons and is fully opaque, and it has been that way
since spec 001. The menu's own doc comment has been describing a feature that
does not exist:

> Menu is the panel's whole interface: preferences, opacity, quit.

There are two items.

**Neither want is the toolkit's to grant**, which is why setting a flag does
not fix this and why the flag that looks right is a trap.
`glance.Options.Translucent` exists, and the design system says where its own
KWin rule is documented that it cannot work here: *"Fyne's desktop backend
never requests a transparent framebuffer, so nothing in the process can draw
through the window (quirk 32)."* The same for the titlebar — `Decorated`
already defaults to false and the window is already created with
`CreateSplashWindow`, and KWin decorates it anyway. Both are the compositor's
to give.

Measured on the machine this was written on, because all of it turns on which
display server the window actually talks to:

- The session is Wayland and the panel is a **native Wayland client**. No X11
  window exists for it at all, so `glance.Window.SetOpacity` — which works by
  writing `_NET_WM_WINDOW_OPACITY`, an X11 property — returns
  `ErrOpacityNeedsCompositor` and can never be the mechanism here.
- The window's `app_id` is `io.ushineko.hayami`, the same string as the `AppID`
  constant, so a rule keyed on it matches.
- KWin's D-Bus is reachable, so a script can be loaded and run and the
  configuration can be reloaded after a write.

The program this replaces solved it with `install_kwin_rule.py`, and the rule
it leaves behind is the shape to copy: `above` and `noborder` forced,
`opacityactive` and `opacityinactive` at 95 and applied-initially rather than
forced, so the user can still override it from the window menu. 95 % is the
number that program has run at for its whole 1.x life, so it is the default
here.

**There are two mechanisms and they are not interchangeable**, which is the
thing to get right:

- A **rule** is persistent. It survives a restart and a compositor restart,
  and it is the only way to be frameless and on top. It is also a write into
  `~/.config/kwinrulesrc`, which is the user's file and nothing this program
  owns, so it happens when the user asks and not on a first run.
- A **script** over D-Bus is live and temporary. It changes the opacity of the
  running window now and leaves nothing behind. It is what a menu item should
  do, because a menu is for trying a value.

So the menu's opacity is live and the preferences' is persistent, and that is
not an inconsistency — it is the difference between the two things Plasma
offers, made visible.

## Requirements

- R1 `internal/desktop` installs, removes and looks up hayami's KWin rule —
  frameless, always on top, and an opacity — and asks KWin to reload after a
  write. It is headless, with no toolkit in it.
- R2 It also sets the opacity of the running window live, over KWin's D-Bus,
  without writing anything to disk.
- R3 The rule is installed only when the user asks. A first run changes
  nothing outside the program's own settings.
- R4 The rule is identifiable in System Settings and removable, and removing
  it gives the titlebar back. Nothing else in `kwinrulesrc` is disturbed.
- R5 The settings file carries the opacity. It is a percentage, and the
  default is the reference's 95.
- R6 The menu has the third item its own documentation claims: an opacity
  submenu, applying live.
- R7 The preferences window can install and remove the rule and set the
  opacity that is written into it.
- R8 A desktop that is not Plasma is not broken by any of it: no KWin, no
  D-Bus or a refused call is a control that says so, not an error and not a
  panel that will not start.
- R9 The command line can install, remove and report the rule, so a person who
  has made the panel frameless can undo it without one.

## Acceptance Criteria

- [x] AC1 A rule written into a `kwinrulesrc` the test owns carries the app ID, `noborder`, `above` and the opacity, forced or applied-initially as each should be. (R1, R5)
- [x] AC2 Installing into a file that already holds unrelated rules leaves every one of them, and their numbering, intact. (R4)
- [x] AC3 Installing twice updates the existing rule rather than adding a second. (R1)
- [x] AC4 Removing takes the rule out and leaves the unrelated ones; removing one that is not there is not an error. (R4)
- [x] AC5 A lookup reports the rule's presence and the opacity it carries. (R1, R7)
- [x] AC6 The live opacity call names KWin's own interface and carries a script that matches only this app ID. (R2)
- [x] AC7 With no KWin and no session bus, install, remove, lookup and the live call each report that plainly and none of them panics or fails the panel. (R8)
- [x] AC8 The settings file round-trips the opacity, and a file without one gets the default. (R5)
- [x] AC9 The menu has three items, and the opacity submenu ticks the value in the settings. (R6)
- [x] AC10 The preferences window offers the rule and the opacity, and a change reaches the store. (R7)
- [x] AC11 The command line installs, removes and reports the rule. (R9)
- [x] AC12 **On this machine**, installing the rule gives a frameless panel at the chosen opacity, and removing it gives the titlebar back. Photographed both ways. Skipped where KWin is absent. (R1, R4)

## Gaps found

**`fynedesygn/glance/kwin.Rule` forces the opacity, and its documentation says
it does not.** The field's comment reads *"It is applied initially rather than
forced, so the user can still override it from the window menu"*, and the rule
it writes carries `opacityactiverule=2`, which is forced. The reference's own
rule uses `4`. The practical difference is that KWin's window menu cannot
override hayami's opacity, so the panel's own menu is the only way to try a
value — which works, but is not what either the library or the reference
intended.

Not worked around here: reimplementing the rule writer to change one line
would be this repository copying from the design system, which is the thing
the project rules forbid. `TestEverythingInTheRuleIsForced` asserts what the
library actually does and says in its comment that it should fail the day this
is fixed, so the workaround is a failing test rather than a silent divergence.

## What the tests caught

Less than the last two specs, and for a reason worth recording: **this one was
verified against the compositor from the first step rather than the last.**
The live opacity call was made against the running KWin before any interface
was built on top of it, and the rule was installed against a real
`kwinrulesrc` before the preferences section existed. Both worked, and neither
was a decoder that had never spoken to anything.

The one thing the tests caught that the eye would not: `Install` and `Remove`
round-trip a real rules file **byte for byte**. It was checked against this
machine's own file — sixteen rules, two of them belonging to the program this
replaces — by diffing before and after, because a program that tidies away
rules it did not create is a failure nobody notices until they wonder where
their other rules went.

One thing was wrong in the tests rather than the code: a settings file written
before this spec was asserted to carry a zero opacity, and the store merges
the file over `Default()`, so it carries the default instead. The behaviour
was better than the assertion, and the assertion was the thing that changed.

## Risks & Assumptions

- **This writes to a file the user owns.** `~/.config/kwinrulesrc` holds every
  window rule on the machine, including two belonging to the program this
  replaces. The tests operate on a file the test wrote, never the real one,
  and AC2 exists because the failure mode — a program that tidies away rules
  it did not create — is one nobody would notice until they wondered where
  their other rules went.
- **Plasma only.** The rule and the script are KWin's. Another compositor gets
  a panel that works and a control that says the feature is not available
  here, which is the same answer the cooler gives a machine with no liquidctl.
- **95 % is a measurement of habit, not of anything.** It is what the
  reference has run at. It is the default because it is familiar, and it is
  recorded as such rather than as a finding.
- **The two mechanisms can disagree.** A live opacity from the menu and a
  persistent one in the rule are different values until the user saves one,
  and after a restart the rule's is what applies. That is the honest behaviour
  of the two things Plasma offers and is said in the interface rather than
  hidden.
- Rollback: revert, and remove the rule. The command line can do the second
  without the program.

## Alternatives Considered

- `glance.Options.Translucent`. Rejected: the design system documents that
  Fyne's desktop backend never asks for a transparent framebuffer, so it
  cannot work.
- `glance.Window.SetOpacity`. Rejected here: it writes an X11 property and
  this panel is a native Wayland client, so it returns
  `ErrOpacityNeedsCompositor`. It is left for the X11 sessions where it does
  work, if that is ever wanted.
- Installing the rule on first run, as the reference's installer effectively
  does. Rejected: it is a write into the user's own configuration, and a
  program that does that unasked is one you cannot trust with the rest of the
  file.
