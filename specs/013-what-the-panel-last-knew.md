# 013 — What the panel last knew

**Issue**: #42

## Context

A section is blank until its first poll lands. Where that poll misses, it stays
blank for a whole interval — fifteen seconds for the peripherals, longer if the
device keeps missing — and a wireless mouse that has been still answers nothing
about one poll in fourteen, measured on the receiver here. So it is the
ordinary case and not an edge.

Startup itself is not slow, and it is worth being exact about that before
adding anything: the window appears in two seconds and all four sources poll in
about one second between them. What is visible is the gap after a *missed*
first poll, not a slow one.

The program already has half of what is needed. `Section.Gone` and
`Cell.Stale` mean "this is the last thing heard, not the current thing", and
both shells draw them dim. What is missing is that nothing survives the process
exiting.

The archetype keeps `last_info` per slot in memory only and starts blank, so
this is new behaviour rather than parity. It does persist usage, under
`${XDG_CACHE_HOME}/claude-usage-widget`, which this program already reads — so
the shape of an XDG cache is settled here, just not for readings.

## Requirements

### The last reading of every section is kept

A cache at `${XDG_CACHE_HOME:-~/.cache}/hayami/sections.json`: the rendered
`view.Section` for each key, with the time it was taken.

The rendered section and not the source's own state, because a section is
already plain data that both shells know how to draw, and because it keeps the
sources out of it — a source gains nothing and has to know nothing.

Every section, not only the peripherals. A rate or a coolant temperature from
the last run is less useful than a battery, but the rule "a section shows what
it last knew until it knows better" is one rule, and a panel with three
sections filled and one blank is harder to read than one that is wholly filled.

### It is restored when the first poll comes back empty

`Start` reads the cache before the window is built. A section whose first live
poll reported nothing is drawn from the cache instead, marked restored; a
section that polled successfully ignores its cached entry entirely and draws
what it just read.

**Before the cards are built**, which matters for more than the first frame. A
card is built once with the pieces its section has and the library takes its
objects at build time, so a section that was empty at build time had nowhere to
put a reading later — that is issue #40, and the grid is now always built to
work around it. Giving the card the cached shape addresses the same fragility
at its source: the card is built the size of what the panel last knew.

### A restored reading is drawn dim, and says so by being dim

`view.Section.Restored` marks it. Both shells draw a restored section the way
they draw `Gone` — the window through the card's stale state, the pane through
its dim colour — because the meaning is the same: this is the last reading, not
the current one.

It is a separate field from `Gone` rather than a reuse, because the pane
appends "(unavailable)" to a `Gone` section's title and that is the wrong thing
to say about a panel that has only just started. A restored section says
nothing extra; being dim is the whole of the message, and it stops being dim as
soon as a live poll lands.

### Nothing older than a day is restored

A battery moves over hours, so the previous evening's reading is still roughly
true and better than a blank. A week later it is furniture, and a device that
has since been sold would have a cell of its own.

Twenty-four hours, checked per entry against the time it was written, so a
panel restarted after a weekend starts blank rather than lying.

This is not `PeripheralsForget`, which is a different question — how long a
device that has gone quiet *within a session* keeps its cell — and stays at ten
minutes.

### Writing it costs nothing noticeable

The cache is written at most once every fifteen seconds, coalesced across every
section into one file, and written atomically: a temporary file in the same
directory and a rename, so a panel killed mid-write leaves the previous cache
rather than half of one.

Not on every poll. The bandwidth section polls every two seconds and a glance
panel has no business writing to disk thirty times a minute.

A cache that cannot be read — absent, truncated, from a future version — is a
cold start and not an error. It is a cache; the panel works without it and must
never fail to start because of it.

## Acceptance criteria

- [x] A section whose first poll returns nothing is drawn from the cache,
      marked restored, and is dim in both shells.
- [x] A section whose first poll succeeds ignores its cached entry.
- [x] A restored section stops being dim when a live poll lands.
- [x] A card is built with the cached section's shape, so a restored cell has
      somewhere to go without relying on `CellSlack`.
- [x] An entry older than 24 hours is not restored.
- [x] The pane does not say "(unavailable)" for a restored section; that stays
      for `Gone`.
- [x] The cache is written at most once every 15 seconds, atomically, and a
      partial or absent file starts cold without an error.
- [x] The panel starts with the peripherals filled after a restart taken while
      the mouse is asleep, verified in a photograph.

      *Simulated, not caught in the wild.* A sleeping mouse cannot be
      arranged on demand, so it was forced: a throwaway build whose first
      three peripherals polls report nothing, reverted immediately and not on
      this branch. The photograph shows the card present seven seconds in,
      "75 %" grey rather than green, and no "(unavailable)".
- [x] `go test ./...` passes; `cmd/hayami-tui` still builds with
      `CGO_ENABLED=0`.

## Gaps found

- **A card could not dim without calling itself unavailable.**
  `glance.Card.SetStale` dimmed the rows *and* showed `GoneMarker`, so a panel
  two seconds old announced its hardware was missing. Fixed upstream:
  `SetLastKnown` is the dimming without the word (fynedesygn #122, PR #123,
  released as v0.1.62).
- **Three faults in this spec'"'"'s own design, each found in a photograph and
  none by a test.** The write replaced the file with only the sections that
  reported *this* run, dropping the entry for the very section the cache
  exists for. `pollOne` polls immediately rather than after its first tick, so
  the restored card was hidden a second later. And a restored section counted
  as having been heard, which disabled the fallback on the next empty poll.
  Each now has a test that fails without its fix; what none of the original
  tests did was run the program.

## Risks & Assumptions

- **A restored reading is a claim about the past presented in the present.**
  Dim is the whole of what distinguishes them, and a reader who misses that
  reads a stale battery as current. The 24-hour bound and the live poll
  replacing it within one interval are what keep the window small.
- **The cache is a new file in the user's cache directory.** Nothing else reads
  it and deleting it costs one cold start, which is the property a cache should
  have.
- **Writing every fifteen seconds is a guess**, not a measurement. It is one
  small file and the interval is longer than every poll but the bandwidth's; if
  it shows up in anything it should be lowered to on-change-only.
- **Rollback**: revert the commit and delete the file. No setting changes
  meaning, and a panel that has never seen the cache behaves exactly as it does
  today.

## Status: COMPLETE
