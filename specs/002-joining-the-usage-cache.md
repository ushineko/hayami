# Spec 002: joining the usage cache

**Issue**: [#4](https://github.com/ushineko/hayami/issues/4)

## Status: COMPLETE

## Executive Summary

`internal/usage` is a full participant in the cache the Python tools share:
the same directory, the same filenames, the same gate, the same non-blocking
lock and the same rule that a failed fetch never clobbers good data. The
payload stays opaque. Two tests are worth more than the rest — one runs the
Python's own `_slug` and compares, and one reads the real cache files on this
machine and skips where there are none. Reviewers should start with
`internal/usage/cached.go`, whose five steps are the protocol.

## Context

The usage section needs a cache before it needs a provider. Two Python
programs already share one, deliberately: both carry a copy of
`usage_cache.py`, and its docstring says they "resolve the identical path and
genuinely share the cache" so that the widget, every `--tui` pane and any
one-shot `--line` call together make about one API request per account per
window. During the port all three run at once, and a hayami with a cache of
its own would double every call to Anthropic and to the Codex app server.

The decision to share is recorded in `.claude/CLAUDE.md`. What changes later is
the location, not the arrangement: when the Python tools are decommissioned the
files may move to a directory named for this program, carrying the format, the
lock and the gate with them.

This spec is the protocol and nothing else. No OAuth, no Codex, no section on
screen. Those are spec 003, and separating them is deliberate: the credential
half writes to `~/.claude-credentials`, which is the one thing in this project
that can damage something outside it, and it should not also be the code that
debugs a lock.

**hayami is a writer.** On a machine that never had the Python, nothing else
will ever create these files, so reading is not enough.

### The protocol, read out of the Python

| | |
|---|---|
| Directory | `${XDG_CACHE_HOME:-~/.cache}/claude-usage-widget`, `~/Library/Caches/claude-usage-widget` on macOS, `%LOCALAPPDATA%\claude-usage-widget\cache` on Windows |
| File | `usage<-provider><-account>.json`, with a sibling `.lock` |
| Slug | every character that is not alphanumeric, `-` or `_` becomes `_`; the provider suffix is empty for `claude`; an empty account has no suffix at all |
| Entry | `{"next_attempt_at": float, "fetched_at": float or null, "data": {...}}` |
| Gate | `now < next_attempt_at` means read `data` and make no call |
| Lock | non-blocking `flock`; a caller that loses the race reads what is there rather than queueing |
| Success | write `next_attempt_at = now + ttl`, `fetched_at = now`, the new data |
| Failure | keep the last-known-good `data` and its `fetched_at`; push the gate by `max(ttl, retry_after)` |
| Write | to `<file>.<pid>.tmp`, then rename |

Two details are worth recording because they look like bugs and are not.
`fetched_at` is null when a fetch has never succeeded, so it is an optional
number and not a zero. And the lock is advisory in the weak sense: where
`flock` is unavailable the Python yields "acquired" and lets the gate
coordinate alone, which is the behaviour to copy rather than to improve.

One detail *does* look like a discrepancy. The Python's `force` is documented
as skipping the freshness gate but not a backoff gate; the code applies it to
both, because they are the same gate. This spec copies the code, not the
docstring: the file is shared and being faithful matters more than being right.

## Requirements

- R1 `internal/usage` resolves the same directory and the same filenames as
  the Python, on every platform it names.
- R2 An entry is read and written with `data` opaque. This build does not
  decode a payload it has no section for, and a field it does not know
  survives a write.
- R3 The write is atomic: a temporary file named for the process, then a
  rename.
- R4 A non-blocking lock, released on return and on death. A caller that does
  not get it reads what is there.
- R5 The gate decides: inside it, no fetch. Outside it, one caller fetches and
  every other reader gets the result.
- R6 A failed fetch keeps the last-known-good payload and its timestamp, and
  pushes the gate by the longer of the ttl and any retry-after.
- R7 A test reads the real cache files when they are present and skips when
  they are not, so a change to the format is reported rather than discovered.

## Acceptance Criteria

- [x] AC1 `TestTheSlugNamesTheSameFileThePythonNames` pins nine cases, and `TestTheSlugAgreesWithThePythonItself` runs the Python's own `_slug` over the same nine and compares, skipping where `usage_cache.py` is not checked out. (R1)
- [x] AC2 An entry round-trips with an unknown field intact, and `fetched_at` distinguishes null from zero. (R2)
- [x] AC3 No `.tmp` file is left behind after a write, and a reader never sees a partial file. (R3)
- [x] AC4 Two callers, one lock: the second reads the cache rather than fetching. (R4, R5)
- [x] AC5 Inside the gate, the fetch function is not called at all. (R5)
- [x] AC6 A fetch that fails leaves the previous payload and its `fetched_at` untouched and moves the gate. (R6)
- [x] AC7 `TestTheRealCacheFilesOnThisMachineStillParse` read the four files this machine holds — `usage-max`, `usage-work`, `usage-codex` and the pre-account `usage` — and skips where a machine has none. (R7)

## Gaps found

None. The library needed no change for this spec.

## Risks & Assumptions

- **Getting the slug wrong is silent.** Nothing errors; hayami writes a file
  nobody reads and both programs poll on their own. AC1 is the only thing
  standing between that and a user noticing their rate limit.
- **The lock is not a mutex.** It coordinates a stampede at cold start and
  nothing more; the gate is what actually stops the second call.
- **A shared file is a contract with code we do not own.** AC7 is the canary,
  and it can only fail on a machine that has the Python installed.
- **A mutation that changes nothing proves nothing.** Removing `_` from the
  set of characters the slug keeps left every test passing, because an
  underscore maps to an underscore either way. The slug is instead pinned by
  two mutations that do change the output: always suffixing the provider, and
  replacing with a hyphen. Both fail the test.
- Rollback: revert. Nothing reads this package yet.

## Alternatives Considered

- hayami's own cache from the start. Rejected: during the port all three
  programs run, and separate caches multiply the upstream calls the shared one
  exists to prevent.
- Decoding `data` into a typed payload here. Deferred to spec 003: this
  package's job is the protocol, and a payload it cannot decode should still
  round-trip through it.
