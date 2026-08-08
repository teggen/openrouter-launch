# openrouter-launch — Handoff

**Last updated:** 2026-08-08 · **State:** Phase 1 complete, Phase 2's
prerequisite refactor complete — both pushed to `main`

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
| Phase 2 | Prerequisite refactor **done** (`internal/launch`, see below); the bubbletea TUI itself is not started |

Working commands, all smoke-tested against the live API:

```bash
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

## Phase 2 — the TUI

The prerequisite refactor is **done**. `internal/launch` now owns the launch
sequence and never touches the terminal:

- `launch.Service{Catalog, Run}` carries both seams as nilable fields. The
  `cli.runner` and `cli.catalogSource` globals are gone; `NewRootCmdWith`
  takes the service, and the TUI will take the same instance.
- `Service.Plan` runs the nine guards and returns a built `agent.Command`
  plus `[]Warning`. A `Warning` with a non-empty `Question` is one the
  caller must confirm.
- Hard stops are typed errors carrying their data: `NotInstalledError.Hint`,
  `UnknownModelError.Suggestions`, `UnsupportedAgentError.Reason`.
- `Service.Launch` records the selection and hands off in one function, so
  Landmine 5's ordering cannot be inverted by a call site. Its `warn`
  callback exists because on Unix nothing after the handoff runs.
- `launch.MergeFilters` bridges `config.Filters` and `openrouter.Filter`.
  `models` reads persisted filters; only the TUI will write them.

Still to do for the TUI itself:

1. Root gains a `RunE` for bare invocation.
2. The `launch.ErrNoModel` branch in `resolveAndRun` becomes "open the
   picker" instead of an error.
3. `internal/tui` imports `internal/launch`. It must never import
   `internal/cli` — `cli` imports `tui`, so that would be a cycle. This is
   why the planner is its own package.
4. Reconsider the shared mutable `&Claude{}` in the registry if a background
   refresh goroutine ever races the `LookPath` field tests patch.

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
- **`go 1.22`** in `go.mod` is a deliberate compatibility floor, not an oversight.
  Nothing in the code needs past it (`slices` is 1.21). The toolchain that builds
  it is whatever the user has — 1.26.5 today. Bump only when a feature requires it.
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
