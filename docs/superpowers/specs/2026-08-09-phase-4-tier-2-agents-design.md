# Phase 4 — Tier 2 agents: six zero-touch launchers, first ConfigWriter, staged-file capability

**Date:** 2026-08-09 · **Status:** approved design (owner review 2026-08-09)
**Depends on:** `2026-08-07-openrouter-launch-design.md` (main spec),
`2026-08-08-phase-3-agents-design.md` (Tier 1 pattern this phase follows)
**Research basis:** `.superpowers/sdd/2026-08-09-tier-2-research/` — eight per-agent
reports (docs citations, version stamps, live-verification lists) plus `findings.md`
(synthesis and the three owner decisions). Every mechanism below was verified against the
agent's **current official documentation** on 2026-08-09, per the discipline that caught
Landmine 18. Nothing is live-verified yet; the live gates below are mandatory before each
launcher ships.

## Goal

Ship all eight Tier 2 agents from the main spec:

- **Six zero-touch launchers**: `qwen`, `cline`, `kimi`, `omp`, `pi`, `hermes` — env vars
  and CLI flags only, the Tier 1 pattern.
- **`openclaw`** — zero-touch env/flags plus one **launcher-owned** staged config file
  (owner-sanctioned third write site; see the amended zero-touch principle below).
- **`droid`** — the first real `ConfigWriter` implementation (fork-and-wait launch,
  marker-owned config entry, restore on exit).

Plus the supporting infrastructure those last two need, and a shadow-credential advisory
warning shared by five agents.

## Owner decisions (2026-08-09, recorded in findings.md)

1. **droid → implement `ConfigWriter`**, not unsupported-with-reason.
2. **openclaw → full interactive support via a sanctioned third write site** — a
   launcher-owned config file passed by env var, never the agent's own config.
3. **Key delivery is env-only + advisory warning** — no `--api-key`-style flags (no key in
   `/proc/<pid>/cmdline`); where a shadowing credential store is cheaply detectable by
   *reading*, the planner warns Landmine 7-style.

## Scope

**In:**

- `internal/agent/{qwen,cline,kimi,omp,pi,hermes,openclaw,droid}.go` + tests.
- `Staged` capability (new): declarative launcher-owned files, materialized centrally.
- First `ConfigWriter` implementation + the fork-and-wait launch path in
  `internal/launch`/`internal/agent` that the main spec promised for it.
- Shadow-credential advisory: per-agent read-only detectors surfacing through the
  planner's existing warning mechanism.
- Landmine 6 amendment: write-site invariant goes from "exactly two" to the enumerated
  list below; the grep verification and HANDOFF change in the same commit as each new
  write site.
- Registry entries for all eight; live verification per agent before its launcher merges.

**Out (unchanged from prior phases unless noted):**

- No auto-installers — install hints only.
- No behavior-default overrides: the launcher configures model/provider/key, it does not
  re-decide agent UX defaults. Consequence accepted explicitly: **cline's `--auto-approve`
  defaults to true** (their CLI auto-approves tool calls); we document it in the cline
  section of the README/help text and do not pass `--auto-approve false`.
  Owner-approved at spec review, 2026-08-09.
- No parsing of omp's `agent.db` (sqlite) for shadow detection — documented caveat
  instead of a runtime warning (no sqlite dependency for one advisory).
- kimi legacy-CLI hard detection: we do not exec the binary to disambiguate (purity);
  we use path heuristics + docs (see kimi section).
- Windows validation on real Windows remains an open item (as since Phase 1).

## The zero-touch principle, amended

Old invariant (Landmine 6): exactly two write sites, both launcher-owned. The principle
was always "never write an **agent's** files"; this phase makes the launcher-owned side
explicit and adds the one sanctioned agent-owned exception:

| # | Path | Owner | Written by | Secret? |
|---|---|---|---|---|
| 1 | `$XDG_CACHE_HOME/openrouter-launch/models.json` | launcher | `openrouter.Cache` | no |
| 2 | `$XDG_CONFIG_HOME/openrouter-launch/config.json` | launcher | `internal/config` | yes (0600) |
| 3 | `$XDG_CONFIG_HOME/openrouter-launch/openclaw.json` | launcher | `Staged` materializer in `launch.Service.Launch` | **no** (model ref only; key stays in env) |
| 4 | `~/.factory/settings.local.json` | **agent (droid)** | `ConfigWriter.Apply`, capability-gated, marker-owned entries only, restore on exit | **no** (`"apiKey": "${OPENROUTER_API_KEY}"` interpolation) |

Rules that survive unchanged: no other writes anywhere in the tree, verified by grep (the
test now asserts this exact enumeration); the API key is never written outside site 2;
`Command()` stays pure — sites 3 and 4 are materialized by the launch service, never by a
launcher method.

## New capability: `Staged`

```go
// Staged is implemented by launchers that need a launcher-owned file on disk
// at launch time. Files are declared as data (pure) and materialized by
// launch.Service.Launch after recordSelection and before handoff — the same
// single side-effect site as Landmine 5.
type Staged interface {
	StagedFiles(Request) ([]StagedFile, error) // pure, like Command
}

type StagedFile struct {
	Path     string // must resolve under the launcher's own XDG config dir
	Contents []byte
	Mode     os.FileMode // 0644 — staged files must never contain secrets
}
```

Deliberately distinct from `ConfigWriter`: `Staged` writes **launcher-owned** files
(idempotent overwrite, no undo, `syscall.Exec` handoff unaffected); `ConfigWriter` writes
**agent-owned** files (backup/undo required, forces fork-and-wait). Do not merge them —
the distinction is the amended Landmine 6 in type form. A test pins that `StagedFile.Path`
outside our config dir is rejected at materialization.

## The fork-and-wait path (ConfigWriter support)

`launch.Service.Launch` today: `recordSelection` → `agent.Run` (`syscall.Exec` on Unix).
When the launcher implements `ConfigWriter` (`Apply(Request) (restore func() error, err
error)` — the interface already exists, unimplemented), the sequence becomes:

`recordSelection` → materialize `Staged` files (if any) → `Apply` → **spawn child and
wait** (`os/exec`, inheriting stdio; never `syscall.Exec`) → `restore()` → exit with the
child's code (reusing `main.go`'s existing exit-code extraction).

- `restore()` runs in a defer so agent crash ≠ skipped restore; launcher crash/SIGKILL
  still leaks the entry — mitigated by marker-based idempotent cleanup on the *next* run
  (see droid).
- Signal handling: forward SIGINT/SIGTERM to the child; do not die before `restore()`.
- Ordering stays in the one function (Landmine 5); a test pins
  recordSelection → stage → apply → run → restore.

## Shadow-credential advisory (the agent-side Landmine 3)

Five agents keep credential stores that outrank the process environment, so a user who
once logged into OpenRouter *inside* the agent gets billed to that stored account no
matter what key we export. Per owner decision: env-only delivery + advisory.

New capability, detected by type assertion like the others:

```go
type CredentialShadowCheck interface {
	// ShadowedCredential reports a non-empty human-readable description if a
	// stored credential exists that would outrank the launched environment.
	// Read-only; best effort; "" means no warning.
	ShadowedCredential() string
}
```

Surfaced through the planner's existing warning path (advisory, warn-and-confirm — never
abort, Landmine 7's rule). Per-agent detectors, all read-only:

| Agent | Detector |
|---|---|
| pi | `~/.pi/agent/auth.json` parses and contains an `openrouter` key |
| omp | none at runtime (agent.db is sqlite) — documented caveat only |
| cline | `~/.cline/data/settings/providers.json` contains `providers.openrouter` with an `apiKey` |
| hermes | `~/.hermes/.env` contains an `OPENROUTER_API_KEY` line, or `~/.hermes/auth.json` has an openrouter pool entry |
| openclaw | an `auth-profiles.json` under `~/.openclaw/agents/*/agent/` contains an `openrouter` profile |

Parse failures and absent files mean no warning (never block a launch on a detector).
Wording pattern: "cline has a saved OpenRouter key that may override the one this launch
provides (its stored credentials outrank the environment). Launch anyway?"

## Per-agent launchers

Shared conventions (all eight): managed args precede passthrough; passthrough model/
provider selectors are rejected by naming the conflicting argument (Phase 3's rule and
rationale — a later flag silently beats the managed config); slugs come from our catalog
and each launcher applies its own transform; every stated version is what the mechanism
was doc-verified against on 2026-08-09 — the plan re-checks at implementation time.

### qwen — Qwen Code (doc-verified on 0.21.8)

```
argv: qwen --auth-type openai --model <slug> <passthrough…>
env:  OPENAI_BASE_URL=https://openrouter.ai/api/v1
      OPENAI_API_KEY=<key>
      OPENROUTER_API_KEY=<key>          # docs' own OpenRouter recipe names this one
      OPENAI_MODEL=<slug>
```

`--auth-type openai` is **mandatory**: without it, persisted or default auth
(`qwen-oauth`) silently ignores every `OPENAI_*` env var (upstream issue #891). Slug
verbatim. Reject passthrough: `-m/--model`, `--auth-type`, `--openai-api-key`,
`--openai-base-url` (each defeats managed config). Install: `LookPath("qwen")`, then the
ollama-derived per-OS fallbacks (npm-global, `~/.local/bin`, nvm glob, Windows npm
paths); hint `npm install -g @qwen-code/qwen-code@latest`. No `Compatible` check.
Precedence hazard for live gate: a user `~/.qwen/settings.json` `modelProviders` entry
whose `id` equals our slug may apply its own `envKey`/`baseUrl` as a package.

### cline — Cline CLI (doc-verified on 3.0.51)

```
argv: cline -P openrouter -m <slug> <passthrough…>
env:  OPENROUTER_API_KEY=<key>
```

Native builtin `openrouter` provider; base URL baked in upstream; slug verbatim. Reject
passthrough: `-P/--provider`, `-m/--model`. Allow `-k/--key` (an explicit user choice,
not a silent conflict). Install: `LookPath("cline")` only; hint `npm install -g cline`.
Shadow detector per table above. `--auto-approve` default stays untouched (see Scope).

### kimi — Kimi Code CLI (doc-verified on 0.34.0)

**Do not port ollama's mechanism.** Ollama's `kimi --config '<json>'` with provider type
`openai_legacy` targets the deprecated legacy Python kimi-cli; the current TypeScript CLI
has neither the flag nor the type (this phase's Landmine 18 analog, caught in research).
The current CLI's documented channel is the `KIMI_MODEL_*` env family (since 0.6.0),
synthesized in memory, "nothing written back":

```
argv: kimi <passthrough…>
env:  KIMI_MODEL_NAME=<slug>                      # verbatim OpenRouter slug
      KIMI_MODEL_API_KEY=<key>
      KIMI_MODEL_PROVIDER_TYPE=openai
      KIMI_MODEL_BASE_URL=https://openrouter.ai/api/v1
      KIMI_MODEL_MAX_CONTEXT_SIZE=<Request.Model context length, omit if unknown>
```

`KIMI_MODEL_*` outranks config.toml; only a `-m` flag beats it — so reject passthrough
`-m/--model` (and `--config/--config-file`, which the new CLI shouldn't have; rejecting
them also refuses the legacy CLI's dialect rather than half-working). Landmine 3's dedupe
covers the five keys we set; other stray `KIMI_MODEL_*` exports (e.g.
`…_THINKING_EFFORT`) pass through — live-gate item, documented if benign. Install:
`LookPath("kimi")`, then `~/.kimi-code/bin/kimi` (the current CLI's own install dir),
then uv tool paths — **in that order**, so a machine with both generations finds the new
one first; if the *only* hit is a uv tool path, warn advisory-style that a legacy
kimi-cli was found (path heuristic, no exec — the legacy CLI ignores `KIMI_MODEL_*` and
would run on the user's Moonshot account). Hint: `curl -fsSL
https://code.kimi.com/kimi-code/install.sh | bash`. Windows requires Git Bash — registry
note, not a platform gate.

### omp — Oh My Pi (doc-verified on 17.2.11)

```
argv: omp --model openrouter/<slug> <passthrough…>     # note the prefix
env:  OPENROUTER_API_KEY=<key>
```

Built-in provider, base URL baked in. **Slug transform: prefix `openrouter/`** — a bare
slug is a valid-looking wrong value; the mutation check must cover the transform both
ways. Reject passthrough: `-m/--model`, `--provider`. Allow `--api-key` (explicit user
choice). Install: `LookPath("omp")`, then `~/.local/bin/omp`, `~/.bun/bin/omp`; hint
`curl -fsSL https://omp.sh/install | sh`. Shadow store is sqlite → documented caveat, no
runtime detector.

### pi — Pi (doc-verified on 0.84.1, npm `@earendil-works/pi-coding-agent`)

```
argv: pi --provider openrouter --model <slug> <passthrough…>   # bare slug
env:  OPENROUTER_API_KEY=<key>
```

Built-in provider; slug verbatim (confirmed against the shipped 0.84.1 catalog: bare
OpenRouter slugs, `:free` variants as literal ids). Reject passthrough: `--provider`,
`--model`. Allow `--api-key`. Install: `LookPath("pi")`, then npm-global layouts; hint
`npm install -g --ignore-scripts @earendil-works/pi-coding-agent`. Shadow detector:
`~/.pi/agent/auth.json`. Note pi curates a tool-capable subset of the OpenRouter catalog;
a model absent from pi's list fails inside pi — live-gate the error shape, no guard in v1.

### hermes — Hermes Agent CLI (doc-verified on v0.20.0)

```
argv: hermes chat --provider openrouter --model <slug> <passthrough…>
env:  OPENROUTER_API_KEY=<key>
      OPENROUTER_BASE_URL=https://openrouter.ai/api/v1    # documented pin, hardening
```

Flags are documented under the `chat` subcommand as per-run overrides ("no mutation to
config.yaml"); CLI args outrank all config files. Slug verbatim (plus hermes-side
`:nitro`/`:floor` suffixes pass through untouched if a user types them). Reject
passthrough: `--provider`, `--model`; reject a passthrough that *begins with another
hermes subcommand* (our managed flags are chat-scoped — clearer to refuse than to
misapply them). **`Compatible`**: hermes rejects models with <64K context at startup, so
`CheckModel` warns (advisory, Landmine 7) when `Model` context < 65536. Install:
`LookPath("hermes")`, then `~/.local/bin/hermes`, Windows `%LOCALAPPDATA%\hermes`; hint
`curl -fsSL https://hermes-agent.nousresearch.com/install.sh | bash`. Shadow detector:
`~/.hermes/.env` / `auth.json` per table.

### openclaw — OpenClaw (doc-verified on 2026.7.1-2)

The one agent using `Staged`. Interactive launch:

```
staged: $XDG_CONFIG_HOME/openrouter-launch/openclaw.json   (0644, overwritten per launch)
        {"agents":{"defaults":{"model":{"primary":"openrouter/<slug>"},
                               "models":{"openrouter/<slug>":{}}}}}
argv:   openclaw tui --local <passthrough…>
env:    OPENCLAW_CONFIG_PATH=<staged file>
        OPENROUTER_API_KEY=<key>
```

Slug transform: prefix `openrouter/`; refs are lowercased by openclaw — pass lowercase.
`tui --local` runs the embedded runtime: no gateway, no daemon (the process-model concern
from research resolved in our favor). **Accepted consequence, stated plainly:**
`OPENCLAW_CONFIG_PATH` replaces the user's whole `~/.openclaw/openclaw.json` for the
session — their channels/plugins config does not load. For a "launch a coding session on
model X" tool this is the point, but it is a sharper session boundary than any other
agent gets. Owner-approved at spec review, 2026-08-09.

One-shot passthrough: if the first passthrough arg is `agent`, skip the staged file and
instead append `--model openrouter/<slug> --auth-env-only` after the passthrough
(documented in-memory config composition; writes nothing). Reject passthrough
`--model` in both forms; reject other leading subcommands (`gateway`, `daemon`,
`onboard`, …) — they are platform administration, not a launch. Do **not** set
`OPENCLAW_STATE_DIR` (the user keeps sessions/pairings). Install: `LookPath("openclaw")`,
fallback `LookPath("clawdbot")`; hint `npm install -g openclaw@latest`. Shadow detector:
auth-profiles per table. Biggest live-gate item of the phase: virgin-state `tui --local`
with only our config — straight to session, or onboarding gate?

### droid — Factory Droid (doc-verified on 0.190.0) — the ConfigWriter

No zero-touch surface exists: Factory documents OpenRouter BYOK, but the only declaration
surface is a `.factory` settings file. Owner decision: first `ConfigWriter`.

**`Apply(Request)`** upserts into `~/.factory/settings.local.json` (the merge-friendly
local layer; never `settings.json`):

```json
{
  "customModels": [{
    "displayName": "openrouter-launch",
    "provider": "generic-chat-completion-api",
    "baseUrl": "https://openrouter.ai/api/v1",
    "model": "<verbatim slug>",
    "apiKey": "${OPENROUTER_API_KEY}",
    "maxOutputTokens": 64000
  }]
}
```

- `displayName: "openrouter-launch"` is the **ownership marker** (dash-safe on purpose —
  droid derives selection IDs from it): Apply replaces any existing marker-owned entry and
  never touches others; stale entries from a crashed run are cleaned by the next Apply.
  Surgical marker-based upsert/removal, **not** whole-file backup/restore — a user editing
  the file mid-session must not have their edits reverted by our restore.
- **Model selection happens in the file, not on argv.** Apply also sets the settings
  file's default-model key (current docs: top-level `model`; ollama wrote
  `sessionDefaultSettings.model` — the live gate determines which the installed version
  honors) to our entry's `custom:openrouter-launch-<index>` ID, capturing the prior value
  in the `restore()` closure. This is forced by purity: the entry's index — and therefore
  its selection ID — is only knowable at Apply time from the merged file, and `Command()`
  cannot read files. So `Command()` emits `droid <passthrough…>` with **no `-m`**, and it
  usefully sidesteps the reported upstream bug of `--model` rejecting `custom:` IDs.
  Fallback if the default-model key is not honored: pass `-m "custom:openrouter-launch-
  <index>"` via a value Apply hands to the fork-and-wait runner (never derived inside
  `Command()`).
- `restore()` removes marker-owned entries only, reinstates the prior default-model value
  (or removes the key if we introduced it), and removes the file if we created it and it
  is otherwise empty.
- The key never touches disk: `${OPENROUTER_API_KEY}` env interpolation (documented for
  settings.local.json) + `OPENROUTER_API_KEY=<key>` in the child env.
- Reject passthrough `-m/--model` (it would silently override the configured selection).
- Launch is fork-and-wait per the ConfigWriter path above; exit code propagates.
- Registry notes: droid requires a Factory account even for BYOK (surface in `agents`
  output); on selection failure droid silently falls back to a Factory-billed model — the
  live gate must prove `-m custom:…` actually selects our entry on the current version
  (a public issue reported `--model` rejecting custom IDs; cadence is ~2 releases/week).
- Install: `LookPath("droid")` only; hint `curl -fsSL https://app.factory.ai/cli | sh`.

## Contingency

Same rule as Phase 3, per agent: if live verification falsifies the documented mechanism,
the agent moves — with evidence recorded in the phase ledger — to whichever bucket fits:
a corrected zero-touch variant, `ConfigWriter`, or unsupported-with-reason. Nothing is
dropped silently. Specific pre-identified fallbacks: qwen's provider-catalog collision
(fallback: also pass `--openai-api-key`/`--openai-base-url`, the documented
highest-priority credential layer — still zero-touch); openclaw interactive failing its
virgin-state gate (fallback: one-shot `agent exec` support only, interactive refuses with
a reason); droid's default-model key not being honored (fallback: `-m
"custom:openrouter-launch-<index>"` with the index handed from Apply to the runner; or
unsupported-with-reason if selection misroutes either way — silent Factory-billed
fallback is exactly the failure class we refuse to ship).

## Testing and verification

**Unit (purity rule: no process spawned):** `Command()` compared as structs per agent —
full argv, env pairs, slug transforms (omp/openclaw prefix, everyone else verbatim, kimi's
context-size threading and omission-when-unknown); conflict rejection naming the argument,
benign passthrough untouched; `StagedFiles` content/path/mode as data; droid `Apply`/
`restore` against fixture files in a temp `HOME` — marker upsert, foreign-entry
preservation, stale-marker cleanup, restore removing only ours; shadow detectors against
fixture stores (present/absent/malformed — malformed means silent no-warning).

**Mutation checks** (the project's recurring lesson — every non-obvious behavior gets
one): swap omp's prefixed slug for bare (and openclaw's), drop qwen's `--auth-type`,
drop a `KIMI_MODEL_*` var, break droid's marker match so it touches a foreign entry,
point a `StagedFile` outside the config dir, invert a shadow detector, remove the
fork-and-wait `restore()` defer. Watch the named test fail; revert.

**Machine-independence (Landmine 8, now wider):** `hermes` and `pi` are genuinely
installed on this machine (`~/.local/bin/`), joining `claude`/`codex`/`opencode`. Every
absent-binary test sets `t.Setenv("HOME", t.TempDir())`; the `HOME=$(mktemp -d)` full-suite
run in HANDOFF must stay green with all Tier 2 binaries invisible.

**Write-site grep:** the verification test updates from "exactly two" to the enumerated
four-site table above, and asserts `internal/agent` contains no file writes outside
`ConfigWriter.Apply`/`restore`.

**Live gates (owner-approved cents-level spend, cheap model, logs in the phase
workspace — each launcher merges only after its gate passes):**

- All eight: one-shot/headless completion through the built binary where the agent has a
  headless mode; confirm the reply came through OpenRouter; zero-touch audit diff of the
  agent's config tree before/after (droid: audit shows *only* the marker entry lifecycle).
- qwen: env-only vs mandatory `--auth-type`; the `modelProviders` collision.
- cline: virgin `~/.cline` with env key only; `-P/-m` honored in interactive TUI.
- kimi: new CLI rejects `--config`; virgin first run with only `KIMI_MODEL_*`; stray
  `KIMI_MODEL_*` leak behavior; legacy binary's behavior with our env (for the advisory
  wording).
- omp: selector `openrouter/<slug>` round-trips unmangled; first-run onboarding.
- pi: `/`- and `:`-bearing slugs parse; catalog-absent model error shape; auth.json
  shadowing demo (for the advisory wording).
- hermes: flags on a fresh `HOME`; `.env`-vs-process-env precedence; the 64K rejection's
  actual behavior.
- openclaw: the virgin-state `tui --local` gate; `agent exec --auth-env-only` flags exist
  on the installed version; whole-config-replacement consequence sanity-checked.
- droid: which default-model key the installed version honors (top-level `model` vs
  `sessionDefaultSettings.model`) and that it actually selects our entry — with the
  `-m custom:…` fallback exercised too (the #787 question); `${OPENROUTER_API_KEY}`
  interpolation works in settings.local.json; restore leaves foreign entries untouched
  and reinstates the prior default; Factory-account requirement's failure mode without
  login.

**Human interactive smoke tests at the end**, same standard as Phases 2–3.

**The standing suite stays green:** `go test ./... -count=1`, `-race` on tui, vet, gofmt,
cross-builds, the `HOME`-redirect run, and the updated write-site grep.

## Sequencing recommendation (for the plan)

Two plans off this one spec:

- **Plan 4a — six zero-touch launchers.** Order: `pi`, `hermes` first (installed on this
  machine — live gates cost nothing to start; both high-confidence), then `qwen`,
  `cline`, `kimi`, `omp`. Includes the shadow-credential capability (used by four of the
  six) and the Landmine 8 widening.
- **Plan 4b — the two infrastructure agents.** `Staged` + openclaw; fork-and-wait +
  `ConfigWriter` + droid; the Landmine 6 amendment lands here (each write site in the
  same commit as its grep update).

4a has no dependency on 4b. Within 4b, `Staged`/openclaw before ConfigWriter/droid —
smaller step first, and the write-site test evolves twice in sequence rather than once in
a lump.

## Landmines that bind this phase

- **1** — all eight agents use the OpenAI-compatible `…/api/v1`; only Claude Code uses
  `…/api`.
- **3** — `ExecArgs` dedupe makes our env win for keys we set; kimi's stray-`KIMI_MODEL_*`
  gap is called out above.
- **5** — side-effect ordering lives in one function; staging and `Apply` join it, they
  do not escape it.
- **6** — amended as specified: launcher-owned sites + the one capability-gated agent-owned
  exception, enumerated and grep-verified; HANDOFF text updates with it.
- **7** — every new warning (hermes context, kimi legacy path, shadow credentials) is
  advisory: warn and confirm, never abort.
- **8** — hermes and pi join the installed-binaries list; `HOME`-isolate or it only
  passes here.
- **10** — any agent that ends up unsupported keeps a stub launcher, never nil.
- **18** — the pattern repeated: kimi's ollama mechanism targets a deprecated CLI. The
  per-agent doc-verification in this spec is the countermeasure; live gates make it stick.
