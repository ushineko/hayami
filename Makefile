default: help

.PHONY: help
help: ## Show this help
	@echo
	@echo "Available commands:"
	@echo
	@awk -F ':|##' '/^[^\t].+?:.*?##/ {printf "\033[36m%-30s\033[0m %s\n", $$1, $$NF}' $(MAKEFILE_LIST)

BINDIR=$(shell go env GOPATH)
MODULE=github.com/ushineko/hayami
VERSION=$(shell cat VERSION)

# A build that is not the release says so, as hotaru's does.
#
# VERSION is the version of record and names the release files, and stamping
# it into every binary meant a panel built from a working tree reported the
# same string as the installed one: the two side by side could not be told
# apart from their About pages.
#
# Released is HEAD sitting exactly on this version's tag with nothing
# modified. Anything else is `0.11.5-1a2b3c4-dev`, which names the commit it
# came from. The release job checks that what it ships says the plain version.
COMMIT=$(shell git rev-parse --short HEAD 2>/dev/null)
DIRTY=$(shell git status --porcelain 2>/dev/null | head -1)
ATTAG=$(shell git describe --exact-match --tags --match 'v$(VERSION)' HEAD 2>/dev/null)
BUILD_VERSION=$(if $(and $(ATTAG),$(if $(DIRTY),,x)),$(VERSION),$(VERSION)$(if $(COMMIT),-$(COMMIT))-dev)

# migrated_fynedo tells Fyne this front end has been through the fyne.Do
# migration, so it stops asking which goroutine it is on. Without it, Fyne
# answers that question with runtime.Stack -- a full traceback -- on every
# Canvas.Refresh. Profiled on a sibling program during a window drag: 52% of
# the process's CPU was printing tracebacks. Every UI mutation off the main
# goroutine here goes through fyne.Do, which is what the tag asserts.
# See fynedesygn docs/fyne-quirks.md, quirk 31.
#
# The terminal panel does not take it: it has no Fyne window and no main
# goroutine to be on the wrong side of.
FYNE_TAGS?=migrated_fynedo

LINT_NAME?=golangci-lint
LINT_VERSION?=v2.12.2
LINT_PROGRAM=$(LINT_NAME)-$(LINT_VERSION)

# Release asset coordinates for the pinned linter version.
# (The upstream install.sh is not used: its checksum extraction matches the
# .sbom.json asset line and fails verification on recent releases.)
# The checksums come from the same release as the tarball, so the checksums
# file is pinned here: a release replaced after this line was written fails.
# Dependabot does not track this; a new LINT_VERSION needs a new hash.
LINT_CHECKSUMS_SHA256=9accc7943a5b4be44416a7d4efa7efb3d18c7f1919d6581cc3536e185301a2d4
LINT_VERSION_NUM=$(LINT_VERSION:v%=%)
LINT_BASE_URL=https://github.com/golangci/golangci-lint/releases/download/$(LINT_VERSION)

.PHONY: install-lint
install-lint: $(BINDIR)/bin/$(LINT_PROGRAM) ## Install linter

$(BINDIR)/bin/$(LINT_PROGRAM):
	@echo "Setting up $(LINT_PROGRAM) ..."
	@set -e; \
	os=$$(uname -s | tr '[:upper:]' '[:lower:]'); \
	arch=$$(uname -m); \
	case "$$arch" in x86_64) arch=amd64;; aarch64|arm64) arch=arm64;; esac; \
	dist="$(LINT_NAME)-$(LINT_VERSION_NUM)-$$os-$$arch"; \
	tmp=$$(mktemp -d); trap 'rm -rf "$$tmp"' EXIT; \
	curl -fsSL "$(LINT_BASE_URL)/$$dist.tar.gz" -o "$$tmp/$$dist.tar.gz"; \
	curl -fsSL "$(LINT_BASE_URL)/$(LINT_NAME)-$(LINT_VERSION_NUM)-checksums.txt" -o "$$tmp/checksums.txt"; \
	sums=$$( (sha256sum "$$tmp/checksums.txt" 2>/dev/null || shasum -a 256 "$$tmp/checksums.txt") | awk '{print $$1}'); \
	if [ "$$sums" != "$(LINT_CHECKSUMS_SHA256)" ]; then echo "checksums file mismatch: want '$(LINT_CHECKSUMS_SHA256)' got '$$sums'"; exit 1; fi; \
	want=$$(awk -v f="$$dist.tar.gz" '$$2 == f {print $$1}' "$$tmp/checksums.txt"); \
	got=$$( (sha256sum "$$tmp/$$dist.tar.gz" 2>/dev/null || shasum -a 256 "$$tmp/$$dist.tar.gz") | awk '{print $$1}'); \
	if [ -z "$$want" ] || [ "$$want" != "$$got" ]; then echo "checksum mismatch for $$dist.tar.gz: want '$$want' got '$$got'"; exit 1; fi; \
	tar -C "$$tmp" -xzf "$$tmp/$$dist.tar.gz"; \
	mkdir -p "$(BINDIR)/bin"; \
	mv -v "$$tmp/$$dist/$(LINT_NAME)" "$(BINDIR)/bin/$(LINT_PROGRAM)"

.PHONY: setup
setup: install-lint ## Setup system for local development
	@echo "Make sure your system path includes GOPATH/bin. See README.md for details."

# golangci-lint type-checks against the standard library sources of whichever Go
# it finds, using a go/types built into the linter binary. A linter built with
# Go 1.26 panics outright ("file requires newer Go version go1.27") on a machine
# whose GOROOT is 1.27. go.mod deliberately carries no `toolchain` line, so the
# pin lives here instead, matching the Go that this linter release was built
# with. Bump it together with LINT_VERSION.
LINT_GO_TOOLCHAIN?=go1.26.0

.PHONY: lint
# The linter's cache records file paths relative to the checkout that filled
# it. Shared between git worktrees, it reports findings against files in a
# worktree that has since been removed. One cache per checkout, ignored.
lint: export GOTOOLCHAIN = $(LINT_GO_TOOLCHAIN)
lint: export GOLANGCI_LINT_CACHE = $(CURDIR)/.cache/golangci-lint
lint: install-lint ## Lint files
	@go version
	$(BINDIR)/bin/$(LINT_PROGRAM) run --timeout 5m0s --config config/.golangci-$(LINT_VERSION).yml ./...

# Headless: the window's tests run under Fyne's test driver, and the audio
# tests skip where no sound server is listening.
.PHONY: check-no-binaries
check-no-binaries: ## Fail if a compiled binary is tracked in the repository
	@command -v file >/dev/null || { echo "check-no-binaries needs file(1)" >&2; exit 1; }; \
	file -b internal/netcount/netcount_bpfel.o | grep -q eBPF \
	    || { echo "internal/netcount/netcount_bpfel.o is not an eBPF object" >&2; exit 1; }; \
	found=$$( { git ls-files -z | xargs -0 -r file --mime-type -- 2>/dev/null \
	    | grep -E 'application/(x-(executable|sharedlib|pie-executable|archive|object|dosexec|coff|mach-binary)|vnd\.microsoft\.portable-executable)' \
	    | cut -d: -f1; git ls-files '*.o' '*.a' '*.so' '*.syso'; } \
	    | sort -u | grep -vxF 'internal/netcount/netcount_bpfel.o'); \
	if [ -n "$$found" ]; then \
	    echo "Compiled binaries are tracked in this repository:" >&2; \
	    echo "$$found" | sed 's/^/  /' >&2; \
	    echo >&2; \
	    echo "Remove them with: git rm --cached <file>" >&2; \
	    echo "They are build output; .gitignore should already cover them." >&2; \
	    exit 1; \
	fi; \
	echo "No compiled binaries are tracked."

# The network helper's eBPF counter (spec 072): compiles
# internal/netcount/bpf/netcount.bpf.c into the object that is committed, so an
# ordinary build needs none of this. Runs in a pinned image, with pinned tools it
# downloads, so CI gets the same bytes (spec 081). Needs docker.
.PHONY: bpf
bpf: ## Rebuild the network helper's eBPF counter (docker; downloads LLVM)
	tools/bpf.sh build

.PHONY: bpf-check
bpf-check: ## Fail if the committed eBPF object is not what its source builds
	tools/bpf.sh check

# The program's types, from this machine's kernel. Committed, because the object
# depends on them; changing them changes the object. Needs bpftool.
.PHONY: bpf-vmlinux
bpf-vmlinux: ## Write vmlinux.h again from the running kernel (bpftool)
	bpftool btf dump file /sys/kernel/btf/vmlinux format c > internal/netcount/bpf/vmlinux.h

.PHONY: test
test: ## Run the tests with the race detector
	go test -race ./...

# Fuzz every parser that has a target (spec 074): each Fuzz function in each
# package under internal/ that has one, for FUZZTIME. A crash is written to
# that package's testdata/fuzz, where it stays as a regression test once the
# parser is fixed. CI runs this on every PR; run it longer by hand before a
# release: make fuzz FUZZTIME=10m.
FUZZTIME ?= 10s
# Workers per target. Go starts one per CPU by default, which on a desk
# takes every core for the length of the run; two, at the lowest priority,
# leaves the machine usable.
FUZZ_PARALLEL ?= 2
FUZZ_PKGS = $(sort $(dir $(shell grep -rl '^func Fuzz' --include='*_test.go' internal)))

.PHONY: fuzz
fuzz: ## Fuzz each parser for FUZZTIME (default 10s)
	@set -e; for pkg in $(FUZZ_PKGS); do \
		for t in $$(grep -h '^func Fuzz' $$pkg*_test.go | sed 's/func \(Fuzz[A-Za-z0-9]*\).*/\1/'); do \
			echo "== $$pkg$$t"; \
			nice -n 19 go test -run='^$$' -fuzz="^$$t\$$" -fuzztime=$(FUZZTIME) -parallel=$(FUZZ_PARALLEL) ./$$pkg; \
		done; \
	done

.PHONY: coverage
coverage: ## Run the tests and open a coverage report
	go test -coverprofile=coverage.out ./... && go tool cover -html=coverage.out

LDFLAGS=-X github.com/ushineko/hayami/internal/buildinfo.version=$(BUILD_VERSION)

.PHONY: build
build: ## Build both panels for the host platform
	CGO_ENABLED=1 go build -trimpath -tags '$(FYNE_TAGS)' -ldflags='$(LDFLAGS)' -o hayami ./cmd/hayami
	CGO_ENABLED=0 go build -trimpath -ldflags='$(LDFLAGS)' -o hayami-tui ./cmd/hayami-tui

# The README's gallery, photographed from this desk's live readings: the panel,
# the pane at 100 and 160 columns, and every page of the preferences window,
# into docs/img/gallery-*.png. Needs KDE/Wayland, kdotool, spectacle and
# alacritty, and python3 with Pillow. Leaves a running panel alone.
.PHONY: screenshots
screenshots: build ## Photograph the README gallery (KDE/Wayland, kdotool, spectacle, alacritty)
	tools/screenshot.sh --all

# The release tarball: one archive for linux-amd64 carrying both panels, the
# installer and what the installer puts on the system.
#
# **One target and not four.** The desktop panel is a Fyne window, so it needs
# CGO, OpenGL and the X11 and Wayland headers, and it links against the
# system's copies of them -- cross-compiling it means a cross toolchain for
# each target, which is a lot of machinery for a program whose only reported
# user runs it on the machine it was written on. The terminal panel would
# cross-build happily on its own; shipping it alone in a second archive would
# be a tarball that installs half the program.
#
# Built on the oldest runner we have, because a dynamically linked binary
# needs at least the glibc it was built against, and a newer one is a tarball
# that will not start on an older distribution.
.PHONY: release
release: build ## Package the binaries into dist/ as a tar.gz with SHA256SUMS
	@set -e; 	rm -rf dist; mkdir -p dist; 	base="hayami-$(VERSION)-linux-amd64"; 	stage="dist/$$base"; 	mkdir -p "$$stage/packaging"; 	cp hayami hayami-tui install.sh uninstall.sh README.md LICENSE "$$stage/"; 	cp packaging/io.ushineko.hayami.desktop packaging/hayami.svg packaging/60-sanshoku.rules "$$stage/packaging/"; 	tar -C dist -czf "dist/$$base.tar.gz" "$$base"; 	rm -rf "$$stage"; 	cd dist && (sha256sum *.tar.gz 2>/dev/null || shasum -a 256 *.tar.gz) > SHA256SUMS
	@ls -l dist/*.tar.gz dist/SHA256SUMS

# The Linux installers (spec 063): the one with the wizard and the one for a
# terminal, each a single program that carries both panels, built by fynstall
# from fynstall.yaml as the Windows ones are (scripts/installer_windows.ps1).
#
# The builder is run from the module, not from PATH: an installer is built
# against the fynstall go.mod requires, and a builder of another version
# would be a second one to keep in step.
#
# After `release` when both are wanted, because that target empties dist/.
FYNSTALL ?= go run github.com/ushineko/fynstall/cmd/fynstall

.PHONY: installer
installer: build ## Build the Linux installers into dist/ with fynstall
	@mkdir -p dist
	go run ./tools/appicon -size 512 -out dist/hayami.png
	$(FYNSTALL) build -c fynstall.yaml -target linux/amd64 -with-cli-only

.PHONY: vuln
vuln: ## Scan for known vulnerabilities, before every tagged release
	go run golang.org/x/vuln/cmd/govulncheck@latest ./...

.PHONY: clean
clean: ## Remove build artifacts
	@rm -rf hayami hayami-tui dist/ coverage.out
