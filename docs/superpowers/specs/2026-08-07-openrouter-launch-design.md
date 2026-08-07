# openrouter-launch — Design

**Date:** 2026-08-07
**Status:** Approved for planning

## Summary

`openrouter-launch` is a Go CLI that picks an OpenRouter model in an interactive
TUI and launches a coding agent (Claude Code, Codex, OpenCode, …) already wired
to that model. It stores named agent+model profiles as favorites.

It is modelled on `ollama launch` (`cmd/launch` in the ollama repo), which solves
the same problem for locally-served models. Two structural ideas are borrowed:
a **declarative registry** of integrations, and a **thin required interface plus
optional capability interfaces**, so N agents never collapse into one switch
statement.

The key difference: `ollama launch` writes and restores agent config files.
`openrouter-launch` does not. Every agent is configured through environment
variables, inline-config environment content, or CLI overrides, so no file the
user depends on is ever mutated.

## Goals

- Browse OpenRouter's ~400 models with search and filters.
- Launch any supported agent against the selected model, fully configured.
- Save and manage named agent+model profiles.
- Never mutate an agent's existing configuration.
- Be scriptable: every interactive path has a flag-driven equivalent.

## Non-goals

- Proxying, translating, or rewriting API traffic.
- Managing OpenRouter credits, billing, or account settings.
- Installing agents automatically without explicit confirmation.
- Supporting GUI applications that authenticate through their own accounts.

## Architecture

```
openrouter-launch/
  main.go
  internal/
    openrouter/   API client, model types, disk cache
    agent/        registry + one file per agent
    config/       ~/.config/openrouter-launch/config.json
    tui/          bubbletea picker: search, filters, profiles
    cli/          cobra commands
```

Module path `github.com/teggen/openrouter-launch`, Go 1.22+.
Dependencies: `spf13/cobra`, `charmbracelet/bubbletea`, `charmbracelet/lipgloss`.

### Required interface

```go
// Launcher computes the process to run. Implementations MUST be pure:
// no file writes, no network, no process spawning.
type Launcher interface {
	Name() string        // canonical id, e.g. "claude"
	DisplayName() string // e.g. "Claude Code"
	Command(Request) (Command, error)
}

type Request struct {
	Model     openrouter.Model
	APIKey    string
	ExtraArgs []string
}

type Command struct {
	Path string   // resolved absolute binary path
	Args []string
	Env  []string // appended to os.Environ()
}
```

Purity is the load-bearing property: it makes every agent testable by comparing
a struct, with no process ever spawned in a test.

### Optional capability interfaces

Detected by type assertion, following `ollama/cmd/launch/launch.go:143-248`:

| Interface | Methods | Purpose |
| --- | --- | --- |
| `Installable` | `CheckInstalled() bool`, `InstallHint() string`, `EnsureInstalled() error` | detection and opt-in install |
| `Compatible` | `CheckModel(Model) error` | warn on bad agent/model pairings |
| `Supported` | `Supported() error` | platform gating |
| `ConfigWriter` | `Apply(Request) (restore func() error, err error)` | escape hatch (see below) |

`EnsureInstalled` and `ConfigWriter` are defined in v1 but implemented by no
agent. `ConfigWriter` exists so an agent with no zero-touch configuration hook
can be added later without reworking the launch flow. When an agent implements
it, the launcher forks and waits instead of `exec`ing, so `restore` can run.

### Registry

```go
type Spec struct {
	Name        string
	Aliases     []string
	Launcher    Launcher
	Description string
	Status      Status // Supported, or Unsupported with a Reason
}
```

A package-level `[]*Spec` plus an `init()` that builds name/alias indexes and
panics on collisions — the same self-checking approach as
`ollama/cmd/launch/registry.go:298`. Unsupported agents stay in the registry and
render greyed out with their reason, rather than silently disappearing.

## Agent support tiers

The target is parity with `ollama launch`'s integration list. Each agent needs
its OpenRouter wiring verified before it ships; the tiers record what is
verified today versus what the implementation must confirm.

### Tier 1 — mechanism verified

| Agent | Mechanism |
| --- | --- |
| `claude` (Claude Code) | env: `ANTHROPIC_BASE_URL=https://openrouter.ai/api` (no `/v1`), `ANTHROPIC_API_KEY=<key>`, `ANTHROPIC_AUTH_TOKEN=""`, plus `ANTHROPIC_DEFAULT_{FABLE,OPUS,SONNET,HAIKU}_MODEL` and `CLAUDE_CODE_SUBAGENT_MODEL` all set to the selected slug. Verified against OpenRouter's Claude Code integration guide. |
| `codex` (OpenAI Codex CLI) | repeated `-c key=value` overrides defining a `model_providers.openrouter` entry (`base_url`, `env_key`, `wire_api`) plus `model_provider` and `-m <slug>`; API key via env. Mechanism verified in `ollama/cmd/launch/codex.go:174`; the OpenRouter-specific provider values need confirmation on the installed codex version. |
| `opencode` | entire config passed inline as JSON via `OPENCODE_CONFIG_CONTENT`. Mechanism verified in `ollama/cmd/launch/opencode.go:63`; OpenCode also has native OpenRouter provider support, so the inline config may be minimal. |

### Tier 2 — expected zero-touch, mechanism to confirm during implementation

`qwen` (qwen-code, expected `OPENAI_BASE_URL`/`OPENAI_API_KEY`/`OPENAI_MODEL`),
`droid` (Factory, BYOK custom models), `hermes` (Nous Research),
`cline`, `kimi`, `omp`, `openclaw`, `pi`.

Each is a separate implementation task: confirm the agent accepts a custom
OpenAI-compatible base URL through env or CLI flags, then implement `Command`.
An agent that turns out to have no zero-touch hook is either promoted to the
`ConfigWriter` path or moved to Tier 3 with a documented reason — it is not
silently dropped.

### Tier 3 — cannot be pointed at OpenRouter

| Agent | Reason |
| --- | --- |
| `copilot` | GitHub Copilot CLI talks to GitHub's own backend; no custom provider. |
| `pool` | Poolside is an enterprise service with its own endpoint. |
| `chatgpt` / Codex app, `claude-desktop`, `hermes-desktop`, `vscode` | GUI applications that authenticate through their own accounts; a launcher cannot inject a provider. |

These appear in listings with their reason so the absence is explained.

## OpenRouter client

`GET https://openrouter.ai/api/v1/models?sort=most-popular` returns the full
catalog in a useful default order. The endpoint is public, so browsing works
before a key is configured.

Fields consumed from each model object: `id`, `name`, `description`,
`context_length`, `pricing.{prompt,completion}`, `supported_parameters`,
`architecture.input_modalities`, `top_provider.max_completion_tokens`.

Pricing is per-token in USD as strings; display converts to $/M tokens
(`value * 1e6`). A model counts as free when both `pricing.prompt` and
`pricing.completion` parse to zero.

**Caching.** Response stored at
`$XDG_CACHE_HOME/openrouter-launch/models.json` (default `~/.cache/...`) with a
24h TTL. `--refresh` forces a fetch. With a warm cache the picker opens
immediately and a background refresh streams updated rows into the live
selector over a channel, the pattern used by `ollama/cmd/tui/selector.go:69`.

## Configuration

`$XDG_CONFIG_HOME/openrouter-launch/config.json` (default `~/.config/...`),
mode 0600, written by temp-file + atomic rename.

```json
{
  "api_key": "sk-or-...",
  "profiles": [
    {
      "name": "opus-cc",
      "agent": "claude",
      "model": "anthropic/claude-opus-4.6",
      "args": ["--resume"]
    }
  ],
  "last_agent": "claude",
  "last_model": "anthropic/claude-opus-4.6",
  "filters": {
    "tools_only": true,
    "free_only": false,
    "min_context": 0,
    "max_price": 0
  }
}
```

Profiles are an ordered list so user ordering survives round-trips. Profile
names are unique among profiles and are not otherwise constrained, since
`profile launch` gives them their own namespace.

**API key precedence:** `OPENROUTER_API_KEY` from the environment wins. If it is
unset, the stored `api_key` is used. If neither exists, the first run that needs
a key prompts for one and offers to save it.

## CLI surface

```
openrouter-launch                       # TUI: profiles + agents → model → launch
openrouter-launch <agent>               # pick a model for that agent, then launch
openrouter-launch <agent> -m <slug>     # launch directly, no TUI
openrouter-launch <agent> -- <args...>  # everything after -- is passed to the agent

openrouter-launch profile list
openrouter-launch profile add --name <n> --agent <a> --model <slug> [-- <args...>]
openrouter-launch profile launch <name>
openrouter-launch profile rm <name>
openrouter-launch profile rename <old> <new>

openrouter-launch models [--tools] [--free] [--provider <p>]
                         [--min-context <tokens>] [--max-price <usd-per-M-completion>]
                         [--refresh]

openrouter-launch agents                # list agents with status and install state
```

`--refresh` and `--yes` (skip confirmations) are global flags.

## TUI

**Screen 1 — root.** A `Profiles` section on top, then `Agents`. Agents show
their install state; Tier 3 agents render greyed with their reason and cannot be
selected. Selecting a profile launches immediately. Selecting an agent advances
to the model picker. The last used entry is preselected via `last_agent`.

**Screen 2 — model picker.** Type-to-search, fuzzy-matched over slug and display
name, ranked by match quality (prefix hits before substring hits), following
`ollama/cmd/tui/selector.go:623`. Filter keys:

| Key | Filter |
| --- | --- |
| `t` | tool-calling only — **on by default**; a coding agent without `tools` in `supported_parameters` is unusable |
| `f` | free models only |
| `c` | cycle minimum context: any → 32k → 128k → 200k → 1M |
| `$` | cycle max price ceiling on completion $/M: any → 1 → 5 → 15 |

Provider filtering needs no dedicated key: typing `anthropic/` narrows to that
vendor through the normal search. Active filters render as a status bar so the
current view is never ambiguous. All four filters persist to `config.filters`
across runs, where `0` means unset for the numeric ones. Search text is not
persisted.

`ctrl+s` on the highlighted model prompts for a name and saves the current
agent + model as a profile.

Row format:

```
anthropic/claude-opus-4.6      200k   $15/$75 per M   [tools]
```

Every filter has an equivalent flag on `models` and on direct launch, so the
tool is fully usable without the TUI.

## Error handling

| Condition | Behavior |
| --- | --- |
| No API key | Error naming `OPENROUTER_API_KEY` and linking https://openrouter.ai/keys. Model browsing still works — the catalog endpoint is public. |
| Network failure, cache present | Use the cached catalog, warn on stderr with its age. |
| Network failure, no cache | Fail with the underlying error. |
| Agent not installed | Print the install command or URL. If `EnsureInstalled` exists, offer to run it on explicit confirmation — never pipe a remote script into a shell silently. |
| Agent unsupported (Tier 3) | Refuse with the registry's stated reason. |
| Incompatible pairing | Warn and require confirmation, do not block. Claude Code with a non-`anthropic/*` slug is the known case: OpenRouter documents that Claude Code may fail on context-management features with other providers. |
| Unknown model slug on direct launch | Fail, and suggest the closest matches from the cached catalog. |

## Process handoff

On Unix, `syscall.Exec` replaces the launcher process with the agent. There is
no surviving parent, so signals, job control, and TTY behavior are exactly as if
the agent had been invoked directly — no forwarding logic to get wrong.

On Windows, `exec.Command` with inherited stdio, propagating the child's exit
code.

An agent using the `ConfigWriter` escape hatch takes the fork-and-wait path
instead, because `restore` requires the parent to outlive the child.

## Testing

- **Agents:** table tests per agent asserting the exact `Command` struct, with
  binary lookup stubbed through an injected `LookPath`. No test spawns an agent.
- **Client:** `httptest` server serving a recorded `/models` fixture; covers
  parsing, pricing conversion, free detection, and pagination.
- **Cache:** TTL expiry, corrupt-file recovery, stale-cache-on-network-failure.
- **Config:** round-trip, profile CRUD, duplicate-name rejection, and an
  assertion that the file is written 0600.
- **Filters and matching:** pure-function tests over a fixture catalog.
- **TUI:** bubbletea `Update` driven by injected `tea.KeyMsg` values, asserting
  cursor movement, filter toggles, and the resulting selection.
- **Registry:** collision panics, alias resolution, tier/status reporting.

## Delivery phases

The agent list is wide, so it is delivered incrementally. Each phase is
independently useful and independently shippable.

1. **Core** — `openrouter` client with cache, `config` with profiles, registry
   with interfaces, `agents`/`models` commands, and `claude` as the single
   agent. Non-interactive only. This proves the whole spine end to end.
2. **TUI** — root screen, model picker, search, the four filters, `ctrl+s`
   profile save.
3. **Tier 1 agents** — add `codex` and `opencode`.
4. **Tier 2 agents** — one task per agent: verify its mechanism against its own
   documentation, then implement `Command` plus its table test. Any agent that
   fails verification moves to Tier 3 with a written reason.

## Assumptions

- Module path `github.com/teggen/openrouter-launch`; the directory is not yet a
  git repository and will be initialized.
- Tier 2 agent mechanisms are confirmed during implementation, one task each.
  Confirmation happens against the agent's own documentation before code is
  written for it.
