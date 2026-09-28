# Spec 010: the panel is a glance window

**Issue**: [#26](https://github.com/ushineko/hayami/issues/26)

## Status: COMPLETE

## Executive Summary

The panel asks the toolkit for translucency, which it turns out to grant, so
the desktop shows through the space between cards on any desktop and with no
compositor involved. The cards are faded separately by a theme, to a
percentage the user sets, so the readings stay legible. `internal/desktop`
installs and removes a KWin rule for the one thing only KWin can do — the
titlebar — and `hayami window install|remove|status` does the same from a
shell. Reviewers should start with `withCardOpacity` in
`internal/gui/theme.go` and the package comment in `internal/desktop`, which
says which of these is the toolkit's and which is the compositor's.

## Context

The panel is described everywhere in this repository as a glance window:
frameless, above other windows, read without being touched. On Plasma it has a
titlebar with window buttons and is fully opaque, and it has been that way
since spec 001. The menu's own doc comment has been describing a feature that
does not exist:

> Menu is the panel's whole interface: preferences, opacity, quit.

There are two items.

**One of the two is the toolkit's and one is the compositor's**, and the first
draft of this spec got that backwards.

It was written believing neither was the toolkit's, on the strength of a
sentence in `kwin.Rule`'s documentation: *"Fyne's desktop backend never
requests a transparent framebuffer, so nothing in the process can draw through
the window (quirk 32)."* That sentence is about Fyne. It is not about
`glance`, which works around exactly that by setting GLFW's transparent
framebuffer hint itself, probing whether it was granted, and swapping in a
theme with a transparent background. Reading a comment in one file as though
it described another is how a whole design came to be pointed at the
compositor.

Measured instead of assumed: `Options.Translucent` is **granted** here —
`Translucent()` answers true and the desktop shows through the space between
cards. So translucency is native, needs no compositor and works on any
desktop.

The titlebar really is the compositor's. `Decorated` already defaults to false,
the window is already created with `CreateSplashWindow`, and KWin decorates it
anyway — which the design system's `NoBorder` field says in as many words.
That is the one thing a rule is for.

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

**The panel has two backgrounds and they want different things.** The window's
own is the space *between* cards, and the design system makes it fully
transparent — that is what a glance window is. The cards are what the readings
sit on, and that transparency deliberately leaves them alone, because a card as
see-through as the gap around it is a card nobody can read a number off.

So the opacity the user sets is the **card's**, applied by a theme. Ninety-five
per cent is barely a fade and that is the point: the desktop behind is
suggested rather than shown, and the figures stay as legible as they are on an
opaque panel. It is drawn by the toolkit, so it works on any desktop, and there
is one mechanism rather than two — the menu and the preferences set the same
setting and the panel re-fades as soon as either does.

The rule is left with the one thing only it can do: the titlebar, and
always-on-top alongside it. A toolkit request for on-top is one the window
manager may decline and a rule survives a compositor restart, so asking twice
is cheap insurance rather than a duplicate. The rule carries **no opacity** —
it used to, and a rule that faded the window as well would fade it twice.

## Requirements

- R1 `internal/desktop` installs, removes and looks up hayami's KWin rule —
  frameless and always on top, and nothing else — and asks KWin to reload
  after a write. It is headless, with no toolkit in it.
- R2 The panel asks the toolkit for translucency and gets it, so the desktop
  shows through the space between cards with no compositor involved.
- R3 The rule is installed only when the user asks. A first run changes
  nothing outside the program's own settings.
- R4 The rule is identifiable in System Settings and removable, and removing
  it gives the titlebar back. Nothing else in `kwinrulesrc` is disturbed.
- R5 The settings file carries the card opacity. It is a percentage, the
  default is the reference's 95, and it fades the card and its border and
  never the text.
- R6 The menu has the third item its own documentation claims: an opacity
  submenu, which saves and takes effect at once.
- R7 The preferences window can install and remove the rule, and set the card
  opacity.
- R8 A desktop that is not Plasma is not broken by any of it: no KWin, no
  D-Bus or a refused call is a control that says so, not an error and not a
  panel that will not start.
- R9 The command line can install, remove and report the rule, so a person who
  has made the panel frameless can undo it without one.

## Acceptance Criteria

- [x] AC1 A rule written into a `kwinrulesrc` the test owns carries the app ID, `noborder` and `above`, both forced, and **no opacity**. (R1)
- [x] AC2 Installing into a file that already holds unrelated rules leaves every one of them, and their numbering, intact. (R4)
- [x] AC3 Installing twice updates the existing rule rather than adding a second. (R1)
- [x] AC4 Removing takes the rule out and leaves the unrelated ones; removing one that is not there is not an error. (R4)
- [x] AC5 A lookup reports whether the rule is installed. (R1, R7)
- [x] AC6 The card is faded by exactly the percentage asked for, the window's own background is left alone, the border fades with the card and the text never does. (R2, R5)
- [x] AC7 With no KWin and no session bus, install, remove and lookup each report that plainly, the rule is still written, and none of them panics or fails the panel. (R8)
- [x] AC8 The settings file round-trips the opacity, and a file without one gets the default. (R5)
- [x] AC9 The menu has three items, and the opacity submenu ticks the value in the settings and saves the one chosen. (R6)
- [x] AC10 The preferences window offers the rule and the opacity, and a change reaches the store. (R7)
- [x] AC11 The command line installs, removes and reports the rule. (R9)
- [x] AC12 **On this machine**, the panel is translucent with no rule installed at all, installing the rule takes the titlebar away, and removing it gives the titlebar back. Photographed. Skipped where KWin is absent. (R1, R2, R4)

## Gaps found

**`kwin.Rule`'s documentation describes the wrong thing in the wrong place.**
Its `Opacity` field says *"This is the only way a glance window is
translucent: Fyne's desktop backend never requests a transparent framebuffer,
so nothing in the process can draw through the window (quirk 32)."* That is
true of Fyne and false of `glance`, which sets the GLFW hint itself in
`grantTranslucent` and swaps in a transparent theme. The sentence sits in the
file a reader consults when deciding how to be translucent, and it points them
away from the feature the same library already provides. Worth a fynedesygn
issue; it cost this spec a complete redesign.

**There is no card opacity in the design system.** `WithTransparentBackground`
zeroes `ColorNameBackground` and leaves `ColorNameButton` alone, which is
right — but a panel that wants its cards *partly* see-through has to write its
own theme wrapper, as `internal/gui/theme.go` does. It is nine lines and it is
plausibly a `glance.Options.CardOpacity`. Worth raising there rather than
being copied by the next program that wants it.

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
- **Translucency is a request the driver can refuse.** `grantTranslucent`
  probes for it and an opaque window is what a refusal gives, which is the
  honest outcome and not a failure. It is reported on some drivers, so a
  machine where the panel is simply opaque is a machine where this was
  refused, not one where something is broken.
- **The panel is not resizable and is as wide as its widest reading.** That is
  the design system's rule — a glance window is whatever its content measures
  — and on this machine the usage section's captions make it 655 px against
  268 px without them. Narrowing it is a change to what usage *says* and
  belongs in its own spec.
- Rollback: revert, and remove the rule. The command line can do the second
  without the program.

## Alternatives Considered

- Doing the translucency with KWin as well, through the rule's own opacity.
  Rejected after the first draft did exactly that: the toolkit grants
  translucency here, so routing it through the compositor makes the feature
  Plasma-only for no reason and fades the window twice if both are set.
- `glance.Window.SetOpacity`. Rejected here: it writes an X11 property and
  this panel is a native Wayland client, so it returns
  `ErrOpacityNeedsCompositor`. It is left for the X11 sessions where it does
  work, if that is ever wanted.
- Installing the rule on first run, as the reference's installer effectively
  does. Rejected: it is a write into the user's own configuration, and a
  program that does that unasked is one you cannot trust with the rest of the
  file.
