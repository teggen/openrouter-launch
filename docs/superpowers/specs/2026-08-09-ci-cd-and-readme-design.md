# CI/CD, Makefile, and README — design

**Date:** 2026-08-09 · **Status:** approved, ready to plan

Closes the last three entries in HANDOFF.md's Open items: *"No `README.md` yet.
No CI. No release/packaging story."*

## Why now

The tool ships eleven agents and 432 tests, and none of it is mechanically
verified by anything other than the owner running commands by hand on one
Linux machine. Three consequences:

- **Machine-dependence is invisible.** Landmine 8 exists because tests passed
  here and would have failed elsewhere. The `HOME`-isolated run that catches
  that class is documented but only ever run manually.
- **Nothing is downloadable.** A public repo with zero tags and no artifacts
  means the only install path is `git clone && go build`.
- **The binary cannot say what it is.** There is no version string anywhere in
  the CLI, so a released artifact is unfalsifiable — you cannot ask it which
  commit it came from.

## Decisions taken (and the alternatives rejected)

| Decision | Chosen | Rejected, and why |
|---|---|---|
| Version source | **Manual git tag** drives build + publish | Conventional-commit automation: the history already carries `feat:`/`fix:` prefixes so it would work, but it makes commit messages load-bearing — a mistyped prefix silently changes the release number |
| Branch model | **`develop` is the working branch; `main` holds released code** | Status quo (direct-to-main) cannot express a beta line |
| Test platforms | **ubuntu + macos + windows**, the latter two `continue-on-error` until green | Linux-only: preserves the "Windows exit-code propagation unverified" open item forever |
| Coverage | **GitHub-native**: job summary, artifact, 80% floor | Codecov: adds a third-party app with repo access to a tool whose entire design principle is not depending on things it does not control |
| Lint gate | **Curated blocking set; gosec advisory via SARIF** | Blocking gosec: 26 findings, near-all inherent to a tool that launches subprocesses and writes an XDG config |
| Build targets | **6** — {linux, darwin, windows} × {amd64, arm64} | 386: Go cannot build darwin/386 at all, and every agent this tool launches ships 64-bit-only installers |
| Release tool | **GoReleaser** | Hand-rolled matrix: ~120 lines of YAML reimplementing archive/checksum/prerelease logic, which is the part most likely to carry a quiet bug |
| License | **MIT** | Apache-2.0's patent grant buys nothing for a CLI wrapper |
| First tag | **`v0.1.0-beta.1` on `develop`**, then `v0.1.0` on `main` | `v1.0.0` would promise a stability the Open items say does not exist |

## Architecture

The Makefile is the **single source of truth for every check**. CI jobs call
`make <target>`; they do not re-spell the commands in YAML. This is the design's
load-bearing choice: it makes "green locally" and "green in CI" the same claim,
rather than two similar-looking claims that drift.

```
Makefile ──────────────┬──> developer runs `make pre-commit`
                       └──> ci.yml runs the same targets
.golangci.yml    ──> make lint
.goreleaser.yaml ──> make snapshot (local) / release.yml (tagged)
internal/version ──> ldflags target for both
```

### The Makefile contract

Target names are **not free choice**. The owner's `/quality` command already
specifies a contract, and the Makefile must satisfy it or the command breaks:

> `make pre-commit` … includes `clean`, `fmt-check`, `vet`, `lint`, `security`,
> `test` … `make fmt` to auto-format, `make test-unit` to run only unit tests,
> `make lint` to see linting issues in detail

`/preflight` additionally expects a Go project to reach **80% coverage** — the
same floor chosen here independently, so the two agree.

**Required by the existing contract:** `clean`, `fmt`, `fmt-check`, `vet`,
`lint`, `security`, `test`, `test-unit`, `pre-commit`.

**Added for this project:**

| Target | What it does | Why it exists |
|---|---|---|
| `build` | `go build` with version ldflags | the local `orl` binary, now self-identifying |
| `test-race` | `go test ./internal/tui/ -race` | CLAUDE.md's documented race command |
| `test-isolated` | `HOME=$(mktemp -d) PATH=… go test ./...` | **Landmine 8** — the machine-independence run, promoted from a documented manual step to a first-class target |
| `cover` / `cover-html` | profile + per-package table / HTML report | |
| `cover-check` | fails under 80% | the floor lives here, not in YAML, so it cannot differ between local and CI |
| `tidy-check` | `go mod tidy` then `git diff --exit-code` | catches dependency drift committed by accident |
| `cross` | `GOOS=windows`/`GOOS=darwin` build | CLAUDE.md's cross-compile check |
| `writesites` | the Landmine 6 write-primitive grep | already a test; the target is for auditing by eye |
| `tools` | `go install` pinned tool versions | see *Toolchain skew* below — this is the fix, not a convenience |
| `snapshot` / `release-check` | `goreleaser build --snapshot` / `goreleaser check` | proves the release config before any tag exists |
| `ci` | everything CI runs, one command | |
| `help` | default target, self-documenting | |

`test-unit` is an alias for `test`: this project has no unit/integration split,
and inventing one to satisfy a target name would be worse than aliasing.

### `ci.yml` — all branches, all PRs

Triggers on `push: branches: ['**']` and `pull_request`, with a concurrency
group that cancels superseded runs on the same ref. Default
`permissions: contents: read`, elevated only where needed.

| Job | Runner | Contents |
|---|---|---|
| `quality` | ubuntu | `make fmt-check vet lint tidy-check cross` |
| `audit` | ubuntu | `make security`, then gosec → SARIF → Security tab |
| `test` | ubuntu **+ macos + windows** | `go test ./... -count=1`; Linux leg also `make test-race cover cover-check` |
| `machine-independence` | ubuntu | `make test-isolated` |

**What `make security` does, exactly**, since the `/quality` contract describes
it as "gosec and govulncheck" but the two have different severities here:
`govulncheck ./...` and `go mod verify` are **blocking** — a known CVE in a
dependency, or a tampered module, should stop the build. `gosec` runs in the
same target but is **advisory**: it prints its findings and does not fail the
target. The `audit` job then re-runs gosec with `-fmt sarif` and uploads the
result to the Security tab, so all 26 findings stay visible and triageable
without gating merges on what are near-certainly false positives for a tool
whose job is launching subprocesses.

`machine-independence` is a **separately named job**, not a step inside `test`,
because it is the check this repo has historically needed most and a green tick
next to its name is the point.

The Windows and macOS legs carry `continue-on-error: true` at first. This is
expected to be red: 22 of the 50 test files reference `/tmp`, `/usr`, `/bin`, or
`/dev/null`. The escape hatch converts a documented unknown into a tracked
signal; it is removed once the legs pass, and that removal is the definition of
done for the Windows open item.

Windows and macOS legs call `go test` directly rather than `make`, because GNU
make is not a dependency worth assuming on those runners for a single command.

### `release.yml` — tag-triggered, branch-guarded

Triggers on `push: tags: ['v*']`.

1. **`verify`** — the full suite on Linux. A tag never publishes untested code,
   regardless of what the branch's own CI said an hour ago.
2. **`release`** (`needs: verify`, `permissions: contents: write`) — checkout at
   `fetch-depth: 0`, then the branch guard, then GoReleaser.

**The branch guard** is the mechanism that implements "main for releases,
develop for betas":

- a tag **without** a prerelease suffix must be reachable from `origin/main`;
- a tag **with** `-beta.N` must be reachable from `origin/develop`;
- otherwise the job fails, naming the mismatch.

Reachability (`git branch -r --contains`) is checked rather than "which branch
was the tag pushed from", because the latter is not a thing git records.

GoReleaser then does the rest. `prerelease: auto` reads the `-beta.N` suffix off
the tag and marks the GitHub release accordingly — so the release/prerelease
split falls out of the tag itself, and the guard's only job is refusing a tag
cut on the wrong line.

### `.goreleaser.yaml`

`CGO_ENABLED=0`, `-trimpath`, `mod_timestamp` for reproducible builds, and
ldflags injecting version/commit/date. `goos: [linux, darwin, windows] ×
goarch: [amd64, arm64]` yields exactly the six targets with no ignore rules.
tar.gz for Unix and zip for Windows via `format_overrides`; README and LICENSE
bundled into each archive; `checksums.txt`; changelog grouped from the existing
conventional-commit prefixes.

The binary is named `openrouter-launch`, matching every invocation in the docs.
README documents `alias orl=openrouter-launch` for the shorter local form.

### `internal/version`

A new package holding `Version = "dev"`, `Commit`, and `Date`, overwritten at
link time. `internal/cli/root.go` sets `Version: version.String()`, and cobra
generates `--version` from it. Source builds honestly report `dev`.

`root_test.go` asserts nothing about help text, so adding the field is safe.

**One test is mandatory here:** the ldflag paths in `.goreleaser.yaml` must be
pinned against the real symbol names. Otherwise renaming the package silently
reverts every future release to `dev` while everything still builds, passes, and
publishes — a defect that announces itself only to a user reading `--version`.
This is the repo's recurring *passes-for-the-wrong-reason* failure mode, in a
new location.

### `.github/dependabot.yml`

`gomod` and `github-actions` ecosystems, weekly, `target-branch: develop`,
`chore(deps)` commit prefix, open-PR limit 5. Targeting `develop` follows from
the branch model: dependency bumps are unreleased work.

### Supply chain

Every third-party action is pinned to a **full commit SHA**, not a tag. This
tool writes an API key to disk at 0600 and the release workflow holds
`contents: write`; a mutable tag on that workflow is the wrong risk to accept.
Pins carry a trailing `# vX.Y.Z` comment so Dependabot can still bump them.

## README

Structure: badges → what it is → the zero-touch principle → install (download
table, `go install`, from source) → quickstart → **agent table with a Verified
column** → configuration, the four write sites, 0600 → known caveats →
development, with links into `docs/superpowers/` and HANDOFF.md.

The agent table's Verified column distinguishes **live-verified** (claude,
codex, opencode, pi, hermes, cline) from **doc-verified-only** (qwen, kimi, omp,
openclaw, droid). Known caveats names `opencode run`'s exit-1-on-success
upstream bug, the unrun droid routing proof, the omp/qwen credential-shadow
gaps, and Windows.

Publishing droid's caveat is not self-flagellation: if droid is misconfigured it
silently bills Factory instead of routing through OpenRouter, so a user choosing
it needs the warning *before* the bill, not after.

## Gotchas found while designing this

Recorded because each already cost investigation, in the style HANDOFF.md's
Landmines use.

**A. Prebuilt Go analysis tools break on toolchain skew, and every one on this
machine is broken right now.** `staticcheck`, `govulncheck`, and
`golangci-lint` v1.64.8 are all built against Go 1.25; the local toolchain is
1.26.5; all three abort with `file requires newer Go version go1.26
(application built with go1.25)` before analysing a single line of this repo.
`gosec` and `go vet` still work. The consequence for CI is structural: tools are
`go install`ed with the job's own `setup-go` toolchain rather than downloaded as
prebuilt binaries, so the same skew cannot recur when Go ticks a version. `make
tools` does the same locally, and running it is what will un-break this machine.

**B. `golangci-lint-action` v9 refuses golangci-lint v1, so `.golangci.yml`
must use the v2 schema — and the locally installed binary will reject it.**
From the action's own source: *"golangci-lint v1 is not supported by
golangci-lint-action >= v7"*. The v2 schema requires `version: "2"`, moves
`gofmt`/`goimports`/`gofumpt` out of `linters.enable` into a separate
`formatters:` section, and merges `gosimple` into `staticcheck`. A future
session that "fixes" the config back to v1 shape because the local v1.64.8
binary rejects it will break CI instead. The fix is to upgrade the local binary
(`make tools`), never to downgrade the config.

**C. GoReleaser v2 renamed `archives.format` to `archives.formats`,** now a
list, as of v2.6; the singular form is deprecated. Same rename inside
`format_overrides`.

## Out of scope

- Homebrew tap (needs a second repo and a cross-repo PAT).
- Container images, Linux packages (deb/rpm), signing/SBOM.
- Fixing the Windows/macOS test failures CI is expected to expose. This phase
  *surfaces* them; fixing them is its own work, informed by what CI reports.
- Closing any of the five doc-verified-only agents' live gates.

## Definition of done

- `make pre-commit` green locally; `make snapshot` produces six archives plus a
  checksums file.
- `ci.yml` green on `develop` for `quality`, `audit`, `machine-independence`,
  and the ubuntu `test` leg; Windows/macOS legs report their real state.
- `v0.1.0-beta.1` on `develop` publishes a **prerelease** with six archives, and
  the downloaded binary's `--version` reports that exact tag and commit.
- `v0.1.0` on `main` publishes a normal release the same way.
- A stable tag cut on `develop` is **refused** by the branch guard — verified,
  not assumed.
- CLAUDE.md and HANDOFF.md updated: branch model, make targets, and the three
  closed Open items.
