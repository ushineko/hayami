# Spec 101: credentials from the macOS Keychain

**Issue**: [#357](https://github.com/ushineko/hayami/issues/357)

## Status: COMPLETE

## Executive Summary

On macOS the usage section read "out of date" and never refreshed: recent
Claude Code keeps the default account's OAuth credentials in the login Keychain
(a generic password, service `Claude Code-credentials`), not in
`~/.claude/.credentials.json`, and this build read only the file. A credential
store is now either a file or, on macOS when the file is absent, the Keychain —
read and written through `/usr/bin/security`. Everything spec 004 built on top
of a store (the parse, the refresh, the write-back that keeps Claude Code's own
keys) is unchanged; only where the bytes come from is new. Reviewers should
start with `internal/claude/keychain.go` and the `write` split in
`internal/claude/refresh.go`.

## Context

Spec 004 made hayami fetch its own usage from a credential file and write a
refreshed token back to it. Claude Code since moved the default account's
credentials into the macOS login Keychain, so on a current macOS the file is
gone and the fetch has nothing to read. hayami is now the only reader of that
credential (the Python widget is retired), so the fix belongs here: reach the
Keychain the way the file was reached, and keep the one rule that governs this
package — a write puts back every key Claude Code had.

The Keychain is global, not under `HOME`. A test that took the credential file
away by pointing `HOME` at a temp dir still found the real Keychain item, so
`testenv.Home` now also points the keychain store at a service that does not
exist (`CLAUDE_USAGE_KEYCHAIN_SERVICE`), the same way it already redirects the
profile root.

## Requirements

- **R1** On macOS, when `~/.claude/.credentials.json` is absent, the default
  store is the login Keychain item `Claude Code-credentials` for the current
  user. When the file is present, nothing changes. Off macOS, nothing changes.
- **R2** A keychain store reads and parses exactly as a file store does, from
  the same `claudeAiOauth` object; a missing item reads as `fs.ErrNotExist`, so
  `writeBack`'s "store was removed" path behaves as it does for a file.
- **R3** A refresh writes the new token back into the keychain item in place
  (`security add-generic-password -U`), preserving every other key, as the file
  write does.
- **R4** No token is logged or printed: a write error carries only `security`'s
  stderr, never the secret.
- **R5** A test can take the keychain store away, so the suite reaches only the
  stores it lays down.

## Acceptance Criteria

- [x] AC1 On darwin with no credential file, `DefaultStore`/`Stores` return the
  keychain store; with the file present they return the file store; off darwin
  the file store, always. (R1)
- [x] AC2 A keychain store round-trips: a seeded item is read into the same
  tokens a file would give, and an unknown key survives a rewrite. (R2, R3)
  — `TestKeychainRoundTrip`, darwin, `HAYAMI_KEYCHAIN_E2E=1`.
- [x] AC3 A keychain store that is not there reads as `fs.ErrNotExist`. (R2)
  — `TestKeychainNotFoundIsErrNotExist`.
- [x] AC4 `encode` changes the three token fields and keeps every other key,
  inside `claudeAiOauth` and beside it. (R3) — `TestEncodePreservesUnknownKeys`.
- [x] AC5 A write failure's error does not contain the secret. (R4)
- [x] AC6 `testenv.Home` neutralises the keychain store, so the existing
  credential tests reach only their own files. (R5)

## Risks & Assumptions

- The Keychain secret is the same JSON the file held (top-level
  `claudeAiOauth`); confirmed by a live reading in the PR.
- The item's account is the login user, which is how Claude Code names it.
- hayami-tui is a different binary than Claude Code, so the first read and the
  first write prompt for Keychain access; "Always Allow" is one-time. A bounded
  timeout keeps an unanswered prompt from wedging a fetch.
- The refreshed token is briefly on the `security … -w` argv (visible to the
  same user via `ps`) during a write, which happens only when the access token
  is expired at fetch time — rare while Claude Code keeps the item fresh.
