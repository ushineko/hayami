# 080 — the supply-chain checks say what they check

**Issue**: #269, #270, #271, #272

## Status: INCOMPLETE

## Context

The adversarial review of #264 (`docs/threat-model.md`, B7) tried spec 075's
checks and found each narrower than its description: a grep that a comment
could satisfy, a command that did not name a workflow or a tag, a binary check
that passed objects, and a dependency flow with no delay.

## Requirements

- R1 The README's `gh attestation verify` names the workflow and the tag, and
  B7 says what it does and does not check (#269).
- R2 The opt-in `winget` install's lack of a pin is recorded, with the reason
  (#269).
- R3 The pin check reads the workflows as YAML, covers `.yml`, `.yaml` and local
  composite actions, requires a digest for `docker://`, and runs in a required
  check (#270).
- R4 B7 records the unpinned inputs behind pinned Actions (#270).
- R5 `make check-no-binaries` fails on tracked objects, `.syso` files and
  Windows programs, with `netcount_bpfel.o` the one exception (#271).
- R6 B7 records what the BPF object's checks do not catch (#271).
- R7 Dependabot waits a week before proposing a version (#272).
- R8 The linter's checksums file is pinned in the `Makefile`, independently of
  the release it came from (#272).
- R9 Repository settings, applied after this merges and after spec 079's:
  `sha_pinning_required`, the secrets check and the BPF check required,
  Dependabot alerts and security updates on.

## Acceptance Criteria

- [x] R1: the README's command names `--signer-workflow` and `--source-ref`;
      both were tried on v0.11.14-rc.2's `SHA256SUMS`, passing for the right
      values and failing for another workflow, tag or branch.
- [x] R2, R4, R6: B2 and B7 record these.
- [x] R3: `TestEveryActionIsPinned` passes; `TestAnUnpinnedActionIsFound` finds
      each form the old check passed, non-string keys, a second document, local
      actions outside `.github`, and container images by tag.
- [x] R5: `make check-no-binaries` passes this tree and fails a planted `.syso`,
      a missing `file(1)`, and an ordinary ELF at the exempt path.
- [x] R7: `cooldown` is set for both ecosystems.
- [x] R8: `make install-lint` installs with the pinned hash and fails with
      another.
- [ ] R9: the settings are on, and a Dependabot PR and this repository's
      workflows still run with `sha_pinning_required` true.

## Threat model delta

Boundary B7, and B2 for the winget line. A contributor or upstream can no
longer satisfy the pin check with a comment or another YAML form, or add native
code with no source without failing a required check. A person checking a
download checks the workflow and the tag as well. What is unchanged: a binary
reviewer compares nothing automatically, some inputs stay unpinned behind pinned
Actions, and a pin's commit is not checked to belong to its repository. `docs/threat-model.md` is updated in this PR. The PR gets the
document's adversarial review.

## Risks & Assumptions

- `sha_pinning_required` may fail a run if an Action's nested Action is pinned
  by tag. It is applied last and turned off the same way if so.
- A week's cooldown delays a fix that is not reported as a security update.
  Accepted.
- A new `LINT_VERSION` needs a new hash; `make install-lint` says so.
- Both structure tests guard against a mistake, not an attacker who can change
  the workflow; `sha_pinning_required` is the enforcement for pins.
- Byte tricks that hide a binary from `file(1)` stay open: Go ignores such a file
  without an embed and a loader, which a reviewer can read.
- Rebuilding the BPF object in CI is left out: it needs a pinned clang and a
  committed `vmlinux.h`. It reproduces byte for byte locally, so a reviewer's
  `make bpf` is a meaningful comparison.
- Rollback: revert the commit, and turn the settings off.

## Verification

Tests as above. The check moved from `security.yml` into `go test`, which runs
in `Test and lint`, already required.

## E2E Test Plan

1. This PR's own checks pass, with the removed job gone. (R3, R5)
2. After the settings: a Dependabot PR runs its checks. (R7, R9)
3. The next pre-release's files with the README's command, exit 0. (R1)

### Prod visibility

Not applicable: repository checks, with no service to observe.
