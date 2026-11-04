# 083 — the plan comes from the reading

**Issue**: #288

## Status: INCOMPLETE

## Executive Summary

An Ultra account was still called "CC max" with the memory from spec 082,
because the memory can only keep a name a record has shown, and the shared
record may never show it. When the record names nothing for a store, a store
saying `max` whose own usage reading has a seven-day window and no five-hour
one is now called Ultra, and `claude-plans.json` is gone. Look first at
`observedPlan` in `internal/panel/usage.go`.

## Context

#235 called the default store by the plan `~/.claude.json` names, believed
only when the record's rate-limit tier is the store's. Spec 082 (#286) kept
that name once seen. On the desk nothing was ever seen: the record held the
enterprise account's tier and no plan name, and Claude Code itself drew "max"
at the top of a new session on the Ultra account. The account's plan was not
in doubt: its usage reading has no five-hour window, which is how #234 defines
Ultra ("Max without the five-hour window").

## Requirements

- R1 A record that vouches for a store and names a plan decides it.
- R2 Otherwise, a store with `subscriptionType: max` whose cached reading has
  a seven-day window and no five-hour window is Ultra.
- R3 Otherwise the store's own word stands. A reading that is absent, empty,
  unreadable, or has no seven-day window shows nothing.
- R4 Nothing is kept between polls: the label follows the reading both ways.
  `claude-plans.json` is neither written nor read.
- R5 As before: the badge follows the same plan for every store; only the
  default store is renamed, not to a name a profile or a cache file has; the
  cache file keeps its name.

## Acceptance Criteria

- [x] R1, R2, R3: `TestAStoresPlanComesFromTheRecordThenItsReading`,
      `TestAnEmptyReadingIsNoSignOfAPlan`, `TestAStoreWithNoCacheFileIsLookedUp`,
      `TestAWeekOnlyPayloadIsOneThatSaysSo`.
- [x] R2: a store saying `enterprise` with the same reading is not Ultra.
      (`TestOnlyAMaxStoreIsReadAsUltra`.)
- [x] R4: the five-hour window returning gives "CC max" with M.
      (`TestThePlanFollowsTheReadingBothWays`.)
- [x] R5: `TestAPlanNameAProfileHasIsNotTaken`, by either source of the plan;
      `TestAPlanNameACacheFileHasIsNotTaken`;
      `TestTheDefaultStoreIsCalledByItsRecordedPlan`, unchanged.
- [x] Each rule fails its test when broken: never firing, any store qualifying,
      the reading before the record, the seven-day or five-hour check dropped,
      the caption check dropped.
- [x] B3's adversarial review run; what survives filed, what is refuted under
      "Checked". (No exploit path; the caption collision fixed here, R5; the
      drawer's stale label filed as #289.)
- [ ] On the desk, after a build is installed: "CC ultra" with U while the
      record describes the enterprise account. (E2E 1.)

## Threat model delta

B3. The shared usage cache, which other programs write, now decides a label
and a badge. It already decided every number beside them. It chooses no token,
store or cache file: those are keyed by the store's name, and the rename is
on screen only. The file the panel wrote in spec 082 is gone. The document's
red-team log and "Checked" are updated in the same PR.

## Risks & Assumptions

- This is an inference from a shape. If Max loses its five-hour window, Max
  accounts are called Ultra until `observedPlan` changes. A record naming the
  plan still wins.
- A record that vouches for an Ultra store and names it "Max" wins (R1), so
  the panel says "CC max". The check is the tier alone, so another Max 20x
  account's record does this too (pre-existing, #235). If Claude Code begins
  to write "Max" for an Ultra account, R1 needs revisiting; the record on the
  desk has not been seen to.
- The panel labels a poll's windows before fetching, and the drawer works the
  name out afresh, so when a reading changes shape the drawer can ask for the
  old caption and report nothing in the cache until the next polls catch up:
  under a minute, rarely. Filed as #289.
- The first poll on a machine with no cache file draws "max" until a reading
  arrives, the same change of caption #235 already made when a record
  appeared.
- A left-over `claude-plans.json` from 0.11.16-rc.2 is read by nothing.
- Rollback: revert the commit.

## E2E Test Plan

1. On the desk, with this build and the record describing the enterprise
   account: the panel's Ultra account is drawn "CC ultra" with U, and
   `~/.cache/claude-usage-widget/usage-max.json` keeps its name. (R2, R5)

### Prod visibility

Not applicable: a desktop program with no production deployment.
