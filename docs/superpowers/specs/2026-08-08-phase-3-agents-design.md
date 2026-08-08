# Phase 3 — Codex and OpenCode launchers, Tier 3 registry

**Date:** 2026-08-08 · **Status:** approved design
**Depends on:** `2026-08-07-openrouter-launch-design.md` (the main spec),
`2026-08-07-phase-2-planner-refactor-design.md`, `2026-08-08-phase-2-tui-design.md`

## Goal

Add the two remaining Tier 1 agents — `codex` (OpenAI Codex CLI) and
`opencode` — as zero-touch launchers, and register three Tier 3 desktop
apps as unsupported-with-reason so their absence is explained in listings.

## Scope

**In:**

- `internal/agent/codex.go` — Codex launcher, pure `Command()`, `Installable`.
- `internal/agent/opencode.go` — OpenCode launcher, pure `Command()`, `Installable`.
- A shared stub launcher type for unsupported agents (Landmine 10 forbids a
  nil `Spec.Launcher`).
- Registry entries: `codex`, `opencode` (supported), and `chatgpt`,
  `claude-desktop`, `hermes-desktop` (unsupported, with reasons).
- Live headless verification of both integrations against the real
  OpenRouter API, then a human interactive smoke test.

**Out (owner decisions during brainstorming):**

- Tier 2 agents (`qwen`, `droid`, `hermes`, `cline`, `kimi`, `omp`,
  `openclaw`, `pi`) — each needs its mechanism verified against its own
  documentation first; that is its own phase.
- `copilot`, `pool`, and `vscode` are **not** registered, although the main
  spec's Tier 3 table lists them. The owner chose to register only the
  desktop apps. Consequence, accepted explicitly: `openrouter-launch
  copilot` reports "unknown agent" rather than an explained refusal.
- No auto-installer (`Installer` capability) for any agent — install hints
  only, matching `claude`.
- No `ConfigWriter` implementation. It remains the escape hatch; see
  Contingency below.

## Approach (chosen: per-agent launchers)

Considered and rejected:

- **A shared "OpenAI-compatible" base launcher** parameterized per agent —
  premature: three agents use three mechanisms (env, CLI flags, inline
  JSON). Extract a base only if Tier 2 later yields several true
  `OPENAI_BASE_URL` clones.
- **Porting ollama's current codex integration** — ollama's
  `cmd/launch/codex.go` now writes `~/.codex/ollama-launch.config.toml`
  plus a model-catalog JSON and ships a `Restore`. Writing agent-owned
  config is a Critical defect here (Landmine 6, zero-touch). The `-c`
  override list it feeds codex is the mechanism we use directly.

Each new agent is its own file with a pure `Command()`, tested by comparing
`Command` structs — the `claude` pattern.

## Codex launcher

`Codex` struct in `internal/agent/codex.go`.

**Command line.** Managed args first, then user passthrough args. Codex
options are global, so managed flags apply even when the passthrough starts
with a subcommand such as `resume`:

```
codex
  -c model_provider="openrouter"
  -c model_providers.openrouter.name="OpenRouter"
  -c model_providers.openrouter.base_url="https://openrouter.ai/api/v1"
  -c model_providers.openrouter.env_key="OPENROUTER_API_KEY"
  -c model_providers.openrouter.wire_api="responses"
  -m <slug>
  <passthrough args…>
```

The base URL here is `openrouter.DefaultBaseURL` (**with** `/v1` — unlike
Claude Code's `agent.AnthropicBaseURL`; Landmine 1 cuts the other way for an
OpenAI-compatible client, which appends `/chat/completions`, not a version
segment).

**Env.** `OPENROUTER_API_KEY=<resolved key>` is set explicitly in
`Command.Env`, so `env_key` resolves even when the user's shell never
exported the key. `ExecArgs`' dedupe (Landmine 3) makes our value win over a
stray export.

**Conflict rejection.** Passthrough args that would defeat the managed
config are rejected with an error naming the conflicting argument: `-m`,
`--model` (and `=`/attached forms), and any `-c`/`--config` override whose
key is `model`, `model_provider`, or starts with `model_providers.`.
Rationale: later `-c` wins in codex, so a user-supplied provider override
after ours would silently point the agent somewhere else while the tool
reports success — the same silent-loss class Landmine 3 exists for. This
follows ollama's `codexValidateExtraArgs`, minus the keys for features we
don't use (`profile`, `model_catalog_json`).

**`wire_api = "responses"`** is the verified value on codex 0.146.1. The
originally planned `"chat"` was tried first live and codex 0.146.1 rejects
it outright (`Error loading config.toml: 'wire_api = "chat"' is no longer
supported. ... set 'wire_api = "responses"'`); `"responses"` was retried
and produced a real completion through OpenRouter. See Testing and
verification below for the dated record.

**Installable.** `LookPath("codex")` only; npm global installs land on
`PATH`. Hint: `npm install -g @openai/codex`.

**Not implemented, deliberately:**

- No version check — it would be impure in `Command()`, and ollama's
  `>= 0.134.0` floor exists for its model-catalog feature, which we don't
  use.
- No `Compatible` warning — every OpenRouter model speaks the
  OpenAI-compatible surface; there is no known bad pairing to warn about.

## OpenCode launcher

`OpenCode` struct in `internal/agent/opencode.go`.

**Command line.** `opencode <passthrough args…>` — configuration travels
entirely in env. Passthrough `-m`/`--model` (and `=`/attached forms) are
rejected, same rationale as codex: the CLI flag would silently beat the
inline config.

**Env.** Two variables:

- `OPENCODE_CONFIG_CONTENT` — minimal inline JSON using opencode's native
  OpenRouter provider:

  ```json
  {"$schema": "https://opencode.ai/config.json", "model": "openrouter/<slug>"}
  ```

- `OPENROUTER_API_KEY=<resolved key>` — opencode's built-in openrouter
  provider reads it.

Model references are `provider/model` split on the **first** slash, so
`openrouter/anthropic/claude-opus-4.6` must parse as provider `openrouter`,
model `anthropic/claude-opus-4.6`. Verified live during the build.

**Escalation path, still zero-touch:** if env-only auth turns out not to
work, the inline config grows an explicit provider block using opencode's
`{env:OPENROUTER_API_KEY}` substitution — config content changes, no file
is ever written. The build records which variant was verified.

**We do not copy from ollama:** its opencode integration writes
`~/.local/state/opencode/model.json` so models appear in opencode's picker.
That is an agent-owned file; skipping it costs only picker convenience
inside opencode.

**Installable.** `LookPath("opencode")`, then the curl installer's
`~/.opencode/bin/opencode` fallback — `opencode.exe` on Windows (ollama's
`findOpenCode` confirms both; the installer does not reliably edit `PATH`). Hint: `curl -fsSL
https://opencode.ai/install | bash` — printed, never executed. Tests that
need the binary absent must redirect `HOME` (Landmine 8 applies to the
fallback path).

## Tier 3 registry and stub launcher

One shared stub type in `internal/agent` satisfying `Launcher`:

```go
type stub struct{ name, display string }
func (s *stub) Command(Request) (Command, error) // returns an error; must be unreachable
```

`Spec.Launcher` must never be nil (Landmine 10) — the stub keeps the
`buildIndex` panic intact. The planner's `UnsupportedAgentError` fires on
`Status.Supported == false` before any `Command` call; a test pins that the
stub's error is unreachable through the launch path.

New registry entries, in display order after the supported agents:

| Name | Reason |
| --- | --- |
| `chatgpt` | ChatGPT / Codex desktop app authenticates through its own account; a launcher cannot inject a provider. |
| `claude-desktop` | Claude Desktop authenticates through its own account; a launcher cannot inject a provider. |
| `hermes-desktop` | Hermes desktop app authenticates through its own account; a launcher cannot inject a provider. |

No TUI or CLI changes: the root screen already renders
`unsupported: <reason>` rows, `agents` already lists status, and
`newLaunchCmds` builds `codex`/`opencode` subcommands from the registry
automatically.

## Contingency

If live verification shows pure `-c` overrides genuinely cannot configure
codex (or `OPENCODE_CONFIG_CONTENT` cannot configure opencode), that agent
routes to the `ConfigWriter` escape hatch **as its own explicit decision**
with the evidence recorded — never silently. This is expected not to
happen; both mechanisms are confirmed in ollama's source.

## Testing and verification

**Unit (no process spawned, per the purity rule):**

- `Command()` output compared as structs: full arg list, env pairs, model
  slug placement, passthrough ordering.
- Conflict rejection: each rejected form (`-m`, `--model`, `--model=x`,
  `-c model_provider=…`, `-c model_providers.x.y=…`,
  `--config=model=…`) errors and names the argument; benign passthrough
  (`resume`, `--full-auto`, `-c foo=bar`, bare `--config` with a benign
  key) passes through unchanged.
- Install hints and fallback paths; absent-binary tests redirect `HOME`
  (Landmine 8).
- Registry integrity: new names collide with nothing (neither new agent
  gets aliases — the names are already short); stub launcher non-nil;
  unsupported entries carry non-empty reasons.
- Stub unreachability: launching an unsupported agent yields
  `UnsupportedAgentError` from the planner, not the stub's error.

**Mutation checks** (per the handoff's core lesson): every non-obvious
behavior gets one — break it deliberately, watch the named test fail,
revert. Minimum set: drop a managed `-c` override, reorder managed args
after passthrough, remove the env dedupe interaction, weaken a conflict
check, swap the opencode model string to a single-segment form.

**TUI:** fixture coverage that a root screen containing unsupported entries
renders them dimmed with reasons (the rendering exists; it has never had
data).

**Live headless, during the build (owner-approved credit spend):**

- `codex exec` one-shot with a cheap model through the managed overrides —
  confirms `base_url`, `env_key`, `wire_api` on codex 0.146.1.
- `opencode run` one-shot — confirms the first-slash model split and
  env-only auth on opencode 1.0.69.
- Each verifies the reply actually came through OpenRouter (the response
  arrives with an OpenRouter generation id, or the failure names the real
  cause).

### Live verification results (2026-08-08)

Run against the real OpenRouter API with `openai/gpt-4o-mini`
(owner-approved cents-level spend), codex 0.146.1, opencode 1.0.69, from a
scratch working directory each time. Full logs (key redacted — never
appeared in any of them, confirmed by grep) live alongside this spec's task
workspace: `.superpowers/sdd/2026-08-08-phase-3-agents/live-*.log`.

- **Codex, raw mechanism.** `wire_api="chat"` is rejected outright by codex
  0.146.1: `Error loading config.toml: 'wire_api = "chat"' is no longer
  supported. ... set 'wire_api = "responses"'`. Retried with
  `wire_api="responses"`, which produced a real "OK" completion through
  OpenRouter. `codex.go` and its test now use `"responses"` (commit
  `20ed482`, `fix(agent): codex wire_api verified as responses`); `base_url`
  and `env_key` were confirmed correct on the same run.
- **Codex, through our binary.** `orl codex -m openai/gpt-4o-mini -- exec
  --skip-git-repo-check "…"` succeeded end to end with the same "OK" reply —
  managed global flags placed before the `exec` subcommand parse correctly.
  No passthrough-subcommand limitation was found; the speculative risk noted
  in the Codex launcher section above did not materialize.
- **OpenCode, raw mechanism.** Env-only auth worked on the first try —
  `OPENCODE_CONFIG_CONTENT` with `"model":"openrouter/openai/gpt-4o-mini"`
  plus `OPENROUTER_API_KEY` produced "OK" with exit 0. The explicit
  `provider.openrouter.options.apiKey` escalation in the Contingency section
  was not needed; no code or config-variant change was made.
- **OpenCode, through our binary.** `orl opencode -m openai/gpt-4o-mini --
  run "…"` also produced the "OK" completion, confirming the same
  mechanism through our arg/env plumbing. Both this run and a raw repeat
  then exited 1 with `Error: [DecimalError] Invalid argument: [object
  Object]` — but only *after* printing "OK". This reproduces identically
  whether invoked raw or through our binary and only starts appearing once
  opencode's own `~/.cache/opencode/models.json` catalog exists locally
  (absent on the very first-ever `opencode run`, which is why the raw check
  above came back clean) — it is opencode's own post-completion cost
  formatting that breaks on this model/provider pairing, not our config
  mechanism. Not one of the auth/protocol/model-not-found failure classes
  this phase's contingency plan covers, and not something `Command()`
  controls, so no code change follows from it; recorded here as a known
  opencode 1.0.69 rough edge.
- **Zero-touch audit.** `~/.codex/config.toml` is byte-identical before and
  after all codex runs (pre-run copy diffed against post-run file: no
  changes; mtime unchanged from before this phase's work). `~/.local/state/
  opencode/` and `~/.opencode` are untouched (same files, same mtimes,
  pre/post). Codex wrote its own session/sandbox bookkeeping
  (`goals_1.sqlite`, `installation_id`, `.sandbox_migration`) and opencode
  populated its own package/model-catalog cache under `~/.cache/opencode`
  and `~/.config/opencode` — both are the tools acting on their own behalf
  (Landmine 6 governs *our* writes, not a CLI's own operational state), not
  files this project's `Command()` or a `ConfigWriter` touched.

**Human interactive smoke test at the end** (same standard as Phase 2):
`openrouter-launch codex`, `openrouter-launch opencode`, picker flow,
and the root screen showing the new supported and unsupported rows.

**The Phase 2 verification suite stays green** — `go test ./... -count=1`,
`-race` on `internal/tui`, `go vet`, `gofmt -l`, Windows/macOS cross-builds,
the `HOME`-redirect full-suite run, and the zero-touch grep (still exactly
two write sites, neither in `internal/agent`).

## Landmines that bind this phase

- **1** — two base URLs: codex/opencode use `…/api/v1`; only Claude Code
  uses `…/api`.
- **3** — `ExecArgs` dedupe is what makes our env win; don't bypass it.
- **6** — zero-touch is absolute: no writes into `~/.codex`, opencode
  state, or anywhere else.
- **8** — absent-binary tests must redirect `HOME` (opencode's
  `~/.opencode/bin` fallback; codex is installed at `~/.local/bin/codex` on
  this machine).
- **10** — unsupported specs get the stub launcher, never nil.
