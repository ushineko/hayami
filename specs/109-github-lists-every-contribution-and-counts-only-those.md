# 109 — GitHub lists every contribution, and counts only those

**Issue**: #386

## Status: COMPLETE

## Executive Summary

The GitHub reader pages through its pull requests, reviews and issues
instead of stopping at a hundred of each, lists the repositories created,
and counts a day as what it lists. The calendar and the restricted count are
no longer asked for, so an organisation's private contributions that this
sign-in cannot see neither inflate the strip nor appear as "N private
contributions not listed". Review the paging loop in `GitHubReader.Read`
first.

## Context

On the desk (2026-11-22) every GitHub day page ended with "52 private
contributions not listed", and the person asked what it meant. It was
`restrictedContributionsCount`: the fortnight's contributions in private
repositories this sign-in cannot see, repeated on every day's page. All 52
were in an organisation's repositories; every contribution GitHub lists is
in the person's own. Asked, the person said they do not want the
organisation's: only their own, private ones included. Their own private
repositories are visible to gh's `repo` scope and listed like any other.

The day counts showed a second gap. Of 800 counted, 695 were listed: 495
commits, 100 pull requests and 100 issues. The fortnight had 139 pull
requests and 112 issues, and the query asked for the first hundred of each.
The rest were two repositories created, not asked for, and the 52.

## Requirements

- R1 Pull requests, reviews and issues are read to the end of their lists,
  a hundred a request; each request after the first asks only for the lists
  with a page left, after their cursors, and not again for the commits. At
  most 10 requests a reading.
- R2 The repositories created in the fortnight are items ("created").
- R3 A day's count is the sum of what it lists. The calendar and
  `restrictedContributionsCount` are not asked for; nothing says a count of
  contributions this sign-in cannot see.
- R4 An item counting less than one is dropped, so no answer makes a day
  negative.

## Acceptance Criteria

- [x] `TestGitHubReadsAFortnightInOneQuery`: days counted from items (a day
      the calendar counts and nothing lists is 0), the repository created
      listed, the query without the calendar or restricted count.
- [x] `TestGitHubReadsEveryPage`: three pages of pull requests and two of
      issues read in three requests; later requests skip the commits and the
      finished lists.
- [x] `TestADayIsItsRepositories` says nothing of private contributions.
- [x] Falsified: stopping after the first request, asking for every list
      every time, and counting days from nothing each fail a test.
- [x] `FuzzGitHubResponse` checks no day is negative, seeded with a paged
      answer; 30 s clean.
- [x] Against the real GitHub (`HAYAMI_DEV_E2E=1`, and a probe printing only
      totals): 748 counted and listed, 495 commits, 139 pull requests, 112
      issues, 2 created; 800 less the organisation's 52.
- [x] `make test`, `make lint`.
- [x] Changelog line under `### Unreleased`.

## Risks & Assumptions

- A busy fortnight is now more than one request: up to 10, each the size of
  the one before. GitHub's GraphQL limit is points per hour; a reading is
  once per refresh interval.
- A person whose sign-in can see an organisation's repositories will have
  that work listed and counted like their own; this change removes only what
  GitHub counts and will not name.
- Rollback: revert the PR.

## Threat model delta

B4 (what hayami parses) and B5 (what it does outside itself): a GitHub
reading is now up to 10 requests, bounded however many pages a host claims,
and a day's count comes from items, so a negative count is dropped. Both
sections of `docs/threat-model.md` updated.

## E2E Test Plan

The installed panel on the next release: the Development drawer's GitHub
strip and a day page with no "private contributions" line, and the
fortnight's pull requests on their days (21 November: the release PRs).

### Prod visibility

Not applicable: a desktop program with no telemetry.
