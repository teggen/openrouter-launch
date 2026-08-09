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
GOSEC_VERSION       := latest
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

.PHONY: build
build: ## Build ./$(BINARY) with version information linked in
	go build -trimpath -ldflags '$(LDFLAGS)' -o $(BINARY) .

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
# keeps gosec's ~19 findings advisory — but it ignores "No such file or
# directory" just as happily. With gosec uninstalled this target printed
#   make: /home/martin/go/bin/gosec: No such file or directory
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
	GOTOOLCHAIN=auto go install github.com/rhysd/actionlint/cmd/actionlint@$(ACTIONLINT_VERSION)
	@echo "installed into $(GOBIN)"

.PHONY: tools-release
tools-release: ## Install goreleaser (separate: it is a slow build)
	GOTOOLCHAIN=auto go install github.com/goreleaser/goreleaser/v2@$(GORELEASER_VERSION)

.PHONY: release-check
release-check: ## Validate .goreleaser.yaml
	@test -x $(GOBIN)/goreleaser || { echo "goreleaser missing — run: make tools-release"; exit 1; }
	$(GOBIN)/goreleaser check

.PHONY: snapshot
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
.PHONY: ci
ci: fmt-check vet lint lint-cross lint-workflows tidy-check cross security test-race cover-check test-isolated ## Everything CI runs
