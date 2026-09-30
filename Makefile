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
	@found=$$(git ls-files -z | xargs -0 -r file --mime-type -- 2>/dev/null \
	    | grep -E 'application/x-(executable|sharedlib|pie-executable|archive)' \
	    | cut -d: -f1); \
	if [ -n "$$found" ]; then \
	    echo "Compiled binaries are tracked in this repository:" >&2; \
	    echo "$$found" | sed 's/^/  /' >&2; \
	    echo >&2; \
	    echo "Remove them with: git rm --cached <file>" >&2; \
	    echo "They are build output; .gitignore should already cover them." >&2; \
	    exit 1; \
	fi; \
	echo "No compiled binaries are tracked."

.PHONY: test
test: ## Run the tests with the race detector
	go test -race ./...

.PHONY: coverage
coverage: ## Run the tests and open a coverage report
	go test -coverprofile=coverage.out ./... && go tool cover -html=coverage.out

LDFLAGS=-X github.com/ushineko/hayami/internal/buildinfo.version=$(VERSION)

.PHONY: build
build: ## Build both panels for the host platform
	CGO_ENABLED=1 go build -trimpath -tags '$(FYNE_TAGS)' -ldflags='$(LDFLAGS)' -o hayami ./cmd/hayami
	CGO_ENABLED=0 go build -trimpath -ldflags='$(LDFLAGS)' -o hayami-tui ./cmd/hayami-tui

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

.PHONY: vuln
vuln: ## Scan for known vulnerabilities, before every tagged release
	go run golang.org/x/vuln/cmd/govulncheck@latest ./...

.PHONY: clean
clean: ## Remove build artifacts
	@rm -rf hayami hayami-tui dist/ coverage.out
