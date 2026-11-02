# 081 — the BPF object is what its source builds

**Issue**: #276

## Status: INCOMPLETE

## Context

The network helper loads `internal/netcount/netcount_bpfel.o` with `CAP_BPF`,
`CAP_NET_ADMIN` and `CAP_PERFMON`. The object is committed so a build needs no
clang (spec 072), and a reviewer cannot read it. The checks so far catch a
mistake: a test hashes the source, and a job warns when the object changes
alone. An object built from other source passes both.

Spec 072 expected clang's output not to be reproducible. Given the same inputs
it nearly is. LLVM's official 23.1.4 release and a machine's own Arch build of it
gave the object committed so far, on that machine. On `ubuntu-latest` the same
release gave different bytes, in `.BTF` only: the same size, with type names in
another order, and every other section identical. A `golang:1.27.3-bookworm`
container on the desk gave the same bytes as `ubuntu-latest`, twice. So the build
runs in a pinned image, on a desk and in CI, and the object it gives is committed.

## Requirements

- R1 `tools/bpf.sh check` rebuilds the object and fails if `internal/netcount`
  differs from what is committed. `make bpf` builds it the same way.
- R2 Its inputs are pinned: the Go image by digest, LLVM's release and libbpf's
  headers by SHA-256, `bpf2go` by version, and `vmlinux.h` by being committed. It
  downloads nothing it has not checked.
- R3 A job runs it on every PR and on `main`, and it is required.
- R4 `make bpf` no longer depends on the running kernel. `make bpf-vmlinux`
  writes the header again when that is wanted.
- R5 B7 records the check and what it trusts.
- R6 A release waits for the check: the job is in the workflow a tag runs, and
  `release` and the build jobs need it.
- R7 The build names `generate.go`, removes what it writes first, requires it
  written, and compares bytes against the committed blobs, so a stray directive,
  a build tag or a `.gitattributes` cannot make it pass.
- R8 `vmlinux.h` may hold only what bpftool writes, line by line, and `bpf/` only
  that and the source. `generate.go`'s directives, and what the source includes, are
  held by tests. Only `Makefile` is a makefile. All enforced in the required tests.

## Acceptance Criteria

- [x] R1, R3: the job passes on this PR's tree, on `ubuntu-latest`.
- [x] R1: on throwaway draft PR #282, an object with one byte changed raised the
      existing warning, which closes spec 075's last criterion.
- [x] R1: on that PR, with the `.c` and `source_hash.go` also changed from `main`, the
      warning stayed silent and the job failed: `netcount_bpfel.o differ: byte 9001`.
- [x] R2: the script's hashes match LLVM's published digest and the libbpf tag;
      `make bpf-check` passes on a desk.
- [x] The new object differs from the old in `.BTF` only, and the two `.BTF`s hold
      the same 1,052 types: 153 forward declarations have swapped IDs.
- [x] R4: `generate.go` calls no `bpftool`; a clean rebuild leaves the tree
      unchanged.
- [x] R5: B7 records it.
- [ ] R3: the job is a required check.
- [x] R6: `TestTheReleaseIsBuiltApartFromWhatPublishesIt` fails a release that
      does not need the job.
- [x] R7: on a clean tree `tools/bpf.sh check` passes; planted cases are on the
      throwaway PR.
- [x] R8: `TestVmlinuxHoldsOnlyTypes` passes, and `TestVmlinuxFaultsAreFound` finds
      each evasion the refutation pass tried: a comment before a directive, a
      digraph, a bare carriage return, a byte order mark, `_Pragma`, a brace on the
      next line, a section attribute, and others.

## Threat model delta

Boundary B7, for what reaches boundary B1's helper. Before: a PR could replace
the object with one built from other source, and only a reviewer running
`make bpf` would see. After: that fails a check a release waits for. An attacker has
to change the script, `generate.go`, the job or the tests too, which shows in the
diff; `vmlinux.h` can change only as types, and Go code can still swap the bytes at
load time. B7 records each.
`docs/threat-model.md` B7 is updated in this PR. The PR gets the document's
adversarial review.

## Risks & Assumptions

- `vmlinux.h` adds 3.4 MB of text to the repository, generated from a kernel's
  BTF. It has type definitions only, and no name from the machine it came from.
  Other projects commit theirs, but the kernel's licence is GPL-2.0 and this
  repository's is MIT; whether a generated header of types needs a note was not
  settled here.
- The job downloads 2 GB on every run, for about a minute. It is not cached, so
  a cache has nothing to poison.
- A new LLVM or libbpf means a new hash and, likely, a new object: `make bpf`
  with those tools, committed together.
- The headers' path changes a few bytes, so the image mounts them where a
  distribution puts them.
- The object's bytes change once, in `.BTF` only. The kernel's verifier was not
  run against the new object here: its programs and relocations are unchanged.
- A build now needs docker and pulls an image from Docker Hub; only `make bpf`
  does, not an ordinary build. Dependabot does not track the digest.
- The GitHub source archive's hash is assumed stable. If it changed the job fails
  closed.
- Rollback: revert the commit, and drop the required check first.

## E2E Test Plan

1. This PR's job passes on `ubuntu-latest`. (R1, R3)
2. Throwaway draft PR, one byte changed in the object: the job fails, and
   `The BPF object changes with its source` warns. (R1, spec 075)
3. Same PR, a comment added to the `.c`, `source_hash.go` updated, the object
   byte still changed: the job fails, and the warning stays silent. (R1)

### Prod visibility

Not applicable: a repository check, with no service to observe. The helper's
behaviour does not change: the object's bytes do not.
