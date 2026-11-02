#!/usr/bin/env bash
#
# Rebuilds the network helper's eBPF object and fails if it is not the one
# committed (spec 081). The object is loaded with kernel capabilities and a
# reviewer cannot read it, so a machine compares it. Every input is pinned:
# LLVM's own release and libbpf's by hash, bpf2go by go.sum and generate.go,
# vmlinux.h by being committed.
#
# Set BPF_TOOLS to a directory to keep the download between runs.

set -euo pipefail

LLVM_VERSION=23.1.4
LLVM_SHA256=25d814fda50d5fb0084b42be09605d8b1595d49d5e2e03a3345d7e112ff6f9f3
LIBBPF_VERSION=1.7.0
LIBBPF_SHA256=7ab5feffbf78557f626f2e3e3204788528394494715a30fc2070fcddc2051b7b

# Where the headers are matters: their path changes the order of the object's
# debug information, by a few bytes. This is where a distribution puts them.
HEADERS=/usr/include/bpf

ME="$(basename "$0")"
die() { echo "$ME: $*" >&2; exit 1; }

cd "$(dirname "${BASH_SOURCE[0]}")/.."

fetch() {
    local url="$1" sum="$2" dest="$3"
    curl -fsSL --retry 3 -o "$dest" "$url"
    echo "$sum  $dest" | sha256sum --check --quiet - || die "$url is not the file this script pins"
}

[[ -z "$(git status --porcelain -- internal/netcount)" ]] \
    || die "internal/netcount has uncommitted changes; a comparison would mean nothing"

TOOLS="${BPF_TOOLS:-$(mktemp -d)}"
mkdir -p "$TOOLS"

if [[ ! -x "$TOOLS/llvm/bin/clang" ]]; then
    fetch "https://github.com/llvm/llvm-project/releases/download/llvmorg-$LLVM_VERSION/LLVM-$LLVM_VERSION-Linux-X64.tar.xz" \
        "$LLVM_SHA256" "$TOOLS/llvm.tar.xz"
    mkdir -p "$TOOLS/llvm"
    tar -xJf "$TOOLS/llvm.tar.xz" -C "$TOOLS/llvm" --strip-components=1 --wildcards \
        '*/bin/clang' '*/bin/clang-[0-9]*' '*/bin/llvm-strip' '*/bin/llvm-objcopy' '*/lib/clang/*'
    rm -f "$TOOLS/llvm.tar.xz"
fi

if [[ ! -f "$TOOLS/libbpf/src/bpf_helpers.h" ]]; then
    fetch "https://github.com/libbpf/libbpf/archive/refs/tags/v$LIBBPF_VERSION.tar.gz" \
        "$LIBBPF_SHA256" "$TOOLS/libbpf.tar.gz"
    mkdir -p "$TOOLS/libbpf"
    tar -xzf "$TOOLS/libbpf.tar.gz" -C "$TOOLS/libbpf" --strip-components=1
    rm -f "$TOOLS/libbpf.tar.gz"
fi

for header in bpf_core_read.h bpf_endian.h bpf_helper_defs.h bpf_helpers.h bpf_tracing.h; do
    if ! cmp -s "$TOOLS/libbpf/src/$header" "$HEADERS/$header"; then
        [[ -n "${CI:-}" ]] || die "$HEADERS/$header is not libbpf $LIBBPF_VERSION's"
        sudo install -D -m 0644 "$TOOLS/libbpf/src/$header" "$HEADERS/$header"
    fi
done

(
    cd internal/netcount
    BPF2GO_CC="$TOOLS/llvm/bin/clang" BPF2GO_STRIP="$TOOLS/llvm/bin/llvm-strip" go generate
)

if [[ -n "$(git status --porcelain -- internal/netcount)" ]]; then
    git status --short -- internal/netcount >&2
    die "the committed BPF object is not what its source builds: run make bpf with these tools, or find out why"
fi
echo "The committed BPF object is what its source builds."
