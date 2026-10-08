# 046 — Settings by section

**Issue**: #143

## Status: COMPLETE

## Context

Two sections have a setting of their own: the bandwidth section watches the
interfaces a person chooses, and the cooler reads LibreHardwareMonitor at an
address (spec 036). Both were root fields of `config.Config` and fields of
`panel.Env`, threaded by hand to every place that builds or refreshes a
source. Adding a setting to a section touched, on `main` (96f789b):

1. a root field on `config.Config` (`internal/config/config.go`: `Interfaces`,
   `LHM`);
2. a field on `panel.Env` (`internal/panel/registry.go`: `Interfaces`, `LHM`);
3. the CLI building the `Env` from the configuration (`internal/cli/cli.go`
   `Options.Env`);
4. the window refreshing a running source when the setting changes
   (`internal/gui/gui.go`: `b.SetInterfaces(c.Interfaces)`);
5. the preferences reading and writing the root field
   (`internal/prefs/bandwidth.go`), wired into the Sections page by hand
   (`internal/prefs/sections.go`, `w.buildInterfaces(s)`).

Five places, plus every test that built an `Env` with the field. Spec 038's
registry had stopped `panel.Sources` growing a parameter per setting, but `Env`
was still growing a field per setting.

## Requirements

- R1 **A setting is declared once, in config.** `config.Setting[T]`
  (`internal/config/sections.go`) is one section's settings: its key, the
  typed struct T it decodes into, and how it reads and writes the root fields
  an older file carries. `config.Bandwidth` (`BandwidthSettings{Interfaces}`)
  and `config.Cooler` (`CoolerSettings{LHM}`) are the two. The zero T is the
  default: no interfaces, LibreHardwareMonitor's default address.
- R2 **Stored under the section.** The settings file carries each section's own
  entry under `sectionSettings.<key>` (`Config.SectionSettings`,
  `map[string]json.RawMessage`, decoded by the section's Setting). The store
  encodes through JSON, so the map round-trips through YAML unchanged.
- R3 **Expand and migrate.** `Setting.Get` reads the section's own entry when
  it is there and decodes, and falls back to the root fields when it is not;
  `Setting.Set` writes both, so a build from before this spec reading the same
  file still finds its setting. An entry that does not decode is treated as
  absent rather than failing the file.
- R4 **The registry hands every source the configuration.** `panel.Env` loses
  `Interfaces` and `LHM` and gains `Settings config.Config`. Each builder reads
  its own section's Setting from it (`config.Bandwidth.Get(e.Settings)`); the
  host's LibreHardwareMonitor address comes from `config.Cooler`. Choosing
  `Env` over a per-spec decoded value: a builder already takes `Env`, and a
  typed Setting read in the builder keeps the type at the one place that uses
  it, where an `any` handed through the registry would lose it.
- R5 **A section's own preferences.** `sectionPrefs`
  (`internal/prefs/sections.go`) maps a section key to its preferences widget;
  the Sections page draws each, in section order, under a separator. The
  bandwidth section's interface chooser is the one entry, and reads and writes
  `config.Bandwidth`. The cooler's address stays settings-file only (spec 036)
  and has no entry. The widget table lives in prefs, not in the panel's
  registry, because the registry is built into the terminal panel, which has
  no toolkit.
- R6 **No visible change.** `doctor`, `readings` and `--help` say what they did;
  the preferences window's Sections page looks as it did.
- R7 `docs/architecture.md` says where a setting goes, and its Status row is in
  place.

Adding a setting to a section now touches **2 places**, or 3 with a widget:

1. a `Setting` value (and its struct) in `internal/config/sections.go`;
2. the section's builder reading it in `internal/panel/registry.go`;
3. optionally, a `sectionPrefs` entry.

`panel.Env`, the CLI and the window are untouched.

## Acceptance Criteria

- [x] An old settings file (root `interfaces`, `lhm`) gives each section its
  settings (`TestAnOldFileGivesEachSectionItsSettings`).
- [x] A file with `sectionSettings` is read from there, ahead of the root
  (`TestASectionsOwnEntryIsReadAheadOfTheRoot`).
- [x] A save writes the section's entry and the root fields both, and reopens
  to the same values (`TestASaveWritesTheSectionsEntryAndTheRootBoth`).
- [x] An entry that does not decode falls back to the root; nothing anywhere is
  the zero settings; Set does not share its map with the value it was given.
- [x] A source reads its own section's settings from `Env.Settings`
  (`TestASourceReadsItsOwnSectionsSettings`, with the root field empty).
- [x] Every Setting and every `sectionPrefs` entry belongs to a section this
  build has.
- [x] Falsified: with the root fallback disabled, the old-file and
  undecodable-entry tests fail; with the section's own entry ignored, the
  own-entry test and the source test fail. Restored, all pass.
- [x] Window tests pass with the harness writing the new shape (it writes
  `sectionSettings`; the old shape is the config tests' to cover). The
  processors test points the cooler at a closed port through
  `sectionSettings.cooler.lhm`, with LibreHardwareMonitor running on the
  desk: had the address not been read from there, the processor would have
  had a temperature and the test would have failed.
- [x] The preferences window's Sections page, photographed on a real window
  from `origin/main` and from this branch: identical but for the version in
  the title bar.
- [x] `doctor` and `--help` identical with digits masked; `readings` identical
  but for volatile values whose digit count changed between runs (a
  temperature's decimal, a timestamp's precision).

## Risks & Assumptions

- **Both shapes are written for now.** A file saved by this build carries the
  root fields and `sectionSettings`; a build from before spec 046 reads the
  root fields and keeps `sectionSettings`, which the store preserves as an
  unknown key it does not drop. A person who edits only the root field by hand
  after this build has written `sectionSettings` will see the section's entry
  win: Get prefers it.
- **The contract step is later.** Removing `Config.Interfaces`, `Config.LHM`
  and the Settings' root readers and writers waits until no build that reads
  only the root fields is in use. It is its own change.
- **No validation beyond decoding.** An address that is not a URL is passed to
  the LibreHardwareMonitor reader as before, which reports it unreachable.
- **Rollback**: revert. Files written by this build stay readable by the
  previous one through the root fields.

## Gaps found

- The preferences' interface chooser does not list the active Wi-Fi interface
  ("Wi-Fi 2") on this desk, before this change and after it: seen in both
  photographs, and not this spec's.
- `internal/core`'s `TestAnOversizedAnswerIsRefused` failed once under the
  full suite's load (1.65 s) and passes alone in 0.06 s; a timing flake, not
  this spec's.

## Verification

2026-10-07, Windows 11, Go 1.26.0, on 96f789b:

- `go test -tags migrated_fynedo ./...`: every package ok but `internal/usage`
  (its Python cross-check, which fails on `main` on this desk) and, once,
  `internal/core`'s oversized-answer test (above).
- Window tests, `HAYAMI_WINDOW_TEST=1`: peripherals, processors,
  LibreHardwareMonitor and Wi-Fi all pass.
- Preferences Sections page, before (from `origin/main`) and after: the same
  rows, separators and chooser, at the same positions.
