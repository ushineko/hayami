# 081 — the BPF object is what its source builds

**Issue**: #276

## Status: INCOMPLETE

## Context

The network helper loads `internal/netcount/netcount_bpfel.o` with `CAP_BPF`,
`CAP_NET_ADMIN` and `CAP_PERFMON`. The object is committed so a build needs no
clang (spec 072), and a reviewer cannot read it. The checks so far catch a
mistake: a test hashes the source, and a job warns when the object changes
alone. An object built from other source passes both.

Spec 072 expected clang's output not to be reproducible. It is, given the same
inputs: LLVM's official 23.1.4 release, a machine's own Arch build of it, in any
build directory, gave the same bytes as the committed object.

## Requirements

- R1 `tools/bpf-check.sh` rebuilds the object and fails if `internal/netcount`
  differs from what is committed.
- R2 Its inputs are pinned: LLVM's release and libbpf's headers by SHA-256,
  `bpf2go` by version, and `vmlinux.h` by being committed. It downloads nothing
  it has not checked.
- R3 A job runs it on every PR and on `main`, and it is required.
- R4 `make bpf` no longer depends on the running kernel. `make bpf-vmlinux`
  writes the header again when that is wanted.
- R5 B7 records the check and what it trusts.

## Acceptance Criteria

- [ ] R1, R3: the job passes on this PR's tree.
- [ ] R1: on a throwaway draft PR, an object with one byte changed fails the job
      and raises the existing warning (which also closes spec 075's last
      criterion).
- [ ] R1: on that PR, an object changed along with the `.c` and `source_hash.go`
      fails the job, where the older checks are silent.
- [x] R2: the script's hashes match LLVM's published digest and the libbpf tag;
      `make bpf-check` passes on a desk with those tools.
- [x] R4: `generate.go` calls no `bpftool`; a clean rebuild leaves the tree
      unchanged.
- [x] R5: B7 records it.
- [ ] R3: the job is a required check.

## Threat model delta

Boundary B7, for what reaches boundary B1's helper. Before: a PR could replace
the object with one built from other source, and only a reviewer running
`make bpf` would see. After: that fails a required check. An attacker has to
change the script's pins or `vmlinux.h` too, which shows in the diff.
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
- `make bpf` with other tools gives an object CI refuses, which is the point, and
  the message says so.
- The headers' path changes a few bytes, so CI installs them where a distribution
  does.
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
