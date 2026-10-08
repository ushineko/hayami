# 038 — One registry for the sections

**Issue**: #126

## Status: COMPLETE

## Context

The architecture review after spec 037 found that a section's identity was
written out in seven places that had to agree, with nothing but reading all
seven to say whether they did. On `main` at 06a82a8:

| # | Where | What it spelled |
|---|---|---|
| 1 | `internal/config/config.go:159` | the default section list, as literals |
| 2 | `internal/panel/panel.go:154` (`Sources`) | a `switch` over the keys |
| 3 | `internal/panel/panel.go:165` (`Keys`) | the keys again, in order |
| 4 | the source's `Key()` (`panel/cooler.go:113`, `panel/usage.go:57`, `panel/peripherals.go:139`, `core/bandwidthsection.go:107`) | the key as a literal |
| 5 | the view builder (`view/cooler.go:83`, `view/bandwidth.go:58`, `view/peripherals.go:199`, `view/usage.go:67`) | `Section{Key, Title, Icon}` literals |
| 6 | `internal/view/view.go:452` | an `Icon*` constant |
| 7 | `internal/gui/icons.go:31` | a `switch` from icon to glyph |

Beside them:

- `Title()` existed on three sources and not the fourth, and nothing called
  it: it was what `core.Section` (`internal/core/section.go`) asked for, an
  interface nothing implemented as such and nothing used.
- `panel.Sources` grew a parameter per section setting: the interfaces
  positionally, then the LibreHardwareMonitor address as a variadic option
  (`WithLHM`, spec 036). `cli.Diagnose` carried both.
- `panel.DeviceScan` was a package global that the cooler and peripherals
  constructors read and `testenv.NoDevices` replaced.
- The preferences listed the sections by their settings key ("cooler").
- The parity test built the sources from `Keys()` and compared their keys
  with `Keys()`, which is true by construction.

## Requirements

- R1 `view.SectionInfo{Key, Title, Icon, Default}` and `view.Sections()`, the
  ordered list of the sections this build has; `view.DefaultSections()` and
  `view.SectionByKey()` read it. One named value per section
  (`view.CoolerInfo` …) for the builder and source that belong to it. The
  list lives in `view` because the settings, the panel and both shells
  already import it, and the settings may not import the panel.
- R2 Each view builder starts from its info (`CoolerInfo.section()`), so no
  builder spells a key, title or icon.
- R3 `panel.Env{Interfaces, Counters, LHM, Scan}` is what sources are built
  over. `panel.Specs()` joins `view.Sections()` with a table of builders by
  key; `panel.Sources(keys, env)` and `panel.Keys()` read the same. A section
  with no builder, or a builder for a section the list does not have, panics
  in `Specs()`, which the registry test calls. `WithLHM`, `Option` and the
  positional interfaces go.
- R4 `config.Default()` takes its sections from `view.DefaultSections()`.
- R5 The window's glyphs are a map keyed by icon name.
- R6 The sources' keys come from their info; the dead `Title()` methods,
  `core.BandwidthSection`'s `Key`/`Title` and `core.Section` are deleted.
- R7 `panel.DeviceScan` becomes `Env.Scan`. The one remaining variable is
  `panel.DefaultScan`, used only by an `Env` with no scan. It stays a
  variable for the commands a test runs end to end, which build their `Env`
  inside the command.
- R8 `cli.Options.Env` builds the `Env` from the settings; `cli.Diagnose`
  takes one.
- R9 The bandwidth source moves out of `panel.go` into `panel/bandwidth.go`,
  as the other three already were.
- R10 The preferences list the sections by title.
- R11 Tests: every section builds a source under its own key; keys, defaults
  and sources follow the one list, skipping an unknown key; every section's
  icon has a glyph in the window; the parity test checks each built source's
  section carries the registry's title and icon.

## Acceptance Criteria

- [x] Adding a section touches **3** places (the `view.Sections` table in
  `view/sections.go`, the builder table in `panel/registry.go`, the window's
  glyph map), or **4** with a glyph of its own (an `Icon*` constant), plus
  the section's own source and builder. It was **7**.
- [x] No change in behaviour: `hayami-tui doctor`, `readings`, `--help` and
  the unknown-section error are identical before and after, numbers masked.
- [x] The preferences list the sections by title, on a real window
  (screenshot looked at).
- [x] Falsified: a section with no builder, a builder with no section, a
  section with no icon, an icon with no glyph, and a builder that spells its
  own title each fail a test.
- [x] Full suite on Windows; Linux vet and `CGO_ENABLED=0` builds of
  `hayami-tui` for Windows and Linux; golangci-lint clean for both.

## Risks & Assumptions

- **The panic in `Specs()`** is a programming error that a test reaches
  before any user does. `Sources()` does not call `Specs()`, so a running
  program never takes that path.
- **`DefaultScan` is still a variable.** Removing it means passing an `Env`
  into the cobra command constructors, which the tests build with no way to
  reach inside them. Left for when that wiring changes.
- **The preferences' labels change** from keys to titles: the one visible
  difference.
- **Rollback**: revert the commit. No settings, cache or file format changes.

## Gaps found

- The parity test still does not compare what the two shells draw; the
  review's phase 4 owns that.
- Per-section settings (`Interfaces`, `LHM`) are still top-level config
  fields, now carried in `Env` instead of positional arguments; settings by
  section is phase 4.

## Verification

2026-10-08, Windows 11, Go 1.26.0, MSYS2 UCRT64 gcc:

- `go test -tags migrated_fynedo ./...`: every package ok but
  `internal/usage`'s Python cross-check, which fails on `main` on this
  machine too (its Python finds `ag-scripts` without `structlog`).
- `GOOS=linux CGO_ENABLED=0 go vet` on view, panel, config, cli, core,
  testenv and `cmd/hayami-tui`: clean. `CGO_ENABLED=0 go build
  ./cmd/hayami-tui` for windows and linux: ok.
- golangci-lint v2.12.2, `GOOS=windows` and `GOOS=linux`, over the changed
  packages, and with cgo over gui, prefs, panel and tests/structure: 0
  issues.
- Before (origin/main) and after builds of `hayami-tui`, run with the same
  throwaway settings (all four sections, one Wi-Fi interface), digits
  masked: `doctor` (11 lines), `readings` (195 lines), `--help` (28 lines)
  and `--sections nope` identical.
- Preferences, Sections page, from a throwaway settings file choosing
  bandwidth and cooler: "Bandwidth", "Cooler" ticked, then "Usage",
  "Peripherals".
- Falsifications: peripherals' builder removed → `panel: section
  "peripherals" has no builder`; a listed `probe` section with no icon →
  "the section draws a bare title"; with an unknown icon → "the window has no
  glyph for \"probe\""; a stray builder → "5 builders for 4 sections"; the
  cooler builder spelling `Title: "Coolers"` → the parity test's
  `expected: "Cooler"`. Each restored and passing.
