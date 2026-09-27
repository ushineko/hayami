# Spec 007: preferences, in a window

**Issue**: [#17](https://github.com/ushineko/hayami/issues/17)

## Status: INCOMPLETE

## Executive Summary

(Populated before the PR opens.)

## Context

This is the headline difference from the program being replaced, and it has
been deferred through six specs because the settings had to exist before
anything could edit them. They do now: which sections are drawn, in what
order, in which arrangement, and which interfaces the bandwidth section
watches.

The Python's right-click menu is six nested submenus deep. That is a settings
dialog wearing a menu's clothes, and the rewrite's stated purpose is to take it
apart: a minimal menu on the panel, and a preferences window on the fynedesygn
shell.

The window is a shell program in its own right — header, navigation, content
scroller, status bar — which is the *other* archetype the design system
describes. `docs/design-system.md` has its rules, and this is the first thing
in hayami that follows them rather than `docs/glance.md`.

One decision worth naming: the preferences are a second window of the same
process, not a second binary. The panel and the preferences share a settings
store, and a change made in one has to reach the other the moment it is made.
Two processes would mean a file watcher and a race for the last write.

Today the bandwidth section watches nothing until an interface is named, which
means a new user sees an empty section and has no way to fill it that does not
involve a text editor. That is the clearest case for this window existing.

## Requirements

- R1 A preferences window on the fynedesygn shell, opened from the panel's
  menu, and a second window of the same process rather than a second binary.
- R2 A Sections screen: which sections are drawn, in what order. Reordering
  and hiding both, and the panel follows without a restart.
- R3 An Arrangement choice: stack, grid or row, described rather than only
  named.
- R4 A Bandwidth screen listing the interfaces this machine has, each one
  watched or not, so nobody has to know an interface's name to choose it.
- R5 The shell's own Appearance section, for the scheme and the text size.
- R6 The panel's menu is Preferences, opacity and quit, and nothing else.
- R7 A change is saved and applied without an explicit save: the settings
  store already debounces its writes.

## Acceptance Criteria

- [ ] AC1 Opening preferences twice raises the window it already made rather than making a second. (R1)
- [ ] AC2 Hiding a section removes it from the panel, and reordering moves it, without a restart. (R2)
- [ ] AC3 The arrangement can be changed and the panel redraws in it. (R3)
- [ ] AC4 The interfaces offered are the ones `/proc/net/dev` reports, and choosing one makes the bandwidth section draw it. (R4)
- [ ] AC5 The Appearance section is present and changes the theme. (R5)
- [ ] AC6 The panel's menu has three items. (R6)
- [ ] AC7 A change reaches the settings file without a save button, and the panel reflects it. (R7)
- [ ] AC8 **Seen on a real window**: both windows screenshotted and looked at, per the rule in `.claude/CLAUDE.md`. (R1–R5)

## Risks & Assumptions

- **Two windows, one store.** They are the same process and the same
  `config.Store`, so the only ordering question is which goroutine writes, and
  every write happens on the UI thread.
- **A section's poll starts and stops with its visibility.** Hiding a section
  from the preferences window has to stop its poll, or a hidden cooler section
  would keep running liquidctl every five seconds for nobody.
- **The shell is the other archetype.** Its rules are in
  `docs/design-system.md`, not `docs/glance.md`, and the two disagree about
  almost everything structural. Reading the wrong page is the likely mistake
  here.
- Rollback: revert. The settings file is unchanged in shape; only the way it
  is edited is new.

## Alternatives Considered

- A second binary, `hayami-prefs`. Rejected: the panel and the preferences
  share a store and a change must reach the panel as it is made. Two processes
  would need a file watcher and would race for the last write.
- Keeping the settings editable only by hand. That is the state this spec
  exists to end; the bandwidth section watching nothing until a file is edited
  is the clearest evidence.
- Putting the choices in the menu, as the Python does. That is what the
  rewrite set out to undo.
