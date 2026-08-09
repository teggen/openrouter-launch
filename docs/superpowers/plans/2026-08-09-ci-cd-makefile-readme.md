# CI/CD, Makefile, and README Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Give the repo a Makefile that is the single source of truth for every check, GitHub Actions that call it, tag-driven multi-platform releases via GoReleaser, and a README — closing HANDOFF.md's last three Open items.

**Architecture:** The Makefile owns every command. CI jobs invoke `make <target>` rather than re-spelling commands in YAML, so "green locally" and "green in CI" are the same claim rather than two similar-looking ones. Releases are triggered by a git tag; a shell script (testable, therefore tested) enforces that stable tags are reachable from `main` and `-beta.N` tags from `develop`; GoReleaser derives release-vs-prerelease from the tag itself.

**Tech Stack:** Go 1.24 (floor from `go.mod`), GNU make, GitHub Actions, GoReleaser v2, golangci-lint v2, govulncheck, gosec, actionlint.

**Spec:** `docs/superpowers/specs/2026-08-09-ci-cd-and-readme-design.md` — read it for *why* before changing *what*.

## Global Constraints

- **Module path:** `github.com/teggen/openrouter-launch`. Released binary name: `openrouter-launch`. Local build name: `orl`.
- **Go version:** read from `go.mod` (`go 1.24.0`) via `go-version-file: go.mod` — never hardcode a Go version in a workflow.
- **Coverage floor:** 80%. It is defined once, in the Makefile's `COVERAGE_MIN`, and nowhere else.
- **Build targets:** exactly six — `{linux, darwin, windows} × {amd64, arm64}`.
- **License:** MIT, `Copyright (c) 2026 teggen`. Use the GitHub handle, never a real name — the owner does not want a full legal name in the repository. This applies to `LICENSE`, the README, and any generated metadata.
- **First tags:** `v0.1.0-beta.1` on `develop`, then `v0.1.0` on `main`. Never `v1.0.0` in this phase.
- **Action pinning:** every third-party action pinned to a full commit SHA with a trailing `# vN` comment. Never a bare tag.
- **Pinned tool versions:** golangci-lint `v2.12.2`, goreleaser `v2.17.1`. Defined in the Makefile; workflows must pin the same strings (Tasks 5 and 6 add tests for this).
- **`.golangci.yml` must use the v2 schema** (`version: "2"`). `golangci-lint-action` v9 rejects v1 outright. If a locally installed v1 binary rejects the config, upgrade the binary via `make tools` — never downgrade the config.
- **Branch situation during this plan:** `develop` was created from `main` before Task 1 and pushed (owner's decision, 2026-08-09 — the end-state branch model applied one phase early). **Tasks 1–7 commit to `develop`**, which is already checked out; do not switch branches. `main` is three commits behind and is not touched until Task 8, which fast-forwards it from `develop` and then cuts `v0.1.0`. The beta tag is cut on `develop` before that merge, so the branch guard sees genuinely diverged branches.
- **Every task that adds or edits a `.go` file must run `make lint` AND
  `make lint-cross` before committing** — not just `go test`, `go vet`, and
  `gofmt`. Learned the expensive way: Task 6 added `tagguard_test.go` (a
  `//go:build !windows` file) and verified with tests + actionlint + vet + fmt,
  none of which run golangci-lint. It carried two `noctx` findings that would
  have turned the first-ever CI run red, and the `GOOS=windows` pass reported
  `0 issues` because the build tag excludes the file there. Task 8's pre-push
  gate caught it before anything was pushed. Lint is per-`GOOS`; a build-tagged
  file is only ever seen by the platform it is tagged for.

- **Existing invariants still bind.** `writesites_test.go` fails if any non-test `.go` file outside the four allowlisted write sites uses a write primitive — no task here adds one. Landmine 8's `HOME`-isolation rule still applies to any test that needs a binary to look absent.

---

### Task 1: Version package, `--version`, and LICENSE

Gives the binary a build identity. Without this, a tagged release is unfalsifiable — you cannot ask a downloaded artifact which commit it came from.

**Files:**
- Create: `internal/version/version.go`
- Create: `internal/version/version_test.go`
- Modify: `internal/cli/root.go` (imports, and the `root := &cobra.Command{...}` literal at lines 57–72)
- Create: `internal/cli/version_test.go`
- Create: `LICENSE`

**Interfaces:**
- Consumes: nothing.
- Produces: package `github.com/teggen/openrouter-launch/internal/version` exporting `var Version, Commit, Date string` and `func String() string`. **Task 4's `.goreleaser.yaml` names these three variables by full import path in its ldflags, and Task 4 adds the test that pins the two together — do not rename them.**

- [ ] **Step 1: Write the failing test for the version package**

Create `internal/version/version_test.go`:

```go
package version

import (
	"strings"
	"testing"
)

// TestDefaultsAreDevPlaceholders documents what a plain `go build` reports.
// The release build overwrites all three via -ldflags -X; `go test` never
// applies those, so these defaults are what this test always sees.
func TestDefaultsAreDevPlaceholders(t *testing.T) {
	if Version != "dev" {
		t.Errorf("Version = %q, want %q for a non-release build", Version, "dev")
	}
	if Commit != "none" {
		t.Errorf("Commit = %q, want %q for a non-release build", Commit, "none")
	}
	if Date != "unknown" {
		t.Errorf("Date = %q, want %q for a non-release build", Date, "unknown")
	}
}

// TestStringReportsEveryInjectedField would pass vacuously if it only checked
// the defaults, because "dev"/"none"/"unknown" are also plausible substrings
// of an unrelated string. It assigns three distinct sentinels instead, so
// dropping any one field from String() fails this test by name.
func TestStringReportsEveryInjectedField(t *testing.T) {
	saved := [3]string{Version, Commit, Date}
	t.Cleanup(func() { Version, Commit, Date = saved[0], saved[1], saved[2] })

	Version, Commit, Date = "v9.9.9-sentinel", "cafebabe", "2026-01-02T03:04:05Z"

	got := String()
	for _, want := range []string{Version, Commit, Date} {
		if !strings.Contains(got, want) {
			t.Errorf("String() = %q, missing %q", got, want)
		}
	}
}
```

- [ ] **Step 2: Run it and watch it fail**

Run: `go test ./internal/version/ -v`
Expected: FAIL — the package does not exist (`no Go files in .../internal/version`).

- [ ] **Step 3: Write the version package**

Create `internal/version/version.go`:

```go
// Package version reports the build identity of the openrouter-launch binary.
package version

import "runtime"

// Version, Commit, and Date are overwritten at link time by the release
// build. Keep them in this package, with these exact names: .goreleaser.yaml
// names all three by full import path in its ldflags, so a rename here would
// still build, test, and publish cleanly while every released binary silently
// reported the placeholders below. TestGoreleaserLdflagsMatchVersionSymbols
// (goreleaser_test.go, package main) pins the two together.
var (
	Version = "dev"
	Commit  = "none"
	Date    = "unknown"
)

// String renders the one-line identity cobra prints for --version.
func String() string {
	return Version + " (commit " + Commit + ", built " + Date + ", " + runtime.Version() + ")"
}
```

- [ ] **Step 4: Run the test and watch it pass**

Run: `go test ./internal/version/ -v`
Expected: PASS, both tests.

- [ ] **Step 5: Mutation check — prove the test can fail**

Temporarily delete `+ ", built " + Date` from `String()`. Run `go test ./internal/version/ -run TestStringReportsEveryInjectedField -v`. Expected: FAIL naming the missing date. Revert the edit and re-run to confirm PASS.

- [ ] **Step 6: Write the failing CLI test**

Create `internal/cli/version_test.go`:

```go
package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/teggen/openrouter-launch/internal/launch"
	"github.com/teggen/openrouter-launch/internal/version"
)

// TestRootPrintsVersion drives the real flag, not the struct field: cobra
// only synthesises --version when Command.Version is non-empty, so asserting
// on the field alone would pass even if the flag never reached the user.
// --version short-circuits before RunE, so this makes no API call.
func TestRootPrintsVersion(t *testing.T) {
	root := NewRootCmdWith(&launch.Service{})

	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"--version"})

	if err := root.Execute(); err != nil {
		t.Fatalf("--version returned an error: %v", err)
	}
	if got := out.String(); !strings.Contains(got, version.String()) {
		t.Errorf("--version printed %q, want it to contain %q", got, version.String())
	}
}
```

- [ ] **Step 7: Run it and watch it fail**

Run: `go test ./internal/cli/ -run TestRootPrintsVersion -v`
Expected: FAIL with `unknown flag: --version` — cobra has not been given a `Version`.

- [ ] **Step 8: Wire the version into the root command**

In `internal/cli/root.go`, add the import to the existing block:

```go
	"github.com/teggen/openrouter-launch/internal/launch"
	"github.com/teggen/openrouter-launch/internal/tui"
	"github.com/teggen/openrouter-launch/internal/version"
```

and add one field to the `root := &cobra.Command{...}` literal, immediately after `Short:`:

```go
		Short: "Launch coding agents against OpenRouter models",
		// Setting Version is what makes cobra synthesise --version.
		Version: version.String(),
```

- [ ] **Step 9: Run the tests and watch them pass**

Run: `go test ./internal/cli/ ./internal/version/ -count=1`
Expected: PASS. Then `go test ./... -count=1` — the whole suite must still be green; `root_test.go` asserts nothing about help text, so nothing else should move.

- [ ] **Step 10: Write the LICENSE**

Create `LICENSE` with the standard MIT text, first line exactly:

```
MIT License

Copyright (c) 2026 teggen
```

The GitHub handle, not a real name — see Global Constraints.

followed by the unmodified MIT body (`Permission is hereby granted, free of charge, …` through `… DEALINGS IN THE SOFTWARE.`).

- [ ] **Step 11: Verify by hand**

```bash
go build -o orl . && ./orl --version
```
Expected: `openrouter-launch version dev (commit none, built unknown, go1.26.5)`.

- [ ] **Step 12: Commit**

```bash
git add internal/version internal/cli/root.go internal/cli/version_test.go LICENSE
git commit -m "feat(cli): add --version and an MIT license

internal/version holds the three symbols the release build overwrites via
ldflags. A plain build reports dev/none/unknown honestly rather than
inventing a number."
```

---

### Task 2: The Makefile

Every later task calls these targets, and so does CI. Target names are **constrained by the owner's existing `/quality` command**, which invokes `pre-commit`, `clean`, `fmt-check`, `vet`, `lint`, `security`, `test`, `fmt`, and `test-unit` by name — this task must satisfy that contract, not invent its own names.

**Files:**
- Create: `Makefile`
- Create: `makefile_test.go` (package `main`, repo root — same convention as the existing `writesites_test.go`)
- Modify: `.gitignore`

**Interfaces:**
- Consumes: `internal/version` from Task 1 (the `LDFLAGS` variable references it).
- Produces: targets `help build clean fmt fmt-check vet lint security test test-unit test-race test-isolated cover cover-html cover-check tidy-check cross writesites tools tools-release lint-workflows snapshot release-check pre-commit ci`, and the variables `GOLANGCI_VERSION := v2.12.2` / `GORELEASER_VERSION := v2.17.1`, which Tasks 5 and 6 assert the workflows pin identically.

- [ ] **Step 1: Write the failing contract test**

Create `makefile_test.go` at the repo root:

```go
package main

import (
	"os"
	"regexp"
	"testing"
)

// The owner's /quality command (~/.claude/commands/quality.md) invokes these
// targets by name, and /preflight invokes a subset. Renaming or dropping one
// breaks those commands silently — nothing in the Go build would notice, and
// the failure would surface as a confusing "No rule to make target" during an
// unrelated session. This is the same class of structural tripwire as
// TestWriteSitesAreExhaustivelyEnumerated.
var qualityContractTargets = []string{
	"clean",
	"fmt",
	"fmt-check",
	"vet",
	"lint",
	"security",
	"test",
	"test-unit",
	"pre-commit",
}

func TestMakefileDeclaresQualityContractTargets(t *testing.T) {
	src, err := os.ReadFile("Makefile")
	if err != nil {
		t.Fatalf("reading Makefile: %v", err)
	}
	for _, target := range qualityContractTargets {
		// Anchored at line start and followed by a colon, so a mention
		// inside a recipe or comment cannot satisfy the assertion.
		re := regexp.MustCompile(`(?m)^` + regexp.QuoteMeta(target) + `:`)
		if !re.Match(src) {
			t.Errorf("Makefile declares no %q target; the /quality command invokes it by name", target)
		}
	}
}
```

- [ ] **Step 2: Run it and watch it fail**

Run: `go test . -run TestMakefileDeclaresQualityContractTargets -v`
Expected: FAIL — `reading Makefile: open Makefile: no such file or directory`.

- [ ] **Step 3: Write the Makefile**

Create `Makefile`:

```makefile
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

## ---- security --------------------------------------------------------

.PHONY: security
security: ## govulncheck + go mod verify (blocking); gosec (advisory)
	@test -x $(GOBIN)/govulncheck || { echo "govulncheck missing — run: make tools"; exit 1; }
	$(GOBIN)/govulncheck ./...
	go mod verify
	@echo "--- gosec (advisory: findings do not fail this target) ---"
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

# GOTOOLCHAIN=auto is required, not optional. All four tools' own go.mod
# files now declare go >= 1.25 (golangci-lint 1.25.0, gosec 1.25.8,
# x/vuln 1.25.0, actionlint 1.25.0 — three of them arrived via @latest, so
# this became true without any change here), while THIS project's floor is
# go 1.24.0. actions/setup-go injects GOTOOLCHAIN=local, which forbids
# fetching a newer toolchain, so the audit job died on
#   go: ...golangci-lint@v2.12.2 requires go >= 1.25.0
#       (running go 1.24.0; GOTOOLCHAIN=local)
# `auto` lets Go fetch a newer toolchain solely to BUILD these tools. It does
# not change what builds or tests this project — that stays pinned to the
# go.mod floor via go-version-file. This is safe against Landmine 25 because
# the skew that breaks analysis is tool OLDER than the tree; a tool built by a
# newer toolchain analyses older code fine.
.PHONY: tools
tools: ## Install the pinned lint/security tools
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

.PHONY: ci
ci: fmt-check vet lint tidy-check cross security test-race cover-check test-isolated ## Everything CI runs
```

- [ ] **Step 4: Run the contract test and watch it pass**

Run: `go test . -run TestMakefileDeclaresQualityContractTargets -v`
Expected: PASS.

- [ ] **Step 5: Mutation check**

Rename the `vet:` target to `govet:` (both the `.PHONY` line and the rule). Re-run the test. Expected: FAIL naming `"vet"`. Revert and confirm PASS.

- [ ] **Step 6: Install the tools — this also un-breaks the local machine**

Run: `make tools`

This replaces the Go-1.25-built `golangci-lint`, `govulncheck`, and `staticcheck` binaries that currently abort against the local Go 1.26.5 toolchain. Verify:

```bash
$(go env GOPATH)/bin/golangci-lint version    # must report v2.12.2
$(go env GOPATH)/bin/govulncheck ./...        # must analyse, not abort
```

- [ ] **Step 7: Exercise the targets that do not need later tasks**

```bash
make help
make fmt-check vet test
make cover-check      # expect ~85.5%, above the 80% floor
make test-isolated
make cross
make tidy-check
```
All must pass. `make lint` will fail until Task 3 creates `.golangci.yml` — that is expected here.

- [ ] **Step 8: Ignore the new artifacts**

Add to `.gitignore`:

```
/coverage.out
/coverage.html
/dist/
```

- [ ] **Step 9: Commit**

```bash
git add Makefile makefile_test.go .gitignore
git commit -m "build: add the Makefile as the single source of truth for checks

Target names satisfy the /quality command's contract; makefile_test.go
pins them so a tidy-up cannot break that command silently. test-isolated
derives the Go bin directory instead of hardcoding /usr/local/go/bin,
which would strip go itself on a CI runner."
```

---

### Task 3: golangci-lint v2 configuration

**Files:**
- Create: `.golangci.yml`

**Interfaces:**
- Consumes: `make lint` and `make tools` from Task 2.
- Produces: a lint gate that `make lint` and the CI `quality` job both pass.

- [ ] **Step 1: Confirm the installed binary is v2**

Run: `$(go env GOPATH)/bin/golangci-lint version`
Expected: `golangci-lint has version v2.12.2 …`. If it reports v1, re-run `make tools` — do not adapt the config to v1.

- [ ] **Step 2: Write the configuration**

Create `.golangci.yml`:

```yaml
# golangci-lint v2 schema. This file MUST stay on version "2":
# golangci-lint-action v9 (used by .github/workflows/ci.yml) refuses v1
# outright — "golangci-lint v1 is not supported by golangci-lint-action >= v7".
# If a locally installed v1 binary rejects this file, upgrade the binary
# (make tools). Downgrading the config breaks CI instead.
version: "2"

run:
  timeout: 5m

linters:
  # "standard" is errcheck, govet, ineffassign, staticcheck, unused.
  # In v2, gosimple was merged into staticcheck — do not try to enable it.
  default: standard
  enable:
    - misspell
    - revive
    - bodyclose
    - noctx
  exclusions:
    rules:
      # Test helpers routinely ignore errors from cleanup and fixture setup
      # where the failure mode is the test failing anyway.
      - path: _test\.go
        linters:
          - errcheck

      # revive's `exported` rule polices comment FORM ("comment on exported X
      # should be of the form ..."), which is prose style, not a defect class.
      # The gate is for bugs.
      #
      # Suppress it HERE, and never via `linters.settings.revive.rules`: that
      # block REPLACES revive's rule set rather than amending it, so naming a
      # single disabled rule there silently turns revive off completely.
      # Measured on this tree: 0 findings with the settings block, 26 without.
      # Every other default rule — error-return, indent-error-flow,
      # unreachable-code, redefines-builtin-id — was inert and nobody noticed.
      - linters:
          - revive
        text: '^exported:'

      # The four unused-parameter hits are all in test helpers whose
      # signatures are fixed by the interface they stub.
      - path: _test\.go
        linters:
          - revive
        text: '^unused-parameter:'

formatters:
  # v2 moved gofmt/goimports/gofumpt out of linters into their own section.
  enable:
    - gofmt

issues:
  # Both default to a nonzero cap (max-issues-per-linter 50, max-same-issues 3)
  # which SILENTLY TRUNCATES the report. Found during execution: the first run
  # of this config showed 18 findings and the true count was 28 — ten
  # identical-message errcheck hits were withheld, including four sibling
  # cleanup lines in droid.go that a partial fix would have missed. A gate that
  # hides its fourth repeat of a finding is not a gate. 0 means unlimited, and
  # can only ever surface more findings, never fewer.
  max-issues-per-linter: 0
  max-same-issues: 0
```

- [ ] **Step 3: Run the linter**

Run: `make lint`

- [ ] **Step 4: Triage whatever it reports**

The finding count is genuinely unknown — no v2 linter has ever run against this tree (every local analysis binary is broken, see Task 2 Step 6). Apply this rule, in order:

1. **A real defect** (an unchecked error that matters, a leaked response body, a missing `context`): fix the code.
2. **Inherent to the design** (this tool launches subprocesses and writes an XDG config by definition): add a narrow `linters.exclusions.rules` entry scoped to the specific path and linter, with a comment naming *why*. Never a blanket `//nolint` with no reason, and never disable a whole linter to silence one site.
3. **Disagreeing with a documented Landmine**: the Landmine wins. Exclude the site and cite the Landmine number in the comment.

Re-run `make lint` until clean.

**Lint is `GOOS`-sensitive.** A build-tagged file is invisible to the linter on
every other platform, so `make lint` alone never sees `internal/agent/exec_windows.go`.
Check the other platform explicitly before declaring the gate clean:

```bash
GOOS=windows $(go env GOPATH)/bin/golangci-lint run ./...
GOOS=darwin  $(go env GOPATH)/bin/golangci-lint run ./...
```

Any exclusion that applies to a Unix-tagged file almost certainly applies to its
Windows counterpart too — write the path pattern to cover both.

- [ ] **Step 5: Confirm nothing regressed**

Run: `go test ./... -count=1 && make fmt-check vet`
Expected: all green. If Step 4 changed any code, this is what proves the change was safe.

- [ ] **Step 6: Commit**

```bash
git add .golangci.yml
git add -u
git commit -m "build: add golangci-lint v2 configuration

v2 schema is mandatory: golangci-lint-action v9 rejects v1 configs.
gofmt moves to the formatters section and gosimple is folded into
staticcheck under v2."
```

---

### Task 4: GoReleaser configuration

**Files:**
- Create: `.goreleaser.yaml`
- Create: `goreleaser_test.go` (package `main`, repo root)

**Interfaces:**
- Consumes: `internal/version`'s three variables (Task 1); `make snapshot`, `make release-check`, `make tools-release` (Task 2).
- Produces: `.goreleaser.yaml` pinning `GORELEASER_VERSION`-compatible config; Task 6's `release.yml` runs `goreleaser release --clean` against it.

- [ ] **Step 1: Write the failing pin test**

Create `goreleaser_test.go` at the repo root:

```go
package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"strings"
	"testing"
)

// .goreleaser.yaml names the version symbols by full import path in its
// ldflags. Renaming the package or one of the variables would still build,
// test, vet, lint, and publish cleanly — and every released binary would
// silently report the "dev" placeholders. Nothing else in the tree connects
// the YAML to the Go declarations, so this test is that connection.
const versionImportPath = "github.com/teggen/openrouter-launch/internal/version"

func TestGoreleaserLdflagsMatchVersionSymbols(t *testing.T) {
	cfg, err := os.ReadFile(".goreleaser.yaml")
	if err != nil {
		t.Fatalf("reading .goreleaser.yaml: %v", err)
	}
	declared := versionVarsDeclared(t)

	for _, name := range []string{"Version", "Commit", "Date"} {
		if !declared[name] {
			t.Errorf("internal/version declares no var %q, but .goreleaser.yaml injects it", name)
		}
		want := "-X " + versionImportPath + "." + name + "="
		if !strings.Contains(string(cfg), want) {
			t.Errorf(".goreleaser.yaml has no ldflag %q — released binaries would report the dev placeholder", want)
		}
	}
}

// versionVarsDeclared returns the package-level var names in internal/version,
// skipping _test.go files so a variable declared only in a test cannot make
// this assertion pass.
func versionVarsDeclared(t *testing.T) map[string]bool {
	t.Helper()
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, "internal/version",
		func(fi fs.FileInfo) bool { return !strings.HasSuffix(fi.Name(), "_test.go") }, 0)
	if err != nil {
		t.Fatalf("parsing internal/version: %v", err)
	}
	found := map[string]bool{}
	for _, pkg := range pkgs {
		for _, file := range pkg.Files {
			for _, decl := range file.Decls {
				gen, ok := decl.(*ast.GenDecl)
				if !ok || gen.Tok != token.VAR {
					continue
				}
				for _, spec := range gen.Specs {
					vs, ok := spec.(*ast.ValueSpec)
					if !ok {
						continue
					}
					for _, n := range vs.Names {
						found[n.Name] = true
					}
				}
			}
		}
	}
	return found
}
```

- [ ] **Step 2: Run it and watch it fail**

Run: `go test . -run TestGoreleaserLdflagsMatchVersionSymbols -v`
Expected: FAIL — `reading .goreleaser.yaml: … no such file or directory`.

- [ ] **Step 3: Write the GoReleaser configuration**

Create `.goreleaser.yaml`:

```yaml
# GoReleaser v2. Note `archives[].formats` is a LIST — v2.6 renamed the
# singular `format`, which is deprecated. Same rename inside format_overrides.
version: 2

project_name: openrouter-launch

before:
  hooks:
    - go mod tidy

builds:
  - id: openrouter-launch
    main: .
    binary: openrouter-launch
    env:
      - CGO_ENABLED=0
    flags:
      - -trimpath
    # Reproducible builds: stamp from the commit, not from wall-clock time.
    mod_timestamp: "{{ .CommitTimestamp }}"
    ldflags:
      - -s -w
      - -X github.com/teggen/openrouter-launch/internal/version.Version={{ .Version }}
      - -X github.com/teggen/openrouter-launch/internal/version.Commit={{ .FullCommit }}
      - -X github.com/teggen/openrouter-launch/internal/version.Date={{ .CommitDate }}
    goos:
      - linux
      - darwin
      - windows
    goarch:
      - amd64
      - arm64

archives:
  - name_template: "{{ .ProjectName }}_{{ .Version }}_{{ .Os }}_{{ .Arch }}"
    formats: [tar.gz]
    format_overrides:
      - goos: windows
        formats: [zip]
    files:
      - README.md
      - LICENSE

checksum:
  name_template: checksums.txt

changelog:
  use: github
  sort: asc
  groups:
    - title: Features
      regexp: '^.*?feat(\(.+\))??!?:.+$'
      order: 0
    - title: Fixes
      regexp: '^.*?fix(\(.+\))??!?:.+$'
      order: 1
    - title: Other changes
      order: 999
  filters:
    exclude:
      - '^docs:'
      - '^docs\('
      - '^test:'
      - '^test\('
      - '^chore:'
      - '^chore\('

release:
  # Reads the -beta.N suffix off the tag and marks the GitHub release a
  # prerelease. This is what implements "main for releases, develop for
  # betas" on the publishing side; .github/scripts/check-tag-branch.sh
  # enforces the branch half.
  prerelease: auto
  draft: false
  footer: |
    ## Install

    Download the archive for your platform, extract it, and put
    `openrouter-launch` on your `PATH`:

    ```
    openrouter-launch --version
    ```

    Verify your download against `checksums.txt`.
```

- [ ] **Step 4: Run the pin test and watch it pass**

Run: `go test . -run TestGoreleaserLdflagsMatchVersionSymbols -v`
Expected: PASS.

- [ ] **Step 5: Mutation check, both directions**

First, delete the `-X …internal/version.Date={{ .CommitDate }}` line from `.goreleaser.yaml`, run the test — expect FAIL naming the missing ldflag; restore it.
Then rename `Version` to `Ver` in `internal/version/version.go`, run the test — expect FAIL naming the missing declaration; restore it. (The second edit also breaks the build, which is itself the point: the *first* edit does not, and that is the failure mode this test exists for.)

**Known transient, accepted (found during execution):** `archives.files` lists
`README.md`, which Task 7 creates. Between this task and Task 7, `make snapshot`
therefore fails with `globbing failed for pattern README.md: file does not
exist` — *after* successfully building and archiving all six targets, so the
config itself is proven. Tasks 5 and 6 do not run `make snapshot`, and Task 7
Step 4 re-runs it and inspects the archive contents, which is where this
self-heals. Do not "fix" it here by dropping `README.md` from the list or by
committing a placeholder README; verify with a temporary file if you need to,
and delete it before committing.

- [ ] **Step 6: Validate the config and build all six artifacts**

```bash
make tools-release
make release-check
make snapshot
ls -1 dist/*.tar.gz dist/*.zip dist/checksums.txt
```

Expected: `release-check` reports no errors; `dist/` contains exactly four `.tar.gz` (linux amd64/arm64, darwin amd64/arm64), two `.zip` (windows amd64/arm64), and `checksums.txt`.

- [ ] **Step 7: Prove the ldflags actually reached the binary**

```bash
tar -xzf dist/openrouter-launch_*_linux_amd64.tar.gz -C /tmp openrouter-launch
/tmp/openrouter-launch --version
```

Expected: a version string containing the snapshot version and the **real commit SHA** — not `dev`, not `none`. This is the check that would have caught a wrong import path in the ldflags; the pin test catches renames, this catches typos.

- [ ] **Step 8: Commit**

```bash
git add .goreleaser.yaml goreleaser_test.go
git commit -m "build: add GoReleaser config for six 64-bit targets

prerelease: auto derives release-vs-prerelease from the tag suffix.
goreleaser_test.go pins the ldflag import paths against the real
declarations — a package rename would otherwise publish binaries that
silently report 'dev'."
```

---

### Task 5: CI workflow and Dependabot

**Files:**
- Create: `.github/workflows/ci.yml`
- Create: `.github/dependabot.yml`
- Create: `workflows_test.go` (package `main`, repo root)
- Modify: `Makefile` (add the `lint-cross` target described below, and add it to the `ci` aggregate)

**Added after Task 3 (do this first, before writing the workflow):** Task 3
established that golangci-lint only ever examines the current `GOOS`, so
`internal/agent/exec_windows.go` had never been linted by anything and did in
fact carry a real finding. `make lint` alone therefore does not mean "the tree
is lint-clean". Add to the `Makefile`, beside `lint`:

```makefile
.PHONY: lint-cross
lint-cross: ## Lint the build-tagged files the default GOOS never sees
	@test -x $(GOBIN)/golangci-lint || command -v golangci-lint >/dev/null || { \
	  echo "golangci-lint missing — run: make tools"; exit 1; }
	GOOS=windows $(or $(wildcard $(GOBIN)/golangci-lint),golangci-lint) run ./...
	GOOS=darwin  $(or $(wildcard $(GOBIN)/golangci-lint),golangci-lint) run ./...
```

It falls back to a `PATH`-resolved `golangci-lint` because CI's
`golangci-lint-action` installs the binary onto `PATH` rather than into
`$(GOBIN)`. Then add `lint-cross` to the `ci` aggregate target's dependency
list, immediately after `lint`. Leave `pre-commit` alone — it stays fast.

**Interfaces:**
- Consumes: every Makefile target from Task 2; `.golangci.yml` from Task 3.
- Produces: `.github/workflows/ci.yml`, which Task 8 verifies green on a real push.

**Action SHAs** (resolved 2026-08-09 — use exactly these):

| Action | Pin |
|---|---|
| `actions/checkout` | `3d3c42e5aac5ba805825da76410c181273ba90b1 # v7` |
| `actions/setup-go` | `b7ad1dad31e06c5925ef5d2fc7ad053ef454303e # v7` |
| `actions/upload-artifact` | `043fb46d1a93c77aae656e7c1c64a875d1fc6a0a # v7` |
| `golangci/golangci-lint-action` | `ba0d7d2ec06a0ea1cb5fa41b2e4a3ab91d21278a # v9` |
| `github/codeql-action` | `5595ccaf912efad79be6eef63a5619ff05969be3 # v4` |

- [ ] **Step 1: Write the failing version-parity test**

Create `workflows_test.go` at the repo root:

```go
package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The Makefile installs the tool version developers run locally; the
// workflows pin the version CI runs. When those drift, `make lint` passes
// locally and CI fails (or worse, the reverse) for reasons no error message
// explains. These tests make the drift a test failure instead.

func makefileVar(t *testing.T, name string) string {
	t.Helper()
	src, err := os.ReadFile("Makefile")
	if err != nil {
		t.Fatalf("reading Makefile: %v", err)
	}
	re := regexp.MustCompile(`(?m)^` + regexp.QuoteMeta(name) + `\s*:?=\s*(\S+)`)
	m := re.FindSubmatch(src)
	if m == nil {
		t.Fatalf("Makefile declares no %s", name)
	}
	return string(m[1])
}

// Anchored to the actual `version:` field, not a bare substring search. A
// plain strings.Contains passes for two wrong reasons: v2.12.2 is a PREFIX of
// v2.12.25, so a genuinely divergent pin satisfies it; and the string
// appearing anywhere — a comment, or a leftover after the whole
// golangci-lint-action step is deleted — satisfies it too.
func TestCIPinsTheMakefilesGolangciLintVersion(t *testing.T) {
	want := makefileVar(t, "GOLANGCI_VERSION")
	ci, err := os.ReadFile(".github/workflows/ci.yml")
	if err != nil {
		t.Fatalf("reading ci.yml: %v", err)
	}
	re := regexp.MustCompile(`(?m)^\s*version:\s*` + regexp.QuoteMeta(want) + `\s*$`)
	if !re.Match(ci) {
		t.Errorf("ci.yml has no `version: %s` field pinning golangci-lint (the Makefile's GOLANGCI_VERSION); local and CI would run different linters", want)
	}
}

// TestWorkflowActionsArePinnedToShas guards the supply chain: this repo's
// release workflow holds contents:write and the tool writes an API key to
// disk, so a mutable tag on a third-party action is the wrong risk. A pin is
// a 40-hex SHA; `uses: actions/checkout@v7` must fail this.
func TestWorkflowActionsArePinnedToShas(t *testing.T) {
	uses := regexp.MustCompile(`(?m)uses:\s+([^\s@]+)@(\S+)`)
	sha := regexp.MustCompile(`^[0-9a-f]{40}$`)

	// Globbed rather than listed, so a workflow added later is covered
	// automatically instead of depending on someone remembering to extend
	// this slice.
	paths, err := filepath.Glob(".github/workflows/*.yml")
	if err != nil {
		t.Fatalf("globbing workflows: %v", err)
	}
	if len(paths) == 0 {
		t.Fatal("no workflows found under .github/workflows/ — this test would pass vacuously")
	}
	for _, path := range paths {
		src, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("reading %s: %v", path, err)
		}
		for _, m := range uses.FindAllStringSubmatch(string(src), -1) {
			action, ref := m[1], m[2]
			if strings.HasPrefix(action, "./") {
				continue // a local composite action has nothing to pin
			}
			if !sha.MatchString(ref) {
				t.Errorf("%s: %s is pinned to %q, want a full 40-character commit SHA", path, action, ref)
			}
		}
	}
}
```

- [ ] **Step 2: Run them and watch them fail**

Run: `go test . -run 'TestCIPins|TestWorkflowActions' -v`
Expected: both FAIL — `reading ci.yml: … no such file or directory`.

- [ ] **Step 3: Write the CI workflow**

Create `.github/workflows/ci.yml`:

```yaml
name: CI

on:
  push:
    branches: ['**']
  pull_request:

permissions:
  contents: read

concurrency:
  group: ci-${{ github.ref }}
  cancel-in-progress: true

jobs:
  quality:
    name: quality
    runs-on: ubuntu-latest
    # Without this, a hung job burns the 360-minute default before anyone notices.
    timeout-minutes: 15
    steps:
      - uses: actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1 # v7
      - uses: actions/setup-go@b7ad1dad31e06c5925ef5d2fc7ad053ef454303e # v7
        with:
          go-version-file: go.mod
          cache: true
      - run: make fmt-check
      - run: make vet
      - uses: golangci/golangci-lint-action@ba0d7d2ec06a0ea1cb5fa41b2e4a3ab91d21278a # v9
        with:
          version: v2.12.2
      # The action lints the default GOOS only, so every build-tagged file for
      # another platform is invisible to it — internal/agent/exec_windows.go
      # had never been linted at all until Task 3 checked by hand. The action
      # puts golangci-lint on PATH, so this reuses the binary it installed.
      - name: Lint the build-tagged files the default GOOS never sees
        run: make lint-cross
      - run: make tidy-check
      - run: make cross

  audit:
    name: audit
    runs-on: ubuntu-latest
    timeout-minutes: 20
    permissions:
      contents: read
      security-events: write
    steps:
      - uses: actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1 # v7
      - uses: actions/setup-go@b7ad1dad31e06c5925ef5d2fc7ad053ef454303e # v7
        with:
          go-version-file: go.mod
          cache: true
      # Built with THIS job's toolchain on purpose. Prebuilt analysis
      # binaries abort on toolchain skew ("file requires newer Go version").
      - name: Install pinned tools
        run: make tools
      - name: govulncheck, go mod verify, advisory gosec
        run: make security
      # NOT `-quiet`: with -quiet, gosec writes NO report file at all when it
      # finds nothing, and -no-fail additionally swallows a failed SSA build
      # (it exits 0 having analysed nothing). Combined with continue-on-error
      # on the upload, that chain can go green having performed zero security
      # analysis — masked today only by the tree happening to have ~30
      # findings. Without -quiet the SARIF is written unconditionally, so a
      # missing file becomes a real signal that the artifact step can catch.
      - name: gosec SARIF report
        if: always()
        run: $(go env GOPATH)/bin/gosec -no-fail -fmt sarif -out gosec.sarif ./...
      - name: Upload SARIF to the Security tab
        if: always()
        continue-on-error: true
        uses: github/codeql-action/upload-sarif@5595ccaf912efad79be6eef63a5619ff05969be3 # v4
        with:
          sarif_file: gosec.sarif
      - name: Keep the SARIF as an artifact too
        if: always()
        uses: actions/upload-artifact@043fb46d1a93c77aae656e7c1c64a875d1fc6a0a # v7
        with:
          name: gosec-sarif
          path: gosec.sarif
          # The one step in this chain that is NOT softened, so it is where a
          # gosec that analysed nothing actually surfaces.
          if-no-files-found: error

  test:
    name: test (${{ matrix.os }})
    runs-on: ${{ matrix.os }}
    timeout-minutes: 20
    # Windows and macOS have never run this suite. They start advisory so a
    # red result reports the gap instead of blocking every push; remove the
    # flag per-OS as each goes green. That removal is the definition of done
    # for HANDOFF's "Windows exit-code propagation is unverified" item.
    continue-on-error: ${{ matrix.experimental }}
    strategy:
      fail-fast: false
      matrix:
        include:
          - os: ubuntu-latest
            experimental: false
          - os: macos-latest
            experimental: true
          - os: windows-latest
            experimental: true
    steps:
      - uses: actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1 # v7
      - uses: actions/setup-go@b7ad1dad31e06c5925ef5d2fc7ad053ef454303e # v7
        with:
          go-version-file: go.mod
          cache: true
      # Called directly rather than through make: GNU make is not worth
      # assuming on the Windows runner for a single command.
      - name: Test
        run: go test ./... -count=1
      - name: Race detector (TUI)
        if: matrix.os == 'ubuntu-latest'
        run: make test-race
      - name: Coverage
        if: matrix.os == 'ubuntu-latest'
        # `shell: bash` is LOAD-BEARING and must not be dropped as noise. With
        # no `shell:` key, GitHub runs `bash -e {0}` — WITHOUT pipefail — so
        # `make cover-check | tee` would exit with tee's status, i.e. 0, and
        # the 80% floor would never fail a run. The "below the floor" message
        # would appear in the job summary while the step, job, and run all went
        # green. Writing `shell: bash` explicitly is what selects
        # `bash --noprofile --norc -eo pipefail {0}`.
        shell: bash
        run: |
          echo '## Coverage' >> "$GITHUB_STEP_SUMMARY"
          echo '```' >> "$GITHUB_STEP_SUMMARY"
          # One make invocation, not two: separate calls re-run the `cover`
          # prerequisite and with it the whole suite a second time.
          make cover-check cover-html 2>&1 | tee -a "$GITHUB_STEP_SUMMARY"
          echo '```' >> "$GITHUB_STEP_SUMMARY"
      - uses: actions/upload-artifact@043fb46d1a93c77aae656e7c1c64a875d1fc6a0a # v7
        if: matrix.os == 'ubuntu-latest'
        with:
          name: coverage-html
          path: coverage.html

  machine-independence:
    # Landmine 8. Its own job, not a step inside `test`: this is the check
    # this repo has historically needed most, and a green tick next to the
    # name is the point.
    name: machine-independence (Landmine 8)
    runs-on: ubuntu-latest
    timeout-minutes: 20
    steps:
      - uses: actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1 # v7
      - uses: actions/setup-go@b7ad1dad31e06c5925ef5d2fc7ad053ef454303e # v7
        with:
          go-version-file: go.mod
          cache: true
      - run: make test-isolated
```

- [ ] **Step 4: Write the Dependabot config**

Create `.github/dependabot.yml`:

```yaml
version: 2
updates:
  # Both ecosystems target develop: dependency bumps are unreleased work,
  # and main holds only released code.
  - package-ecosystem: gomod
    directory: /
    schedule:
      interval: weekly
    target-branch: develop
    open-pull-requests-limit: 5
    commit-message:
      prefix: chore(deps)

  - package-ecosystem: github-actions
    directory: /
    schedule:
      interval: weekly
    target-branch: develop
    open-pull-requests-limit: 5
    commit-message:
      prefix: chore(deps)
```

- [ ] **Step 5: Run the tests and watch them pass**

Run: `go test . -run 'TestCIPins|TestWorkflowActions' -v`
Expected: both PASS.

- [ ] **Step 6: Mutation check**

Change `version: v2.12.2` in `ci.yml` to `version: v2.11.0`. Re-run — expect `TestCIPinsTheMakefilesGolangciLintVersion` to FAIL. Restore.
Then change one `uses:` line to a bare `@v7`. Re-run — expect `TestWorkflowActionsArePinnedToShas` to FAIL naming that action. Restore.

- [ ] **Step 7: Validate the YAML locally**

Run: `make lint-workflows`
Expected: no output (actionlint is silent when clean). Fix anything it reports before committing — this is the only chance to catch a syntax error without burning a CI run.

- [ ] **Step 8: Commit**

```bash
git add .github/workflows/ci.yml .github/dependabot.yml workflows_test.go
git commit -m "ci: add the CI workflow and Dependabot config

Four jobs on every branch: quality, audit, a three-OS test matrix
(Windows/macOS advisory until green), and Landmine 8's isolated run as
its own named job. Actions are SHA-pinned and workflows_test.go fails if
one is not."
```

---

### Task 6: Release workflow and the branch guard

**Files:**
- Create: `.github/scripts/check-tag-branch.sh`
- Create: `tagguard_test.go` (package `main`, repo root)
- Create: `.github/workflows/release.yml`
- Modify: `workflows_test.go` (extend the SHA-pin test to cover `release.yml`)

**Interfaces:**
- Consumes: `.goreleaser.yaml` (Task 4); `GORELEASER_VERSION` from the Makefile (Task 2).
- Produces: `.github/scripts/check-tag-branch.sh <tag> [stable-branch] [prerelease-branch]`, exit 0 to allow and non-zero to refuse.

The guard lives in a **script file rather than inline YAML** specifically so it can be tested locally against a throwaway repo. The spec's definition of done says a mis-cut tag must be *verified* refused, not assumed — that is only possible if the logic is callable outside Actions.

- [ ] **Step 1: Write the failing guard test**

Create `tagguard_test.go` at the repo root:

```go
//go:build !windows

package main

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The release workflow's branch guard decides whether a tag may publish.
// Getting it backwards would either block every legitimate release or, worse,
// let a stable tag cut on develop publish as though it came from main. Git
// records no "branch this tag was pushed from", so the guard tests
// reachability — and this test builds a repo whose four tags discriminate
// every case.
//
//	main:    base ────────────────── v0.1.0 (stable, on main)
//	          ├── develop: beta work  v0.2.0-beta.1 (prerelease, on develop)
//	          │                       v0.2.0        (stable, NOT on main)
//	          └── hotfix:  stray work v0.3.0-beta.1 (prerelease, NOT on develop)
func TestTagBranchGuard(t *testing.T) {
	script, err := filepath.Abs(filepath.Join(".github", "scripts", "check-tag-branch.sh"))
	if err != nil {
		t.Fatal(err)
	}

	repo := t.TempDir()
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
		}
	}

	git("init", "-q", "-b", "main")
	git("config", "user.email", "test@example.com")
	git("config", "user.name", "test")
	git("commit", "-q", "--allow-empty", "-m", "base")
	git("tag", "v0.1.0")

	git("checkout", "-q", "-b", "develop")
	git("commit", "-q", "--allow-empty", "-m", "beta work")
	git("tag", "v0.2.0-beta.1")
	git("tag", "v0.2.0")

	git("checkout", "-q", "-b", "hotfix", "main")
	git("commit", "-q", "--allow-empty", "-m", "stray work")
	git("tag", "v0.3.0-beta.1")

	cases := []struct {
		tag   string
		allow bool
		why   string
	}{
		{"v0.1.0", true, "stable tag reachable from main"},
		{"v0.2.0-beta.1", true, "prerelease tag reachable from develop"},
		{"v0.2.0", false, "stable tag cut on develop must be refused"},
		{"v0.3.0-beta.1", false, "prerelease tag not reachable from develop must be refused"},
	}

	for _, tc := range cases {
		t.Run(tc.tag+"/"+map[bool]string{true: "allow", false: "refuse"}[tc.allow], func(t *testing.T) {
			cmd := exec.Command("bash", script, tc.tag, "main", "develop")
			cmd.Dir = repo
			out, err := cmd.CombinedOutput()
			if tc.allow && err != nil {
				t.Fatalf("%s: guard refused a valid tag: %v\n%s", tc.why, err, out)
			}
			if !tc.allow && err == nil {
				t.Fatalf("%s: guard allowed it\n%s", tc.why, out)
			}
		})
	}
}
```

- [ ] **Step 2: Run it and watch it fail**

Run: `go test . -run TestTagBranchGuard -v`
Expected: FAIL — bash cannot find the script.

- [ ] **Step 3: Write the guard script**

Create `.github/scripts/check-tag-branch.sh`:

```bash
#!/usr/bin/env bash
# Enforce the branch model: a stable tag (vX.Y.Z) must be reachable from the
# stable branch, a prerelease tag (vX.Y.Z-beta.N) from the prerelease branch.
#
# Git records no "branch this tag was pushed from" — reachability is the only
# thing that actually exists, so it is what this checks.
#
# The branch names are arguments rather than constants so tagguard_test.go can
# drive this against a throwaway repo with local branches.
#
# Usage: check-tag-branch.sh <tag> [stable-branch] [prerelease-branch]
set -euo pipefail

tag="${1:?usage: check-tag-branch.sh <tag> [stable-branch] [prerelease-branch]}"
stable_branch="${2:-origin/main}"
pre_branch="${3:-origin/develop}"

if [[ "$tag" != v* ]]; then
  echo "refusing: tag '$tag' does not start with 'v'" >&2
  exit 1
fi

# Any hyphen after the version core is a semver prerelease. This matches the
# same tags GoReleaser's `prerelease: auto` treats as prereleases, so the
# guard and the publisher cannot disagree.
# Strip semver build metadata BEFORE looking for a prerelease hyphen: '+build-5'
# legally contains a hyphen while being a STABLE version, and GoReleaser (which
# parses with Masterminds/semver) would publish it as stable. Without this the
# guard calls it a prerelease and would let it through on develop.
core="${tag%%+*}"
if [[ "$core" == *-* ]]; then
  want="$pre_branch"
  kind="prerelease"
else
  want="$stable_branch"
  kind="stable"
fi

if ! git rev-parse --verify --quiet "$want" >/dev/null; then
  echo "refusing: $kind tag '$tag' requires branch '$want', which does not exist" >&2
  exit 1
fi

if git merge-base --is-ancestor "$tag^{commit}" "$want"; then
  echo "ok: $kind tag '$tag' is reachable from '$want'"
  exit 0
fi

echo "refusing: $kind tag '$tag' is not reachable from '$want'." >&2
echo "  stable tags (vX.Y.Z) must be cut on ${stable_branch}." >&2
echo "  prerelease tags (vX.Y.Z-beta.N) must be cut on ${pre_branch}." >&2
exit 1
```

Then: `chmod +x .github/scripts/check-tag-branch.sh`

- [ ] **Step 4: Run the test and watch all four cases pass**

Run: `go test . -run TestTagBranchGuard -v`
Expected: PASS, four subtests.

- [ ] **Step 5: Mutation check — the one that matters most**

Invert the prerelease detection in the script (change `if [[ "$tag" == *-* ]]` to `if [[ "$tag" != *-* ]]`). Re-run. Expected: FAIL on multiple subtests, including `v0.2.0/refuse` — proving the test can tell "refuses a mis-cut stable tag" from "refuses everything". Revert and confirm PASS.

- [ ] **Step 6: Write the release workflow**

Create `.github/workflows/release.yml`:

```yaml
name: Release

on:
  push:
    tags: ['v*']

permissions:
  contents: read

jobs:
  verify:
    name: verify
    runs-on: ubuntu-latest
    timeout-minutes: 15
    steps:
      - uses: actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1 # v7
      - uses: actions/setup-go@b7ad1dad31e06c5925ef5d2fc7ad053ef454303e # v7
        with:
          go-version-file: go.mod
          cache: true
      # A tag never publishes untested code, regardless of what the branch's
      # own CI run said an hour ago.
      - run: make fmt-check
      - run: make vet
      - run: make test

  release:
    name: release
    needs: verify
    runs-on: ubuntu-latest
    timeout-minutes: 30
    permissions:
      contents: write
    steps:
      - uses: actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1 # v7
        with:
          # GoReleaser needs full history for the changelog, and the branch
          # guard needs the remote branch refs.
          fetch-depth: 0
      - name: Enforce the branch model
        run: |
          git fetch --no-tags --quiet origin main || true
          git fetch --no-tags --quiet origin develop || true
          .github/scripts/check-tag-branch.sh "${GITHUB_REF_NAME}"
      - uses: actions/setup-go@b7ad1dad31e06c5925ef5d2fc7ad053ef454303e # v7
        with:
          go-version-file: go.mod
          cache: true
      - uses: goreleaser/goreleaser-action@f06c13b6b1a9625abc9e6e439d9c05a8f2190e94 # v7
        with:
          version: v2.17.1
          args: release --clean
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
          # GORELEASER_CURRENT_TAG is NOT redundant with the tag that triggered
          # this workflow. GoReleaser never reads GITHUB_REF/GITHUB_REF_NAME; it
          # resolves its own tag as
          #     git tag --points-at HEAD --sort -version:refname | head -1
          # and this project's release flow puts TWO tags on one commit: the
          # beta is cut on develop, develop is fast-forwarded into main, then
          # the stable tag is cut on the same commit. That sort puts
          # v0.1.0-beta.1 ahead of v0.1.0 — so pushing the stable tag would
          # rebuild and republish the BETA release, with a green workflow.
          # Measured, not theorised. This pins publisher to trigger.
          GORELEASER_CURRENT_TAG: ${{ github.ref_name }}

      # Added after Task 4's review. goreleaser_test.go pins the ldflag import
      # paths, but it is a raw substring match over the YAML with no structural
      # awareness — a commented-out or misindented ldflag line still passes it,
      # and the binary then ships reporting "dev". Extracting the real artifact
      # and asserting it knows its own tag is the only check that closes that
      # gap, and putting it here makes it continuous rather than something a
      # human remembers to do at release time.
      - name: The published binary must know its own version
        run: |
          set -euo pipefail
          tar -xzf dist/openrouter-launch_*_linux_amd64.tar.gz -C /tmp openrouter-launch
          got=$(/tmp/openrouter-launch --version)
          echo "$got"
          case "$got" in
            *dev*|*none*|*unknown*)
              echo "::error::released binary reports placeholder build info: $got" >&2
              echo "the ldflags never reached it — see .goreleaser.yaml's builds.ldflags" >&2
              exit 1 ;;
          esac
          # Anchored with a trailing space: --version prints
          #   openrouter-launch version <ver> (commit <sha>, ...
          # so an unanchored match lets 0.1.0 be satisfied by 0.1.0-beta.1 —
          # exactly the collision GORELEASER_CURRENT_TAG above prevents, and
          # this is the check that must not fail to notice it.
          case "$got" in
            *"${GITHUB_REF_NAME#v} "*) echo "ok: reports its own tag" ;;
            *) echo "::error::binary reports '$got', which does not contain tag ${GITHUB_REF_NAME}" >&2
               exit 1 ;;
          esac
          # The spec's definition of done requires the commit too, not just the tag.
          case "$got" in
            *"$GITHUB_SHA"*) echo "ok: reports its own commit" ;;
            *) echo "::error::binary reports '$got', which does not contain commit $GITHUB_SHA" >&2
               exit 1 ;;
          esac
```

- [ ] **Step 7: Extend the pin tests to cover release.yml**

**`TestWorkflowActionsArePinnedToShas` already covers `release.yml` — do not edit
it.** Task 5's review replaced its hardcoded one-element file list with
`filepath.Glob(".github/workflows/*.yml")` precisely so a workflow added later
would be covered without anyone remembering to extend a slice. Confirm that by
running it and watching it fail if you temporarily un-pin an action in your new
`release.yml`; that is this step's real work.

Then add:

```go
// Anchored to the `version:` field for the same two reasons its golangci-lint
// sibling is: v2.17.1 is a PREFIX of v2.17.10, and a bare substring is also
// satisfied by the string surviving in a comment after the whole goreleaser
// step is deleted. Both holes were demonstrated on the real tree.
func TestReleaseWorkflowPinsTheMakefilesGoreleaserVersion(t *testing.T) {
	want := makefileVar(t, "GORELEASER_VERSION")
	wf, err := os.ReadFile(".github/workflows/release.yml")
	if err != nil {
		t.Fatalf("reading release.yml: %v", err)
	}
	re := regexp.MustCompile(`(?m)^\s*version:\s*` + regexp.QuoteMeta(want) + `\s*$`)
	if !re.Match(wf) {
		t.Errorf("release.yml has no `version: %s` field pinning goreleaser (the Makefile's GORELEASER_VERSION); `make snapshot` and the published release would be built by different versions", want)
	}
}
```

- [ ] **Step 8: Run everything and validate the YAML**

```bash
go test . -count=1
make lint-workflows
```
Expected: all root-package tests PASS; actionlint silent.

- [ ] **Step 9: Commit**

```bash
git add .github/workflows/release.yml .github/scripts/check-tag-branch.sh tagguard_test.go workflows_test.go
git commit -m "ci: add the tag-triggered release workflow and branch guard

The guard lives in a script, not inline YAML, so tagguard_test.go can
drive all four reachability cases against a throwaway repo — including
the one that matters, a stable tag cut on develop being refused."
```

---

### Task 7: README

**Files:**
- Create: `README.md`

**Interfaces:**
- Consumes: the agent registry (`./orl agents`), the release artifact names from Task 4, the Makefile targets from Task 2.
- Produces: `README.md`, which `.goreleaser.yaml` bundles into every archive.

- [ ] **Step 1: Confirm the agent list against the code, not from memory**

Run: `go run . agents`

The table below was generated from that command on 2026-08-09. If the output differs, the code wins — update the table.

- [ ] **Step 2: Write the README**

Create `README.md`:

````markdown
# openrouter-launch

[![CI](https://github.com/teggen/openrouter-launch/actions/workflows/ci.yml/badge.svg)](https://github.com/teggen/openrouter-launch/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/teggen/openrouter-launch?include_prereleases)](https://github.com/teggen/openrouter-launch/releases)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

Pick a model from OpenRouter's catalog and launch a coding agent already
configured to use it.

```
openrouter-launch                              # interactive: pick an agent, pick a model
openrouter-launch claude -m anthropic/claude-opus-4.6
openrouter-launch models --tools --free --provider anthropic
openrouter-launch profile add --name opus-cc --agent claude --model anthropic/claude-opus-4.6
```

## Zero-touch configuration

**This tool never writes into an agent's own configuration.** Agents are
configured through environment variables, inline-config env content, or CLI
overrides. There is no `--restore` because there is nothing to restore.

It writes exactly four files, and only two of them ever exist for a typical
launch:

| Path | Owner | Holds a secret? |
|---|---|---|
| `$XDG_CACHE_HOME/openrouter-launch/models.json` | this tool | no |
| `$XDG_CONFIG_HOME/openrouter-launch/config.json` | this tool | **yes — mode 0600** |
| `$XDG_CONFIG_HOME/openrouter-launch/openclaw.json` | this tool (openclaw only) | no |
| `~/.factory/settings.local.json` | **Factory Droid** (droid only) | no — the key is an env interpolation |

The fourth is the single sanctioned exception: Factory exposes no environment
variable or flag that selects a custom model, so `droid` gets a capability-gated
writer that touches only its own marker-owned entry and restores the file when
the session ends.

Your OpenRouter API key is read from `OPENROUTER_API_KEY` or from
`config.json`. It is never written anywhere else, and never passed on a command
line.

## Install

Download an archive from [Releases](https://github.com/teggen/openrouter-launch/releases),
extract it, and put `openrouter-launch` on your `PATH`.

| OS | x86-64 | arm64 |
|---|---|---|
| Linux | `openrouter-launch_<v>_linux_amd64.tar.gz` | `openrouter-launch_<v>_linux_arm64.tar.gz` |
| macOS (Intel / Apple Silicon) | `openrouter-launch_<v>_darwin_amd64.tar.gz` | `openrouter-launch_<v>_darwin_arm64.tar.gz` |
| Windows | `openrouter-launch_<v>_windows_amd64.zip` | `openrouter-launch_<v>_windows_arm64.zip` |

Verify your download against `checksums.txt`, then confirm the binary:

```bash
openrouter-launch --version
```

With a Go toolchain:

```bash
go install github.com/teggen/openrouter-launch@latest
```

From source (this is what produces the short `orl` name used throughout the
development docs):

```bash
git clone https://github.com/teggen/openrouter-launch.git
cd openrouter-launch
make build          # produces ./orl
```

If you prefer the short name for a downloaded binary:
`alias orl=openrouter-launch`.

## Supported agents

**Live-verified** means a real completion was produced through OpenRouter using
the installed agent, with a before/after audit confirming nothing was written
into the agent's own config. **Doc-verified** means the mechanism was
established from the agent's documentation and pinned by tests, but has not
been run against a real install.

| Agent | Name | How it is pointed at OpenRouter | Verified |
|---|---|---|---|
| Claude Code | `claude` | environment variables | live |
| Codex CLI | `codex` | managed `-c` overrides plus `-m` | live |
| OpenCode | `opencode` | `OPENCODE_CONFIG_CONTENT` inline JSON | live |
| Pi | `pi` | environment variables | live |
| Hermes Agent | `hermes` | environment variables | live |
| Cline CLI | `cline` | environment variables | live |
| Qwen Code | `qwen` | `--auth-type openai` plus `OPENAI_*` | doc |
| Kimi Code CLI | `kimi` | `KIMI_MODEL_*` environment family | doc |
| Oh My Pi | `omp` | `--model openrouter/<slug>` | doc |
| OpenClaw | `openclaw` | staged config via `OPENCLAW_CONFIG_PATH` | doc |
| Factory Droid | `droid` | marker-owned entry in `settings.local.json`, restored on exit | doc |

`chatgpt`, `claude-desktop`, and `hermes-desktop` are registered as
**unsupported with a stated reason**: a desktop app authenticates through its
own account, so a launcher cannot inject a provider. Running them reports that
reason rather than "unknown agent".

Run `openrouter-launch agents` for the live list, including what is installed.

## Known caveats

- **`droid`'s routing has never been proven.** The check that distinguishes
  "routed through OpenRouter" from "silently billed to your Factory account" —
  launching with a deliberately invalid OpenRouter key and confirming it fails
  with an auth error — has not been run. Do this before using `droid` for
  anything you care about the bill for.
- **`opencode run` can exit 1 after succeeding.** Once opencode's own
  `models.json` cache is populated, it prints the completion and then exits 1
  with `Error: [DecimalError] Invalid argument: [object Object]`. This
  reproduces with a raw `opencode run`, with none of this tool's code involved.
  Do not gate scripts on that exit code.
- **`omp` and `qwen` get no stored-credential warning.** Both can hold
  credentials that outrank the environment (omp's live in a SQLite database,
  which this tool will not take a dependency on to read). Other agents warn
  when a stored credential may shadow the key you passed; these two cannot.
- **`qwen`'s routing can be silently overridden by your own settings.** If
  `~/.qwen/settings.json` has a `modelProviders.openai[]` entry whose `id`
  matches the launched model slug, it may take precedence over this tool's
  `--auth-type openai` plus `OPENAI_*` configuration — the session would then
  not route through OpenRouter, with no warning from this tool. This has never
  been confirmed against a real qwen install; `HANDOFF.md` calls it the most
  consequential open question left after qwen's launcher shipped.
- **Windows is built and tested in CI but has never been run in anger.** Exit
  code propagation in particular is unverified on real Windows.
- **A model that is not `anthropic/*` under Claude Code is advisory, not
  blocked.** You get a warning and a confirm; it works for many models.

## Commands

| Command | What it does |
|---|---|
| `openrouter-launch` | interactive: profiles, agents, model picker |
| `openrouter-launch <agent>` | pick a model for that agent, then launch |
| `openrouter-launch <agent> -m <slug> -- <args>` | launch directly, passing `<args>` through |
| `openrouter-launch agents` | list agents and installation status |
| `openrouter-launch models` | list models; `--tools --free --provider --min-context --max-price` |
| `openrouter-launch profile add\|list\|launch\|rm\|rename` | named agent+model favorites |
| `openrouter-launch --refresh …` | bypass the cached catalog |
| `openrouter-launch --version` | build identity |

## Development

```bash
make help           # every target
make pre-commit     # clean, fmt-check, vet, lint, security, test
make ci             # everything CI runs
make tools          # install the pinned lint/security tools
make build          # ./orl with version info linked in
make snapshot       # build all six release artifacts locally, publish nothing
```

`make tools` installs analysis tools with your local Go toolchain on purpose —
prebuilt binaries break whenever your Go version moves ahead of theirs.

Branches: `develop` is the working branch, `main` holds released code. Stable
tags (`vX.Y.Z`) are cut on `main`, prerelease tags (`vX.Y.Z-beta.N`) on
`develop`; CI refuses a tag cut on the wrong branch.

Versioning is [semantic](https://semver.org/). For this tool that means:
**major** — a command or flag removed or renamed, a breaking `config.json`
schema change, an agent dropped, or a change to the environment contract handed
to a launched agent; **minor** — a new agent, flag, or screen; **patch** —
fixes.

Design docs live in `docs/superpowers/specs/`, implementation plans in
`docs/superpowers/plans/`, and `HANDOFF.md` is the canonical project state —
including a numbered **Landmines** list of invariants that each cost real
debugging. Read it before changing anything that looks like it could be
simplified.

## License

MIT — see [LICENSE](LICENSE).
````

- [ ] **Step 3: Check every internal link and command**

```bash
go run . --help
go run . agents
```
Confirm the command table matches reality, and that `LICENSE`, `HANDOFF.md`,
`docs/superpowers/specs/`, and `docs/superpowers/plans/` all exist.

- [ ] **Step 4: Confirm GoReleaser bundles it**

Run: `make snapshot && tar -tzf dist/openrouter-launch_*_linux_amd64.tar.gz`
Expected: the listing contains `README.md`, `LICENSE`, and `openrouter-launch`.

- [ ] **Step 5: Commit**

```bash
git add README.md
git commit -m "docs: add README

Agent table carries a Verified column distinguishing live-verified from
doc-verified-only, and Known caveats names droid's unrun routing proof
before a user can be billed by it."
```

---

### Task 8: Branch model, doc updates, and live verification

The only task that touches the outside world. Everything before it is verifiable locally; this one proves the machinery actually works.

**Files:**
- Modify: `CLAUDE.md` (Commands section, Workflow section)
- Modify: `HANDOFF.md` (Current state, Landmines, Open items, Verify the tree is sound)

- [ ] **Step 1: Full local gate before anything is pushed**

```bash
make pre-commit
make ci
make snapshot
go test ./... -count=1
```
Expected: all green. Do not proceed past a failure.

- [ ] **Step 2: Update CLAUDE.md**

Replace the shell block under `## Commands` with:

````markdown
```bash
make help                                        # every target
make build                                       # ./orl with version info
make test                                        # full suite
make pre-commit                                  # clean, fmt-check, vet, lint, security, test
make ci                                          # everything CI runs
make tools                                       # install pinned lint/security tools
go test ./internal/tui/ -run TestName -v         # single test
make test-race                                   # race check (TUI package)
make cross                                       # cross-compile check
make test-isolated                               # machine-independence run (Landmine 8)
make snapshot                                    # all six release artifacts, published nowhere
```

`make tools` installs analysis binaries with the local Go toolchain
deliberately: prebuilt ones abort once your Go moves ahead of theirs.
````

Replace the `## Workflow` first paragraph with:

```markdown
`develop` is the working branch; `main` holds released code. Stable tags
(`vX.Y.Z`) are cut on `main`, prerelease tags (`vX.Y.Z-beta.N`) on `develop`,
and `.github/scripts/check-tag-branch.sh` refuses a tag cut on the wrong
branch. (Through Phase 4b this was direct-to-main with no branches at all;
that changed when CI landed.)
```

- [ ] **Step 3: Update HANDOFF.md**

1. In **Open items**, delete the final two bullets — `No README.md yet.` and `No CI. No release/packaging story.`
2. Add three Landmines, continuing the numbering:

```markdown
**25. Go toolchain skew breaks the analysis tools in BOTH directions, and
the two have opposite remedies.**

*Direction A — tool older than the tree.* A prebuilt binary built by an older
Go cannot parse a newer stdlib: `staticcheck`, `govulncheck`, and golangci-lint
v1.64.8 all aborted on this machine with `file requires newer Go version
go1.26 (application built with go1.25)` before analysing a single line. `make
tools` `go install`s them instead, and CI does the same inside the job rather
than downloading prebuilt binaries. If a tool suddenly refuses to run after a
Go upgrade, this is why — re-run `make tools`, do not pin Go backwards.

*Direction B — the tree's Go older than the tool.* Every one of these tools
now declares `go >= 1.25` in its own `go.mod`, while this project's floor is
`go 1.24.0`. `actions/setup-go` injects `GOTOOLCHAIN=local`, so CI's audit job
died on `requires go >= 1.25.0 (running go 1.24.0; GOTOOLCHAIN=local)` — with
no change to this repo, because three of the four are installed at `@latest`.
The remedy is the opposite of Direction A's: `make tools` sets
`GOTOOLCHAIN=auto` so Go fetches a newer toolchain *solely to build the tools*.
What builds and tests this project is unchanged — `go-version-file: go.mod`
still pins that to the declared floor. Do not "fix" a Direction-B failure by
raising `go.mod`'s floor; that would silently drop support for users on 1.24
to satisfy a linter's build requirement.

**26. `.golangci.yml` must stay on the v2 schema.**
`golangci-lint-action` v9 rejects v1 configs outright ("golangci-lint v1 is
not supported by golangci-lint-action >= v7"). v2 moved
gofmt/goimports/gofumpt into a `formatters:` section and folded `gosimple`
into `staticcheck`. If a locally installed v1 binary rejects the config,
upgrade the binary — downgrading the config turns a local inconvenience into
a broken pipeline. `make lint` refuses to run a v1 binary for this reason.

**27. Landmine 8's isolated run must derive the Go bin directory, not
hardcode `/usr/local/go/bin`.** The documented command hardcoded that path;
on a CI runner Go lives in a tool cache, so the hardcoded form strips `go`
itself out of `PATH` and the target fails for a reason that has nothing to do
with machine-independence. `make test-isolated` uses `dirname $(command -v
go)` and keeps the rest of the stripping intact — the point is hiding
`~/.local/bin`, not hiding the toolchain.
```

3. In **Verify the tree is sound**, add above the existing block:

```markdown
`make ci` runs all of the below in one command, and is exactly what
`.github/workflows/ci.yml` invokes.
```

4. In **Current state**, add rows:

```markdown
| CI | `.github/workflows/ci.yml` — quality, audit, three-OS test matrix (Windows/macOS advisory), machine-independence; all branches |
| Releases | tag-driven via GoReleaser; six 64-bit targets; stable tags on `main`, `-beta.N` on `develop`, guard-enforced |
```

- [ ] **Step 4: Commit the docs**

```bash
git add CLAUDE.md HANDOFF.md
git commit -m "docs: record the branch model, make targets, and three CI landmines

Closes HANDOFF's last three open items (no README, no CI, no release
story) and records the toolchain-skew, golangci-v2-schema, and
isolated-PATH gotchas that each cost investigation."
```

- [ ] **Step 5: Push `develop` and watch CI run for the first time**

`develop` already exists and is checked out (see Global Constraints). This push
is the first time any of these workflows have ever executed.

```bash
git push origin develop
gh run watch
```

Expected: `quality`, `audit`, `machine-independence`, and `test (ubuntu-latest)` green. `test (macos-latest)` and `test (windows-latest)` may be red — that is the tracked signal, not a blocker.

- [ ] **Step 6: Record what the advisory legs actually reported**

```bash
gh run view --log-failed > /tmp/ci-first-run.log
```

Add a short bullet to HANDOFF.md's Open items naming the real failures, for
example: *"macOS: green. Windows: N tests fail, all path-related (`/tmp`
literals in …)."* This converts a guess into evidence — it is the whole reason
the legs were made advisory rather than skipped.

```bash
git add HANDOFF.md
git commit -m "docs: record the first CI run's Windows/macOS results"
git push origin develop
```

- [ ] **Step 7: Cut the first beta and verify the prerelease path**

```bash
git tag v0.1.0-beta.1
git push origin v0.1.0-beta.1
gh run watch
```

Then verify, in this order:

```bash
gh release view v0.1.0-beta.1                       # must be marked Pre-release
cd "$(mktemp -d)"
gh release download v0.1.0-beta.1 -R teggen/openrouter-launch -p '*linux_amd64*'
tar -xzf openrouter-launch_*_linux_amd64.tar.gz
./openrouter-launch --version
cd -
```

Expected: six archives plus `checksums.txt` attached; the release marked
**Pre-release**; `--version` reporting `v0.1.0-beta.1` and the real commit SHA —
**not** `dev`, **not** `none`. A `dev` here means the ldflags never reached the
binary and the release is unusable, regardless of everything else being green.

- [ ] **Step 8: Verify the guard refuses a mis-cut tag — on the real workflow**

`develop` carries every commit from Tasks 1–7 and `main` carries none of them,
so a **stable** tag cut here is exactly the mistake the guard exists to catch:

```bash
git tag v0.9.9            # stable-shaped, but on develop-only history
git push origin v0.9.9
gh run watch
```

Expected: the `release` job **fails** at "Enforce the branch model" with
`refusing: stable tag 'v0.9.9' is not reachable from 'origin/main'`, and **no
GitHub release is created**. Confirm both:

```bash
gh release view v0.9.9 || echo "correct: no release was created"
```

Then clean up completely:

```bash
git push origin :refs/tags/v0.9.9
git tag -d v0.9.9
```

This is the spec's "verified, not assumed" requirement. `tagguard_test.go`
already proved the logic in isolation; this proves the workflow wiring.

- [ ] **Step 9: Fast-forward main and cut the stable release**

```bash
git checkout main
git merge --ff-only develop     # main is strictly behind, so this must succeed
git push origin main
gh run watch                    # CI on main for the first time
git tag v0.1.0
git push origin v0.1.0
gh run watch
```

Expected: the merge is a clean fast-forward (if git refuses it, stop — something
committed to `main` behind your back, and the guard's assumptions no longer
hold); then a normal, **not** prerelease, release with the same six archives and
`--version` reporting `v0.1.0`.

- [ ] **Step 10: Final state check**

```bash
gh release list
git log --oneline --decorate -5
gh run list --limit 5
```

Expected: `v0.1.0` (Latest) and `v0.1.0-beta.1` (Pre-release); no stray
`v0.9.9`; recent runs green except the advisory legs.

- [ ] **Step 11: Update HANDOFF.md with the final state and push**

Set `**Last updated:**` to today and `**State:**` to note that CI/CD and the
first release shipped. Record the beta and stable tags, and the fact that
`develop` is now the working branch.

The handoff commit belongs on `develop` — it is ordinary work, and `main` should
move only through a release merge:

```bash
git checkout develop
git merge --ff-only main        # pick up the v0.1.0 tag's commit if main moved
git add HANDOFF.md
git commit -m "docs: hand off after the CI/CD and README phase"
git push origin develop
```

Leave `main` at `v0.1.0`. From here on, work happens on `develop` and reaches
`main` only when a release is cut.

---

## Plan self-review

**Spec coverage.** Every spec section maps to a task: Makefile contract → Task 2; `ci.yml` four jobs → Task 5; `release.yml` + branch guard → Task 6; `.goreleaser.yaml` → Task 4; `internal/version` + the mandatory ldflag pin test → Tasks 1 and 4; Dependabot → Task 5; supply-chain SHA pinning → Task 5 (test) and Task 6 (extended to `release.yml`); README with the Verified column → Task 7; MIT license → Task 1; gotchas A/B/C → Task 8 as Landmines 25–27; every definition-of-done item → Task 8 Steps 5–10.

**One thing the spec did not anticipate**, found while writing Task 2 and now folded in as Landmine 27: CLAUDE.md's literal `PATH="/usr/local/go/bin:/usr/bin:/bin"` cannot be copied into CI, because `setup-go` installs Go into a tool cache and the hardcoded path would strip `go` itself. `make test-isolated` derives the directory instead.

**Placeholder scan.** No TBD/TODO. The one genuinely unknown quantity — how many findings golangci-lint v2 reports on a tree no v2 linter has ever seen — is Task 3 Step 4, which gives a three-branch triage rule with a worked ordering rather than "fix any issues".

**Type consistency.** `version.Version`/`Commit`/`Date` and `version.String()` are used identically in Tasks 1, 4, and 7. `makefileVar` is defined in Task 5 and reused by Task 6's added test in the same file. `GOLANGCI_VERSION`/`GORELEASER_VERSION` are spelled identically in the Makefile (Task 2) and both parity tests (Tasks 5, 6). The guard script's argument order — `<tag> [stable-branch] [prerelease-branch]` — matches its invocation in `tagguard_test.go` and in `release.yml`.
