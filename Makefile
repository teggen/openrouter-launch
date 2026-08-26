# openrouter-launch — the single source of truth for every check.
#
# CI (.github/workflows/ci.yml) invokes these targets rather than re-spelling
# the commands in YAML, so "green locally" and "green in CI" are the same
# claim. The target names clean/fmt/fmt-check/vet/lint/security/test/
# test-unit/pre-commit are a contract with the owner's /quality command —
# makefile_test.go pins them.

BINARY      := orl
PKG         := github.com/teggen/openrouter-launch
VERSION_PKG := $(PKG)/internal/version

# Pinned tool versions. `make tools` installs them with the LOCAL Go
# toolchain on purpose: prebuilt analysis binaries break on toolchain skew
# (every one on the author's machine died with "file requires newer Go
# version go1.26 (application built with go1.25)"). Bump here, in one place;
# the workflows assert they pin the same strings.
GOLANGCI_VERSION    := v2.12.2
GORELEASER_VERSION  := v2.17.1
# Pinned, unlike its two siblings below: check-gosec-analysis.sh (Landmine
# 28) keys on the exact wording of gosec's own log lines ("Checking file:",
# "no ssa result", etc.) to tell a real analysis from one that silently
# analysed nothing. That asymmetry is why this one can't float — a reworded
# "Checking file:" line fails the guard CLOSED (red CI on a sound tree, loud
# and easy to diagnose), but a reworded *failure* signature fails it OPEN
# (green CI on a genuinely broken analysis, the exact hole the guard exists
# to close). Pinning removes the failure-open branch entirely; bump this
# deliberately, and re-run gosecguard_test.go against the new version's log
# wording before trusting it again.
GOSEC_VERSION       := v2.28.0
GOVULNCHECK_VERSION := latest
ACTIONLINT_VERSION  := latest

COVERAGE_MIN := 80
COVERPROFILE := coverage.out
COVERHTML    := coverage.html

VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT  := $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE    := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -s -w \
	-X $(VERSION_PKG).Version=$(VERSION) \
	-X $(VERSION_PKG).Commit=$(COMMIT) \
	-X $(VERSION_PKG).Date=$(DATE)

GOBIN := $(shell go env GOPATH)/bin

# Landmine 8's isolated run strips the user's own PATH so really-installed
# agents (claude, pi, hermes, cline) become invisible. It cannot hardcode
# /usr/local/go/bin the way the docs did: CI's Go lives in a tool cache, and
# a hardcoded path silently removes `go` itself.
GO_BIN_DIR    := $(shell dirname $$(command -v go))
ISOLATED_PATH ?= $(GO_BIN_DIR):/usr/bin:/bin

.DEFAULT_GOAL := help

.PHONY: help
help: ## List the available targets
	@awk 'BEGIN { FS = ":.*##" } \
	     /^[a-zA-Z0-9_-]+:.*##/ { printf "  \033[36m%-16s\033[0m %s\n", $$1, $$2 }' \
	     $(MAKEFILE_LIST)

## ---- build -----------------------------------------------------------

# GOWORK=off, so the binary links the TAGGED github.com/teggen/agentlaunch
# from go.mod rather than the local checkout ./go.work points at.
#
# A binary is an artifact that outlives the shell that made it. Built inside
# the workspace it records `dep github.com/teggen/agentlaunch (devel)` — no
# version, no hash, no way to tell afterwards which source went into it, and
# `go version -m` cannot recover it. That is a bad property for the one output
# here somebody might keep, copy, or install. Tests keep the workspace,
# because exercising local module edits is the entire reason it exists.
#
# Use build-workspace when you WANT the local module in the binary.
.PHONY: build
build: export GOWORK := off
build: ## Build ./$(BINARY) against the tagged agentlaunch (what a release links)
	go build -trimpath -ldflags '$(LDFLAGS)' -o $(BINARY) .

# The counterpart, for trying a local agentlaunch change in the real binary
# before tagging it. Named rather than a flag on `build` so the artifact's
# provenance is a choice someone made, not a property of their environment.
.PHONY: build-workspace
build-workspace: ## Build ./$(BINARY) against the LOCAL agentlaunch checkout (./go.work)
	@test -f go.work || { echo "no go.work here — this target needs one; see README"; exit 1; }
	go build -trimpath -ldflags '$(LDFLAGS)' -o $(BINARY) .
	@echo "NOTE: $(BINARY) links agentlaunch from ./go.work, not the tagged module."

.PHONY: clean
clean: ## Remove build artifacts and coverage output
	rm -f $(BINARY) openrouter-launch $(COVERPROFILE) $(COVERHTML)
	rm -rf dist/

## ---- format and lint -------------------------------------------------

.PHONY: fmt
fmt: ## Format all Go source in place
	gofmt -w .

.PHONY: fmt-check
fmt-check: ## Fail if any file is not gofmt-clean
	@out=$$(gofmt -l .); \
	if [ -n "$$out" ]; then \
	  echo "not gofmt-clean:"; echo "$$out"; \
	  echo "run: make fmt"; exit 1; \
	fi; \
	echo "gofmt: clean"

.PHONY: vet
vet: ## Run go vet
	go vet ./...

.PHONY: lint
lint: ## Run golangci-lint (requires v2 — see .golangci.yml)
	@test -x $(GOBIN)/golangci-lint || { echo "golangci-lint missing — run: make tools"; exit 1; }
	@$(GOBIN)/golangci-lint version 2>&1 | grep -qE 'version v?2\.' || { \
	  echo ".golangci.yml uses the v2 schema and the installed binary is v1."; \
	  echo "Upgrade the binary (make tools) — do NOT downgrade the config,"; \
	  echo "golangci-lint-action v9 rejects v1 outright."; exit 1; }
	$(GOBIN)/golangci-lint run ./...

.PHONY: lint-workflows
lint-workflows: ## Validate the GitHub Actions YAML
	@test -x $(GOBIN)/actionlint || { echo "actionlint missing — run: make tools"; exit 1; }
	$(GOBIN)/actionlint

.PHONY: lint-cross
lint-cross: ## Lint the build-tagged files the default GOOS never sees
	@test -x $(GOBIN)/golangci-lint || command -v golangci-lint >/dev/null || { \
	  echo "golangci-lint missing — run: make tools"; exit 1; }
	GOOS=windows $(or $(wildcard $(GOBIN)/golangci-lint),golangci-lint) run ./...
	GOOS=darwin  $(or $(wildcard $(GOBIN)/golangci-lint),golangci-lint) run ./...

## ---- security --------------------------------------------------------

# gosec's `test -x` guard is NOT redundant with the `-` prefix on its recipe
# line. The `-` tells make to ignore that line's exit status, which is what
# keeps gosec's 14 remaining findings advisory — but it ignores "No such file or
# directory" just as happily. With gosec uninstalled this target printed
#   make: $(GOBIN)/gosec: No such file or directory
#   make: [Makefile:112: security] Error 127 (ignored)
# and exited 0, so `make security` reported success having run no gosec at
# all. gosec was the only tool here without the presence guard its three
# siblings have. Measured, not theorised.
.PHONY: security
security: ## govulncheck + go mod verify (blocking); gosec (advisory)
	@test -x $(GOBIN)/govulncheck || { echo "govulncheck missing — run: make tools"; exit 1; }
	$(GOBIN)/govulncheck ./...
	go mod verify
	@echo "--- gosec (advisory: findings do not fail this target) ---"
	@test -x $(GOBIN)/gosec || { echo "gosec missing — run: make tools"; exit 1; }
	-@$(GOBIN)/gosec -quiet ./...

## ---- test ------------------------------------------------------------

.PHONY: test
test: ## Run the full test suite
	go test ./... -count=1

.PHONY: test-unit
test-unit: test ## Alias for `test` (this project has no unit/integration split)

.PHONY: test-race
test-race: ## Race detector on the TUI package
	go test ./internal/tui/ -race -count=1

.PHONY: test-isolated
test-isolated: ## Landmine 8: suite must stay green with real installs invisible
	HOME=$$(mktemp -d) PATH="$(ISOLATED_PATH)" go test ./... -count=1

.PHONY: cover
cover: ## Run the suite with coverage and print the total
	go test ./... -count=1 -covermode=atomic -coverprofile=$(COVERPROFILE)
	@go tool cover -func=$(COVERPROFILE) | tail -1

.PHONY: cover-html
cover-html: cover ## Write $(COVERHTML)
	go tool cover -html=$(COVERPROFILE) -o $(COVERHTML)
	@echo "wrote $(COVERHTML)"

.PHONY: cover-check
cover-check: cover ## Fail if total coverage is below $(COVERAGE_MIN)%
	@total=$$(go tool cover -func=$(COVERPROFILE) | tail -1 | awk '{print $$3}' | tr -d '%'); \
	echo "total coverage: $$total% (floor $(COVERAGE_MIN)%)"; \
	awk -v t="$$total" -v m="$(COVERAGE_MIN)" 'BEGIN { exit !(t + 0 >= m + 0) }' || { \
	  echo "coverage $$total% is below the $(COVERAGE_MIN)% floor"; exit 1; }

## ---- repository invariants -------------------------------------------

.PHONY: tidy-check
tidy-check: ## Fail if go.mod/go.sum are not tidy
	go mod tidy
	@git diff --exit-code go.mod go.sum || { \
	  echo "go.mod/go.sum were not tidy — commit the result of 'go mod tidy'"; exit 1; }

.PHONY: cross
cross: ## Cross-compile check (windows, darwin)
	GOOS=windows go build ./...
	GOOS=darwin go build ./...

.PHONY: writesites
writesites: ## Landmine 6: show every write primitive outside tests
	@grep -rn "os.WriteFile\|os.Create\|os.MkdirAll\|os.Rename\|OpenFile\|CreateTemp" \
	  --include="*.go" . | grep -v _test || true

## ---- tooling and release --------------------------------------------

# GOTOOLCHAIN=auto is currently INERT — and must stay anyway.
#
# The history: all four tools' own go.mod files declare go >= 1.25
# (golangci-lint 1.25.0, gosec 1.25.8, x/vuln 1.25.0, actionlint 1.25.0 —
# three of them arrived there via @latest, so it became true with no change
# here), while this project's floor was then go 1.24.0. actions/setup-go
# injects GOTOOLCHAIN=local, which forbids fetching a newer toolchain, so the
# audit job died on
#   go: ...golangci-lint@v2.12.2 requires go >= 1.25.0
#       (running go 1.24.0; GOTOOLCHAIN=local)
#
# The floor has since moved to go 1.25 (a security floor — 1.24 is EOL; see
# Landmine 25's third clause), which is >= every one of those tools' own
# requirement. So `auto` no longer has anything to fetch and changes nothing
# today. Do NOT delete it as dead weight: it is what keeps the next tool that
# raises its floor to 1.26 from reproducing the exact failure above, and it
# keeps contributors on an older toolchain able to run `make tools` at all.
# It cannot drag the project forward — `auto` applies only to BUILDING these
# tools; what builds and tests this project stays pinned to the go.mod floor
# via go-version-file. It is also safe against Landmine 25, because the skew
# that breaks analysis is a tool OLDER than the tree; a tool built by a newer
# toolchain analyses older code fine.
.PHONY: tools
tools: ## Install the pinned lint/security tools with the local toolchain
	GOTOOLCHAIN=auto go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_VERSION)
	GOTOOLCHAIN=auto go install github.com/securego/gosec/v2/cmd/gosec@$(GOSEC_VERSION)
	GOTOOLCHAIN=auto go install golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION)
	$(MAKE) tools-actionlint
	@echo "installed into $(GOBIN)"

# Split out from `tools` so ci.yml's `quality` job can install just this one
# tool for `lint-workflows`, instead of the other three `tools` also builds
# (golangci-lint is already installed there by golangci-lint-action; gosec
# and govulncheck belong to the `audit` job, not `quality`). `tools` still
# depends on this rather than repeating the go-install line, so the version
# pin and install command stay declared exactly once.
.PHONY: tools-actionlint
tools-actionlint: ## Install actionlint alone (with the local toolchain)
	GOTOOLCHAIN=auto go install github.com/rhysd/actionlint/cmd/actionlint@$(ACTIONLINT_VERSION)

.PHONY: tools-release
tools-release: ## Install goreleaser (separate: it is a slow build)
	GOTOOLCHAIN=auto go install github.com/goreleaser/goreleaser/v2@$(GORELEASER_VERSION)

.PHONY: release-check
release-check: ## Validate .goreleaser.yaml
	@test -x $(GOBIN)/goreleaser || { echo "goreleaser missing — run: make tools-release"; exit 1; }
	$(GOBIN)/goreleaser check

# GOWORK=off for the same reason `build` sets it, and with more at stake:
# these are the six artifacts a release publishes, and .goreleaser.yaml's
# before-hook runs `go mod tidy`, which must resolve agentlaunch exactly as
# the release workflow does — from the proxy, at the version in go.mod.
# A snapshot built inside the workspace would silently ship a local checkout.
.PHONY: snapshot
snapshot: export GOWORK := off
snapshot: ## Build every release artifact locally, publishing nothing
	@test -x $(GOBIN)/goreleaser || { echo "goreleaser missing — run: make tools-release"; exit 1; }
	$(GOBIN)/goreleaser release --snapshot --clean --skip=publish

## ---- aggregates ------------------------------------------------------

.PHONY: pre-commit
pre-commit: clean fmt-check vet lint security test ## The /quality gate

# lint-workflows is in this list because .github/workflows/*.yml is the one
# file class in this repo with an AUTOMATED editor (Dependabot bumps action
# SHAs weekly) and, until now, no automated validator: actionlint was pinned
# and installed by `make tools`, but nothing ever ran it.
# GOWORK=off is LOAD-BEARING, and target-specific so it reaches every
# prerequisite below.
#
# github.com/teggen/agentlaunch is developed alongside this repo, and
# ~/projects/go.work makes the local checkout shadow the published module for
# every go command. That is what you want while editing both — and it means a
# plain `make ci` can pass against source that exists only on this machine.
# CI has no workspace, so it resolves the tagged version from the proxy
# instead, and the two can differ silently: an API change made locally and
# never tagged goes green here and red there.
#
# Turning the workspace off for this target makes `make ci` the same claim CI
# makes. `make test` and the rest deliberately keep the workspace, because
# editing both modules together is the reason it exists.
.PHONY: ci
ci: export GOWORK := off
ci: fmt-check vet lint lint-cross lint-workflows tidy-check cross security test-race cover-check test-isolated ## Everything CI runs (against the TAGGED dependency, not the workspace)
