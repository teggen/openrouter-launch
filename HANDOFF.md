# openrouter-launch — Handoff

**Last updated:** 2026-08-08 · **State:** Phase 2 complete — TUI shipped

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
| Verification | `go test ./...` green, `go vet` clean, `gofmt -l .` empty, Linux/macOS/Windows cross-build |
| Agents shipped | Claude Code only |
| Phase 2 | Complete: root screen, model picker, filters, profile save, API-key prompt |

Working commands, all smoke-tested against the live API:

```bash
openrouter-launch                     # bare invocation: opens the root screen
openrouter-launch claude              # no -m: straight to the picker
openrouter-launch agents
openrouter-launch models --tools --free --provider anthropic
openrouter-launch models --min-context 200000 --max-price 5
openrouter-launch claude -m anthropic/claude-opus-4.6 -- --resume
openrouter-launch profile add --name opus-cc --agent claude --model anthropic/claude-opus-4.6
openrouter-launch profile list|launch|rm|rename
```

## Where things are

```
docs/superpowers/specs/2026-08-07-openrouter-launch-design.md            the spec — read for WHY
docs/superpowers/specs/2026-08-07-phase-2-planner-refactor-design.md     spec for the internal/launch refactor
docs/superpowers/plans/2026-08-07-phase-1-core.md                        the Phase 1 plan
docs/superpowers/plans/2026-08-07-phase-2-planner-refactor.md            the plan that built internal/launch
.superpowers/sdd/progress.md                                             build ledger (gitignored)
.superpowers/sdd/*-report.md                                             per-task reports (gitignored)

main.go                      entry point + exit-code extraction
internal/openrouter/         model type, HTTP catalog client, disk cache, filters
internal/config/             XDG config, API key resolution, profile CRUD
internal/agent/              Launcher interface, registry, Claude launcher, process handoff
internal/launch/             the terminal-free planner: guards, warnings, typed conditions
internal/tui/                the bubbletea screens and the session driver
internal/cli/                cobra command tree
```

The ledger at `.superpowers/sdd/progress.md` is gitignored but present in the
working tree. It records every task's commits, every review finding, and the
reasoning behind each deferral. **Read it before re-litigating any decision.**

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

`ConfigWriter` is the **escape hatch** for a future agent with no zero-touch
configuration path. No agent implements it. When one does, that agent takes a
fork-and-wait launch path instead of `syscall.Exec`, so its `restore` can run.

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

**6. Zero-touch is absolute.** The only two write sites in the entire tree are
`$XDG_CACHE_HOME/openrouter-launch/models.json` and
`$XDG_CONFIG_HOME/openrouter-launch/config.json`. Verified by exhaustive grep, not
assertion. Any code writing into an agent's own config is a Critical defect.

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

## Phase 2 — complete

The TUI ships: root screen (profiles + agents), model picker with
type-to-search and four filters, `ctrl+s` profile save, API-key prompt,
notice screens for the planner's typed errors.

Two deliberate divergences from the original design doc, both recorded in
`docs/superpowers/specs/2026-08-08-phase-2-tui-design.md`:

- **Filters are on `alt+t/f/c/p`, not bare `t/f/c/$`.** The original key
  table collided with type-to-search: `anthropic` contains a `t`. Key
  handling switches on `msg.String()` so `"alt+t"` is a distinct case from
  `"t"`; a mutation test pins that a chord can never fall through to the
  search box.
- **`go 1.22` became `go 1.24`**, which bubbletea requires from v1.3.8.

**Deferred:** the background catalog refresh streaming into the live picker.
The cache carries a 24h TTL, so a warm cache is already current and the only
window it improves is the moment after expiry — for a goroutine, a channel,
and re-ranking a list the user is navigating. Note that Phase 2 note 4, on
the shared mutable `&Claude{}` in the registry, was conditional on that
goroutine and therefore does not apply.

## Phase 3+ — more agents

The spec's agent tiers are in the design doc. Tier 1 (mechanism verified):
`claude` shipped; `codex` (repeated `-c` overrides) and `opencode`
(`OPENCODE_CONFIG_CONTENT` inline JSON) are next and their mechanisms are
confirmed from the ollama source. Tier 2 (`qwen`, `droid`, `hermes`, `cline`,
`kimi`, `omp`, `openclaw`, `pi`) each need their mechanism verified against their
own documentation *before* code is written. Tier 3 (`copilot`, `pool`, and the
desktop apps) genuinely cannot be pointed at OpenRouter and stay registered with a
stated reason.

## Open items

- **Windows exit-code propagation is unverified on real Windows.** The extraction
  logic is unit-tested with a synthetic `*exec.ExitError`, but nobody has run the
  binary on Windows.
- **10 of 17 deferred Minor findings** were triaged as defer-or-drop with reasons,
  recorded in the ledger. Seven were fixed.
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

**The single most useful lesson, if you continue this way:** nine of the ten
Important findings during implementation were defects in *the plan's test code*,
not in the implementers' work — tests that passed for the wrong reason. Real
examples: `strings.Contains(out, "installed")` could never fail because
`"installed"` is a substring of `"not installed"`; a required-flag test still
passed with the guard deleted because a different error surfaced and it only
checked `err != nil`; a `Suggest` test was outright impossible to pass because the
query matched nothing. Naming that specific failure pattern in reviewer prompts
caught it every time afterward. Ask of every test: *would this fail if the
behavior it names were broken?*

## Verify the tree is sound

```bash
go test ./... -count=1
go vet ./... && gofmt -l .
GOOS=windows go build ./... && GOOS=darwin go build ./...
go build -o /tmp/orl . && /tmp/orl agents
/tmp/orl models --tools=false | head -5
/tmp/orl models | head -5
```

The last two hit the live OpenRouter API. Bare `models` should be a subset of
`models --tools=false` — `config.defaults()` sets `Filters.ToolsOnly: true`,
and a coding agent without tool calling is unusable, so the unfiltered form
only widens the list. If you have ever persisted a config with
`Filters.ToolsOnly: false` (`$XDG_CONFIG_HOME/openrouter-launch/config.json`),
that saved value wins over the built-in default and the two commands will
list the same models — check the file before assuming a bug.
