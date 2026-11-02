#!/usr/bin/env bash
#
# Builds the network helper's eBPF object, and checks the committed one is what
# that build gives (spec 081). The object is loaded with kernel capabilities and
# a reviewer cannot read it, so a machine compares it.
#
#   tools/bpf.sh build    write the object into the tree
#   tools/bpf.sh check    fail if the committed object is not what build writes
#
# Every input is pinned: the Go image by digest, LLVM's release and libbpf's by
# SHA-256, bpf2go by generate.go, vmlinux.h by being committed. It runs in the
# image because the same tools outside it give other bytes on another machine:
# the same program, with its type names in another order.
#
# Needs docker. Set BPF_TOOLS to a directory to keep the download between runs.

set -euo pipefail

IMAGE=golang@sha256:d9b6a1beb9474adc2a1ce842573639b44b2a0dd49b6669f51d3821baf9ec37c9 # 1.27.3-bookworm
LLVM_VERSION=23.1.4
LLVM_SHA256=25d814fda50d5fb0084b42be09605d8b1595d49d5e2e03a3345d7e112ff6f9f3
LIBBPF_VERSION=1.7.0
LIBBPF_SHA256=7ab5feffbf78557f626f2e3e3204788528394494715a30fc2070fcddc2051b7b

ME="$(basename "$0")"
die() { echo "$ME: $*" >&2; exit 1; }

MODE="${1:-}"
[[ "$MODE" == build || "$MODE" == check ]] || die "usage: $ME build|check"

cd "$(dirname "${BASH_SOURCE[0]}")/.."
REPO="$PWD"

fetch() {
    local url="$1" sum="$2" dest="$3"
    curl -fsSL --retry 3 -o "$dest" "$url"
    echo "$sum  $dest" | sha256sum --check --quiet - || die "$url is not the file this script pins"
}

if [[ "$MODE" == check && -n "$(git status --porcelain -- internal/netcount)" ]]; then
    die "internal/netcount has uncommitted changes; a comparison would mean nothing"
fi

TOOLS="${BPF_TOOLS:-$(mktemp -d)}"
mkdir -p "$TOOLS"
TOOLS="$(cd "$TOOLS" && pwd)"

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

docker run --rm --user "$(id -u):$(id -g)" \
    -v "$REPO:/src" \
    -v "$TOOLS/llvm:/tools/llvm:ro" \
    -v "$TOOLS/libbpf/src:/usr/include/bpf:ro" \
    -e HOME=/tmp -e GOCACHE=/tmp/cache -e GOPATH=/tmp/go -e GOTOOLCHAIN=local \
    -e BPF2GO_CC=/tools/llvm/bin/clang -e BPF2GO_STRIP=/tools/llvm/bin/llvm-strip \
    -w /src/internal/netcount "$IMAGE" go generate

[[ "$MODE" == check ]] || exit 0

if [[ -n "$(git status --porcelain -- internal/netcount)" ]]; then
    git status --short -- internal/netcount >&2
    die "the committed BPF object is not what its source builds: run make bpf, or find out why"
fi
echo "The committed BPF object is what its source builds."
