# 045 — Usage providers as a table

**Issue**: #145

## Status: COMPLETE

## Context

The usage section reads two providers, Claude and Codex, and what makes one
provider different from the other was written as branches wherever it
mattered. Before this spec (base `cac356c`):

| Where | What it branched on |
|---|---|
| `internal/usage/accounts.go:44` | `Label`: `CX` for Codex, `CC` for anything else |
| `internal/usage/accounts.go:95` | `Accounts`: Claude sorts before every other provider |
| `internal/usage/accounts.go:127`, `:136` | the pre-profile file is dropped for Claude only |
| `internal/usage/accounts.go:159` | `account`: a filename suffix `-codex` is Codex, anything else Claude |
| `internal/usage/accounts.go:179` | `Windows`: Codex's decoder, else Claude's |
| `internal/usage/path.go:75` | `Slug`: no suffix for Claude |
| `internal/panel/usage.go:233` | `fetcher`: the app-server for Codex, a credential store otherwise |
| `internal/panel/usage.go:248` | `decode`: the same decoder choice as `Windows`, a second time |
| `internal/panel/usage.go:280`, `:292` | `merge`: only Claude's credential stores add accounts |
| `internal/panel/usage.go:125` | the line for no account: "no Claude or Codex account" |

`internal/claude`, `internal/codex` and `internal/cli` carried no provider
branches: the first two are each one provider's fetch code, and `cli` reaches
usage only through the section.

The view is provider-agnostic already: it draws the label and badge the
source gives it (`view.UsageWindow.Account`, `.Badge`).

## Requirements

- R1 `usage.Spec` names a provider once: its cache name, its display name, the
  shorthand a meter label leads with, and its decoder. `usage.Providers()`
  is the table, default first; `usage.Lookup` finds one (an unknown name is
  read as the default, which is what a file with no recognised suffix has
  always been); `usage.Decode` decodes an account's payload by its provider;
  `usage.Displays` joins the display names for a sentence.
- R2 `usage.DefaultProvider` is the provider whose files carry no suffix.
  `Slug`, `account`, the pre-profile rule and the order of accounts read the
  table and that constant. The slug, the cache path, the lock, the gate and
  the file format are unchanged: they are shared with the Python tools.
- R3 The panel's `usageProviders` table says, per provider, what one poll finds
  on this machine (`providerState`): the accounts it announces before the
  cache has a file for them, their badges, and how one account is fetched (nil
  for read-only). `gather`, `refresh` and `merge` read it; `fetcher` and
  `decode` are gone. Credentials stay out of `internal/usage`, whose doc
  promises it opens no credential store.
- R4 No behaviour change, held by a file written from the code before the
  change.

## Acceptance Criteria

- [x] Pinned before the change (`1afdf1d`): `internal/panel/usage_pin_test.go`
  and `testdata/usage.golden` record every window and reason the section
  gives on six desks — nothing at all; cached Claude and Codex accounts with
  no store and no codex; a codex on the PATH; credential stores with plans
  (badges) and a closed gate with nothing behind it; the pre-profile file
  beside a profile; a payload that does not decode. Every gate is closed, so
  nothing is fetched and nothing reaches the network. Unchanged after it.
- [x] No provider name outside the two tables, the constants and one doc
  comment: `grep -rn 'ProviderCodex\|ProviderClaude'` finds nothing else in
  non-test code.
- [x] `doctor`'s usage line and `readings --sections usage` (30 lines of JSON)
  from a build of `1afdf1d` and one of this change, on a desk with a Claude
  account and no Codex, are identical with digits masked.
- [x] Falsified: `usage.Lookup` answering the default for every name fails the
  pin, `TestALabelLeadsWithTheProvider` and
  `TestAccountsAreFoundByListingTheCacheAndNothingElse`; badges read from the
  wrong provider's state fail the pin. Restored, both pass.
- [x] Adding a provider touches 3 places (below), from 8.

### Touch points to add a provider

Before: the constant; a shorthand constant and `Label`'s branch; `account`'s
suffix parse; `Windows`' decoder branch; the panel's `fetcher` branch; its
`decode` branch; `merge` (Claude only); the no-account sentence. **8.**

After: the constant; an entry in `usage`'s table; an entry in the panel's
`usageProviders`. **3**, beside the provider's own fetch and decode code. The
no-account sentence, the label, the order of accounts and the filename parse
follow from the table.

## Risks & Assumptions

- **The slug is the compatibility hinge** (CLAUDE.md). `Slug` compares against
  `DefaultProvider`, which is `ProviderClaude`: the same comparison by name.
  `TestTheSlugAgreesWithThePythonItself` is the guard and fails on this desk
  for the reason it fails on `main` (its Python lacks `structlog`); CI runs it.
- **Badges are now looked up by provider and name** (they were by name alone).
  A Claude store is never nameless (the default store is `max`), and a Codex
  account had no badge, so nothing a person sees changes; a Codex account with
  a profile name that matched a Claude profile would have borrowed that
  profile's plan letter before, and no longer does.
- **Rollback**: revert. No setting, cache file or format changes.

## Gaps found

- The usage pin found that `encoding/json`'s decode error names a Go type that
  differs between Go 1.26 and 1.27 for the same input (`claudePayload` and
  `claudeWindow`); the pin masks it.

## Verification

2026-10-07, Windows 11:

- `go test -tags migrated_fynedo ./...`: every package ok but `internal/usage`'s
  Python cross-check, which fails on `main` here too.
- The pin passes under Go 1.26.0 and 1.27.0.
- `GOOS=linux CGO_ENABLED=0 go vet` on usage, claude, codex, panel and cli;
  `CGO_ENABLED=0 go build ./cmd/hayami-tui` for windows and linux; golangci-lint
  v2.12.2 for windows and linux: clean.
