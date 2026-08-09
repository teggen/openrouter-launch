# openrouter-launch — Handoff

**Last updated:** 2026-08-09 · **State:** Phase 4a complete — six zero-touch Tier 2 launchers (pi, hermes, qwen, cline, kimi, omp); pi/hermes/cline live-verified, qwen/kimi/omp doc-verified-only

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
| Branch | `main` — user chose direct-to-main, no feature branches |
| Phase 1 | Complete: 27 commits, 137 tests, ~1,570 LOC + ~2,510 test LOC |
| Phase 2 | Complete: root screen, model picker, filters, profile save, API-key prompt |
| Phase 3 | Complete: codex + opencode launchers, Tier 3 registry, live-verified against OpenRouter |
| Phase 4a | Complete: six zero-touch Tier 2 launchers — pi, hermes, qwen, cline, kimi, omp — plus shared passthrough-conflict helpers (`internal/agent/args.go`) and the `CredentialShadowCheck` advisory capability (`WarnShadowedCredential`). Live-gated end to end through the built binary: pi, hermes, cline (Task 9). Doc-verified-only, gate skipped by owner scope: qwen, kimi, omp. |
| Tests | 411 total, 169 of them in `internal/tui` (unchanged since Phase 3 — no TUI screens touched this phase); the Phase 4a agents and their shared helpers account for the growth from 372 |
| Verification | `go test ./...` green, `go vet` clean, `gofmt -l .` empty, `-race` clean, Linux/macOS/Windows cross-build |
| Agents shipped | claude, codex, opencode, pi, hermes, qwen, cline, kimi, omp; 3 desktop apps registered unsupported |
| Pushed | Yes — `origin/main` is current as of Phase 4a (Task 10 push). It had been 47 commits behind for the whole planner refactor and TUI build, and separately ahead by 14 commits through the Phase 4a build before this push; a revision of this file has twice now wrongly claimed to be current when it wasn't. Check `git status -sb` rather than trusting this row. |

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
docs/superpowers/plans/2026-08-09-phase-4b-configwriter-openclaw-droid.md  the plan for Plan 4b — openclaw + droid, next
CLAUDE.md                                                                quick operational layer for Claude Code sessions; points here
.superpowers/sdd/progress.md                                             Phases 1-2 build ledger (gitignored)
.superpowers/sdd/*-report.md                                             Phases 1-2 per-task reports (gitignored)
.superpowers/sdd/2026-08-08-phase-3-agents/                              Phase 3 workspace: ledger (progress.md), task briefs/reports, live-*.log evidence cited by Landmine 18 and Open items
.superpowers/sdd/2026-08-09-tier-2-research/                             per-agent doc-verification notes (pi.md, hermes.md, qwen.md, cline.md, kimi.md, omp.md, openclaw.md, droid.md, findings.md) written before any Phase 4 code
.superpowers/sdd/2026-08-09-phase-4a-tier-2-zero-touch/                  Phase 4a workspace: ledger (progress.md), task briefs/reports, whole-branch review diffs
.superpowers/sdd/2026-08-09-phase-4a/                                    Task 9's live-gate evidence: live-{pi,hermes,cline}-*.log (12 files)

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
`Installer`, `Compatible`, `PlatformSupported`, `ConfigWriter`.

`ConfigWriter` is the **escape hatch** for an agent with no zero-touch
configuration path. `droid` implements it. When an agent implements it, that
agent takes a fork-and-wait launch path instead of `syscall.Exec`, so its
`restore` can run.

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

**6. Zero-touch is absolute.** Launcher-owned writes only: three write sites are
`$XDG_CACHE_HOME/openrouter-launch/models.json`,
`$XDG_CONFIG_HOME/openrouter-launch/config.json`, and staged files under the
config dir via `stageFiles` in `internal/launch/handoff.go`. One capability-gated
agent-owned exception: `droid` implements `ConfigWriter` to upsert a single
marker-owned entry into `~/.factory/settings.local.json` (fork-and-wait launch
path, restore on exit). Verified by exhaustive grep, not assertion. Any code
writing into an agent's own config outside `ConfigWriter` is a Critical defect.

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

**Next: Plan 4b** (`docs/superpowers/plans/2026-08-09-phase-4b-configwriter-openclaw-droid.md`)
adds `openclaw` (a `Staged`-launch variant sharing omp's `openrouter/`-prefix
slug dialect, Landmine 21) and `droid` (the first agent to implement
`ConfigWriter`, taking the fork-and-wait launch path instead of
`syscall.Exec` so its `restore` can run afterward — see "Architecture in one
page" above for why that escape hatch exists and has sat unused until now).

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
- **Six new agents' interactive TUIs have not been driven by a human.** pi,
  hermes, qwen, cline, kimi, and omp were only ever exercised through their
  own headless/one-shot flags — three of them live (pi, hermes, cline,
  Task 9), three doc-verified-only (qwen, kimi, omp, per above). Nobody has
  sat at `openrouter-launch pi` (etc.) with no passthrough args and driven
  the agent's own interactive session end to end — the same gap Phase 3
  left open for codex/opencode, now six agents wider.
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
  use). The four still open from the TUI phase are named in
  `.superpowers/sdd/progress.md`: a picker clamp test that measures the cursor
  reset rather than the clamp (the property is structurally guaranteed — the
  fix is a rename), `isTTY`'s real body never being invoked by any test, the
  headless program tests failing by hanging rather than asserting, and a dead
  `opts.Agent != nil` branch in `rootOrDone()` left as defense in depth per
  Landmine 15.
- **`go 1.24`** in `go.mod` is bubbletea's floor (from v1.3.8), not an oversight
  — see "Phase 2 — complete" above. It replaced Phase 1's deliberate 1.22 floor;
  nothing else in the code needs past 1.24. The toolchain that builds it is
  whatever the user has — 1.26.5 today. Bump only when a feature requires it.
- No `README.md` yet.
- No CI. No release/packaging story.

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

**Write-site verification** (Landmine 6): the three launcher-owned write sites
are `models.json` (cache), `config.json` (config), and staged files written in
`stageFiles` (`internal/launch/handoff.go`). Exhaustive grep to verify nothing
else writes into the tree:

```bash
grep -rE '(WriteFile|MkdirAll|Create|OpenFile|Truncate)\s*\(' \
  internal/ | grep -v '_test\.go' | grep -v '\.go:.*\/\/'
```

Expected output: only `config.Save` (config.json), `openrouter.Snapshot` with
cached models, `stageFiles` (staged files under config dir), and `droid.go`
(`~/.factory/settings.local.json` via ConfigWriter). Any other write is a
Critical defect per Landmine 6.

The last two hit the live OpenRouter API. Bare `models` should be a subset of
`models --tools=false` — `config.defaults()` sets `Filters.ToolsOnly: true`,
and a coding agent without tool calling is unusable, so the unfiltered form
only widens the list. If you have ever persisted a config with
`Filters.ToolsOnly: false` (`$XDG_CONFIG_HOME/openrouter-launch/config.json`),
that saved value wins over the built-in default and the two commands will
list the same models — check the file before assuming a bug.
