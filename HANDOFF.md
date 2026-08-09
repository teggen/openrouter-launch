# openrouter-launch — Handoff

**Last updated:** 2026-08-09 · **State:** Phase 4 complete (4a + 4b) — all eight Tier 2 launchers shipped (pi, hermes, qwen, cline, kimi, omp, openclaw, droid); pi/hermes/cline live-verified, the other five (qwen, kimi, omp, openclaw, droid) doc-verified-only — their live gates were skipped by owner decision, droid's routing proof most importantly (see Open items)

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
| Phase 2 | Complete: root screen, model picker, filters, profile save, API-key prompt |
| Phase 3 | Complete: codex + opencode launchers, Tier 3 registry, live-verified against OpenRouter |
| Phase 4a | Complete: six zero-touch Tier 2 launchers — pi, hermes, qwen, cline, kimi, omp — plus shared passthrough-conflict helpers (`internal/agent/args.go`) and the `CredentialShadowCheck` advisory capability (`WarnShadowedCredential`). Live-gated end to end through the built binary: pi, hermes, cline (Task 9). Doc-verified-only, gate skipped by owner scope: qwen, kimi, omp. |
| Phase 4b | Complete: the `Staged` capability (write site #3, launcher-owned files, boundary-checked in `stageFiles`), `openclaw` (a `Staged` consumer sharing omp's `openrouter/`-prefix dialect), the fork-and-wait launch path (`agent.RunWait` + `launch.launchConfigWriter`), and `droid` (the first `ConfigWriter`, write site #4, marker-owned entry in `~/.factory/settings.local.json`). Task 5's live gates for both new agents were skipped by owner decision (2026-08-09) — openclaw and droid ship doc-verified-only, same posture as qwen/kimi/omp. **Tier 2 is now complete: all eight agents shipped.** |
| Tests | 432 total, 169 of them in `internal/tui` (unchanged since Phase 3 — no TUI screens touched in 4a or 4b); the growth from 411 (Phase 4a's count) is Phase 4b's `Staged`/openclaw/fork-and-wait/droid work |
| Verification | `go test ./...` green, `go vet` clean, `gofmt -l .` empty, `-race` clean (including `internal/tui`), Windows/macOS cross-build clean, all confirmed 2026-08-09 (Task 6); the `HOME`-isolated machine-independence run (Landmine 8) also green |
| Agents shipped | claude, codex, opencode, plus all eight Tier 2 agents (pi, hermes, qwen, cline, kimi, omp, openclaw, droid); 3 desktop apps (chatgpt, claude-desktop, hermes-desktop) registered unsupported |
| CI | `.github/workflows/ci.yml` — quality, audit, three-OS test matrix (Windows/macOS advisory), machine-independence; all branches |
| Releases | tag-driven via GoReleaser; six 64-bit targets; stable tags on `main`, `-beta.N` on `develop`, guard-enforced |
| Pushed | Yes — `origin/main` is current as of Phase 4b (Task 6 push). Check `git status -sb` rather than trusting this row; it has been wrong before (see the strikethrough history in earlier revisions of this file). |

Working commands, all smoke-tested against the live API:

```bash
openrouter-launch agents
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
docs/superpowers/specs/2026-08-08-phase-3-agents-design.md               spec for codex/opencode + Tier 3, with live-verified values
docs/superpowers/plans/2026-08-07-phase-1-core.md                        the Phase 1 plan
docs/superpowers/plans/2026-08-07-phase-2-planner-refactor.md            the plan that built internal/launch
docs/superpowers/plans/2026-08-08-phase-2-tui.md                         the plan that built internal/tui
docs/superpowers/plans/2026-08-08-phase-3-agents.md                      the plan that built Phase 3 (its wire_api="chat" is the frozen pre-verification value; the spec records the correction)
docs/superpowers/specs/2026-08-09-phase-4-tier-2-agents-design.md        spec for pi/hermes/qwen/cline/kimi/omp/openclaw/droid, live-verification results appended
docs/superpowers/plans/2026-08-09-phase-4a-tier-2-zero-touch.md          the plan that built Phase 4a (this phase)
docs/superpowers/plans/2026-08-09-phase-4b-configwriter-openclaw-droid.md  the plan that built Phase 4b — Staged, openclaw, fork-and-wait, droid; complete
CLAUDE.md                                                                quick operational layer for Claude Code sessions; points here
.superpowers/sdd/progress.md                                             Phases 1-2 build ledger (gitignored)
.superpowers/sdd/*-report.md                                             Phases 1-2 per-task reports (gitignored)
.superpowers/sdd/2026-08-08-phase-3-agents/                              Phase 3 workspace: ledger (progress.md), task briefs/reports, live-*.log evidence cited by Landmine 18 and Open items
.superpowers/sdd/2026-08-09-tier-2-research/                             per-agent doc-verification notes (pi.md, hermes.md, qwen.md, cline.md, kimi.md, omp.md, openclaw.md, droid.md, findings.md) written before any Phase 4 code
.superpowers/sdd/2026-08-09-phase-4a-tier-2-zero-touch/                  Phase 4a workspace: ledger (progress.md), task briefs/reports, whole-branch review diffs
.superpowers/sdd/2026-08-09-phase-4a/                                    Task 9's live-gate evidence: live-{pi,hermes,cline}-*.log (12 files)
.superpowers/sdd/2026-08-09-phase-4b-configwriter-openclaw-droid/        Phase 4b workspace: ledger (progress.md), task briefs/reports, whole-branch review diffs per task, Task 6's verification report

main.go                      entry point + exit-code extraction
internal/openrouter/         model type, HTTP catalog client, disk cache, filters
internal/config/             XDG config, API key resolution, profile CRUD
internal/agent/              Launcher interface, registry, Claude launcher, process handoff
internal/launch/             the terminal-free planner: guards, warnings, typed conditions
internal/tui/                the bubbletea screens and the session driver
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

**6. Zero-touch is absolute — amended twice (Phase 4b Tasks 1 and 4) to its
final four-site form.** The original invariant was "exactly two write
sites, both launcher-owned." The principle was always "never write an
**agent's** files"; Phase 4b made the launcher-owned side explicit and
added the one sanctioned agent-owned exception. This table matches the
Phase 4 spec's (`docs/superpowers/specs/2026-08-09-phase-4-tier-2-agents-design.md`)
verbatim:

| # | Path | Owner | Written by | Secret? |
|---|---|---|---|---|
| 1 | `$XDG_CACHE_HOME/openrouter-launch/models.json` | launcher | `openrouter.Cache` | no |
| 2 | `$XDG_CONFIG_HOME/openrouter-launch/config.json` | launcher | `internal/config` | yes (0600) |
| 3 | `$XDG_CONFIG_HOME/openrouter-launch/openclaw.json` | launcher | `Staged` materializer in `launch.Service.Launch` (`stageFiles`) | **no** (model ref only; key stays in env) |
| 4 | `~/.factory/settings.local.json` | **agent (droid)** | `ConfigWriter.Apply`, capability-gated, marker-owned entries only, restore on exit | **no** (`"apiKey": "${OPENROUTER_API_KEY}"` interpolation) |

Rules that survive unchanged: no other writes anywhere in the tree,
verified by exhaustive grep and now pinned by
`TestWriteSitesAreExhaustivelyEnumerated` (see "Verify the tree is
sound"); the API key is never written outside site 2; `Command()` stays
pure — sites 3 and 4 are materialized by the launch service or `Apply`,
never by a launcher's `Command` method. Any code writing into an agent's
own config outside `ConfigWriter`, or any write anywhere in `internal/agent`
outside `droid.go`'s `Apply`/`restore`/`writeDroidSettingsFile`, is a
Critical defect.

**7. `CheckModel` incompatibility is advisory.** Warn and confirm; never abort.
Claude Code with a non-`anthropic/*` model works for many models; OpenRouter only
warns that some context-management features may fail. Hard-blocking would make the
tool refuse valid setups.

**8. Tests that need a binary to look ABSENT must set `HOME` to a temp dir.**
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

**24. A `ConfigWriter` agent (currently only droid) never takes the
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

## Phase 2 — complete

The TUI ships: root screen (profiles + agents), model picker with
type-to-search and four filters, `ctrl+s` profile save, API-key prompt,
notice screens for the planner's typed errors.

Four deliberate divergences from the original design doc, all recorded in
`docs/superpowers/specs/2026-08-08-phase-2-tui-design.md`:

- **Filters are on `alt+t/f/c/p`, not bare `t/f/c/$`.** The original key
  table collided with type-to-search: `anthropic` contains a `t`. Key
  handling switches on `msg.String()` so `"alt+t"` is a distinct case from
  `"t"`; a mutation test pins that a chord can never fall through to the
  search box.
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

## Open items

- **The interactive TUI was driven by a human for the first time in the
  2026-08-08 smoke-test launch** (see "Current state"): the picker rendered,
  `moonshotai/kimi-k3` was picked, the Landmine 7 advisory confirm was
  accepted, and the handoff produced a working session. Which entry point was
  used — bare `openrouter-launch` (root screen) or `openrouter-launch claude`
  (straight to the picker) — went unrecorded, so the root screen specifically
  remains human-unverified. Also still unverified: the picker's type-to-search
  and `alt+t/f/c/p` filter chords, `ctrl+s` profile save, `esc` navigation
  back out of a screen, and whether filter state actually persists to
  `config.json` on exit.
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

  Closing this means giving those tests platform-aware fixtures, not changing
  the launchers. Until then `test (windows-latest)` stays `continue-on-error`;
  `test (macos-latest)` is now a candidate to have the flag removed, since it
  is green on its first run.
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

`make ci` runs all of the below in one command, and is exactly what
`.github/workflows/ci.yml` invokes.

```bash
go test ./... -count=1
go test ./internal/tui/ -race -count=1
go vet ./... && gofmt -l .
GOOS=windows go build ./... && GOOS=darwin go build ./...
go build -o /tmp/orl . && /tmp/orl agents
/tmp/orl models --tools=false | head -5
/tmp/orl models | head -5
/tmp/orl bogus                      # must error: unknown command
/tmp/orl < /dev/null                # must refuse, naming --model

# Landmine 8: nothing may depend on Claude Code, pi, hermes, or cline
# being installed here.
HOME=$(mktemp -d) PATH="/usr/local/go/bin:/usr/bin:/bin" go test ./... -count=1
```

The `HOME` line is the one that catches machine-dependent tests. It must be
fully green — `claude`, `pi`, `hermes`, and (since Task 9) `cline` are all
really installed on this machine (see Landmine 8), and a test that forgot
to isolate `HOME` passes here and fails everywhere else.

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

The last two hit the live OpenRouter API. Bare `models` should be a subset of
`models --tools=false` — `config.defaults()` sets `Filters.ToolsOnly: true`,
and a coding agent without tool calling is unusable, so the unfiltered form
only widens the list. If you have ever persisted a config with
`Filters.ToolsOnly: false` (`$XDG_CONFIG_HOME/openrouter-launch/config.json`),
that saved value wins over the built-in default and the two commands will
list the same models — check the file before assuming a bug.
