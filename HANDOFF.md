# openrouter-launch — Handoff

**Last updated:** 2026-08-10 · **State:** **the models table sorts, on `develop`, unreleased.** Every catalog column is sortable — MODEL, CONTEXT, INPUT/M, OUTPUT/M, TOOLS — from the picker's `ctrl+f` screen (renamed **Filter & Sort**, two new rows: *Sort by* and *Direction*) and from `orl models --sort <col> [--desc]`. The ordering primitive is `openrouter.SortModels` (`internal/openrouter/sort.go`), shared by both surfaces, stable, and composed OUTSIDE `Rank` so a chosen column beats search relevance; models with unparseable pricing sort last in BOTH directions (Landmine 38). The choice persists under a new top-level `sort` key in `config.json`. Shipped with it: the price columns are retitled **INPUT/M** and **OUTPUT/M** (the `Model` fields keep OpenRouter's `pricing.prompt`/`pricing.completion` wire names). Before that: **listing tables landed on `develop`, unreleased.** Every listing — `orl agents` (with and without `--all`), `orl profile list`, `orl models`, and the TUI root screen — now renders as a bordered `lipgloss/table` with a dedicated status column (`✓ installed` / `✗ not installed` / `⚠ unsupported` / `⚠ unknown agent`), color auto-detected from the destination writer, and a new shared `internal/ui` package so the CLI and the TUI cannot drift. `agents --all` wraps to 100 columns instead of the 227-column line it used to emit, the root screen gained a measured scroll window because boxing pushed it past a 24-line terminal, and the TUI model picker renders the same catalog table, shedding columns on a narrow terminal. A pre-existing chrome overflow was fixed with it: the picker's key footer was a fixed 85 columns and overflowed even an 80-column terminal (Landmines 30-35). Before that: **CI/CD phase complete and `v0.1.1` is released** — the project has a Makefile, GitHub Actions CI, tag-driven GoReleaser releases, a README, an MIT licence, and three public releases (`v0.1.0-beta.1` prerelease on `develop`, then `v0.1.0`, then `v0.1.1`, both on `main`). All three were verified by extracting the published archive and running `--version`. `v0.1.1` is the code-scanning triage: five gosec permission findings fixed, fourteen dismissed as false positives, **zero open alerts** (Landmine 29). This sits on top of Phase 4 (4a + 4b), which shipped all eight Tier 2 launchers (pi, hermes, qwen, cline, kimi, omp, openclaw, droid); pi/hermes/cline live-verified, the other five doc-verified-only — their live gates were skipped by owner decision, droid's routing proof most importantly (see Open items)

Read this first if you are picking the project up with no prior context.

## What this is

A Go CLI that picks a model from OpenRouter's catalog and launches a coding agent
(currently Claude Code) already configured to use it, plus named agent+model
favorites called profiles.

It is modelled on `ollama launch` (`cmd/launch/` in the ollama repo, which the
user has checked out at `/home/martin/projects/ollama`). Two ideas were borrowed:
a **declarative registry** of integrations, and a **thin required interface plus
optional capability interfaces** detected by type assertion — that is what keeps
N agents from collapsing into one switch statement.

**The one deliberate divergence from ollama:** `ollama launch` writes provider
profiles into each agent's own config files and ships a `--restore` to undo it.
This tool never does that. Every agent is configured through environment
variables, inline-config env content, or CLI overrides. This is the **zero-touch
principle** and it is the design's central claim — see Landmine 6.

## Current state

| | |
|---|---|
| Repo | `github.com/teggen/openrouter-launch` (HTTPS remote, `gh` credential helper) |
| Branch | `develop` is the working branch; `main` holds released code. (Through Phase 4b this was direct-to-main with no branches at all — that changed when CI landed.) |
| Phase 1 | Complete: 27 commits, 137 tests, ~1,570 LOC + ~2,510 test LOC |
| Phase 2 | Complete: root screen, model picker, filters, profile save, API-key prompt. Filters moved to a `ctrl+f` screen on 2026-08-10 (Landmine 37); that screen became **Filter & Sort** later the same day when column sorting landed (Landmine 38). |
| Phase 3 | Complete: codex + opencode launchers, Tier 3 registry, live-verified against OpenRouter |
| Phase 4a | Complete: six Tier 2 launchers — pi, hermes, qwen, cline, kimi, omp — plus shared passthrough-conflict helpers (`internal/agent/args.go`) and the `CredentialShadowCheck` advisory capability (`WarnShadowedCredential`). Live-gated end to end through the built binary: pi, hermes, cline (Task 9). Doc-verified-only, gate skipped by owner scope: qwen, kimi, omp. **Five of the six are zero-touch; cline is not, as of 2026-08-09** — env-only delivery cannot configure an interactive cline session, and its Task 9 gate could not see that because it ran one-shot prompts against a virgin `~/.cline` (both conditions hide the two mechanisms; see Landmine 36). cline is now `-k` on argv plus a snapshot/restore `ConfigWriter` (write site 5). |
| Phase 4b | Complete: the `Staged` capability (write site #3, launcher-owned files, boundary-checked in `stageFiles`), `openclaw` (a `Staged` consumer sharing omp's `openrouter/`-prefix dialect), the fork-and-wait launch path (`agent.RunWait` + `launch.launchConfigWriter`), and `droid` (the first `ConfigWriter`, write site #4, marker-owned entry in `~/.factory/settings.local.json`). Task 5's live gates for both new agents were skipped by owner decision (2026-08-09) — openclaw and droid ship doc-verified-only, same posture as qwen/kimi/omp. **Tier 2 is now complete: all eight agents shipped.** |
| Tests | **544** total, verified by both counting methods below. The models-table sort added 25: 7 in `internal/openrouter` (the five columns in both directions, unknown-pricing-last, stability, no-mutation, the zero Sort, `ParseSortKey`, `SortKeys`), 2 in `internal/ui` (the header rename, `SortLabel` against `ModelHeaders`), 1 in `internal/config`, 2 in `internal/launch` (`SortFrom` degrading, `MergeSort`'s changed-predicate), 4 in `internal/cli` (`--sort output` both directions, the rejected typo, the tolerated bad config value, the persisted sort), and 9 in `internal/tui` (3 filter-state, 2 picker — composition outside `Rank` and surviving a search edit, 3 filter&sort screen, 1 driver persistence). It was **519** before. Earlier: the filters-screen change (Landmine 37) added 23 net: 2 for `escLatch`, +7/−6 on the picker (three `ctrl+f` cases, the split chord, the deferred esc, the footer, and the alt chords collapsing into one inertness test, against the five alt-chord tests deleted), 11 for the filters screen, 4 driver tests for the `ctrl+f` round trip, and 5 headless `liveScreens` tests (the split-read regression, the raw 0x06 ctrl+f byte, and three for the new closure). It was **496** before — this row said 495, which was off by one and is corrected here; both counting methods now agree on 519. The cline `-k` fix (Landmine 36) added 6 net — 7 new (argv key, the `ConfigWriter` assertion, four `Apply`/restore cases) minus the deleted `TestClineShadowedCredential`, whose premise the fix invalidated. It was 489 before. The listing-tables change added 30 net — the CLI/root-screen pass added 26 (10 `internal/ui`, 11 `internal/cli`, 5 `internal/tui`), then the picker pass added 4 more and moved three catalog-rendering tests from `internal/tui` to `internal/ui` with `ModelCells`, the chrome-overflow fix added 4, and the Windows fix added 1 (`TestTestHomeIsolatesTheHomeDirectoryOnEveryPlatform`). It was 454 before. Earlier history: 454, 452 after the code-scanning triage (which added 4 permission tests: config dir, cache file+dir, staged-file dir, `~/.factory`); the `agents` listing change then added 3 CLI tests and 1 TUI test and **deleted 2 TUI tests** whose contract it reversed — the first drop in the tui count since Phase 3. Count with `go test ./... -list '.*' \| grep -c '^Test'` (or `grep -rc '^func Test' --include='*_test.go' .` — both agree). 436 when the CI/CD phase started; it added 12 (`internal/version` ×3, Makefile contract, workflow pins ×3, GoReleaser, tag guard ×2, gosec analysis guard ×2). The "432" this row used to claim was accurate at the Phase 4 handoff and went stale before the phase began; "446" (the count as of the final fix wave's own handoff) went stale within that same commit, since the fix wave's `gosecguard_test.go` added the two gosec-guard tests it is counted from. |
| Verification | `make ci` is the one command — fmt, vet, lint (3 GOOS), actionlint on the workflows, tidy, cross-build, security, race, 86.2% coverage vs an 80% floor, and the Landmine 8 isolated run. Green locally 2026-08-10 after the models-table sort (86.6% coverage vs the 80% floor) and, before it, after the filters-screen change (Landmine 37); last confirmed in GitHub Actions on run `31338565751` (all six jobs, Windows included) and again by the `v0.2.0` release workflow. It is the *mechanical* gate only; the live-API smoke test under "Verify the tree is sound" is manual — and cline's own gate is now interactive, per Landmine 36. |
| Agents shipped | claude, codex, opencode, plus all eight Tier 2 agents (pi, hermes, qwen, cline, kimi, omp, openclaw, droid); 3 desktop apps (chatgpt, claude-desktop, hermes-desktop) registered unsupported |
| CI | `.github/workflows/ci.yml` — quality, audit, three-OS test matrix (**all three blocking** as of 2026-08-09: Windows' 19 platform-fixture failures are closed and its `experimental` flag is off), machine-independence; all branches |
| Releases | tag-driven via GoReleaser; six 64-bit targets; stable tags on `main`, `-beta.N` on `develop`, guard-enforced. **Shipped: `v0.1.0-beta.1` (Pre-release), `v0.1.0`, `v0.1.1`, and `v0.2.0` (Latest)**, all 2026-08-09, six archives + `checksums.txt` each. `v0.1.1` and `v0.2.0` both went straight to stable with no beta (owner decision), so on each only ONE tag sits on the commit and the `GORELEASER_CURRENT_TAG` collision below could not arise. `v0.2.0` is a **minor** bump, not the patch first proposed: `main` had fallen 20 commits behind, so the release carried the whole listing-tables redesign (9 `feat` commits) and the Windows fixes alongside the cline `-k` fix, and the `-k` change itself rejects a passthrough flag that used to be allowed. Check what `main..develop` actually holds before picking the next number |
| Go floor | `go 1.25` — a **security** floor, not a dependency floor (Landmine 25, third clause). Minor-only on purpose so `setup-go` resolves the newest patch; it resolved `go1.25.12` on the first run. |
| Pushed | Yes, both branches. `origin/main` is at the `v0.2.0` commit `70970d8`; `origin/develop` is **one commit ahead**, and that one commit is this handoff update itself — the fast-forward left the branches level and then writing this row un-levelled them, which is the whole reason this row keeps going stale. No unreleased *code* is outstanding. Check `git status -sb` and `git log --oneline main..develop` rather than trusting this row; it has been wrong before, and it went stale twice within an hour of being written. |

Working commands, all smoke-tested against the live API:

```bash
openrouter-launch agents          # launchable agents only
openrouter-launch agents --all    # ...plus the unsupported desktop apps, with reasons
openrouter-launch models --tools --free --provider anthropic
openrouter-launch models --min-context 200000 --max-price 5
openrouter-launch claude -m anthropic/claude-opus-4.6 -- --resume
openrouter-launch profile add --name opus-cc --agent claude --model anthropic/claude-opus-4.6
openrouter-launch profile list|launch|rm|rename
openrouter-launch codex -m openai/gpt-4o-mini -- exec --skip-git-repo-check "…"
openrouter-launch opencode -m openai/gpt-4o-mini -- run "…"  # exit code caveat — see Open items

# Phase 4a — Tier 2. Forms below are what Task 9 actually ran through the
# built binary; qwen/kimi/omp were never installed this phase (owner scope)
# and ship doc-verified-only — the forms shown for them are inferred from
# their Command() implementations, not smoke-tested.
openrouter-launch pi -m openai/gpt-4o-mini -- -p "…"       # live-verified, Task 9 (pi 0.80.3)
openrouter-launch hermes -m openai/gpt-4o-mini -- -q "…"   # live-verified, Task 9 (Hermes Agent v0.20.0)
openrouter-launch cline -m openai/gpt-4o-mini -- "…"       # live-verified, Task 9 (Cline CLI 3.0.52)
openrouter-launch qwen -m openai/gpt-4o-mini -- -p "…"     # doc-verified only — gate skipped (owner scope)
openrouter-launch kimi -m moonshotai/kimi-k3               # doc-verified only — gate skipped (owner scope)
openrouter-launch omp -m openai/gpt-4o-mini -- -p "…"      # doc-verified only — gate skipped (owner scope)

# Phase 4b — the remaining two Tier 2 agents. Neither was installed this
# phase (Task 5 skipped by owner decision) — forms below are inferred from
# Command()/Apply(), not smoke-tested through the built binary.
openrouter-launch openclaw -m openai/gpt-4o-mini                        # doc-verified only — tui --local, staged config (write site #3)
openrouter-launch openclaw -m openai/gpt-4o-mini -- agent exec "…"      # doc-verified only — one-shot, --model/--auth-env-only appended automatically, no staged file
openrouter-launch droid -m openai/gpt-4o-mini                           # doc-verified only — ConfigWriter, fork-and-wait, interactive
openrouter-launch droid -m openai/gpt-4o-mini -- exec "…"               # doc-verified only — ConfigWriter, fork-and-wait, headless
```

Two more commands open interactive bubbletea screens and are **not** covered
by that smoke-testing. This environment has no TTY, and `liveScreens()`
(`internal/tui/program.go`) checks `isTTY()` before any catalog call — so
both were only ever confirmed to refuse cleanly with no TTY attached. They
never made an API call in that testing, and their interactive behavior had
never been driven by a human — until the 2026-08-08 launch recorded below,
which exercised the picker but not its filters or profile save:

```bash
openrouter-launch                     # bare invocation: opens the root screen
openrouter-launch claude              # no -m: straight to the picker
```

**Agent-session smoke test, 2026-08-08 — the tool's first real end-to-end
launch.** A human drove the interactive TUI, picked `moonshotai/kimi-k3` from
the picker, and accepted the Landmine 7 advisory confirm for a
non-`anthropic/*` model; the env handoff then produced a working Claude Code
session against OpenRouter. That session was sanity-checked from inside: Bash
execution, read-only git, the Read tool, and a create/read/delete write
confined to `/tmp` all worked, and no project files were touched. Tool-use
round-trips through the proxy are functional on a non-`anthropic/*` model —
direct evidence for Landmine 7's "works for many models". Which picker
features the launch exercised went unrecorded; the sub-features remain open —
see Open items.

## Where things are

```
docs/superpowers/specs/2026-08-07-openrouter-launch-design.md            the spec — read for WHY
docs/superpowers/specs/2026-08-07-phase-2-planner-refactor-design.md     spec for the internal/launch refactor
docs/superpowers/specs/2026-08-08-phase-2-tui-design.md                  spec for the TUI
docs/superpowers/specs/2026-08-10-picker-filters-screen-design.md        spec for the ctrl+f filters screen and the esc latch
docs/superpowers/specs/2026-08-10-models-sort-design.md                  spec for column sorting + the INPUT/OUTPUT rename
docs/superpowers/plans/2026-08-10-models-sort.md                         the plan that built the sort, eight TDD tasks
docs/superpowers/specs/2026-08-08-phase-3-agents-design.md               spec for codex/opencode + Tier 3, with live-verified values
docs/superpowers/plans/2026-08-07-phase-1-core.md                        the Phase 1 plan
docs/superpowers/plans/2026-08-07-phase-2-planner-refactor.md            the plan that built internal/launch
docs/superpowers/plans/2026-08-08-phase-2-tui.md                         the plan that built internal/tui
docs/superpowers/plans/2026-08-08-phase-3-agents.md                      the plan that built Phase 3 (its wire_api="chat" is the frozen pre-verification value; the spec records the correction)
docs/superpowers/specs/2026-08-09-phase-4-tier-2-agents-design.md        spec for pi/hermes/qwen/cline/kimi/omp/openclaw/droid, live-verification results appended
docs/superpowers/plans/2026-08-09-phase-4a-tier-2-zero-touch.md          the plan that built Phase 4a
docs/superpowers/plans/2026-08-09-phase-4b-configwriter-openclaw-droid.md  the plan that built Phase 4b — Staged, openclaw, fork-and-wait, droid; complete
docs/superpowers/specs/2026-08-09-ci-cd-and-readme-design.md              spec for the CI/CD phase — read for WHY the branch model, coverage floor, and 6 targets are what they are
docs/superpowers/plans/2026-08-09-ci-cd-makefile-readme.md                the plan that built the CI/CD phase; amended repeatedly during execution as CI falsified assumptions
CLAUDE.md                                                                quick operational layer for Claude Code sessions; points here
.superpowers/sdd/progress.md                                             Phases 1-2 build ledger (gitignored)
.superpowers/sdd/*-report.md                                             Phases 1-2 per-task reports (gitignored)
.superpowers/sdd/2026-08-08-phase-3-agents/                              Phase 3 workspace: ledger (progress.md), task briefs/reports, live-*.log evidence cited by Landmine 18 and Open items
.superpowers/sdd/2026-08-09-tier-2-research/                             per-agent doc-verification notes (pi.md, hermes.md, qwen.md, cline.md, kimi.md, omp.md, openclaw.md, droid.md, findings.md) written before any Phase 4 code
.superpowers/sdd/2026-08-09-phase-4a-tier-2-zero-touch/                  Phase 4a workspace: ledger (progress.md), task briefs/reports, whole-branch review diffs
.superpowers/sdd/2026-08-09-phase-4a/                                    Task 9's live-gate evidence: live-{pi,hermes,cline}-*.log (12 files)
.superpowers/sdd/2026-08-09-phase-4b-configwriter-openclaw-droid/        Phase 4b workspace: ledger (progress.md), task briefs/reports, whole-branch review diffs per task, Task 6's verification report
.superpowers/sdd/2026-08-09-ci-cd-makefile-readme/                       CI/CD phase workspace: ledger (progress.md), task briefs/reports, review diffs, final-fix and residual-fix reports

Makefile                     THE single source of truth for every check; CI calls these targets
.github/workflows/ci.yml     quality, audit, 3-OS test matrix, machine-independence — all branches
.github/workflows/release.yml  tag-triggered: verify -> branch guard -> GoReleaser -> binary self-check
.github/scripts/check-tag-branch.sh    the branch guard (a script so it is testable; see tagguard_test.go)
.github/scripts/check-gosec-analysis.sh  refuses a gosec run that analysed nothing (Landmine 28)
.github/dependabot.yml       gomod + github-actions, weekly, targeting develop
.goreleaser.yaml             6 targets ({linux,darwin,windows} x {amd64,arm64}), archives, checksums
.golangci.yml                golangci-lint v2 schema (Landmine 26 — never downgrade to v1)
README.md                    user-facing docs; ships inside every release archive
LICENSE                      MIT, "Copyright (c) 2026 teggen"

main.go                      entry point + exit-code extraction
internal/version/            Version/Commit/Date, overwritten by release ldflags (pinned by goreleaser_test.go)
internal/openrouter/         model type, HTTP catalog client, disk cache, filters
internal/config/             XDG config, API key resolution, profile CRUD
internal/agent/              Launcher interface, registry, Claude launcher, process handoff
internal/launch/             the terminal-free planner: guards, warnings, typed conditions
internal/tui/                the bubbletea screens and the session driver
internal/ui/                 the shared table renderer: border style, palette, status vocabulary
internal/cli/                cobra command tree
```

The ledgers (flat `.superpowers/sdd/progress.md` for Phases 1-2, per-plan
`.superpowers/sdd/2026-08-08-phase-3-agents/progress.md` for Phase 3) are
gitignored but present in the working tree. They record every task's commits,
every review finding, and the reasoning behind each deferral. **Read them
before re-litigating any decision.**

## Architecture in one page

**`Launcher` is the only required interface**, and `Command` is a pure function:

```go
type Launcher interface {
	Name() string
	DisplayName() string
	Command(Request) (Command, error)   // MUST be pure
}

type Command struct{ Path string; Args []string; Env []string }
```

Purity is load-bearing: it makes every agent testable by comparing a struct, with
no process ever spawned in a test. Do not introduce a side effect into it.

Everything else is opt-in, detected by type assertion: `Installable`,
`Installer`, `Compatible`, `PlatformSupported`, `CredentialShadowCheck`,
`Staged`, `ConfigWriter`.

`ConfigWriter` is the **escape hatch** for an agent with no zero-touch
configuration path. `droid` implements it, as of Phase 4b. When an agent
implements it, that agent takes a fork-and-wait launch path instead of
`syscall.Exec`, so its `restore` can run (Landmine 24).

`Staged` is the escape hatch for an agent whose model selection needs a
*file* but never needs to touch the agent's own config: `StagedFiles(Request)
([]StagedFile, error)`, pure like `Command`, declares launcher-owned files
that `launch.Service.Launch` materializes under our own config dir.
`openclaw` implements it, as of Phase 4b. `Staged` and `ConfigWriter` look
similar but are a deliberate two-capability split, not a spectrum: `Staged`
writes OUR files (idempotent overwrite, no undo needed, `syscall.Exec`
handoff unaffected); `ConfigWriter` writes the AGENT'S file (backup and
restore required, forces fork-and-wait). Do not merge them.

**`Cache` deliberately does NOT implement `Catalog`.** `Catalog` is the narrow,
swappable source abstraction (an official SDK could implement it later);
`Cache` is the application-facing layer that wraps a `Catalog` and adds
provenance via `Snapshot`. Do not "simplify" this by merging them.

## Landmines

Each of these cost real debugging or was caught only in review. Do not undo them.

**1. Two base URLs that must never be unified.**
`openrouter.DefaultBaseURL` = `https://openrouter.ai/api/v1` (catalog fetching).
`agent.AnthropicBaseURL` = `https://openrouter.ai/api` — **no `/v1`**. Claude Code
appends its own version segment; a `/v1` here breaks it.

**2. `ANTHROPIC_AUTH_TOKEN` must be present-but-empty, never omitted.**
If unset, Claude Code falls back to authenticating against Anthropic directly.
That is why it appears in the env list with an empty value.

**3. `ExecArgs` must dedupe the environment.** *(Was a Critical bug.)*
`execve(2)` does not deduplicate `envp`, and POSIX `getenv` returns the **first**
match. The original `append(os.Environ(), c.Env...)` therefore let a user's
exported `ANTHROPIC_*` beat ours — Claude Code would have run against their own
Anthropic account while the tool reported success. `ExecArgs` now drops inherited
entries whose key appears in `c.Env`. Note `os/exec` (the Windows path) dedupes
keeping the *last* value, so Windows was accidentally correct and Linux was not.
If you touch `ExecArgs`, re-run its mutation check.

**4. Unknown pricing is never free.** A model whose price fails to parse carries
`PricingUnknown: true`. `IsFree()` returns false for it, `--free` and
`--max-price` exclude it, and it renders as `"?"`. Decoding stays tolerant so one
malformed catalog entry cannot break every launch.

**5. Save the last selection BEFORE the process handoff.** Lives in
`launch.Service.Launch` (`internal/launch/handoff.go`), which calls
`recordSelection` and then hands off in one function so no call site can
invert the order — it used to live in `resolveAndRun`, which is why the two
are no longer allowed to drift apart. On Unix `agent.Run` uses `syscall.Exec`
and replaces the process — nothing after it executes.

**6. Zero-touch is absolute — amended three times (Phase 4b Tasks 1 and 4,
then the cline env-inertness fix) to its current five-site form.** The
original invariant was "exactly two write sites, both launcher-owned." The
principle was always "never write an **agent's** files"; Phase 4b made the
launcher-owned side explicit and added the first sanctioned agent-owned
exception. Sites 1-4 match the Phase 4 spec's
(`docs/superpowers/specs/2026-08-09-phase-4-tier-2-agents-design.md`)
verbatim; site 5 postdates it:

| # | Path | Owner | Written by | Secret? |
|---|---|---|---|---|
| 1 | `$XDG_CACHE_HOME/openrouter-launch/models.json` | launcher | `openrouter.Cache` | no |
| 2 | `$XDG_CONFIG_HOME/openrouter-launch/config.json` | launcher | `internal/config` | yes (0600) |
| 3 | `$XDG_CONFIG_HOME/openrouter-launch/openclaw.json` | launcher | `Staged` materializer in `launch.Service.Launch` (`stageFiles`) | **no** (model ref only; key stays in env) |
| 4 | `~/.factory/settings.local.json` | **agent (droid)** | `ConfigWriter.Apply`, capability-gated, marker-owned entries only, restore on exit | **no** (`"apiKey": "${OPENROUTER_API_KEY}"` interpolation) |
| 5 | `~/.cline/data/settings/providers.json` | **agent (cline)** | `ConfigWriter` **restore only** — `Apply` snapshots raw bytes + mode and writes nothing; cline itself persists the key that `-k` supplies (Landmine 36) | the key lands there **during the session, written by cline**, and restore removes it |

Site 5 is the one entry where a secret of the user's genuinely touches an
agent's config file, and it is not us who puts it there — which is exactly
why the capability is implemented: without `Apply`'s snapshot the key
persists in cline's store forever, becoming the saved credential every
later launch resolves. It is also the only site whose write primitive exists
solely to *undo* a write.

Rules that survive unchanged: no other writes anywhere in the tree,
verified by exhaustive grep and now pinned by
`TestWriteSitesAreExhaustivelyEnumerated` (see "Verify the tree is
sound"); the API key is never written **by us** outside site 2;
`Command()` stays pure — sites 3, 4, and 5 are materialized by the launch
service or `Apply`, never by a launcher's `Command` method. Any code
writing into an agent's own config outside `ConfigWriter`, or any write
anywhere in `internal/agent` outside `droid.go`'s
`Apply`/`restore`/`writeDroidSettingsFile` and `cline.go`'s
`Apply`/`restore`/`writeClineProvidersFile`, is a Critical defect.

**7. `CheckModel` incompatibility is advisory.** Warn and confirm; never abort.
Claude Code with a non-`anthropic/*` model works for many models; OpenRouter only
warns that some context-management features may fail. Hard-blocking would make the
tool refuse valid setups.

**8. Tests that need a binary to look ABSENT must isolate the home
directory — via `testHome(t)`, not a bare `t.Setenv("HOME", …)`.**
*(Amended after the first Windows run: `HOME` alone isolates nothing on
that platform.)* `os.UserHomeDir` reads `HOME` on Unix but **`USERPROFILE`**
on Windows, so for the whole of Phases 1-4 every one of these tests
resolved the *real* user's home on Windows. That was not only 17 failing
tests: `Droid.Apply` genuinely **wrote `~/.factory/settings.local.json`
into the developer's own profile** on every `go test ./internal/agent/`.
`testHome(t)` (`internal/agent/home_test.go`, with a copy in
`internal/cli/harness_test.go` — a `_test.go` helper cannot cross package
boundaries) redirects `HOME`, `USERPROFILE`, `APPDATA`, and `LOCALAPPDATA`
together; the last two because `Hermes.findPath` and `Qwen.findPath`
consult them on Windows.
`TestTestHomeIsolatesTheHomeDirectoryOnEveryPlatform` pins the property
with one assertion that holds through `HOME` on Unix and `USERPROFILE` on
Windows, so the platform nobody runs locally is covered by the same line as
the one everybody does.

The original rule, still true:
Claude Code is genuinely installed on this machine at `~/.local/bin/claude`, and
`Claude.findPath` falls back to that path and `~/.claude/local/claude`. Without
`t.Setenv("HOME", t.TempDir())` such a test passes or fails depending on the
machine. An implementer once "fixed" this by deleting the fallback — that removes
support for every user whose install isn't on `PATH`. Don't.

This machine's really-installed list grew in Phase 4a: **pi** and **hermes**
were already present before the phase started (`~/.local/bin/pi`,
`~/.local/bin/hermes`, both hit by `Pi.findPath`/`Hermes.findPath`'s
home-dir fallback exactly like Claude Code), and **cline** was installed
during Task 9's live gate (`npm install -g cline` → `/home/martin/.local/bin/cline`,
owner-approved). `Cline.CheckInstalled` has no home-dir fallback — it only
calls `exec.LookPath`, so `HOME` isolation alone doesn't hide it; a test
needing cline absent must also control `PATH`. Every `findPath`/`CheckInstalled`
test added this phase does isolate correctly — that's what the
`HOME=$(mktemp -d) go test ./... -count=1` line in the verification suite
actually proves — but the next agent to add a home-dir fallback should add
its own binary to this list, not assume the pattern is self-documenting.

**9. Config is written 0600 via temp file + atomic rename**, with the mode set on
the temp file *before* the write. It holds an API key.

**10. `Spec.Launcher` must never be nil.** `buildIndex` panics at package init if
it is. Phase 2 adds unsupported-agent specs — give them a stub launcher, don't
remove the panic. `newLaunchCmds` runs inside `NewRootCmdWith` (called by both
`NewRootCmd` and every CLI test harness via `NewRootCmdWith(h.svc)`), so a nil
launcher would crash the binary — and every test — on construction, not just
one subcommand.

**11. `tui.Run` must never launch.** It returns an approved `launch.Plan`; the
caller calls `Service.Launch`. Every bubbletea program must have torn down
before `syscall.Exec`, or the agent inherits a raw-mode terminal. The
one-program-per-screen architecture makes this structural — there is no other
ordering available — so do not "simplify" it into a single program with a
screen enum.

**12. `--refresh` is spent exactly once.** `Service.Snapshot` runs twice per
launch: once to fill the picker, once inside `Plan`. `session.takeRefresh`
hands it to whichever runs first and `Plan` gets `Refresh: false` after the
picker has already refreshed. Passing it to both doubles the API traffic for
one launch while looking correct. Pinned by `TestRunSpendsRefreshExactlyOnce`.

**13. `internal/tui` must not import `internal/cli`, `cobra`, or `pflag`.**
`cli` imports `tui`. The cli edge is compiler-enforced today (it would be a
cycle), but cobra is not — pinned by `TestTUIDependsOnNeitherCLINorCobra`.

**14. `Args: cobra.NoArgs` on root is deliberate — don't delete it as
redundant, and don't credit it with more than it does.** It states the
no-positional-args constraint explicitly, in `internal/cli/root.go`. But
cobra's `legacyArgs` fallback happens to reject an unrecognized subcommand
here too, whenever the root command has subcommands and no parent — with or
without `Args: cobra.NoArgs`, and adding a `RunE` does not change that.
Verified against a standalone cobra v1.10.2 program, independently, twice.
`TestUnknownSubcommandStillErrors` pins the *contract* (`openrouter-launch
bogus` must error, not silently open the picker) but not this mechanism —
the test passes with `NoArgs` present or removed. Keep `NoArgs`: it is the
guard that does not depend on incidental properties of the command tree. Do
not write documentation claiming it is the *only* thing preventing the
silent-picker outcome; that claim is false and was disproved twice.

**15. `openrouter-launch <agent>` (no `-m`) must exit 1 on a fatal plan
error, not silently exit 0.** *(Was a real bug, found and fixed during the
TUI build.)* With `Options.Agent` set, there is no root screen to fall back
to. The `NotInstalledError`, `UnsupportedAgentError`, and
`UnsupportedPlatformError` branches of `handlePlanError` used to dead-end
through `rootOrDone()`, which returns `stateDone` with `Options.Agent` still
set; `retreat()` turned that into `ErrCancelled`, and the CLI maps
`ErrCancelled` to a silent exit 0. Result: `openrouter-launch claude` with
Claude Code not installed reported success, while `openrouter-launch claude
-m <slug>` exited 1 for the identical condition. Fixed by `noticeThenFatal`
(`internal/tui/tui.go`): it shows the notice and, only when there is no root
to return to, ends the session with the original error instead of
`ErrCancelled`. `rootOrDone()` itself was deliberately left unchanged —
`backState()` also reaches it on legitimate user-initiated retreats
(declining the confirm screen, cancelling the API-key prompt), and those
must keep exiting 0 because backing out really is a cancellation. The naive
fix — routing those three branches through `rootOrDone()` unconditionally,
or changing `rootOrDone()` itself — reintroduces this exact regression.

**16. Every closure in `liveScreens` must be tested by running a real
program, never by asserting it is non-nil.** *(This was the headline finding
of the whole-branch review.)* `internal/tui/program.go` is the only seam
between the driver — tested against injected stub closures — and the screen
models, tested as models. Neither side can notice a mistake in the glue. When
its only test asserted the five fields were non-nil, **seven** separate
inversions of those closures left the entire suite green, including `confirm`
returning `!m.answer` (answering "no" to "Launch anyway?" would *launch*) and
`prompt` ignoring `m.submitted` (pressing esc at the API-key prompt would
write an empty `api_key` into the user's config). The fix drives each closure
end to end through its actual bubbletea program, headlessly:

```go
runProgram(newPickerModel(in),
    tea.WithInput(strings.NewReader("\x1b[B\r")),  // Down, Enter
    tea.WithOutput(&buf),
    tea.WithoutSignalHandler())
```

No TTY is needed. If you add a screen, add its closure test the same way —
`internal/tui/program_test.go` has the pattern. Two honest gaps are inherent
and documented there: `tea.WithInput` bypasses raw-mode setup, and
`WithoutSignalHandler` means the real SIGINT path is not exercised. Note
these tests fail by *hanging* if key handling regresses, not by asserting.

**17. `chromeHeight` is 9, not 8, and recounting the chrome lines will tell
you 8.** `View()` writes 8 lines of non-list chrome, but its output also ends
in `"\n"`, so splitting it the way bubbletea's renderer does yields one more
element than the newline count. The renderer drops from the *top* when the
split exceeds the terminal height — and line 0 is the title/search line, so
an off-by-one makes the live search echo invisible at every terminal height.
This shipped once. The full accounting is in the constant's comment in
`internal/tui/picker.go`; `TestPickerViewFitsAndKeepsTitleVisibleAtVariousHeights`
pins it against the renderer's actual arithmetic.

**18. Codex's `wire_api` value was wrong in the plan and the design doc —
`"chat"` is rejected outright by codex ≥0.146.1.** The Phase 3 plan specified
`wire_api="chat"` on its own — the ollama source it was ported from actually
specified `wire_api="responses"` and would have agreed with the live
verification below. Live verification (2026-08-08,
`.superpowers/sdd/2026-08-08-phase-3-agents/`) ran that exact value against a
real `codex exec` and got a config-load-time error, not a network failure:

```
Error loading config.toml: `wire_api = "chat"` is no longer supported.
How to fix: set `wire_api = "responses"` in your provider config.
```

(full transcript: `live-codex-chat-rejected.log`). `"responses"` was tried
next and produced a real completion through OpenRouter
(`live-codex-raw.log`, `live-codex-orl.log`). `internal/agent/codex.go` now
emits `wire_api="responses"` — this is the live-verified value, not a
guess. If you are "fixing" this back to `"chat"` because it matches an
older doc or looks more idiomatic: don't — the ollama source it was ported
from agrees with `"responses"`, not `"chat"`. That value is falsified and
breaks codex on any version ≥0.146.

**19. hermes's context floor is 64,000 (decimal), not 65,536 — a future
"cleanup" back to 65536 is wrong, not tidier.** The Phase 4 design doc and
the first cut of `internal/agent/hermes.go` encoded `hermesMinContext =
65536`, reading hermes's own "64K" wording as the binary kilobyte. Task 9's
live gate (2026-08-09) proved otherwise: hermes's real startup rejection
reads `below the minimum 64,000 required by Hermes Agent`, and
`microsoft/wizardlm-2-8x22b` — OpenRouter-reported context exactly 65,535,
i.e. below 65,536 but above 64,000 — **passed** hermes's own context gate
and only failed afterward for an unrelated reason (`HTTP 404: No endpoints
found that support tool use`). That boundary result pins the real floor at
or below 65,535, consistent with hermes's stated 64,000 and inconsistent
with 65,536. `hermesMinContext` is now `64000`; `TestHermesCheckModelContextFloor`
pins three boundary cases (63,999 rejected; 64,000 and 65,000 accepted) and
was run against the *unmodified* 65536 code first — it failed there, which
is what proves it exercises the real threshold rather than passing
vacuously (the Landmine 18 protocol, applied again). If you are "fixing"
this back to 65536 because it looks like the cleaner binary-K value: don't
— it is falsified by a live rejection message and a boundary probe, not a
guess. Commit `bfaed0d`.

**20. Do not port ollama's kimi `--config` mechanism — it targets the
deprecated legacy CLI, not Kimi Code.** ollama's `cmd/launch/kimi.go`
configures kimi by passing a full config inline as JSON via `kimi --config
'<json>'`, with provider type `"openai_legacy"`. Both that flag and that
type exist only in the deprecated Python kimi-cli: `--config`/`--config-file`
landed in legacy kimi-cli 0.68, and ollama's own installer URL
(`code.kimi.com/install.sh`) is, verbatim in the downloaded script header,
"Legacy kimi-cli (Python) installer - DEPRECATED". The current Kimi Code
CLI has neither the flag nor the `openai_legacy` type — its provider types
are `kimi`/`anthropic`/`openai`/`openai_responses`/`google-genai`/`vertexai`.
Porting ollama's mechanism as-is would have repeated Landmine 18's mistake
verbatim: doc-verified against the wrong generation of the same-named
binary. `internal/agent/kimi.go` instead uses the `KIMI_MODEL_*` env family
(`KIMI_MODEL_NAME`, `KIMI_MODEL_API_KEY`, `KIMI_MODEL_PROVIDER_TYPE=openai`,
`KIMI_MODEL_BASE_URL`, optional `KIMI_MODEL_MAX_CONTEXT_SIZE`) — documented
as writing nothing back to config.toml and outranking it; only a `-m` flag
beats it, which the launcher never passes and rejects in passthrough. Both
CLI generations install a binary literally named `kimi`, so
`Kimi.findPath`'s search order (Kimi Code's own `~/.kimi-code/bin` first,
then the legacy uv-tools/`.local/bin` paths) IS the disambiguation, and
`Kimi.ShadowedCredential` flags a uv-tools-only resolution as likely-legacy.
This ships doc-verified-only — Task 9's live gate for kimi was skipped by
owner scope — so treat the legacy-vs-new disambiguation as unconfirmed
against a real install; see Open items.

**21. omp's model selector takes an `openrouter/`-prefixed slug; every
other Tier 1/2 agent takes a bare OpenRouter slug — do not unify the two
dialects.** pi, hermes, qwen, cline, and kimi all pass `req.Model.ID`
straight through — pi's own doc comment states it plainly: "pi's catalog
keys models by bare OpenRouter slugs." omp is the opposite: its model
selector is `<provider>/<slug>`, and the prefix IS the provider selection
in omp's dialect. `internal/agent/omp.go`'s `Command` builds `"--model",
"openrouter/" + req.Model.ID`. Plan 4b's `openclaw` shares omp's ancestor
and is expected to need the same prefix. Neither dialect fails loudly on
the other's input — a bare slug is valid-LOOKING input for omp (it just
silently fails to select the openrouter provider) and a prefixed slug is
valid-LOOKING for everyone else (it becomes part of a literal, wrong, model
id) — which is exactly why each launcher's own test must pin its dialect
and FAIL under the other's, not just assert its own success:
`TestPiCommandPathArgsEnv` (bare slug) and `TestOMPCommandPathArgsEnv`
(prefixed slug) are that pair; the Phase 4a plan named them the proof (Task
3 mutation 1, Task 8 mutation 1). If you add another agent, doc-verify
which dialect it speaks before writing `Command` — don't assume either one.

**22. `OPENCLAW_CONFIG_PATH` replaces the user's whole OpenClaw config for
the session — deliberate, owner-approved; do not "fix" it by merging.**
`openclaw`'s `tui --local` has no `--model` flag and no model-selection env
var (see `internal/agent/openclaw.go`'s type comment) — the only way to
point it at a model is a config file, and openclaw reads *one* config path.
`Command` sets `OPENCLAW_CONFIG_PATH` to a launcher-owned file under our own
config dir (write site #3, `Staged`-materialized, holding only
`agents.defaults.model.{primary,models}`), which means the user's
`~/.openclaw/openclaw.json` — channels, plugins, everything else — does not
load for a launched session. This was weighed and accepted at spec review:
merging the two configs would mean parsing and rewriting a config format we
do not own, which is exactly the write-into-an-agent's-own-config move
Landmine 6 forbids. If you are tempted to "fix" the whole-config-replacement
as a UX gap, don't — it is a deliberate scope boundary, not an oversight.

**23. droid's model selection stays in `~/.factory/settings.local.json`,
never a `-m custom:<id>` argv flag.** Two independent reasons, not one: (a)
purity — the `custom:<displayName>-<index>` ID (droid's own selection
syntax) is only knowable after `Apply` has computed the entry's index, and
`Command` (which builds argv) MUST stay pure per the `Launcher` interface's
contract, so it cannot compute or receive that index; (b) a public report
(Factory-AI/factory#787, cited in
`.superpowers/sdd/2026-08-09-tier-2-research/droid.md`) describes `droid
exec --model custom:…` rejecting valid custom IDs outright on some version
— the `-m` path is upstream-flaky even where it exists. `internal/agent/droid.go`'s
`Apply` instead upserts `settings["model"]` directly so droid's own
default-model resolution picks the entry without any flag involved, and
`Command` passes only `req.ExtraArgs` through unchanged. Do not add a `-m
custom:…` flag to `Command`'s argv to "make it more explicit" — it
reintroduces both the purity violation and exposure to the upstream bug,
and this path was never live-verified against 0.190.0 (Task 5 skipped; see
Open items).

**24. A `ConfigWriter` agent (droid and cline) never takes the
`syscall.Exec` handoff — it goes through fork-and-wait so `restore` can
run.** `launch.Service.Launch` (`internal/launch/handoff.go`) type-asserts
`p.Spec.Launcher` against `agent.ConfigWriter`; if it satisfies the
interface, `launchConfigWriter` runs instead of `s.run`: `Apply` writes the
agent's config, `s.runWait` spawns the agent as a waited-on child
(`internal/agent/exec_wait.go`, `os/exec`, inherited stdio, SIGINT/SIGTERM
forwarded to the child), then `restore()` always runs afterward — including
after the child exits nonzero — and the run error survives a restore
failure via `errors.Join`, so `main.go`'s exit-code extraction still sees
the original `*exec.ExitError`. `Staged` (openclaw) and `ConfigWriter`
(droid) are a deliberate two-capability split for exactly this reason:
`Staged` writes launcher-owned files with no undo needed, so `syscall.Exec`
stays fine; `ConfigWriter` writes into the agent's *own* file and therefore
needs the undo, which requires the process to still be alive afterward. Do
not merge the two capabilities or route `Staged` launches through
fork-and-wait "for consistency" — that would cost every zero-touch agent
the clean process replacement Landmine 5 relies on, for no benefit.

**25. Go toolchain skew breaks the analysis tools in BOTH directions, and the
two have opposite remedies.** Both were hit for real, a day apart. Read which
direction you are in before "fixing" anything.

*Direction A — tool older than the tree.* Prebuilt analysis binaries break
when your Go moves ahead of theirs: `staticcheck`, `govulncheck`, and
golangci-lint v1.64.8 all aborted on this machine with `file requires newer Go
version go1.26 (application built with go1.25)` before analysing a single
line. A binary built by Go 1.25 cannot parse a tree compiled by Go 1.26.
`make tools` `go install`s them from source instead, and CI does the same
inside the job rather than downloading prebuilt binaries. If a tool suddenly
refuses to run after a Go upgrade, this is why — re-run `make tools`, do not
pin Go backwards.

*Direction B — the tree's Go older than the tool.* The mirror image, and it
took down the `audit` job on the very first CI run this project ever had
(run 31306598239, 2026-08-09). All four pinned tools' own `go.mod` files
declare `go >= 1.25` (golangci-lint 1.25.0, gosec 1.25.8, x/vuln 1.25.0,
actionlint 1.25.0 — three of them arrived via `@latest`, so this became true
without any change in this repo), while this project's floor was `go 1.24.0`
at the time (it is `go 1.25` now — see the third clause below).
`actions/setup-go` injects `GOTOOLCHAIN=local` into every step of the job,
which forbids fetching a newer toolchain, so `make tools` died immediately on:

```
go: github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.12.2:
    github.com/golangci/golangci-lint/v2@v2.12.2 requires go >= 1.25.0
    (running go 1.24.0; GOTOOLCHAIN=local)
```

The remedy is `GOTOOLCHAIN=auto` on the `go install` lines in `tools` and
`tools-release` — Go then fetches a newer toolchain **solely to build the
tools**. What compiles and tests this project is untouched: that stays pinned
to the `go.mod` floor via CI's `go-version-file: go.mod`, which is the entire
point of testing on the floor. This does not reintroduce Direction A, because
the skew that breaks analysis is tool *older* than the tree; a tool built by a
newer toolchain analysing older code is fine.

**Do NOT "fix" a Direction-B failure by raising `go.mod`'s `go` directive.**
That silently drops every user on the old floor in order to satisfy a
linter's build requirement — trading the project's actual supported range for
a tooling convenience. Fix the tool install, not the product's floor.

*Third clause — the one case where raising the floor IS right, and how to
tell it apart.* The rule above is about **tooling convenience**. It does not
apply when the floor itself has gone **end-of-life**, because then the floor
is a security defect rather than a compatibility promise. That happened here
on 2026-08-09, one run after Direction B was written: with `make tools`
fixed, the `audit` job got one step further and govulncheck reported **27
reachable standard-library vulnerabilities**, every one `Found in:
<pkg>@go1.24`, with live traces through `internal/openrouter/client.go`'s
`http.Client.Do`/`io.ReadAll` — i.e. this tool's real HTTPS path to
OpenRouter. About 17 were fixed inside the 1.24 series, but **ten are fixed
only in `go1.25.8`–`go1.25.12` and have no 1.24 fix at all**: Go supports the
newest two majors, 1.26 had shipped, so 1.24 was EOL and those ten were
permanently unpatched. Released binaries are built by CI, so a 1.24 floor
meant shipping them. The floor moved to `go 1.25` for that reason and no
other. The test: *is the old floor still receiving security patches?* If yes,
fix the tooling. If no, raise the floor — and say in the commit that EOL is
why.

Note the directive is **`go 1.25`, minor-only, with no patch component**.
`setup-go` installs exactly what the directive names, so `go 1.25.0` would
pin CI to the *oldest* 1.25 patch and reproduce the identical failure one
minor later; the minor-only form resolves to the newest 1.25.x.

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

**28. `gosec` exiting 0 is not evidence that gosec ran — but this is
narrower coverage than "catches a broken tree", not general insurance
against one.** `-no-fail` is what keeps this repo's 14 findings advisory,
and it also swallows the case where the analysis never happened. Measured on
a deliberately broken package, then on this real tree with one type error
added to `internal/launch/plan.go`:

```
[gosec] Error building the SSA representation of the package launch:
        package launch has type errors, skipping SSA analysis, no ssa result
exit 0 — SARIF written — still 19 results
```

Four packages' SSA analysis silently did not run, the SARIF looked *identical*
in size and result count, and every downstream control *after* gosec in
`ci.yml`'s `audit` job was satisfied — including `if-no-files-found: error`,
which only ever asked whether a file exists.

**A re-review measured, rather than assumed, whether a type-error tree alone
can reach that point at all — it cannot.** `make security` runs
`govulncheck` before gosec, with no `-` prefix, so a non-zero exit there
aborts the recipe on the spot. Reproducing the exact injected error above:
`govulncheck ./...` fails immediately (`loading packages: ... undefined:
totallyUndefinedSymbolReference`, exit 1) and `make security` — and with it
the `audit` job — reddens at that line, before gosec's own line ever runs.
So these two guards' real, non-redundant coverage is the set of cases
govulncheck's blocking exit does *not* already catch:

- **gosec walking zero packages** for a reason unrelated to any type error —
  a path or glob mistake, an exclusion misconfiguration, gosec pointed at an
  empty tree — where `govulncheck`'s separate invocation would not
  necessarily fail the same way, or at all;
- **SSA dying in a package `govulncheck` never reached** — the two tools
  load packages independently, so a future divergence between their scopes
  (a flag, an exclude list, a path change on either one) could leave a
  broken package visible to gosec's SSA build and invisible to
  `govulncheck`'s;
- **a future softening of `make security`** — if `govulncheck`'s line ever
  gains the `-` prefix that makes gosec's line advisory, or the two calls
  are reordered, the blocking-first-runs-first property this section relies
  on disappears silently, and these two guards become the only thing
  standing between a broken gosec analysis and a green job.

Do not restate this landmine as "a type-error tree can go green" — that
claim was checked directly and is false as of this tree's `make security`
ordering; the honest claim is the narrower one above. Two guards close the
narrower gap and neither is optional:

- `.github/scripts/check-gosec-analysis.sh`, run as its own audit step,
  greps the teed gosec log for the failed-analysis signature and for at
  least one `Checking file:` line. It deliberately **never** looks at the
  SARIF's result count — keying on "no results" would make a genuinely clean
  tree impossible to achieve *and* would still miss the case above, where the
  result count did not move. `gosecguard_test.go` pins both directions; four
  mutations of the script each fail a complementary set of subtests.
- `make security`'s `test -x $(GOBIN)/gosec` guard. The `-` prefix on that
  recipe line tells make to ignore the exit status, which is what keeps
  findings advisory — but it ignored `Error 127 (ignored)` just as happily,
  so with gosec uninstalled `make security` exited **0** having run no gosec
  at all. gosec was the only tool there without the guard its siblings have.

**29. The 14 gosec findings `make security` still prints are triaged false
positives, dismissed in the Security tab — do not "fix" them, and do not
add `#nosec`.** Code scanning had 19 open alerts on 2026-08-09. Five were
real least-privilege drift and were fixed (see the commit "close the five
gosec permission findings"); the other 14 were each checked against the
code and dismissed with a per-alert reason. Dismissal is a GitHub-side
state only: gosec still emits all 14 locally and into the SARIF, so seeing
them in `make security` output is expected, not a regression.

| Rule | × | Sites | Why it is not a defect |
|---|---|---|---|
| G101 | 2 | `config.go:12`, `droid.go:90` | The first is a const holding an env var *name*; the second is the literal `"${OPENROUTER_API_KEY}"`, droid's own interpolation syntax — the mechanism that keeps the key **off** disk. Neither is a credential. |
| G117 | 1 | `config.go:83` | The `APIKey` field really is marshaled — that is write site #2, the tool's one sanctioned credential write. 0600 + atomic rename are the controls (Landmine 9). |
| G304 | 7 | `config.go:56`, `pi.go:94`, `cline.go:78`, `hermes.go:143/157`, `openclaw.go:173`, `droid.go:137` | Every one reads a fixed, well-known path under the invoking user's own `$HOME` (or our own config path). No caller-supplied path component exists. |
| G703 | 2 | `hermes.go:65`, `qwen.go:68` | `os.Stat` on `findPath` candidates built from `$HOME`/`$APPDATA`/`$LOCALAPPDATA`. The "taint" is the user's own environment. |
| G204 | 2 | `exec_unix.go:15`, `exec_wait.go:21` | Spawning the agent the user chose with the args they passed **is the product**. The real control here is `ExecArgs`' env dedup (Landmine 3). |

**An alert stays open while it is present on any analysed ref — query with
`ref` before concluding a fix did not land.** This was observed, not
theorised. After the fix landed on `develop`, `?ref=refs/heads/develop&state=open`
returned **zero** while the Security tab's default cross-ref view still
showed all five as open, because `main`'s newest analysis was the `v0.1.0`
commit `5ee7ea5`, which still contained the old code. Under this project's
branch model `main` moves only by fast-forward when a release is cut, so
the gap lasted until `v0.1.1`. The moment CI rescanned `main` at `c05c7b1`
(analysis: 14 results) all five flipped to **fixed** on their own, with no
manual action. Current state: **zero open alerts**, 14 dismissed, 5 fixed.
Expect the same lag the next time a fix lands on `develop`.

Two of these are actively dangerous to "resolve": silencing G117 would mean
not storing the user's key, and silencing G204 would mean not launching an
agent. `#nosec` was rejected as the mechanism because the tree has none
today, and blanket suppression comments would blunt *future* findings at
exactly the sites — the exec path and the credential write — that most
deserve a second look. If a new alert appears at one of these sites, read
it; do not assume it belongs to this list.

**30. `ui.Table.MaxWidth` is a cap applied in two passes — never pass it
straight to lipgloss's `table.Width`.** `table.Width` is a *target*, not a
maximum: it **expands** a narrower table to exactly that width, so the
obvious one-liner would stretch every listing to a uniform 100 columns.
`Theme.Render` (`internal/ui/ui.go`) builds the table at its natural width,
measures, and re-renders capped only on overflow.
`TestMaxWidthDoesNotExpandATableUnderTheCap` pins it, and fails under
exactly that one-liner. Related, same file: row rules are drawn **iff** the
cap binds, because that is when cells wrap and two multi-line rows
otherwise read as a single block — measured, not assumed. And `Table.Role`
must have no side effects: lipgloss calls it twice per cell (a measure pass
and a render pass).

**31. `TestAgentsOutputStaysNarrow` must render a SYNTHETIC spec, not the
live registry — and that is why `agentsTable` takes its specs as an
argument.** The widest real description leaves the table at 94 columns,
comfortably under the 100-column cap, so against the registry the test
passes with the cap deleted: it would assert nothing at all. It now builds
a spec with a 200-character description and renders it through
`ui.Render`. If you "simplify" `agentsTable` back into `newAgentsCmd` by
having it call `agent.List()`/`agent.Installed` itself, this test loses its
only way to fail. The same reasoning applies to `profilesTable`'s injected
`lookup`: `profile add` validates the agent name, so the `⚠ unknown agent`
row is unreachable from a test that cannot supply its own lookup.

**32. The root screen measures its own rendered height; it does not
subtract a chrome constant.** This is Landmine 17's rule applied to a
second screen, and the reason is stronger here: the root screen's chrome
depends on how many *table frames* fall inside the scroll window (each
costs a top border, a header row, and a header rule), which depends on
where the window starts, which depends on the cursor. There is no constant
to compute. `rootModel.View` shrinks the window until
`lipgloss.Height(out) <= m.height`, and
`TestRootViewFitsTheTerminalHeight` pins it at four terminal heights ×
three cursor positions. The window is derived from the cursor alone with
**no stored offset**, which is what keeps `View` a pure function of the
model — do not add an `offset` field "to match the picker"; the picker
needs one because its `Update` owns the scroll, and this screen's does not.

**33. The picker's catalog table must never wrap a row, and `tableFrame` is
measured rather than written as 4.** Two separate traps in one screen.
*(a)* `ui.Table.MaxWidth` is deliberately unused in `renderCatalog`
(`internal/tui/picker.go`): it wraps an overlong cell, and a two-line row
makes the table taller than `listHeight` budgeted for, which pushes the
title off the top — Landmine 17's outcome again. MODEL is truncated with an
explicit `…` instead. *(b)* `chromeHeight` is now `nonListChrome +
tableFrame`, and `tableFrame` is computed at init by rendering a one-row
table and subtracting the row. Landmine 17 exists because that budget was
counted by hand and came out one short; the table added four more lines to
the same budget, so the identical mistake was available a second time.
Hardcoding `tableFrame` fails `TestPickerViewFitsAndKeepsTitleVisibleAtVariousHeights`.
*(c)* The key footer **wraps**, so `chromeHeight` is a method rather than a
constant: `nonListChrome` counts one footer line, and every extra one has
to come out of the list. A fixed budget lets a wrapped footer push the
title off the top — the same outcome a third time.
`TestPickerFooterWrapsAndIsPaidForOutOfTheList` pins that `listHeight`
actually shrinks when the footer grows, which a width-only assertion cannot
see. The root screen needs none of this arithmetic: its `View` measures and
shrinks, so extra footer lines cost rows automatically.

**34. The picker sheds catalog columns on a narrow terminal, and MODEL is
exempt — the exemption is only observable below ~16 columns.** A bordered
table cannot be cut mid-line the way the deleted `clampRow` cut a
preformatted row, and five columns have a floor near 62, so
`catalogDropOrder` drops COMPLETION/M, PROMPT/M, CONTEXT, TOOLS in that
order until it fits. The shedding loop stops **as soon as the table fits**,
which is why `TestPickerShedsCatalogColumnsOnNarrowTerminals` has to probe
at width **10**: at 40 or even 20 the loop never reaches the end of the
drop list, so adding MODEL to `catalogDropOrder` passes those widths and
the test proves nothing. Note also that truncation and shedding both narrow
the table, so a width assertion alone cannot tell them apart — deleting the
MODEL truncation just makes the loop drop one more column and still fits.
`TestPickerTruncatesALongModelIDRatherThanSheddingAColumn` is what pins
which mechanism runs.

**35. Chrome lines are as capable of overflowing the terminal as table rows
are, and the footer did for the whole of Phase 2.** The picker's key hints
were a fixed 85-column string, so they overflowed every terminal narrower
than that — including the common 80 — and the width test never caught it
because it only ever measured the *model rows*. Three separate lines had
the same defect: the title (whose search echo is user-controlled and
unbounded), the filters/count status line, and the footer. Hints are now a
`[]string` packed by `hintLines`, which breaks BETWEEN hints and never
inside one, and the two single-line pieces go through `clampLine`. When
adding chrome to any screen, assume it will be rendered on a 40-column
terminal, and assert on **every** line of `View()` — a test scoped to the
interesting rows is how this survived as long as it did.

**36. cline's key MUST go on argv via `-k`; `OPENROUTER_API_KEY` alone
cannot configure an interactive cline session, and the launcher's original
env-only design never could.** Reported as a bug (2026-08-09): launching
cline landed on "Connect a model provider to get started." Two independent
mechanisms, both measured live against 3.0.52:

- **The TUI's provider gate reads persisted settings only** — never the
  environment, cold or warm. With no saved key it renders the onboarding
  wizard whatever the environment holds. This is the reported symptom, and
  on its own it makes env-only delivery unusable for interactive launches
  (the normal case: no prompt argument means the TUI).
- **The process we exec does not call the model.** Cline runs sessions
  through a long-lived **hub daemon** (`--cline-hub-daemon`, one per data
  dir, local WebSocket, lock file at
  `~/.cline/data/locks/hub/production.json`). Its credential chain —
  `apiKey` → OAuth resolver → `apiKeyEnv` — reads **its own**
  `process.env`, i.e. the environment of whatever first spawned it. Env
  delivery therefore works only while our launch is the one starting the
  daemon; a daemon started hours earlier from the user's shell keeps serving
  that shell's key. Proof: a launch carrying a **dummy**
  `OPENROUTER_API_KEY` returned a real completion, because the daemon held
  the user's genuine key (confirmed in `/proc/<pid>/environ`). So even
  one-shot env-only launches were not merely fragile — they silently billed
  whichever key the daemon happened to start with.

**Why the Phase 4a live gate passed anyway** (worth internalising before
trusting any future gate): Task 9 really did run cline end to end on a
virgin `~/.cline` with no `cline auth` and the env key only, and got `OK`
back. Both of its conditions hide the bug — it passed a **prompt**, so the
TUI gate never ran, and `~/.cline` was **virgin**, so its own invocation
spawned the daemon and the daemon inherited the gate's environment. A gate
that exercises one-shot mode on a cold daemon cannot see either mechanism.
When gating an agent, launch it the way users launch it: interactively, and
a second time with its state already warm.

`-k` is the only delivery verified to reach the model call, and it outranks
both the daemon's environment and a **saved** `providers.json` key (all
three measured; the ordering is explicit in the CLI's own `resolveApiKey`,
where the passed key is checked first). The costs are accepted deliberately:
the key is visible in `/proc/<pid>/cmdline`, and **cline persists it into
`~/.cline/data/settings/providers.json`** — which is why cline is a
`ConfigWriter` (write site 5) whose `Apply` writes nothing and only
snapshots, so restore can put the file back byte-for-byte.

Two corollaries. (a) The env var is still set, on purpose: on a cold start
our client is what spawns the daemon, so our env becomes the daemon's and a
stray user export does not — the Landmine 3 class, one process removed.
(b) `Cline.ShadowedCredential` was **removed**, not repaired: its premise
(a saved key outranks the launch's key) is false once `-k` is passed, and
`TestClineDoesNotClaimCredentialShadowing` pins the removal so re-adding it
has to answer a test. Do not "restore zero-touch" by dropping `-k` back to
env-only — that is precisely the bug. Note also that ollama's cline
integration writes `globalState.json` with `welcomeViewCompleted`; that key
does not exist anywhere in the 3.x CLI binary and is pure VS Code-era
legacy.

**37. The picker's filters live on a `ctrl+f` screen. Do NOT put them back
on `alt+t/f/c/p`, and do NOT delete `escLatch`.** Reported as two bugs
(2026-08-10, owner running `orl` over SSH): the alt chords typed their letter
into the search box, and nothing on screen said what the chords meant.

The model was never at fault. `handleKey` switched on `key.String()` before
the search-append branch, and the append branch guards on `!key.Alt`, so
`alt+t` could not fall through — `TestAltKeysRenderDistinctlyFromPlainKeys`
still pins the bubbletea rendering that rests on. The `alt` was being lost
*before* `tea.KeyMsg`, and only two things can do that:

- **The read-boundary split.** bubbletea 1.3.10 sets `canHaveMoreData :=
  numBytes == len(buf)` against a 256-byte buffer (`key.go:579`), so a read
  returning a lone `\x1b` looks like a complete event boundary and
  `detectOneMsg` reports a bare `KeyEscape` (`key.go:707`). The rune from the
  next read then arrives unmodified — closing the screen *and* typing the
  letter.
- **A terminal that sends no ESC at all**, where a bare `t` carries no
  evidence Alt was held and no amount of parsing can recover it.

Both are covered, because the client emulator was on the far end of an SSH
session and could not be observed from the server. `escLatch` (40 ms, one
`tea.Tick`) defers every `esc` on the picker and the filters screen: a plain
rune inside the window means the pair was a split chord and **both halves are
swallowed**; anything else, or the tick, resolves a real esc. `ctrl+f` is the
terminal-independent half — a control key the search box can never claim.

Three things that look like cleanups and are not:

- **`ctrl+f` has no `len(m.visible) == 0` guard**, unlike `enter` and
  `ctrl+s`. A filter combination matching nothing is exactly when the screen
  is needed; guarding it traps the user with `esc` as the only exit.
- **The match count is `Rank(Apply(...), search)`, not `Apply` alone.**
  `filterState` carries the session's search, and a count built from `Apply`
  would contradict the picker's own status line.
- **Testing the split needs a reader that returns `\x1b` and `t` on separate
  `Read` calls** (`chunkReader`). `bytes.NewBufferString("\x1bt")` hands
  bubbletea both in one read, which parses as a single `alt+t` — that version
  passes with `escLatch` deleted and proves nothing. Same trap for any test
  wanting two rune keypresses: one read of `"xy"` is one `KeyRunes` message
  (`key.go:697`). And do NOT append a trailing `ctrl+c` to a headless input
  as a "fail fast instead of hanging" backstop: both bytes arrive in one
  read, both `KeyMsg`s are queued, and the model processes the `ctrl+c`
  *after* the earlier key's `tea.Quit` — overwriting the choice and breaking
  the passing case.

Deliberately deferred: `prompt.go` has the same text-input hazard but
advertises no chords, so nothing invites the keypress. Design:
`docs/superpowers/specs/2026-08-10-picker-filters-screen-design.md`.

**38. Sorting composes OUTSIDE `Rank`, and unknown pricing sorts last in
both directions.** Two independent traps in one small feature, and both
survive a compile.

*(a) Composition.* `pickerModel.recompute` is
`SortModels(Rank(Apply(...), search), sort)`: the chosen column beats search
relevance, and relevance survives only as the stable sort's tie-break. Moving
`SortModels` inside `Rank`'s argument type-checks, reads identically at a
glance, and inverts the owner's decision. `TestPickerSortAppliesOutsideRank`
only catches that with a fixture where the two orders genuinely DISAGREE — it
searches `"o"`, where `Rank` puts `openai/o1-mini` first (ID prefix) while
cheapest-output puts `qwen/qwen3-coder:free` first. A fixture where they agree
passes either way, which is this project's recurring review finding in its
purest form.

*(b) Unknown pricing.* A model with `PricingUnknown` carries `0.0` in both
price fields and renders `?`, so a numeric comparison heads a cheapest-first
list with models whose price is simply not known — Landmine 4's false
"it's free" claim by a new route. `unknownLast`
(`internal/openrouter/sort.go`) therefore runs BEFORE the `Desc` swap, which
is why the naive version is wrong ascending and *accidentally right*
descending; `TestUnknownPricingSortsLastWhicheverWayTheArrowPoints` asserts
both directions for exactly that reason, and the mutation that reorders those
two statements fails only the `desc=true` half.

Two more things measured rather than assumed while building this, both of
which had a first version that proved nothing:

- **A fixture whose input and output prices rank models identically cannot
  tell the two comparators apart.** The first `sortFixture` had prices
  1/2, 3/9, 15/75 — monotone in both columns — and swapping
  `PromptPricePerM` for `CompletionPricePerM` left the suite green.
  `c/asymmetric` (cheapest input, dearest output) is what makes the columns
  distinguishable.
- **An all-equal tie group is not a stability probe.** With every comparison
  false, pdqsort detects an already-ordered run and returns it untouched, so
  `sort.Slice` passes a test built from one — at any size. `TestSortModelsIsStable`
  uses four scrambled key groups instead, and fails under `sort.Slice`.

`SortModels` returns a new slice and never sorts the caller's; the picker
holds the catalog for the life of the session, and `Rank` copies for the same
reason.


## Phase 2 — complete

The TUI ships: root screen (profiles + agents), model picker with
type-to-search, four filters and a column sort, `ctrl+s` profile save,
API-key prompt, notice screens for the planner's typed errors.

Four deliberate divergences from the original design doc, all recorded in
`docs/superpowers/specs/2026-08-08-phase-2-tui-design.md`:

- **Filters are on a `ctrl+f` screen, not bare `t/f/c/$` and no longer on
  `alt+t/f/c/p` either.** The original key table collided with
  type-to-search: `anthropic` contains a `t`. The alt chords that replaced it
  were correct in the model but not reachable on every terminal — see
  Landmine 37, which supersedes this and is the binding version. That screen is now
  **Filter & Sort**: it carries the column sort as two more rows of the same
  declarative table (Landmine 38).
- **`go 1.22` became `go 1.24`**, which bubbletea requires from v1.3.8.
- **The API key is saved unconditionally, and the prompt says so.** The spec
  originally promised an "offer to save". There is no offer: the prompt
  discloses, before you type, that the key will be written to the config
  path (resolved through `config.Path()`, so it honors `XDG_CONFIG_HOME`).
  The owner chose disclosure over a real yes/no because a
  decline-and-still-launch path would need an `APIKey` override threaded
  through `internal/launch` — `Plan` resolves the key from config or the
  environment only. This is the tool's one credential write; the file is
  0600 (Landmine 9).
- **`ctrl+c` cancels the whole session in one press, from any screen.** It is
  deliberately *not* an alias for `esc`, which still means "go back one
  step". This matters because bubbletea holds the terminal in raw mode with
  `ISIG` off, so there is no SIGINT fallback — when `ctrl+c` was aliased to
  `esc`, leaving a picker reached after declining a confirm took three
  presses and nothing could abort the process. Each screen returns a
  distinct cancellation the driver checks before any other routing.

**Deferred:** the background catalog refresh streaming into the live picker.
The cache carries a 24h TTL, so a warm cache is already current and the only
window it improves is the moment after expiry — for a goroutine, a channel,
and re-ranking a list the user is navigating. If this is ever built, the
registry's shared mutable `&Claude{}` needs reconsidering: a background
refresh goroutine could race the `LookPath` field that tests patch (see
Landmine 8, on `Claude.findPath`'s fallback paths and the requirement to set
`HOME` to a temp dir so tests can make the binary look absent). That concern
does not apply today, precisely because the refresh goroutine was never
built.

## Phase 4a — Tier 2 zero-touch, complete

The spec's agent tiers are in the design doc. **Tier 1 is complete**:
`claude`, `codex` (managed `-c` overrides + `-m`, `wire_api="responses"` —
live-verified on codex 0.146.1, see Landmine 18), and `opencode`
(`OPENCODE_CONFIG_CONTENT` minimal inline JSON + `OPENROUTER_API_KEY`
env-only auth, first-slash model split — live-verified on opencode 1.0.69)
all ship, plus `chatgpt`, `claude-desktop`, and `hermes-desktop` registered
unsupported-with-reason. Both new launchers were verified end to end through
the built binary too (`orl codex -- exec …`, `orl opencode -- run …`), so
root-level `-m` before a passthrough subcommand parses fine on codex
0.146.1 — the passthrough-ordering risk the design doc flagged did not
materialize.

**Tier 2 zero-touch is now complete**: `pi`, `hermes`, `qwen`, `cline`,
`kimi`, and `omp` — six launchers, plus shared passthrough-conflict helpers
(`internal/agent/args.go`: `rejectModelFlag`, `rejectFlags`) and a new
opt-in advisory capability, `agent.CredentialShadowCheck` /
`ShadowedCredential() string` (`internal/agent/agent.go`), surfaced by the
planner as `launch.WarnShadowedCredential` (`internal/launch/plan.go`) —
warn-and-confirm, never abort, same posture as Landmine 7. Each agent's
mechanism was doc-verified before code, the same discipline Tier 1
followed and that caught codex's `wire_api` value (Landmine 18) — this
phase's own catches are Landmine 19 (hermes's real context floor) and
Landmine 20 (the kimi legacy-CLI trap ollama's port would have repeated).

Task 9's live gates ran three of the six end to end through the built
binary against real OpenRouter completions — **pi** (0.80.3), **hermes**
(Hermes Agent v0.20.0), and **cline** (CLI 3.0.52, installed during the
gate, owner-approved) — each with a before/after `ls -laR` audit of the
agent's own config tree confirming no write into it. The hermes gate also
**confirmed live**, not just documented, what `Hermes.ShadowedCredential()`'s
advisory asserts: a stale `OPENROUTER_API_KEY` line in `~/.hermes/.env`
really does win over a correct value in the process environment (`HTTP
401: User not found` with the bogus `.env` key present, `OK` with the same
line commented out) — direct validation of the shadow-credential advisory
mechanism, not just a documented risk. **qwen, kimi, and omp ship
doc-verified-only** — their gates were explicitly skipped by owner scope in
Task 9 (not run and failed); see Open items for what remains unanswered,
most importantly qwen's `modelProviders` collision.

Tier 3 desktop apps genuinely cannot be pointed at OpenRouter and stay
registered with a stated reason. **Owner decision:** `copilot`, `pool`, and
`vscode` — though listed in the main spec's Tier 3 table — are deliberately
**not** registered; `openrouter-launch copilot` (etc.) reporting "unknown
agent" is accepted behavior, not a gap to fill.

**They are registered but no longer *listed* (owner decision, 2026-08-09).**
`agents` and the TUI root screen both hide `!Status.Supported` specs; `agents
--all` shows them with the reason. This is presentation only — the specs, their
launch subcommands, and the `UnsupportedAgentError` notice are all untouched, so
`openrouter-launch chatgpt` still explains itself. The reason to hide them was
mechanical, not cosmetic: `agents` renders through `tabwriter`, which pads every
column to its widest cell, so the three ~99-character reasons stretched the
STATUS column of **all 14 rows** and pushed the table to 227 columns. Two TUI
tests were deleted rather than adapted — `TestRootViewShowsUnsupportedAgentsWithTheirReason`
asserted the exact inverse of the new behavior, and `TestRootCursorSkipsUnsupportedAgents`
would have kept passing for the wrong reason (with the row filtered out, Down
still lands on the next agent, so it could no longer fail). `TestAgentsOutputStaysNarrow`
now pins the rendered width rather than the row list, so a future long
description reintroduces the failure by any route.

**Followed by Plan 4b** (`docs/superpowers/plans/2026-08-09-phase-4b-configwriter-openclaw-droid.md`),
which shipped `openclaw` and `droid` and closed Tier 2 out entirely — see
"Phase 4b — ConfigWriter, openclaw, droid, complete" below.

## Phase 4b — ConfigWriter, openclaw, droid, complete

Four coding tasks, in dependency order (`Staged` before `ConfigWriter` —
smaller step first, and the write-site grep evolves twice in sequence
rather than once in a lump), then Task 5 (live gates, skipped by owner
decision) and Task 6 (this verification + handoff pass).

**`Staged`** (`internal/agent/agent.go`) is the third write-site capability:
`StagedFiles(Request) ([]StagedFile, error)`, pure like `Command`, declaring
launcher-owned files materialized by `launch.Service.Launch` (`stageFiles`
in `internal/launch/handoff.go`) after `recordSelection` and before
handoff — the same single side-effect site Landmine 5 already required.
`stageFiles` enforces the path boundary itself (`filepath.Rel` against
`config.Dir()`, rejecting anything that resolves outside it) rather than
trusting callers — a path-prefix bug here would be a write-anywhere
primitive, which is why its tests pin both a straightforward escape and a
naive-`HasPrefix` sibling-path regression (commits `3cdb9fb..a7a8ce1`).

**`openclaw`** (`internal/agent/openclaw.go`) is the first and only
consumer of `Staged` so far: `tui --local` has no `--model` flag, so its
model selection is a staged config file at
`$XDG_CONFIG_HOME/openrouter-launch/openclaw.json` holding only
`agents.defaults.model.{primary,models}`, pointed to via
`OPENCLAW_CONFIG_PATH` (Landmine 22). One-shot `agent exec` passthrough
skips staging entirely — `--model` and `--auth-env-only` are appended by
`Command` and compose config in memory, openclaw's own documented
zero-touch path. Model refs are `openrouter/`-prefixed and **lowercased**
(`openclawModelRef`) — openclaw normalizes refs to lowercase, sharing omp's
dialect (Landmine 21) with an extra wrinkle. `findPath` falls back to the
pre-rename `clawdbot` binary name. `ShadowedCredential` detects a stored
OpenRouter auth profile under
`~/.openclaw/agents/*/agent/auth-profiles.json` (precedence against the env
key is undocumented — see Open items).

**Fork-and-wait** (`internal/agent/exec_wait.go`'s `RunWait`; wired into
`internal/launch/handoff.go`'s `launchConfigWriter`) is the launch path any
`ConfigWriter` agent takes instead of `syscall.Exec` — see Landmine 24 for
the mechanism and why it must never merge with `Staged`'s `syscall.Exec`
path.

**`droid`** (`internal/agent/droid.go`) is the first real `ConfigWriter`
implementation — Factory's only OpenRouter declaration surface is a
settings file; no env var or flag defines a custom model. `Apply` upserts
one marker-owned (`displayName: "openrouter-launch"`) `customModels` entry
into `~/.factory/settings.local.json` (write site #4; the merge-friendly
local layer, never `settings.json`) with `apiKey: "${OPENROUTER_API_KEY}"`
env interpolation so the key never touches disk, and points the top-level
`model` key at it — all via atomic temp-file-then-rename writes (Landmine
9's shape). `restore` puts both back, preserving any foreign (non-marker)
`customModels` entries untouched, and removes the file entirely if it
created it. Model selection lives in the file, never `-m custom:<id>` on
argv (Landmine 23).

**Task 5's live gates were skipped by owner decision (2026-08-09).**
openclaw and droid both ship doc-verified-only, joining 4a's qwen/kimi/omp
in that posture — five of the eight Tier 2 agents are now doc-verified-only,
three live-verified (pi, hermes, cline). See Open items for exactly what
each skipped gate leaves unconfirmed; droid's routing proof in particular
is flagged must-do-before-real-use, not a routine open item.

**Tier 2 is now COMPLETE — all eight agents shipped**: pi, hermes, qwen,
cline, kimi, omp, openclaw, droid. What remains is not a new phase: the
five skipped live gates above, human interactive smoke tests across all
eight Tier 2 agents (plus codex, opencode, and the root TUI itself, already
listed as human-unverified in Open items from Phase 2/3), and the standing
open items below. Tier 3 (desktop apps that cannot be pointed at
OpenRouter) is already registered with stated reasons as of Phase 4a —
there is no further Tier 3 work, and no Tier 4 exists to plan toward.

## CI/CD, README, and the first release — complete

Shipped: `internal/version` + `--version`, an MIT `LICENSE`, a `README.md`, the
`Makefile` (the single source of truth — CI invokes its targets rather than
re-spelling commands in YAML), `.golangci.yml` on the v2 schema,
`.goreleaser.yaml` for six 64-bit targets, `ci.yml`, `release.yml` with
`.github/scripts/check-tag-branch.sh`, and Dependabot for gomod and
github-actions (both targeting `develop`).

**The branch model**: `develop` is the working branch, `main` holds released
code, and `main` moves only by fast-forward from `develop` when a release is
cut. Stable tags on `main`, `-beta.N` on `develop`; the guard enforces it by
*reachability*, since git records no "branch this tag was pushed from".

**Everything below was verified live on 2026-08-09, not assumed:**

- **`v0.1.0-beta.1`** published as a **Pre-release** from `develop`; **`v0.1.0`**
  published as **Latest** from `main`. Six archives plus `checksums.txt` each.
  Both verified by downloading the published archive, checking its sha256
  against `checksums.txt`, and running the extracted binary:
  `openrouter-launch version 0.1.0 (commit 5ee7ea5f…, go1.25.12)` — a real tag
  and a real commit, not the `dev`/`none` placeholders.
- **The stable push created a NEW release** rather than overwriting the beta's
  (distinct release IDs; the beta is still there, still marked Pre-release).
- **`v0.1.1`** (release id `367550004`, distinct from `v0.1.0`'s `367530782`)
  published as **Latest** from `main` at `c05c7b1`, seven assets, verified the
  same way: `sha256sum -c checksums.txt` OK, and the extracted binary reports
  `openrouter-launch version 0.1.1 (commit c05c7b1e…, go1.25.12)`. It went
  **straight to stable with no beta** — so `git tag --points-at` has a single
  tag and the `GORELEASER_CURRENT_TAG` collision below stayed dormant; that
  pin is still load-bearing for any release that does cut a beta first. The
  released binary was additionally run against the live catalog with
  `XDG_CACHE_HOME` redirected, confirming the shipped artifact really creates
  its cache dir `700` and `models.json` `600` — the Landmine 29 fix proven in
  the published binary, not only in tests.
- **The branch guard really refuses.** A deliberately mis-cut stable tag,
  `v0.9.9`, was pushed from `develop`-only history. The `release` job failed at
  *Enforce the branch model* with `refusing: stable tag 'v0.9.9' is not
  reachable from 'origin/main'`, **GoReleaser never ran** (the step was
  skipped), and no release was created. The tag was then deleted from both
  remote and local. `tagguard_test.go` had already proved the logic in
  isolation; this proved the workflow wiring.
- **`GORELEASER_CURRENT_TAG` is load-bearing, and the trap is real.** After the
  fast-forward, `v0.1.0` and `v0.1.0-beta.1` sit on the same commit, and
  GoReleaser's own resolution — `git tag --points-at HEAD --sort
  -version:refname | head -1` — returns **`v0.1.0-beta.1`**, confirmed by
  running it. Without the pin, pushing the stable tag would have rebuilt and
  republished the *beta*, with a green workflow. The stable run's log reads
  `using tags previous=<unknown> current=v0.1.0`. If you ever remove that env
  var, this is what breaks, and it breaks silently.

**The advisory OS leg**: `test (windows-latest)` alone is now
`continue-on-error`. Confirmed on a real run: a red Windows leg leaves the
overall run **green** (`conclusion: success`) while the job itself still shows
as failed — so the signal is visible without blocking. See Open items for what
Windows actually reported. `test (macos-latest)` shipped advisory too and had
its flag removed in the final fix wave: it has been green on every run since
the first, and the plan's definition of done was "the escape hatch comes off
per-OS once that OS passes". A macOS regression is now blocking.

**The final fix wave** (after the whole-branch review, on `develop`, no
re-release — both published artifacts were verified correct and neither is
affected) closed the last member of this phase's recurring failure class,
"a control that can go green having checked nothing": see Landmine 28 for
the gosec pair. It also made `.goreleaser.yaml` validated *before* a tag can
publish (`goreleaser check` in `release.yml`'s `verify` job — previously
nothing validated it until after the beta had shipped and `main` had been
fast-forwarded), added `lint-workflows` to `make ci` (actionlint was pinned
and installed but ran nowhere, while Dependabot edits workflow YAML weekly),
keyed `ci.yml`'s concurrency group on the PR head ref so same-repo PRs stop
running the matrix twice, and corrected the documentation findings recorded
under the individual items above.

**A scoped re-review of that fix wave** found two of its own fixes did not
achieve their stated purpose, plus a stale count inside the commit that
should have fixed it. All three closed together, still on `develop`, still
no re-release: the concurrency-group fallback was `github.ref` (unfixed —
see the corrected comment on `group:` in `ci.yml`), the `lint-workflows`
addition only ever ran on a contributor's machine because Dependabot PRs
never invoke `make ci` (closed by adding the same validation to `ci.yml`'s
`quality` job directly — see the divergence list below), and the Tests row
above was one commit stale the day it was written. `GOSEC_VERSION` was also
pinned and Landmine 28's scope restated in the same pass — see that landmine
for both.

## Open items

- **The interactive TUI was driven by a human for the first time in the
  2026-08-08 smoke-test launch** (see "Current state"): the picker rendered,
  `moonshotai/kimi-k3` was picked, the Landmine 7 advisory confirm was
  accepted, and the handoff produced a working session. Which entry point was
  used — bare `openrouter-launch` (root screen) or `openrouter-launch claude`
  (straight to the picker) — went unrecorded, so the root screen specifically
  remains human-unverified. Also still unverified: the picker's type-to-search,
  the `ctrl+f` filters screen that replaced the `alt+t/f/c/p` chords
  (Landmine 37), `ctrl+s` profile save, `esc` navigation back out of a screen,
  and whether filter state actually persists to `config.json` on exit.
- **`codex` and `opencode`'s interactive TUIs are not yet driven by a
  human.** Both were live-verified against OpenRouter (Task 4,
  2026-08-08) only through headless one-shot invocations — `codex exec
  --skip-git-repo-check` and `opencode run`, both directly and through the
  built binary. Nobody has yet sat at `openrouter-launch codex` or
  `openrouter-launch opencode` with no passthrough args and driven the
  agent's own interactive session end to end.
- **`opencode run`'s exit code cannot be trusted once its models.json cache
  is populated — this is opencode's own bug, not ours.** On opencode
  1.0.69, `openrouter-launch opencode -m <slug> -- run "…"` prints the
  completion correctly and then exits 1 with `Error: [DecimalError] Invalid
  argument: [object Object]`, once `~/.cache/opencode/models.json` (its own
  model/pricing catalog, downloaded on first use) has been populated —
  a clean first run before that cache exists exits 0. Reproduced both
  through our binary and with a raw `opencode run` invocation with none of
  this project's code involved at all; evidence at
  `.superpowers/sdd/2026-08-08-phase-3-agents/live-opencode-raw-repro.log`
  and documented in the Phase 3 design doc's live-verification section. Any
  script that gates on `openrouter-launch opencode`'s exit code should know
  a `0` completion can still surface as `1` — this is not a regression to
  chase in this repo, and it stays open until opencode fixes it upstream.
- **Windows exit-code propagation is unverified on real Windows.** The extraction
  logic is unit-tested with a synthetic `*exec.ExitError`, but nobody has run the
  binary on Windows.
- **The first CI run settled the advisory OS legs, and the prediction was
  wrong about macOS.** Run 31306598239, 2026-08-09, the first time this suite
  had ever executed off Linux. **macOS: green** — fully, no failures. The
  guess going in was that 22 of 50 test files reference Unix paths so both
  legs would likely be red; macOS simply passed. **Windows: 19 failures in 2
  of 9 packages** (`internal/agent` 18, `internal/launch` 1); `internal/tui`,
  `internal/cli`, `internal/config`, `internal/openrouter`, `internal/version`
  and the root package all passed. None of the 19 is a logic defect — they are
  three clusters of platform semantics:
  - six Unix home-dir `findPath` fallbacks (pi, hermes, qwen ×2, omp, kimi),
    e.g. `CheckInstalled = false with binary at ~/.local/bin/hermes`;
  - five `ShadowedCredential` fixtures (cline, hermes, pi, openclaw, kimi)
    built on Unix-shaped credential paths;
  - eight droid/staging tests, split between Windows path construction
    (`open C:\Users\RUNNER~1\...\.factory\settings.local.json: The system
    cannot find the path specified`) and the 0600 mode assertions Windows
    cannot satisfy at all (`after Apply: mode = -rw-rw-rw-, want 0600
    preserved`) — Windows has no POSIX permission bits, so Landmine 9's shape
    is unassertable there as currently written.

  **Closed 2026-08-09, and the prediction held exactly: platform-aware
  fixtures, zero launcher changes.** It took two rounds, because the first
  round's fix hid the rest. Round one — `testHome(t)`, which redirects
  `USERPROFILE`/`APPDATA`/`LOCALAPPDATA` alongside `HOME` (see the amended
  Landmine 8) — closed 14 of the 19 and revealed that on Windows these tests
  had been writing `~/.factory/settings.local.json` into the developer's
  REAL profile. Round two closed the other five, which were three unrelated
  things: one more file-mode assertion; kimi's fixtures writing a bare
  `kimi` where `kimiCodePath` appends `.exe`, so findPath never saw the file
  and the legacy path won (the inverse of the assertion); and qwen's
  fixtures asserting Unix dot-dirs when `findPath`'s Windows branch only
  probes `%APPDATA%/%LOCALAPPDATA%\npm` — now selected by GOOS, so Windows
  gets real coverage rather than a skip. `test (windows-latest)` had its
  `experimental` flag **removed**; all three OS legs are blocking.
  `test (macos-latest)` had its flag removed in the final fix wave — it was
  green on its first run and on every run since.
- **Dependabot version updates work; Dependabot security *alerts* are
  disabled.** The two are separate features. Version updates began the moment
  `main` gained `.github/dependabot.yml` (it reads config from the *default*
  branch only) — both ecosystems ran green within minutes of the fast-forward
  and opened **zero** PRs, i.e. everything is genuinely current, which is a
  stronger statement than silence. But `GET /repos/…/dependabot/alerts`
  returns `403 Dependabot alerts are disabled for this repository`. Enabling
  them is a repo-settings toggle nobody has flipped. **This is a real
  detection gap, not just a notification one — and it is asymmetric between
  the two ecosystems:**
  - `gomod`: `make security`'s govulncheck genuinely does cover this on
    every push, against the same Go vulnerability database. For Go modules
    the "notification, not detection" framing holds.
  - `github-actions`: **nothing covers it.** govulncheck analyses Go
    modules and Go call graphs; it has no idea `.github/workflows/*.yml`
    exists. And every action here is SHA-pinned (`TestWorkflowActionsArePinnedToShas`
    enforces it), so a pinned action is frozen at that commit — a
    vulnerability disclosed against it later moves nothing and warns
    nobody. Dependabot version updates raise a PR when a *newer* version
    ships, which is not the same signal and is not driven by the advisory
    database. Enabling security alerts is the only thing that closes this.
- **Three of the six Phase 4a launchers ship doc-verified-only; their live
  gates were skipped by owner scope (Task 9), not run and failed:**
  - **qwen** — the `modelProviders` collision is the specific unresolved
    item: a user's `~/.qwen/settings.json` with a `modelProviders.openai[]`
    entry whose `id` equals the launched slug may override our
    `--auth-type openai` + `OPENAI_*` configuration. Task 5's owner ruling
    was explicit that no runtime detector ships until live evidence settles
    it, and Task 9's gate was where that evidence would have come from —
    it was skipped, so the ruling still stands unconfirmed. This is the
    single most consequential open question left by Phase 4a. qwen also has
    no `CredentialShadowCheck` implementation at all (same ruling covers
    that omission).
  - **kimi** — a virgin-`HOME` first run and the legacy-vs-new binary
    disambiguation (`Kimi.findPath`'s search-order heuristic and
    `ShadowedCredential`'s uv-tools-path check, Landmine 20) are
    doc-verified only, never exercised against a real install.
  - **omp** — the `openrouter/<slug>` selector round-trip and first-run
    onboarding behavior (does a fresh `~/.omp` demand interactive setup
    before accepting `--model`/`-p`?) are doc-verified only.
- **The stored-credential advisory (`CredentialShadowCheck`) has coarse
  edges.** `omp`'s stored credentials live in `~/.omp/agent/agent.db`
  (sqlite) and are documented to outrank the env key ("env vars are a
  fallback, not an override" — omp's own docs), but `OMP` does not
  implement `ShadowedCredential` at all: the code comment in
  `internal/agent/omp.go` records the tradeoff explicitly — no sqlite
  dependency for one advisory, the caveat lives in the spec and README
  instead. A user with a stored omp OpenRouter credential gets no warning
  from this tool. `qwen` has the same gap (see above), on the same Task 5
  ruling. Revisit if either agent ever exposes its credential state as
  JSON or plain text.
- **All eight Tier 2 agents' interactive sessions have not been driven by a
  human.** pi, hermes, qwen, cline, kimi, and omp were only ever exercised
  through their own headless/one-shot flags — three of them live (pi,
  hermes, cline, Task 9), three doc-verified-only (qwen, kimi, omp, per
  above). **openclaw and droid are two more, unexercised the same way** —
  droid's fork-and-wait interactive session and openclaw's `tui --local`
  have never been run against a real installed binary (Task 5 skipped, see
  below). Nobody has sat at `openrouter-launch pi` (etc.) with no
  passthrough args and driven the agent's own interactive session end to
  end — the same gap Phase 3 left open for codex/opencode, now eight agents
  wider. The owner drives these smoke tests after Task 6 (Plan 4b's
  execution notes name this explicitly).
- **Task 5's live gates for openclaw and droid were skipped by owner
  decision (2026-08-09) — both ship doc-verified-only, same posture as
  4a's qwen/kimi/omp.** Everything Task 5 would have confirmed is now open:
  - **openclaw** — whether `agent exec` and `--auth-env-only` exist and
    behave as documented on an installed version (recent surface, not to be
    pinned from memory per the caution that caught Landmine 18); virgin-
    state `tui --local` first-run behavior (does a fresh `OPENCLAW_STATE_DIR`
    with no prior onboarding run cleanly, per docs' "safe defaults" claim,
    or does it hit a first-run gate the way ollama's own port assumed?);
    and auth-profile-vs-env precedence for `tui --local`
    (`ShadowedCredential` only detects that a stored profile *exists* —
    which one actually wins was never tested against a real session, e.g.
    via `/model status`).
  - **droid** — which default-model key the installed version actually
    honors: current docs describe a top-level `model` key (what `Apply`
    writes), but ollama's own port wrote `sessionDefaultSettings.model`,
    and both forms have been observed in the wild (see
    `.superpowers/sdd/2026-08-09-tier-2-research/droid.md`); whether
    `${OPENROUTER_API_KEY}` interpolation actually resolves in
    `settings.local.json` on a real droid; whether `restore` reproduces the
    pre-`Apply` file byte-for-byte on a real (not synthetic) settings file;
    and, most importantly, **THE ROUTING PROOF**: a launch with a
    deliberately bogus OpenRouter key MUST fail with an OpenRouter auth
    error. This was the spec's own demotion gate for droid (`ConfigWriter`
    vs. unsupported-with-reason) and it has never been run. A completion
    that succeeds anyway means droid silently fell back to a
    Factory-billed model instead of routing through OpenRouter — the
    silent-billing failure mode the spec's research flagged
    (Factory-AI/factory#1061). **Flag this as must-do-before-any real droid
    use** — it is not a routine open item, it is the one check that tells
    you whether droid is wired correctly at all.
- **The fork-and-wait SIGINT/SIGTERM path has no automated test.**
  `internal/agent/exec_wait.go`'s `RunWait` forwards `os.Interrupt` and
  `syscall.SIGTERM` to the child (Landmine 24), but no test drives an
  actual signal through it — the same class of honest gap as the TUI's
  `WithoutSignalHandler` caveat in Landmine 16. Both are documented, not
  hidden, and both would need a real subprocess/pty to close properly.
- **Deferred Minor findings live in the ledgers**, per phase, each with a reason:
  Phase 1 deferred 10 of 17 and fixed 7; the TUI phase carried 16 into its
  whole-branch review, which fixed 8, deferred 4, and dropped 4 as already
  resolved or harmless. Phase 3's final review fixed its 2 Importants
  (Landmine 18's attribution, the unsupported-subcommand CLI test) and
  deferred 6 Minors with rulings — see
  `.superpowers/sdd/2026-08-08-phase-3-agents/progress.md`; the largest are
  two extra conflict-override spellings codex validation misses
  (`-c=key=val`, bare `model_providers` table assignment) and the
  model-flag matcher now duplicated across codex/opencode (extract on third
  use). Phase 4b's ledger
  (`.superpowers/sdd/2026-08-09-phase-4b-configwriter-openclaw-droid/progress.md`)
  records 10 more Minors deferred across its four coding tasks (path-boundary
  hardening notes, an `agent.go` doc comment now stale since droid implements
  `ConfigWriter`, a mode-normalization side effect in `writeDroidSettingsFile`,
  among others) — none blocked merge; see the ledger for the full list and
  reasons. The four still open from the TUI phase are named in
  `.superpowers/sdd/progress.md`: a picker clamp test that measures the cursor
  reset rather than the clamp (the property is structurally guaranteed — the
  fix is a rename), `isTTY`'s real body never being invoked by any test, the
  headless program tests failing by hanging rather than asserting, and a dead
  `opts.Agent != nil` branch in `rootOrDone()` left as defense in depth per
  Landmine 15.
- **`go 1.25`** in `go.mod` is a **security floor, not a dependency floor** —
  and that distinction is the whole point. No dependency requires it:
  bubbletea needs 1.24.0 and cobra 1.15, so the *code* would still build on
  1.24. It reads 1.25 because 1.24 went end-of-life and CI (which builds every
  released binary) found 27 reachable stdlib vulnerabilities on it, ten of
  them with no 1.24 fix in existence — see Landmine 25's third clause for the
  evidence and the raise-or-don't test. The history: Phase 1 chose 1.22
  deliberately, Phase 2 moved to 1.24 because bubbletea v1.3.8 required it (a
  genuine dependency floor), and the CI/CD phase moved it to 1.25 for
  security. Deliberately minor-only, no patch component, so `setup-go`
  resolves the newest 1.25.x rather than pinning the oldest. Raise it again
  when 1.25 goes EOL, or when a dependency or feature genuinely needs more —
  not to satisfy a linter.

## How this was built

Spec → plan → subagent-driven execution: one fresh implementer subagent per task,
a spec+quality review after each, fix rounds until clean, then a whole-branch
review. Task briefs were extracted to files rather than pasted into prompts.

**The single most useful lesson, if you continue this way:** the Important
findings are overwhelmingly defects in *the plan's test code*, not in the
implementers' work — tests that pass, or could never pass, for the wrong
reason. Phase 1: nine of ten. The TUI phase: eight more, across seven of its
eleven tasks. Real examples from this repo:

- `strings.Contains(out, "installed")` could never fail — `"installed"` is a
  substring of `"not installed"`.
- A required-flag test passed with the guard deleted, because a different
  error surfaced and it only checked `err != nil`.
- A `Suggest` test was impossible to pass: the query matched nothing, and
  `Suggest` is literal substring containment, not fuzzy matching. **This one
  recurred in the TUI phase after Phase 1 had already recorded it here** —
  writing the lesson down did not stop me reproducing it.
- A cursor-boundary test could not tell "stop" from "wrap": its fixture had
  two selectable rows, which form a 2-cycle under wraparound, so both
  semantics produced identical results for the press counts used.
- A price-ceiling test asserted a set `Apply` could never produce — a free
  model clears any positive ceiling.
- A refresh test held for *any* implementation, because a cold cache fetches
  regardless of the flag.
- View assertions satisfied by text that renders unconditionally (a filter
  name that also appears in the key footer).

Two things caught these reliably. **Naming the failure pattern explicitly in
reviewer prompts**, with concrete examples from this repo. And **requiring a
mutation check per non-obvious behavior**: break it deliberately, watch the
named test fail, revert. A test you have never seen fail is not evidence.
Ask of every test: *would this fail if the behavior it names were broken?*

## Verify the tree is sound

`make ci` runs the **mechanical** checks below in one command:

```
fmt-check vet lint lint-cross lint-workflows tidy-check cross \
security test-race cover-check test-isolated
```

(there is no bare `test` in that list because the full suite runs inside
`cover-check`.) The `/tmp/orl` lines below are **not** covered by it, and
no CI job performs them either: they hit the live OpenRouter API, so they
are a manual smoke test you run by hand.

`.github/workflows/ci.yml` invokes those same Makefile targets rather than
re-spelling the commands in YAML — but it is *not* literally `make ci`, and
three differences are deliberate (verified against the actual `ci` target
and the actual `ci.yml`, item by item — `lint-workflows` is deliberately
**not** in this list: both `make ci` and `ci.yml`'s `quality` job now run
it, so it is no longer a divergence; see below):

1. the `quality` job runs `golangci/golangci-lint-action` where `make ci`
   runs `make lint`. The action installs the pinned binary and puts it on
   `PATH`, which is exactly what the following `make lint-cross` step then
   reuses;
2. the `audit` job runs `make tools` first, to build all four pinned
   analysis binaries with that job's own toolchain (Landmine 25). The
   `quality` job installs only actionlint the same way (`make
   tools-actionlint`), not the other three `make tools` also builds —
   golangci-lint is already on `PATH` via the action above, and gosec /
   govulncheck belong to `audit`, not `quality`;
3. the `audit` job adds the gosec SARIF report, its failed-analysis guard,
   and the upload steps, none of which exist as a Makefile target.

`lint-workflows` itself used to be a fourth, undocumented divergence: it was
added to `make ci` in the final fix wave, but nothing in `ci.yml` ever ran
it, so it validated workflow YAML only on a contributor's machine — never on
a Dependabot PR, the one case the addition was written to cover, since
Dependabot PRs never run a local `make ci`. Fixed by giving `ci.yml`'s
`quality` job its own `make tools-actionlint` + `make lint-workflows` steps
(above), so the exact same Makefile target now runs in both places and the
rationale is actually met.

```bash
go test ./... -count=1
go test ./internal/tui/ -race -count=1
go vet ./... && gofmt -l .
GOOS=windows go build ./... && GOOS=darwin go build ./...

# Landmine 8: nothing may depend on Claude Code, pi, hermes, or cline
# being installed here. Landmine 27: use the target, never a hand-written
# PATH — a hardcoded /usr/local/go/bin strips `go` itself on a CI runner.
make test-isolated

# --- live-API smoke test; NOT run by `make ci` or by any CI job ---
go build -o /tmp/orl . && /tmp/orl agents
/tmp/orl models --tools=false | head -5
/tmp/orl models | head -5
/tmp/orl bogus                      # must error: unknown command
/tmp/orl < /dev/null                # must refuse, naming --model
```

`make test-isolated` is the line that catches machine-dependent tests. It
must be fully green — `claude`, `pi`, `hermes`, and (since Task 9) `cline`
are all really installed on this machine (see Landmine 8), and a test that
forgot to isolate `HOME` passes here and fails everywhere else.

**Write-site verification** (Landmine 6, four sites — see the table above):
grep for every write primitive, `CreateTemp` included. This is the exact
form that matters: a pattern using bare `Create` does **not** catch
`os.CreateTemp(...)` (the atomic-write shape `config.Save` and
`writeDroidSettingsFile` both use, since `Create` requires `Create(`
immediately and `CreateTemp(` never matches that) — an earlier revision of
this check used exactly that narrower pattern and would have silently
missed both.

```bash
grep -rn "os.WriteFile\|os.Create\|os.MkdirAll\|os.Rename\|OpenFile\|CreateTemp" \
  --include="*.go" . | grep -v _test
```

Expected hits, exhaustively: `internal/openrouter` (`cache.go`, writing
`models.json`), `internal/config` (`config.go`, writing `config.json`),
`internal/launch/handoff.go` (`stageFiles`, staged files under our config
dir), and `internal/agent/droid.go` (`Apply`/`restore`/
`writeDroidSettingsFile`, writing `~/.factory/settings.local.json`). Any
other file — any other hit inside `internal/agent` in particular — is a
Critical defect per Landmine 6. Confirmed exhaustive against exactly these
four files, 2026-08-09 (Task 6).

This grep is no longer just a manual step: `TestWriteSitesAreExhaustivelyEnumerated`
(`writesites_test.go`, package `main`, root of the module) runs the same
check as a regression tripwire on every `go test ./...` — it walks every
non-test `.go` file, matches the same write-primitive pattern, and fails if
a hit turns up outside `writeSiteAllowlist`'s four files, or if one of
those four stops having a hit (the allowlist going stale in the safe
direction). The grep above is still worth running by hand when auditing,
but the tree cannot silently regress between audits anymore.

On the two `/tmp/orl models` lines above (the live-API ones): bare `models`
should be a subset of
`models --tools=false` — `config.defaults()` sets `Filters.ToolsOnly: true`,
and a coding agent without tool calling is unusable, so the unfiltered form
only widens the list. If you have ever persisted a config with
`Filters.ToolsOnly: false` (`$XDG_CONFIG_HOME/openrouter-launch/config.json`),
that saved value wins over the built-in default and the two commands will
list the same models — check the file before assuming a bug.
